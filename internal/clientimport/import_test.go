package clientimport

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/TylerAkins/wow-database/internal/record"
)

func TestImportSelectedTablesToCanonicalSnapshot(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input")
	if err := os.MkdirAll(filepath.Join(input, "tables"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schemaVersion":1,"gameVersion":"forever","source":"foreverclient","client":{"product":"wow_classic","version":"1.60.1","buildNumber":70338},"definitions":{"repositoryRevision":"dbd-sha"},"tables":[{"name":"Faction","fileDataId":1,"status":"written","recordCount":2,"definitionSha256":"def-hash"},{"name":"UnknownTable","fileDataId":2,"status":"written","recordCount":1,"definitionSha256":"other"}]}`
	if err := os.WriteFile(filepath.Join(input, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := "{\"kind\":\"table\",\"schemaVersion\":1,\"gameVersion\":\"forever\",\"source\":\"foreverclient\",\"table\":\"Faction\",\"fileDataId\":1,\"clientBuild\":70338,\"definitionSha256\":\"def-hash\"}\n" +
		"{\"kind\":\"record\",\"id\":1,\"fields\":{\"name\":\"Alliance\",\"flags\":0}}\n" +
		"{\"kind\":\"record\",\"id\":2,\"fields\":{\"name\":\"\",\"description\":null}}\n"
	if err := os.WriteFile(filepath.Join(input, "tables", "Faction.jsonl"), []byte(rows), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "canonical")
	if err := Import(input, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "records.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var got []record.Record
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var row record.Record
		err := decoder.Decode(&row)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	if len(got) != 2 || got[0].EntityType != "faction" {
		t.Fatalf("records = %#v", got)
	}
	if string(got[0].Attributes["flags"]) != "0" {
		t.Fatalf("zero field lost: %s", got[0].Attributes["flags"])
	}
	if got[1].FieldStates["description"] != "unavailable" {
		t.Fatalf("null state = %#v", got[1].FieldStates)
	}
	loaded, _, err := record.ReadDataset(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d rows", len(loaded))
	}
}

func TestImportRejectsHeaderManifestMismatchAndPreservesOldOutput(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input")
	if err := os.MkdirAll(filepath.Join(input, "tables"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schemaVersion":1,"gameVersion":"forever","source":"foreverclient","client":{"version":"1.60.1","buildNumber":70338},"definitions":{},"tables":[{"name":"Faction","fileDataId":1,"status":"written","recordCount":1,"definitionSha256":"right"}]}`
	if err := os.WriteFile(filepath.Join(input, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := "{\"kind\":\"table\",\"schemaVersion\":1,\"gameVersion\":\"forever\",\"source\":\"foreverclient\",\"table\":\"Faction\",\"clientBuild\":70338,\"definitionSha256\":\"wrong\"}\n"
	if err := os.WriteFile(filepath.Join(input, "tables", "Faction.jsonl"), []byte(rows), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "canonical")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "marker"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Import(input, out); err == nil {
		t.Fatal("Import succeeded with mismatched table header")
	}
	marker, err := os.ReadFile(filepath.Join(out, "marker"))
	if err != nil || string(marker) != "keep" {
		t.Fatalf("old output changed: %q, %v", marker, err)
	}
}
