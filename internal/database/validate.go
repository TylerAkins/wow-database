package database

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/TylerAkins/wow-database/internal/record"
)

type DatasetSummary struct {
	Path                  string `json:"path"`
	Flavor                string `json:"flavor"`
	Source                string `json:"source"`
	Revision              string `json:"revision"`
	RecordCount           int    `json:"recordCount"`
	DanglingRelationships int    `json:"danglingRelationships"`
}

func ValidateTree(root string) ([]DatasetSummary, error) {
	var datasets []DatasetSummary
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "manifest.json" {
			return nil
		}
		dir := filepath.Dir(path)
		clean := filepath.ToSlash(filepath.Clean(dir))
		if !strings.Contains(clean, "/sources/") && !strings.Contains(clean, "/compilation") {
			return nil // Raw scraper artifacts have their own manifests and validators.
		}
		rows, manifest, err := record.ReadDataset(dir)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		if err := validateDatasetPath(dir, manifest); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		dangling, err := validateReferences(rows)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		datasets = append(datasets, DatasetSummary{Path: dir, Flavor: manifest.Flavor, Source: manifest.Source, Revision: manifest.SourceRevision, RecordCount: len(rows), DanglingRelationships: dangling})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(datasets) == 0 {
		return nil, fmt.Errorf("no canonical datasets found under %s", root)
	}
	return datasets, nil
}

func validateReferences(rows []record.Record) (int, error) {
	known := map[string]map[string]bool{}
	for _, row := range rows {
		if known[row.EntityType] == nil {
			known[row.EntityType] = map[string]bool{}
		}
		known[row.EntityType][row.EntityID] = true
	}
	dangling := 0
	for _, row := range rows {
		if row.EntityType != "relationship" || row.Attributes["relationship"] == nil {
			continue
		}
		var relation struct {
			FromType string `json:"fromType"`
			FromID   string `json:"fromId"`
			ToType   string `json:"toType"`
			ToID     string `json:"toId"`
		}
		if err := decodeAttribute(row.Attributes["relationship"], &relation); err != nil {
			return 0, fmt.Errorf("relationship %s: relationship object is invalid: %w", row.EntityID, err)
		}
		if relation.FromType == "" || relation.FromID == "" || relation.ToType == "" || relation.ToID == "" {
			return 0, fmt.Errorf("relationship %s requires fromType/fromId/toType/toId", row.EntityID)
		}
		if !known[relation.FromType][relation.FromID] || !known[relation.ToType][relation.ToID] {
			// Dangling links are counted, not rejected, because snapshots may be partial.
			dangling++
		}
	}
	return dangling, nil
}

func decodeAttribute(data []byte, target any) error {
	if len(data) == 0 {
		return fmt.Errorf("missing attribute")
	}
	return json.Unmarshal(data, target)
}

func validateDatasetPath(path string, manifest record.Manifest) error {
	clean := filepath.ToSlash(filepath.Clean(path))
	if strings.Contains(clean, "/sources/") || strings.Contains(clean, "/compilation") {
		if !strings.Contains(clean, "/"+manifest.Flavor+"/") {
			return fmt.Errorf("dataset path flavor differs from manifest")
		}
		if filepath.Base(clean) != manifest.Source {
			return fmt.Errorf("dataset directory name differs from source %s", manifest.Source)
		}
	}
	return nil
}
