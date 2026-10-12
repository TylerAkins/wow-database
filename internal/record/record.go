package record

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SchemaVersion = 1

var sources = map[string]bool{"wowhead": true, "allthethings": true, "questiedb": true, "foreverclient": true, "compilation": true}
var flavors = map[string]bool{"era": true, "forever": true}
var entityTypes = map[string]bool{
	"quest": true, "npc": true, "item": true, "object": true, "zone": true,
	"map": true, "spell": true, "faction": true, "achievement": true,
	"profession": true, "relationship": true,
}
var fieldStates = map[string]bool{"missing": true, "unsupported": true, "unavailable": true, "invalid": true, "empty": true, "conflicting": true}

// Record is one source's normalized view of an entity. Unknown source-specific
// data belongs in Attributes; field states distinguish absence from known-empty.
type Record struct {
	SchemaVersion  int                        `json:"schemaVersion"`
	Flavor         string                     `json:"flavor"`
	EntityType     string                     `json:"entityType"`
	EntityID       string                     `json:"entityId"`
	Source         string                     `json:"source"`
	SourceRevision string                     `json:"sourceRevision"`
	Attributes     map[string]json.RawMessage `json:"attributes"`
	FieldStates    map[string]string          `json:"fieldStates,omitempty"`
	Provenance     map[string]FieldProvenance `json:"provenance,omitempty"`
}

type FieldProvenance struct {
	Source        string              `json:"source,omitempty"`
	Revision      string              `json:"revision,omitempty"`
	Sources       []SourceObservation `json:"sources,omitempty"`
	Path          string              `json:"path,omitempty"`
	SelectionRule string              `json:"selectionRule,omitempty"`
}

type SourceObservation struct {
	Source   string `json:"source"`
	Revision string `json:"revision"`
}

type Manifest struct {
	SchemaVersion  int      `json:"schemaVersion"`
	Flavor         string   `json:"flavor"`
	Source         string   `json:"source"`
	SourceRevision string   `json:"sourceRevision"`
	RecordCount    int      `json:"recordCount"`
	RecordsSHA256  string   `json:"recordsSha256"`
	GeneratedAt    string   `json:"generatedAt,omitempty"`
	Status         string   `json:"status,omitempty"`
	Limitations    []string `json:"limitations,omitempty"`
}

func (r Record) Key() string { return r.Flavor + "/" + r.EntityType + "/" + r.EntityID }

func (r Record) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schemaVersion must be %d", SchemaVersion)
	}
	if !flavors[r.Flavor] {
		return fmt.Errorf("unsupported flavor %q", r.Flavor)
	}
	if !entityTypes[r.EntityType] {
		return fmt.Errorf("unsupported entityType %q", r.EntityType)
	}
	if strings.TrimSpace(r.EntityID) == "" || strings.ContainsAny(r.EntityID, "\r\n\t") {
		return fmt.Errorf("entityId must be a non-empty single-line identifier")
	}
	if !sources[r.Source] {
		return fmt.Errorf("unsupported source %q", r.Source)
	}
	if strings.TrimSpace(r.SourceRevision) == "" {
		return fmt.Errorf("sourceRevision is required")
	}
	if r.Attributes == nil {
		return fmt.Errorf("attributes must be an object")
	}
	for name, raw := range r.Attributes {
		if strings.TrimSpace(name) == "" || len(raw) == 0 || !json.Valid(raw) {
			return fmt.Errorf("attribute %q must contain valid JSON", name)
		}
		if state := r.FieldStates[name]; state != "" && state != "invalid" && state != "empty" {
			return fmt.Errorf("attribute %q cannot have field state %q", name, state)
		}
	}
	for name, state := range r.FieldStates {
		if name == "" || !fieldStates[state] {
			return fmt.Errorf("field %q has unsupported state %q", name, state)
		}
		if state == "empty" {
			raw, ok := r.Attributes[name]
			if !ok || !isEmptyJSON(raw) {
				return fmt.Errorf("field %q marked empty must have an explicit empty attribute", name)
			}
		}
		if state != "invalid" && state != "empty" {
			if _, ok := r.Attributes[name]; ok {
				return fmt.Errorf("field %q marked %q must not also have an attribute", name, state)
			}
		}
	}
	for name, p := range r.Provenance {
		if name == "" || (r.Attributes[name] == nil && r.FieldStates[name] == "") {
			return fmt.Errorf("provenance for %q has no corresponding attribute or state", name)
		}
		if p.Source != "" || p.Revision != "" {
			if p.Source != r.Source || p.Revision != r.SourceRevision {
				return fmt.Errorf("provenance for %q does not match the record source revision", name)
			}
		}
		for _, observation := range p.Sources {
			if !sources[observation.Source] || observation.Source == "compilation" || observation.Revision == "" {
				return fmt.Errorf("provenance for %q contains an invalid source observation", name)
			}
		}
		if p.Source == "" && len(p.Sources) == 0 {
			return fmt.Errorf("provenance for %q has no source revision", name)
		}
	}
	return nil
}

func isEmptyJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte(`""`)) || bytes.Equal(trimmed, []byte("[]")) || bytes.Equal(trimmed, []byte("{}"))
}

// ReadJSONL validates every record and rejects duplicate entity keys per source.
func ReadJSONL(reader io.Reader, source, flavor, revision string) ([]Record, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var records []Record
	seen := map[string]bool{}
	line := 0
	for {
		var r Record
		err := decoder.Decode(&r)
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if r.Source != source || r.Flavor != flavor || r.SourceRevision != revision {
			return nil, fmt.Errorf("line %d: record source, flavor, or revision differs from manifest", line)
		}
		if seen[r.Key()] {
			return nil, fmt.Errorf("line %d: duplicate record %s", line, r.Key())
		}
		seen[r.Key()] = true
		records = append(records, r)
	}
	return records, nil
}

// WriteDataset writes a deterministic JSONL snapshot and checksum manifest.
func WriteDataset(outputDir string, records []Record, manifest Manifest) error {
	if outputDir == "" {
		return fmt.Errorf("output directory is required")
	}
	if !sources[manifest.Source] || !flavors[manifest.Flavor] || strings.TrimSpace(manifest.SourceRevision) == "" {
		return fmt.Errorf("manifest source, flavor, and sourceRevision are required and must be supported")
	}
	ordered := append([]Record(nil), records...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].EntityType != ordered[j].EntityType {
			return ordered[i].EntityType < ordered[j].EntityType
		}
		return ordered[i].EntityID < ordered[j].EntityID
	})
	var content bytes.Buffer
	seen := map[string]bool{}
	for i, r := range ordered {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("record %d: %w", i+1, err)
		}
		if r.Source != manifest.Source || r.Flavor != manifest.Flavor || r.SourceRevision != manifest.SourceRevision {
			return fmt.Errorf("record %d does not match manifest source, flavor, and revision", i+1)
		}
		if seen[r.Key()] {
			return fmt.Errorf("duplicate record %s", r.Key())
		}
		seen[r.Key()] = true
		encoded, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("record %s: %w", r.Key(), err)
		}
		content.Write(encoded)
		content.WriteByte('\n')
	}
	manifest.SchemaVersion = SchemaVersion
	manifest.RecordCount = len(ordered)
	hash := sha256.Sum256(content.Bytes())
	manifest.RecordsSHA256 = hex.EncodeToString(hash[:])
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestBytes = append(manifestBytes, '\n')

	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, ".dataset-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	name, stored, err := storedRecords(content.Bytes())
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(temp, name), stored, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(temp, "manifest.json"), manifestBytes, 0o644); err != nil {
		return err
	}
	old := outputDir + ".old"
	_ = os.RemoveAll(old)
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

func ReadDataset(dir string) ([]Record, Manifest, error) {
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	if manifest.SchemaVersion != SchemaVersion || !flavors[manifest.Flavor] || !sources[manifest.Source] || manifest.SourceRevision == "" {
		return nil, Manifest{}, fmt.Errorf("invalid dataset manifest")
	}
	content, err := os.ReadFile(filepath.Join(dir, "records.jsonl"))
	if os.IsNotExist(err) {
		compressed, readErr := os.ReadFile(filepath.Join(dir, "records.jsonl.gz"))
		if readErr != nil {
			return nil, Manifest{}, readErr
		}
		reader, gzipErr := gzip.NewReader(bytes.NewReader(compressed))
		if gzipErr != nil {
			return nil, Manifest{}, gzipErr
		}
		content, err = io.ReadAll(reader)
		reader.Close()
	}
	if err != nil {
		return nil, Manifest{}, err
	}
	records, err := ReadJSONL(bytes.NewReader(content), manifest.Source, manifest.Flavor, manifest.SourceRevision)
	if err != nil {
		return nil, Manifest{}, err
	}
	hash := sha256.Sum256(content)
	if manifest.RecordCount != len(records) || manifest.RecordsSHA256 != hex.EncodeToString(hash[:]) {
		return nil, Manifest{}, fmt.Errorf("dataset manifest count or checksum does not match records.jsonl")
	}
	return records, manifest, nil
}

func storedRecords(content []byte) (string, []byte, error) {
	if len(content) < 50*1024*1024 {
		return "records.jsonl", content, nil
	}
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(content); err != nil {
		return "", nil, err
	}
	if err := writer.Close(); err != nil {
		return "", nil, err
	}
	return "records.jsonl.gz", buffer.Bytes(), nil
}
