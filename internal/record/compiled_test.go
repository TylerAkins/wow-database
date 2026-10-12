package record

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCompilationVersionCompatibility(t *testing.T) {
	row := Record{SchemaVersion: 1, Flavor: "forever", Source: "compilation", SourceRevision: "rev", EntityType: "quest", EntityID: "1", Attributes: map[string]json.RawMessage{"name": json.RawMessage(`"Quest"`)}}
	manifest := Manifest{Flavor: "forever", Source: "compilation", SourceRevision: "rev"}
	dir := filepath.Join(t.TempDir(), "compiled")
	if err := WriteDataset(dir, []Record{row}, manifest); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadDataset(dir); err != nil {
		t.Fatal("legacy compilation rejected", err)
	}
	manifest.CompilationFormatVersion = 2
	if err := WriteDataset(dir, []Record{row}, manifest); err == nil {
		t.Fatal("format 2 without audit accepted")
	}
	row.Attributes["sourceData"] = json.RawMessage(`{"questiedb":{"revision":"q","attributes":{"name":"Quest"}}}`)
	if err := WriteDataset(dir, []Record{row}, manifest); err != nil {
		t.Fatal(err)
	}
	row.Attributes["questUiThemeId"] = json.RawMessage(`1.5`)
	if err := WriteDataset(dir, []Record{row}, manifest); err == nil {
		t.Fatal("fractional quest UI metadata accepted")
	}
	delete(row.Attributes, "questUiThemeId")
	row.Attributes["startedBy"] = json.RawMessage(`[[9]]`)
	if err := WriteDataset(dir, []Record{row}, manifest); err == nil {
		t.Fatal("legacy provider shape accepted as format 2")
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	m["compilationFormatVersion"] = 3
	data, _ = json.Marshal(m)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600)
	if _, _, err := ReadDataset(dir); err == nil {
		t.Fatal("future version accepted")
	}
	manifest.Source = "questiedb"
	manifest.CompilationFormatVersion = 2
	if err := WriteDataset(dir, nil, manifest); err == nil {
		t.Fatal("format marker accepted on source dataset")
	}
}
