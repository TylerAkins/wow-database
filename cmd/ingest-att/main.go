package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TylerAkins/wow-database/internal/att"
	"github.com/TylerAkins/wow-database/internal/record"
)

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "att-import:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("ingest-att", flag.ContinueOnError)
	root := flags.String("att-root", "", "local AllTheThings checkout")
	revision := flags.String("revision", "", "full upstream commit SHA; defaults to checkout HEAD")
	generatedAt := flags.String("generated-at", "", "upstream commit time in RFC3339 format; defaults to commit time")
	out := flags.String("out", "data/forever/sources/allthethings", "canonical source dataset output directory")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *root == "" {
		return fmt.Errorf("--att-root is required")
	}
	sha := *revision
	if sha == "" {
		var err error
		sha, err = git(*root, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
	}
	if !fullSHA.MatchString(sha) {
		return fmt.Errorf("--revision must be a full lowercase Git commit SHA")
	}
	when := *generatedAt
	if when == "" {
		var err error
		when, err = git(*root, "show", "-s", "--format=%cI", sha)
		if err != nil {
			return err
		}
	}
	if _, err := time.Parse(time.RFC3339, when); err != nil {
		return fmt.Errorf("--generated-at must be RFC3339: %w", err)
	}
	rows, meta, err := att.Export(*root)
	if err != nil {
		return err
	}
	catalog, err := att.ReadEntityCatalog(*root)
	if err != nil {
		return err
	}
	records, err := normalize(rows, meta.Unmapped, sha)
	if err != nil {
		return err
	}
	catalogRecords, err := normalizeEntities(catalog, sha)
	if err != nil {
		return err
	}
	records = append(records, catalogRecords...)
	limitations := []string{
		"Exports supported Forever quest declarations and explicit ATT NPC, item, object, and achievement declarations with literal labels, selected fields, source paths, and nested containment relationships.",
		"ATT is a nested catalog, not a complete entity database. Unrecognized Lua expressions and fields, standalone map/zone definitions, and explicit NPC/item/object coordinates are not normalized.",
		"Quest-provider coordinate expressions outside the supported literal grammar fail ingestion instead of being evaluated.",
	}
	if len(meta.Unmapped) > 0 {
		limitations = append(limitations, fmt.Sprintf("%d quest-provider coordinate association(s) could not be mapped and are included as issue records.", len(meta.Unmapped)))
	}
	manifest := record.Manifest{Source: "allthethings", Flavor: "forever", SourceRevision: sha, GeneratedAt: when, Status: "partial", Limitations: limitations}
	if err := record.WriteDataset(*out, records, manifest); err != nil {
		return err
	}
	fmt.Printf("ATT Forever: wrote %d records from %s\n", len(records), sha)
	return nil
}

func git(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(output)), nil
}

func normalize(rows []map[string]any, unmapped []att.Report, revision string) ([]record.Record, error) {
	var out []record.Record
	for _, row := range rows {
		id, ok := row["id"].(int)
		if !ok || id <= 0 {
			return nil, fmt.Errorf("ATT quest has invalid ID %v", row["id"])
		}
		fields, ok := row["fields"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("ATT quest %d has no fields object", id)
		}
		attributes := map[string]json.RawMessage{}
		provenance := map[string]record.FieldProvenance{}
		for name, value := range fields {
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("ATT quest %d field %s: %w", id, name, err)
			}
			attributes[name] = encoded
			provenance[name] = record.FieldProvenance{Source: "allthethings", Revision: revision, Path: "ATT .contrib/.db/forever"}
		}
		for _, name := range []string{"startedBy", "finishedBy", "places"} {
			if value, exists := row[name]; exists {
				encoded, err := json.Marshal(value)
				if err != nil {
					return nil, fmt.Errorf("ATT quest %d field %s: %w", id, name, err)
				}
				attributes[name] = encoded
				provenance[name] = record.FieldProvenance{Source: "allthethings", Revision: revision, Path: "ATT .contrib/.db/forever"}
			}
		}
		out = append(out, record.Record{SchemaVersion: record.SchemaVersion, Flavor: "forever", EntityType: "quest", EntityID: strconv.Itoa(id), Source: "allthethings", SourceRevision: revision, Attributes: attributes, Provenance: provenance})
		links, err := relationshipRows(id, row, fields, revision)
		if err != nil {
			return nil, err
		}
		out = append(out, links...)
	}
	for _, issue := range unmapped {
		attrs := map[string]json.RawMessage{}
		issueData, _ := json.Marshal(map[string]any{"kind": "unmapped-coordinate", "questId": issue.QuestID, "npcId": issue.NPCID, "file": issue.File, "reason": issue.Reason})
		attrs["issue"] = issueData
		id := fmt.Sprintf("issue:unmapped-coordinate:%d:%d:%s", issue.QuestID, issue.NPCID, filepath.Base(issue.File))
		out = append(out, record.Record{SchemaVersion: record.SchemaVersion, Flavor: "forever", EntityType: "relationship", EntityID: id, Source: "allthethings", SourceRevision: revision, Attributes: attrs, FieldStates: map[string]string{"resolvedCoordinate": "unavailable"}, Provenance: map[string]record.FieldProvenance{"issue": {Source: "allthethings", Revision: revision, Path: issue.File}, "resolvedCoordinate": {Source: "allthethings", Revision: revision, Path: issue.File}}})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EntityType != out[j].EntityType {
			return out[i].EntityType < out[j].EntityType
		}
		return out[i].EntityID < out[j].EntityID
	})
	return out, nil
}

func normalizeEntities(catalog att.EntityCatalog, revision string) ([]record.Record, error) {
	type observation struct {
		Path   string         `json:"path"`
		Label  string         `json:"label,omitempty"`
		Fields map[string]any `json:"fields,omitempty"`
	}
	type edgeObservation struct {
		Path string `json:"path"`
	}
	entityObservations := map[string][]observation{}
	entityTypes := map[string]string{}
	for _, declaration := range catalog.Declarations {
		key := declaration.EntityType + ":" + strconv.Itoa(declaration.EntityID)
		entityTypes[key] = declaration.EntityType
		entityObservations[key] = append(entityObservations[key], observation{
			Path: declaration.SourcePath, Label: declaration.Label, Fields: declaration.Fields,
		})
	}
	edgeObservations := map[string][]edgeObservation{}
	for _, edge := range catalog.Containments {
		id := fmt.Sprintf("att:contains:%s:%d:%s:%d", edge.ParentType, edge.ParentID, edge.ChildType, edge.ChildID)
		seen := false
		for _, existing := range edgeObservations[id] {
			if existing.Path == edge.SourcePath {
				seen = true
				break
			}
		}
		if !seen {
			edgeObservations[id] = append(edgeObservations[id], edgeObservation{Path: edge.SourcePath})
		}
	}
	var out []record.Record
	for key, rows := range entityObservations {
		separator := strings.IndexByte(key, ':')
		kind, id := key[:separator], key[separator+1:]
		encoded, err := json.Marshal(rows)
		if err != nil {
			return nil, fmt.Errorf("ATT %s %s observations: %w", kind, id, err)
		}
		attributes := map[string]json.RawMessage{"observations": encoded}
		out = append(out, record.Record{
			SchemaVersion: record.SchemaVersion, Flavor: "forever", EntityType: entityTypes[key], EntityID: id,
			Source: "allthethings", SourceRevision: revision, Attributes: attributes,
			Provenance: map[string]record.FieldProvenance{"observations": {Source: "allthethings", Revision: revision, Path: ".contrib/.db/forever"}},
		})
	}
	for id, rows := range edgeObservations {
		edgeParts := strings.Split(id, ":")
		if len(edgeParts) != 6 {
			return nil, fmt.Errorf("invalid ATT containment identifier %q", id)
		}
		parentID, err := strconv.Atoi(edgeParts[3])
		if err != nil {
			return nil, err
		}
		childID, err := strconv.Atoi(edgeParts[5])
		if err != nil {
			return nil, err
		}
		relationship, err := json.Marshal(map[string]any{
			"kind": "att-group-containment", "fromType": edgeParts[2], "fromId": strconv.Itoa(parentID),
			"toType": edgeParts[4], "toId": strconv.Itoa(childID),
		})
		if err != nil {
			return nil, err
		}
		observations, err := json.Marshal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record.Record{
			SchemaVersion: record.SchemaVersion, Flavor: "forever", EntityType: "relationship", EntityID: id,
			Source: "allthethings", SourceRevision: revision,
			Attributes: map[string]json.RawMessage{"relationship": relationship, "observations": observations},
			Provenance: map[string]record.FieldProvenance{
				"relationship": {Source: "allthethings", Revision: revision, Path: ".contrib/.db/forever"},
				"observations": {Source: "allthethings", Revision: revision, Path: ".contrib/.db/forever"},
			},
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EntityType != out[j].EntityType {
			return out[i].EntityType < out[j].EntityType
		}
		return out[i].EntityID < out[j].EntityID
	})
	return out, nil
}

func relationshipRows(questID int, row map[string]any, fields map[string]any, revision string) ([]record.Record, error) {
	var out []record.Record
	appendLink := func(id string, attributes map[string]any) error {
		relationship, err := json.Marshal(attributes)
		if err != nil {
			return err
		}
		encoded := map[string]json.RawMessage{"relationship": relationship}
		provenance := map[string]record.FieldProvenance{}
		provenance["relationship"] = record.FieldProvenance{Source: "allthethings", Revision: revision, Path: "ATT .contrib/.db/forever"}
		out = append(out, record.Record{SchemaVersion: record.SchemaVersion, Flavor: "forever", EntityType: "relationship", EntityID: id, Source: "allthethings", SourceRevision: revision, Attributes: encoded, Provenance: provenance})
		return nil
	}
	for _, role := range []string{"startedBy", "finishedBy"} {
		providers, _ := row[role].([]any)
		for _, value := range providers {
			provider, ok := value.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := provider["type"].(string)
			id, ok := provider["id"].(int)
			if !ok || kind == "" {
				continue
			}
			attrs := map[string]any{"kind": "quest-provider", "fromType": "quest", "fromId": strconv.Itoa(questID), "toType": kind, "toId": strconv.Itoa(id), "role": role}
			if spawns, exists := provider["spawns"]; exists {
				attrs["spawns"] = spawns
			}
			linkID := fmt.Sprintf("quest:%d:%s:%s:%d", questID, role, kind, id)
			if err := appendLink(linkID, attrs); err != nil {
				return nil, err
			}
		}
	}
	for _, field := range []string{"preQuestSingle", "preQuestGroup", "exclusiveTo", "breadcrumbs"} {
		ids, ok := fields[field].([]any)
		if !ok {
			continue
		}
		for index, value := range ids {
			id, ok := value.(int)
			if !ok {
				continue
			}
			attrs := map[string]any{"kind": field, "fromType": "quest", "fromId": strconv.Itoa(questID), "toType": "quest", "toId": strconv.Itoa(id), "order": index}
			linkID := fmt.Sprintf("quest:%d:%s:%d:%d", questID, field, index, id)
			if err := appendLink(linkID, attrs); err != nil {
				return nil, err
			}
		}
	}
	if next, ok := fields["nextQuestInChain"].(int); ok && next > 0 {
		attrs := map[string]any{"kind": "nextQuestInChain", "fromType": "quest", "fromId": strconv.Itoa(questID), "toType": "quest", "toId": strconv.Itoa(next)}
		if err := appendLink(fmt.Sprintf("quest:%d:next:%d", questID, next), attrs); err != nil {
			return nil, err
		}
	}
	return out, nil
}
