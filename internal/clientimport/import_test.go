package clientimport

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
	"testing"

	"github.com/TylerAkins/wow-database/internal/database"
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

func questExtraction(t *testing.T, compressed bool) string {
	t.Helper()
	dir := t.TempDir()
	names := []string{"QuestV2", "QuestInfo", "QuestLineXQuest", "QuestPOIBlob", "QuestPOIPoint"}
	fields := []string{`{"ID":42,"UniqueBitFlag":0,"UiQuestDetailsThemeID":null,"FutureMask":18446744073709551615}`, `{"ID":42,"InfoName_lang":"Elite"}`, `{"ID":42,"QuestID":42,"QuestLineID":7,"OrderIndex":0}`, `{"ID":42,"QuestID":42,"MapID":1,"UiMapID":2482}`, `{"ID":42,"QuestPOIBlobID":42,"X":5147,"Y":-1466,"Z":1344}`}
	tables := []map[string]any{}
	for i, name := range names {
		count := 1
		if name == "QuestV2" {
			count = 2
		}
		tables = append(tables, map[string]any{"name": name, "fileDataId": i + 1, "status": "partial_encrypted", "recordCount": count, "definitionSha256": "definition"})
		content := fmt.Sprintf(`{"kind":"table","schemaVersion":1,"gameVersion":"forever","source":"foreverclient","table":%q,"fileDataId":%d,"clientBuild":70338,"definitionSha256":"definition"}`+"\n"+`{"kind":"record","id":42,"fields":%s}`+"\n", name, i+1, fields[i])
		if name == "QuestV2" {
			content += `{"kind":"record","id":43,"fields":{"ID":43,"UniqueBitFlag":123}}` + "\n"
		}
		filename := name + ".jsonl"
		body := []byte(content)
		if compressed {
			filename += ".gz"
			var buffer bytes.Buffer
			writer := gzip.NewWriter(&buffer)
			if _, err := writer.Write(body); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			body = buffer.Bytes()
		}
		if err := os.WriteFile(filepath.Join(dir, filename), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := map[string]any{"schemaVersion": 1, "gameVersion": "forever", "source": "foreverclient", "client": map[string]any{"version": "1.60.1.70338", "buildNumber": 70338}, "definitions": map[string]any{"repositoryRevision": "dbd-sha"}, "tables": tables}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestQuestTablesImportAndCompileWithoutDuplicateQuestIDs(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(fmt.Sprintf("compressed=%v", compressed), func(t *testing.T) {
			input := questExtraction(t, compressed)
			sources := filepath.Join(t.TempDir(), "sources")
			out := filepath.Join(sources, "foreverclient")
			if err := Import(input, out); err != nil {
				t.Fatal(err)
			}
			rows, _, err := record.ReadDataset(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 6 {
				t.Fatalf("expected two quests and four support records, got %d", len(rows))
			}
			var quest record.Record
			for _, row := range rows {
				if row.EntityType == "quest" && row.EntityID == "42" {
					quest = row
				}
				if row.EntityType != "quest" && row.EntityID == "42" {
					t.Fatalf("support row uses quest identity: %+v", row)
				}
			}
			if string(quest.Attributes["UniqueBitFlag"]) != "0" || quest.FieldStates["UiQuestDetailsThemeID"] != "unavailable" || string(quest.Attributes["extractionStatus"]) != `"partial_encrypted"` {
				t.Fatalf("quest metadata lost: %+v", quest)
			}
			if string(quest.Attributes["FutureMask"]) != "18446744073709551615" {
				t.Fatalf("wide integer changed: %s", quest.Attributes["FutureMask"])
			}
			if quest.Provenance["UniqueBitFlag"].Path != "QuestV2.jsonl" {
				t.Fatalf("incorrect provenance: %+v", quest.Provenance)
			}
			att := record.Record{SchemaVersion: 1, Flavor: "forever", EntityType: "quest", EntityID: "42", Source: "allthethings", SourceRevision: "att", Attributes: map[string]json.RawMessage{"name": json.RawMessage(`"ATT title"`)}}
			if err := record.WriteDataset(filepath.Join(sources, "allthethings"), []record.Record{att}, record.Manifest{Flavor: "forever", Source: "allthethings", SourceRevision: "att"}); err != nil {
				t.Fatal(err)
			}
			compiled := filepath.Join(t.TempDir(), "compilation")
			if err := database.Compile("forever", sources, compiled, ""); err != nil {
				t.Fatal(err)
			}
			merged, _, err := record.ReadDataset(compiled)
			if err != nil {
				t.Fatal(err)
			}
			quests := map[string]record.Record{}
			for _, row := range merged {
				if row.EntityType == "quest" {
					if _, exists := quests[row.EntityID]; exists {
						t.Fatal("duplicate quest")
					}
					quests[row.EntityID] = row
				}
			}
			if len(quests) != 2 || string(quests["42"].Attributes["name"]) != `"ATT title"` || string(quests["42"].Attributes["UniqueBitFlag"]) != "0" {
				t.Fatalf("bad merged quests: %+v", quests)
			}
		})
	}
}

func TestArchiveChecksumMismatchPreservesSnapshot(t *testing.T) {
	input := questExtraction(t, true)
	manifest, err := os.ReadFile(filepath.Join(input, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(manifest)
	index := fmt.Sprintf(`{"manifestSha256":%q,"files":{"QuestV2.jsonl.gz":{"recordsSha256":"incorrect","recordCount":2}}}`, hex.EncodeToString(hash[:]))
	if err := os.WriteFile(filepath.Join(input, "files.json"), []byte(index), 0o600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "marker"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Import(input, out); err == nil {
		t.Fatal("accepted mismatched raw-table checksum")
	}
	marker, err := os.ReadFile(filepath.Join(out, "marker"))
	if err != nil || string(marker) != "keep" {
		t.Fatalf("previous snapshot changed: %v", err)
	}
}
