package database

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TylerAkins/wow-database/internal/record"
)

type ResolutionPolicy struct {
	Fields map[string][]string `json:"fields"`
}

type Candidate struct {
	Source      string          `json:"source"`
	Revision    string          `json:"revision"`
	Value       json.RawMessage `json:"value,omitempty"`
	State       string          `json:"state,omitempty"`
	Path        string          `json:"path,omitempty"`
	Reason      string          `json:"reason,omitempty"`
	SourceField string          `json:"sourceField,omitempty"`
}

type Conflict struct {
	Flavor       string      `json:"flavor"`
	EntityType   string      `json:"entityType"`
	EntityID     string      `json:"entityId"`
	Field        string      `json:"field"`
	Selected     string      `json:"selectedSource,omitempty"`
	Rule         string      `json:"selectionRule,omitempty"`
	Candidates   []Candidate `json:"candidates"`
	Observations []Candidate `json:"observations,omitempty"`
}

type sourceData struct {
	manifest record.Manifest
	records  []record.Record
}

func Compile(flavor, sourcesRoot, outputDir, policyPath string) error {
	if flavor != "forever" {
		return fmt.Errorf("unsupported flavor %q", flavor)
	}
	entries, err := os.ReadDir(sourcesRoot)
	if err != nil {
		return fmt.Errorf("read sources directory: %w", err)
	}
	var inputs []sourceData
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rows, manifest, err := record.ReadDataset(filepath.Join(sourcesRoot, entry.Name()))
		if err != nil {
			return fmt.Errorf("source %s: %w", entry.Name(), err)
		}
		if manifest.Flavor != flavor || manifest.Source != entry.Name() {
			return fmt.Errorf("source directory %s manifest is %s/%s, expected %s/%s", entry.Name(), manifest.Flavor, manifest.Source, flavor, entry.Name())
		}
		inputs = append(inputs, sourceData{manifest: manifest, records: rows})
	}
	if len(inputs) == 0 {
		return fmt.Errorf("no source snapshots found in %s", sourcesRoot)
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].manifest.Source < inputs[j].manifest.Source })
	policy := ResolutionPolicy{Fields: map[string][]string{}}
	if policyPath != "" {
		data, err := os.ReadFile(policyPath)
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&policy); err != nil {
			return fmt.Errorf("read resolution policy: %w", err)
		}
		if policy.Fields == nil {
			policy.Fields = map[string][]string{}
		}
	}
	for field, priority := range policy.Fields {
		if strings.Count(field, ".") != 1 || strings.TrimSpace(field) != field || len(priority) == 0 {
			return fmt.Errorf("invalid resolution policy entry %q", field)
		}
		seen := map[string]bool{}
		for _, source := range priority {
			if (source != "questiedb" && source != "wowhead" && source != "allthethings" && source != "foreverclient") || seen[source] {
				return fmt.Errorf("invalid or duplicate source %q in policy %s", source, field)
			}
			seen[source] = true
		}
	}

	grouped := map[string][]record.Record{}
	manifests := make([]record.Manifest, 0, len(inputs))
	for _, input := range inputs {
		manifests = append(manifests, input.manifest)
		for _, row := range input.records {
			grouped[row.Key()] = append(grouped[row.Key()], row)
		}
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	compiled := make([]record.Record, 0, len(keys))
	var conflicts []Conflict
	for _, key := range keys {
		merged, found, err := mergeRecords(grouped[key], policy)
		if err != nil {
			return fmt.Errorf("compile %s: %w", key, err)
		}
		compiled = append(compiled, merged)
		conflicts = append(conflicts, found...)
	}
	inputBytes, err := json.Marshal(struct {
		Manifests []record.Manifest `json:"manifests"`
		Policy    ResolutionPolicy  `json:"policy"`
		Version   int               `json:"normalizationVersion"`
	}{manifests, policy, NormalizationVersion})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(inputBytes)
	revision := hex.EncodeToString(digest[:])
	for i := range compiled {
		compiled[i].SourceRevision = revision
	}
	return writeCompilation(outputDir, compiled, record.Manifest{
		Source: "compilation", Flavor: flavor, SourceRevision: revision, Status: "generated", CompilationFormatVersion: CompilationFormatVersion,
	}, conflicts)
}

func mergeRecords(rows []record.Record, policy ResolutionPolicy) (record.Record, []Conflict, error) {
	if len(rows) == 0 {
		return record.Record{}, nil, fmt.Errorf("no source rows")
	}
	base := rows[0]
	originals := rows
	audit := map[string]sourceAudit{}
	normalized := make([]record.Record, 0, len(rows))
	for _, row := range rows {
		n, a, err := normalizeRecord(row)
		if err != nil {
			return record.Record{}, nil, err
		}
		if _, exists := audit[row.Source]; exists {
			return record.Record{}, nil, fmt.Errorf("duplicate source observation %s", row.Source)
		}
		audit[row.Source] = a
		normalized = append(normalized, n)
	}
	rows = normalized
	result := record.Record{SchemaVersion: record.SchemaVersion, Flavor: base.Flavor, EntityType: base.EntityType, EntityID: base.EntityID, Source: "compilation", SourceRevision: "pending", Attributes: map[string]json.RawMessage{}, FieldStates: map[string]string{}, Provenance: map[string]record.FieldProvenance{}}
	fieldSet := map[string]bool{}
	for _, row := range rows {
		for field := range row.Attributes {
			fieldSet[field] = true
		}
		for field := range row.FieldStates {
			fieldSet[field] = true
		}
	}
	fields := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	var conflicts []Conflict
	for _, field := range fields {
		values := map[string][]record.Record{}
		states := map[string][]record.Record{}
		for _, row := range rows {
			if raw, ok := row.Attributes[field]; ok && (row.FieldStates[field] == "" || row.FieldStates[field] == "empty") {
				canonical, err := canonicalJSON(raw)
				if err != nil {
					return record.Record{}, nil, fmt.Errorf("%s: %w", field, err)
				}
				values[canonical] = append(values[canonical], row)
			} else if state := row.FieldStates[field]; state != "" {
				states[state] = append(states[state], row)
			}
		}
		if len(values) == 0 {
			result.FieldStates[field] = "missing"
			allRows := []record.Record{}
			for _, owners := range states {
				allRows = append(allRows, owners...)
			}
			for _, state := range []string{"missing", "unavailable", "unsupported", "conflicting", "invalid"} {
				if len(states[state]) > 0 {
					result.FieldStates[field] = state
				}
			}
			if len(allRows) > 0 {
				result.Provenance[field] = provenance(allRows)
			}
			continue
		}
		if len(values) == 1 {
			for raw, owners := range values {
				result.Attributes[field] = json.RawMessage(raw)
				result.Provenance[field] = provenance(owners)
				if isExplicitEmpty(json.RawMessage(raw)) {
					result.FieldStates[field] = "empty"
				}
			}
			continue
		}
		found := Conflict{Flavor: result.Flavor, EntityType: result.EntityType, EntityID: result.EntityID, Field: field, Candidates: append(valueCandidates(values), stateCandidates(states)...), Observations: originalCandidates(originals, field, audit)}
		found.Selected, found.Rule = selectSource(result.EntityType, field, values, policy)
		conflicts = append(conflicts, found)
		if found.Selected == "" {
			result.FieldStates[field] = "conflicting"
			allRows := []record.Record{}
			for _, owners := range values {
				allRows = append(allRows, owners...)
			}
			result.Provenance[field] = provenance(allRows)
			continue
		}
		for raw, owners := range values {
			for _, owner := range owners {
				if owner.Source == found.Selected {
					result.Attributes[field] = json.RawMessage(raw)
					prov := provenance(owners)
					prov.SelectionRule = found.Rule
					result.Provenance[field] = prov
					if isExplicitEmpty(json.RawMessage(raw)) {
						result.FieldStates[field] = "empty"
					}
					break
				}
			}
			if result.Attributes[field] != nil {
				break
			}
		}
	}
	if len(result.FieldStates) == 0 {
		result.FieldStates = nil
	}
	if len(result.Provenance) == 0 {
		result.Provenance = nil
	}
	auditBytes, err := json.Marshal(audit)
	if err != nil {
		return record.Record{}, nil, err
	}
	result.Attributes["sourceData"] = auditBytes
	return result, conflicts, nil
}

func isExplicitEmpty(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte(`""`)) || bytes.Equal(trimmed, []byte("[]")) || bytes.Equal(trimmed, []byte("{}"))
}

func canonicalJSON(raw json.RawMessage) (string, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(canonicalNumbers(value))
	return string(encoded), err
}

func provenance(rows []record.Record) record.FieldProvenance {
	out := record.FieldProvenance{}
	seen := map[string]bool{}
	for _, row := range rows {
		key := row.Source + "\x00" + row.SourceRevision
		if !seen[key] {
			seen[key] = true
			out.Sources = append(out.Sources, record.SourceObservation{Source: row.Source, Revision: row.SourceRevision})
		}
	}
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].Source < out.Sources[j].Source })
	return out
}

func valueCandidates(values map[string][]record.Record) []Candidate {
	var out []Candidate
	for value, rows := range values {
		for _, row := range rows {
			out = append(out, Candidate{Source: row.Source, Revision: row.SourceRevision, Value: json.RawMessage(value)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return string(out[i].Value) < string(out[j].Value)
	})
	return out
}

func stateCandidates(states map[string][]record.Record) []Candidate {
	var out []Candidate
	for state, rows := range states {
		for _, row := range rows {
			out = append(out, Candidate{Source: row.Source, Revision: row.SourceRevision, State: state})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].State < out[j].State
	})
	return out
}

func selectSource(entityType, field string, values map[string][]record.Record, policy ResolutionPolicy) (string, string) {
	for _, source := range policy.Fields[entityType+"."+field] {
		for _, rows := range values {
			for _, row := range rows {
				if row.Source == source {
					return source, "schemas/field-resolution.json: " + entityType + "." + field
				}
			}
		}
	}
	return "", ""
}

func writeCompilation(outputDir string, rows []record.Record, manifest record.Manifest, conflicts []Conflict) error {
	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, ".compile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	dataset := filepath.Join(temp, "dataset")
	if err := record.WriteDataset(dataset, rows, manifest); err != nil {
		return err
	}
	entries, err := os.ReadDir(dataset)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if err := os.Rename(filepath.Join(dataset, name), filepath.Join(temp, name)); err != nil {
			return err
		}
	}
	if err := os.Remove(dataset); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(temp, "conflicts.json"), conflicts); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(temp, "conflicts.md"), []byte(renderConflicts(conflicts)), 0o644); err != nil {
		return err
	}
	if _, _, err := record.ReadDataset(temp); err != nil {
		return fmt.Errorf("validate staged compilation: %w", err)
	}
	if _, err := validateReferences(rows); err != nil {
		return fmt.Errorf("validate staged references: %w", err)
	}
	if err := writeJSON(filepath.Join(temp, "reconciliation.json"), reconciliationReport(rows, conflicts)); err != nil {
		return err
	}
	old := outputDir + ".old"
	if _, err := os.Stat(old); err == nil {
		return fmt.Errorf("backup already exists: %s", old)
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(outputDir); err == nil {
		if err := os.Rename(outputDir, old); err != nil {
			return err
		}
	}
	if err := os.Rename(temp, outputDir); err != nil {
		_ = os.Rename(old, outputDir)
		return err
	}
	return os.RemoveAll(old)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func renderConflicts(conflicts []Conflict) string {
	var out strings.Builder
	out.WriteString("# Compilation conflicts\n\n")
	if len(conflicts) == 0 {
		out.WriteString("No conflicting values were found.\n")
		return out.String()
	}
	selected := 0
	for _, c := range conflicts {
		if c.Selected != "" {
			selected++
		}
	}
	fmt.Fprintf(&out, "%d field conflict(s): %d selected, %d unresolved. See `conflicts.json` for candidates and `reconciliation.json` for dispositions.\n\n", len(conflicts), selected, len(conflicts)-selected)
	for _, row := range conflicts {
		fmt.Fprintf(&out, "- `%s/%s/%s.%s`: ", row.Flavor, row.EntityType, row.EntityID, row.Field)
		if row.Selected != "" {
			fmt.Fprintf(&out, "selected `%s` using %s", row.Selected, row.Rule)
		} else {
			out.WriteString("left unresolved")
		}
		out.WriteByte('\n')
	}
	return out.String()
}
