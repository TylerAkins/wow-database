package record

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validRecord() Record {
	return Record{
		SchemaVersion: SchemaVersion, Flavor: "forever", EntityType: "quest", EntityID: "10",
		Source: "questiedb", SourceRevision: "abc123",
		Attributes:  map[string]json.RawMessage{"requiredLevel": json.RawMessage("0"), "objectives": json.RawMessage("[]")},
		FieldStates: map[string]string{"description": "unavailable", "objectives": "empty"},
		Provenance:  map[string]FieldProvenance{"requiredLevel": {Source: "questiedb", Revision: "abc123"}},
	}
}

func TestRecordValidatesZeroEmptyAndUnavailableSeparately(t *testing.T) {
	if err := validRecord().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordRejectsFalseEmptyAndContradictoryStates(t *testing.T) {
	r := validRecord()
	r.Attributes["objectives"] = json.RawMessage("[1]")
	if err := r.Validate(); err == nil {
		t.Fatal("nonempty value marked empty was accepted")
	}
	r = validRecord()
	r.FieldStates["requiredLevel"] = "unavailable"
	if err := r.Validate(); err == nil {
		t.Fatal("value marked unavailable was accepted")
	}
}

func TestReadJSONLRejectsDuplicateKeysAndManifestMismatch(t *testing.T) {
	r := validRecord()
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	input := string(body) + "\n" + string(body) + "\n"
	if _, err := ReadJSONL(strings.NewReader(input), "questiedb", "forever", "abc123"); err == nil || !strings.Contains(err.Error(), "duplicate record") {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := ReadJSONL(strings.NewReader(string(body)), "questiedb", "era", "abc123"); err == nil {
		t.Fatal("manifest flavor mismatch was accepted")
	}
}

func TestDatasetRoundTripIsDeterministicAndChecksummed(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshot")
	records := []Record{validRecord()}
	manifest := Manifest{Source: "questiedb", Flavor: "forever", SourceRevision: "abc123"}
	if err := WriteDataset(dir, records, manifest); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "records.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	loaded, gotManifest, err := ReadDataset(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || gotManifest.RecordCount != 1 || gotManifest.RecordsSHA256 == "" {
		t.Fatalf("round-trip records=%d manifest=%+v", len(loaded), gotManifest)
	}
	if err := WriteDataset(dir, records, manifest); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(dir, "records.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("identical inputs produced different source records")
	}
	if err := os.WriteFile(filepath.Join(dir, "records.jsonl"), []byte("corrupt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadDataset(dir); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
}

func TestLargeDatasetCompressedRoundTripAndReplacement(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dataset")
	r := validRecord()
	r.Attributes["bulk"] = json.RawMessage(`"` + strings.Repeat("x", 50*1024*1024) + `"`)
	manifest := Manifest{Source: r.Source, Flavor: r.Flavor, SourceRevision: r.SourceRevision}
	if err := WriteDataset(dir, []Record{r}, manifest); err != nil {
		t.Fatal(err)
	}
	compressedPath := filepath.Join(dir, "records.jsonl.gz")
	first, err := os.ReadFile(compressedPath)
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := ReadDataset(dir)
	if err != nil || len(rows) != 1 || string(rows[0].Attributes["bulk"]) != string(r.Attributes["bulk"]) {
		t.Fatalf("compressed round trip failed: %v", err)
	}
	if err := WriteDataset(dir, []Record{r}, manifest); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(compressedPath)
	if err != nil || string(first) != string(second) {
		t.Fatalf("compressed output changed: %v", err)
	}
	if err := os.WriteFile(compressedPath, first[:len(first)-8], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadDataset(dir); err == nil {
		t.Fatal("accepted truncated gzip")
	}
	delete(r.Attributes, "bulk")
	if err := WriteDataset(dir, []Record{r}, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(compressedPath); !os.IsNotExist(err) {
		t.Fatalf("stale compressed output retained: %v", err)
	}
	if _, _, err := ReadDataset(dir); err != nil {
		t.Fatal(err)
	}
}

func TestReadJSONLPreservesStaticProviderProvenance(t *testing.T) {
	input := `{"schemaVersion":1,"flavor":"forever","entityType":"quest","entityId":"10","source":"questiedb","sourceRevision":"abc123","attributes":{"requiredLevel":20},"provenance":{"requiredLevel":{"source":"questiedb","revision":"abc123","path":"src/corrections/Forever/quest.lua","provider":"Forever/quest.lua:Load"}}}`
	rows, err := ReadJSONL(strings.NewReader(input), "questiedb", "forever", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(rows[0])
	if err != nil || !strings.Contains(string(encoded), `"provider":"Forever/quest.lua:Load"`) {
		t.Fatalf("provider lost in round trip: %s %v", encoded, err)
	}
}
