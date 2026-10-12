// Package clientimport converts local DB2 extractor output to canonical records.
package clientimport

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TylerAkins/wow-database/internal/record"
)

type report struct {
	SchemaVersion int    `json:"schemaVersion"`
	GameVersion   string `json:"gameVersion"`
	Source        string `json:"source"`
	Client        struct {
		Version     string `json:"version"`
		BuildNumber uint32 `json:"buildNumber"`
	} `json:"client"`
	Definitions struct {
		RepositoryRevision string `json:"repositoryRevision"`
	} `json:"definitions"`
	Tables []struct {
		Name             string `json:"name"`
		FileDataID       uint32 `json:"fileDataId"`
		Status           string `json:"status"`
		RecordCount      int    `json:"recordCount"`
		DefinitionSHA256 string `json:"definitionSha256"`
	} `json:"tables"`
}

type tableHeader struct {
	Kind             string `json:"kind"`
	SchemaVersion    int    `json:"schemaVersion"`
	GameVersion      string `json:"gameVersion"`
	Source           string `json:"source"`
	Table            string `json:"table"`
	FileDataID       uint32 `json:"fileDataId"`
	ClientBuild      uint32 `json:"clientBuild"`
	DefinitionSHA256 string `json:"definitionSha256"`
}

type tableRow struct {
	Kind   string         `json:"kind"`
	ID     int32          `json:"id"`
	Fields map[string]any `json:"fields"`
}

var tableTypes = map[string]string{
	"Achievement": "achievement",
	"AreaTable":   "zone",
	"Creature":    "npc",
	"Faction":     "faction",
	"ItemSparse":  "item",
	"Map":         "map",
	"SpellName":   "spell",
}

// Import reads a completed local extraction and atomically writes its canonical dataset.
func Import(inputDir, outputDir string) error {
	data, err := os.ReadFile(filepath.Join(inputDir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read client extraction manifest: %w", err)
	}
	var input report
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("decode client extraction manifest: %w", err)
	}
	if input.SchemaVersion != 1 || input.GameVersion != "forever" || input.Source != "foreverclient" || input.Client.Version == "" || input.Client.BuildNumber == 0 {
		return fmt.Errorf("client extraction manifest has unsupported or missing identity fields")
	}
	manifestHash := sha256.Sum256(data)
	revision := fmt.Sprintf("client:%s/%d;dbd:%s;manifest:%s", input.Client.Version, input.Client.BuildNumber, input.Definitions.RepositoryRevision, hex.EncodeToString(manifestHash[:]))
	var rows []record.Record
	var limitations []string
	seenTables := map[string]bool{}
	for _, table := range input.Tables {
		if table.Name == "" || seenTables[table.Name] {
			return fmt.Errorf("manifest has empty or duplicate table name %q", table.Name)
		}
		seenTables[table.Name] = true
		entityType, supported := tableTypes[table.Name]
		if !supported {
			limitations = append(limitations, "DB2 table "+table.Name+" is preserved in local extractor output but has no canonical entity mapping.")
			continue
		}
		if table.Status != "written" && table.Status != "partial_encrypted" {
			limitations = append(limitations, "DB2 table "+table.Name+" was not imported because its extraction status is "+table.Status+".")
			continue
		}
		file, err := os.Open(filepath.Join(inputDir, "tables", table.Name+".jsonl"))
		if err != nil {
			return fmt.Errorf("open extracted table %s: %w", table.Name, err)
		}
		parsed, err := readTable(file, table, input, entityType, revision)
		closeErr := file.Close()
		if err != nil {
			return fmt.Errorf("table %s: %w", table.Name, err)
		}
		if closeErr != nil {
			return closeErr
		}
		rows = append(rows, parsed...)
	}
	if len(rows) == 0 {
		return fmt.Errorf("client extraction contains no supported canonical rows")
	}
	if input.Definitions.RepositoryRevision == "" {
		limitations = append(limitations, "WoWDBDefs checkout revision was unavailable in the extractor manifest.")
	}
	return record.WriteDataset(outputDir, rows, record.Manifest{Source: "foreverclient", Flavor: "forever", SourceRevision: revision, Status: "partial", Limitations: limitations})
}

func readTable(reader io.Reader, table struct {
	Name             string `json:"name"`
	FileDataID       uint32 `json:"fileDataId"`
	Status           string `json:"status"`
	RecordCount      int    `json:"recordCount"`
	DefinitionSHA256 string `json:"definitionSha256"`
}, input report, entityType, revision string) ([]record.Record, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	line := 0
	var header tableHeader
	var rows []record.Record
	seen := map[string]bool{}
	for scanner.Scan() {
		line++
		if len(strings.TrimSpace(scanner.Text())) == 0 {
			return nil, fmt.Errorf("line %d is blank", line)
		}
		if line == 1 {
			if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
				return nil, fmt.Errorf("line 1 header: %w", err)
			}
			if header.Kind != "table" || header.SchemaVersion != 1 || header.GameVersion != "forever" || header.Source != "foreverclient" || header.Table != table.Name || header.FileDataID != table.FileDataID || header.ClientBuild != input.Client.BuildNumber || header.DefinitionSHA256 != table.DefinitionSHA256 {
				return nil, fmt.Errorf("header identity differs from manifest")
			}
			continue
		}
		var row tableRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if row.Kind != "record" || row.Fields == nil {
			return nil, fmt.Errorf("line %d has invalid row shape", line)
		}
		id := strconv.FormatInt(int64(row.ID), 10)
		if seen[id] {
			return nil, fmt.Errorf("line %d duplicates row id %s", line, id)
		}
		seen[id] = true
		attrs := map[string]json.RawMessage{}
		states := map[string]string{}
		for name, value := range row.Fields {
			if value == nil {
				states[name] = "unavailable"
				continue
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("line %d field %s: %w", line, name, err)
			}
			attrs[name] = encoded
		}
		provenance := map[string]record.FieldProvenance{}
		for name := range attrs {
			provenance[name] = record.FieldProvenance{Source: "foreverclient", Revision: revision, Path: "tables/" + table.Name + ".jsonl"}
		}
		for name := range states {
			provenance[name] = record.FieldProvenance{Source: "foreverclient", Revision: revision, Path: "tables/" + table.Name + ".jsonl"}
		}
		attrs["clientTable"] = json.RawMessage(strconv.Quote(table.Name))
		provenance["clientTable"] = record.FieldProvenance{Source: "foreverclient", Revision: revision, Path: "manifest.json"}
		if table.Status == "partial_encrypted" {
			attrs["extractionStatus"] = json.RawMessage(`"partial_encrypted"`)
			provenance["extractionStatus"] = record.FieldProvenance{Source: "foreverclient", Revision: revision, Path: "manifest.json"}
		}
		rows = append(rows, record.Record{SchemaVersion: record.SchemaVersion, Flavor: "forever", EntityType: entityType, EntityID: id, Source: "foreverclient", SourceRevision: revision, Attributes: attrs, FieldStates: states, Provenance: provenance})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if line == 0 {
		return nil, fmt.Errorf("empty table file")
	}
	if len(rows) != table.RecordCount {
		return nil, fmt.Errorf("manifest says %d rows, file contains %d", table.RecordCount, len(rows))
	}
	return rows, nil
}
