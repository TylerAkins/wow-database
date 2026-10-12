package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TylerAkins/wow-database/internal/record"
)

func sourceRecord(source, revision string, attrs map[string]string, states map[string]string) record.Record {
	values := map[string]json.RawMessage{}
	for field, value := range attrs {
		values[field] = json.RawMessage(value)
	}
	return record.Record{SchemaVersion: record.SchemaVersion, Flavor: "forever", EntityType: "quest", EntityID: "42", Source: source, SourceRevision: revision, Attributes: values, FieldStates: states}
}

func writeSource(t *testing.T, root, source string, rows ...record.Record) string {
	t.Helper()
	dir := filepath.Join(root, source)
	if err := record.WriteDataset(dir, rows, record.Manifest{Source: source, Flavor: "forever", SourceRevision: "rev-" + source}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCompileRetainsComplementaryFieldsAndUnresolvedConflict(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sources")
	_ = os.MkdirAll(root, 0o755)
	writeSource(t, root, "wowhead", sourceRecord("wowhead", "rev-wowhead", map[string]string{"name": `"From Wowhead"`, "coordinates": `[]`}, map[string]string{"coordinates": "empty", "details": "unavailable"}))
	writeSource(t, root, "questiedb", sourceRecord("questiedb", "rev-questiedb", map[string]string{"name": `"From Questie"`, "preQuestSingle": `[7]`}, nil))
	out := filepath.Join(t.TempDir(), "compilation")
	if err := Compile("forever", root, out, ""); err != nil {
		t.Fatal(err)
	}
	rows, manifest, err := record.ReadDataset(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || manifest.Source != "compilation" {
		t.Fatalf("records=%d manifest=%+v", len(rows), manifest)
	}
	row := rows[0]
	if string(row.Attributes["preQuestSingle"]) != "[7]" {
		t.Fatalf("prerequisites lost: %s", row.Attributes["preQuestSingle"])
	}
	if row.FieldStates["name"] != "conflicting" {
		t.Fatalf("name conflict state = %q", row.FieldStates["name"])
	}
	if row.FieldStates["coordinates"] != "empty" || string(row.Attributes["coordinates"]) != "[]" {
		t.Fatalf("explicit empty lost: %#v %#v", row.FieldStates, row.Attributes)
	}
	if row.FieldStates["details"] != "unavailable" {
		t.Fatalf("unavailable state lost: %#v", row.FieldStates)
	}
	if got := len(row.Provenance["preQuestSingle"].Sources); got != 1 || row.Provenance["preQuestSingle"].Sources[0].Source != "questiedb" {
		t.Fatalf("field provenance = %+v", row.Provenance["preQuestSingle"])
	}
	var conflicts []Conflict
	data, err := os.ReadFile(filepath.Join(out, "conflicts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &conflicts); err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0].Field != "name" || conflicts[0].Selected != "" || len(conflicts[0].Candidates) != 2 {
		t.Fatalf("conflicts = %+v", conflicts)
	}
}

func TestCompileUsesExplicitFieldPolicyAndIsDeterministic(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sources")
	_ = os.MkdirAll(root, 0o755)
	writeSource(t, root, "wowhead", sourceRecord("wowhead", "rev-wowhead", map[string]string{"name": `"Web name"`}, nil))
	writeSource(t, root, "questiedb", sourceRecord("questiedb", "rev-questiedb", map[string]string{"name": `"Questie name"`}, nil))
	policy := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(policy, []byte(`{"fields":{"quest.name":["questiedb"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "compiled")
	if err := Compile("forever", root, out, policy); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(out, "records.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []record.Record
	if rows, _, err = record.ReadDataset(out); err != nil {
		t.Fatal(err)
	}
	if string(rows[0].Attributes["name"]) != `"Questie name"` || rows[0].Provenance["name"].SelectionRule == "" {
		t.Fatalf("policy was not recorded: %+v", rows[0])
	}
	var conflicts []Conflict
	conflictData, err := os.ReadFile(filepath.Join(out, "conflicts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(conflictData, &conflicts); err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0].Selected != "questiedb" {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	if err := Compile("forever", root, out, policy); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(out, "records.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("repeated compilation changed records")
	}
}

func TestCompileFailurePreservesPublishedOutput(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sources")
	_ = os.MkdirAll(root, 0o755)
	writeSource(t, root, "wowhead", sourceRecord("wowhead", "rev-wowhead", map[string]string{"name": `"Stable"`}, nil))
	out := filepath.Join(t.TempDir(), "compiled")
	if err := Compile("forever", root, out, ""); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(out, "records.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "wowhead", "records.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := Compile("forever", root, out, ""); err == nil {
		t.Fatal("missing source records accepted")
	}
	after, err := os.ReadFile(filepath.Join(out, "records.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed compile replaced published output")
	}
}

func TestCompareReportsSemanticFieldsAndFlavorBoundary(t *testing.T) {
	root := t.TempDir()
	left := writeSource(t, root, "wowhead", sourceRecord("wowhead", "rev-wowhead", map[string]string{"name": `"A"`, "requiredLevel": "20"}, nil))
	rightRows := []record.Record{sourceRecord("questiedb", "rev-questiedb", map[string]string{"name": `"B"`, "requiredLevel": "20"}, nil)}
	right := filepath.Join(root, "other")
	if err := record.WriteDataset(right, rightRows, record.Manifest{Source: "questiedb", Flavor: "forever", SourceRevision: "rev-questiedb"}); err != nil {
		t.Fatal(err)
	}
	comparison, err := Compare(left, right, false)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Counts.Changed != 1 || len(comparison.Entities[0].Differences) != 1 || comparison.Entities[0].Differences[0].Path != "name" {
		t.Fatalf("comparison = %+v", comparison)
	}
	reportDir := filepath.Join(t.TempDir(), "report")
	if err := WriteComparison(reportDir, comparison); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(reportDir, "comparison.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCompareCoordinateToleranceRequiresMatchingMapOrZone(t *testing.T) {
	root := t.TempDir()
	left := writeSource(t, root, "wowhead", sourceRecord("wowhead", "rev-wowhead", map[string]string{"position": `{"zone":14,"x":20.0,"y":30.0}`}, nil))
	writeAt := func(name, revision, position string) string {
		dir := filepath.Join(root, name)
		row := sourceRecord(name, revision, map[string]string{"position": position}, nil)
		if err := record.WriteDataset(dir, []record.Record{row}, record.Manifest{Source: name, Flavor: "forever", SourceRevision: revision}); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	nearby := writeAt("questiedb", "rev-near", `{"zone":14,"x":20.005,"y":29.995}`)
	comparison, err := CompareWithTolerance(left, nearby, false, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Counts.Equal != 1 {
		t.Fatalf("nearby coordinates should match within tolerance: %+v", comparison)
	}
	differentZone := writeAt("questiedb", "rev-other", `{"zone":15,"x":20.005,"y":29.995}`)
	comparison, err = CompareWithTolerance(left, differentZone, false, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Counts.Changed != 1 {
		t.Fatalf("different zone must not use coordinate tolerance: %+v", comparison)
	}
	if _, err := CompareWithTolerance(left, nearby, false, -1); err == nil {
		t.Fatal("negative tolerance was accepted")
	}
}

func TestValidateTreeCountsDanglingReferencesWithoutRejectingSnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data", "forever", "sources")
	_ = os.MkdirAll(root, 0o755)
	relation := record.Record{SchemaVersion: 1, Flavor: "forever", EntityType: "relationship", EntityID: "quest:1->npc:9", Source: "allthethings", SourceRevision: "rev-allthethings", Attributes: map[string]json.RawMessage{"relationship": json.RawMessage(`{"fromType":"quest","fromId":"1","toType":"npc","toId":"9","kind":"giver"}`)}}
	writeSource(t, root, "allthethings", relation)
	summary, err := ValidateTree(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil || len(summary) != 0 {
		t.Fatal("missing root accepted")
	}
	summary, err = ValidateTree(filepath.Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 1 || summary[0].DanglingRelationships != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if !strings.Contains(summary[0].Path, "allthethings") {
		t.Fatalf("dataset path = %q", summary[0].Path)
	}
}

func TestForeverCompilationIncludesBaselineAndAdditionsWithoutDuplicateQuests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sources")
	baseline := sourceRecord("questiedb", "rev-questiedb", map[string]string{"name": `"Classic baseline"`}, nil)
	writeSource(t, root, "questiedb", baseline)
	shared := sourceRecord("allthethings", "rev-allthethings", map[string]string{"name": `"Classic baseline"`}, nil)
	addition := sourceRecord("allthethings", "rev-allthethings", map[string]string{"name": `"Forever addition"`}, nil)
	addition.EntityID = "90001"
	writeSource(t, root, "allthethings", shared, addition)
	writeSource(t, root, "wowhead", sourceRecord("wowhead", "rev-wowhead", map[string]string{"name": `"Classic baseline"`}, nil))
	out := filepath.Join(t.TempDir(), "compilation")
	if err := Compile("forever", root, out, ""); err != nil {
		t.Fatal(err)
	}
	rows, _, err := record.ReadDataset(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].EntityID != "42" || rows[1].EntityID != "90001" {
		t.Fatalf("expected one baseline and one addition: %+v", rows)
	}
	if len(rows[0].Provenance["name"].Sources) != 3 {
		t.Fatalf("shared quest provenance lost: %+v", rows[0].Provenance)
	}
	if err := Compile("era", root, out, ""); err == nil {
		t.Fatal("accepted separate Era compilation")
	}
}

func TestCompareIgnoresOnlySourceAuditRevisionChurn(t *testing.T) {
	l := record.Record{SchemaVersion: 1, Flavor: "forever", EntityType: "npc", EntityID: "5", Source: "compilation", SourceRevision: "compiled-old", Attributes: map[string]json.RawMessage{"name": json.RawMessage(`"A"`), "sourceData": json.RawMessage(`{"wowhead":{"revision":"revision-old","attributes":{"name":"A"},"provenance":{"name":{"source":"wowhead","revision":"revision-old","path":"npcs.lua"}}}}`)}}
	r := l
	r.SourceRevision = "compiled-new"
	r.Attributes = map[string]json.RawMessage{"name": json.RawMessage(`"A"`), "sourceData": json.RawMessage(`{"wowhead":{"revision":"revision-new","attributes":{"name":"A"},"provenance":{"name":{"source":"wowhead","revision":"revision-new","path":"npcs.lua"}}}}`)}
	differences, err := diffRecords(l, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(differences) != 0 {
		t.Fatalf("revision churn changed semantics: %+v", differences)
	}
	r.Attributes["sourceData"] = json.RawMessage(`{"wowhead":{"revision":"revision-new","attributes":{"name":"B"},"provenance":{"name":{"source":"wowhead","revision":"revision-new","path":"npcs.lua"}}}}`)
	differences, err = diffRecords(l, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(differences) == 0 {
		t.Fatal("original source value change was ignored")
	}
}
