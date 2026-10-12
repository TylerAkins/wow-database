package database

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/TylerAkins/wow-database/internal/record"
)

type Difference struct {
	Path  string          `json:"path"`
	Left  json.RawMessage `json:"left,omitempty"`
	Right json.RawMessage `json:"right,omitempty"`
}

type EntityDiff struct {
	Flavor      string       `json:"flavor,omitempty"`
	EntityType  string       `json:"entityType"`
	EntityID    string       `json:"entityId"`
	Status      string       `json:"status"`
	Differences []Difference `json:"differences,omitempty"`
}

type Comparison struct {
	Left   string `json:"left"`
	Right  string `json:"right"`
	Counts struct {
		LeftOnly  int `json:"leftOnly"`
		RightOnly int `json:"rightOnly"`
		Changed   int `json:"changed"`
		Equal     int `json:"equal"`
	} `json:"counts"`
	Entities []EntityDiff `json:"entities"`
}

func Compare(leftDir, rightDir string, ignoreFlavor bool) (Comparison, error) {
	return CompareWithTolerance(leftDir, rightDir, ignoreFlavor, 0)
}

func CompareWithTolerance(leftDir, rightDir string, ignoreFlavor bool, coordinateTolerance float64) (Comparison, error) {
	if coordinateTolerance < 0 || math.IsNaN(coordinateTolerance) || math.IsInf(coordinateTolerance, 0) {
		return Comparison{}, fmt.Errorf("coordinate tolerance must be a finite nonnegative number")
	}
	left, leftManifest, err := record.ReadDataset(leftDir)
	if err != nil {
		return Comparison{}, fmt.Errorf("left dataset: %w", err)
	}
	right, rightManifest, err := record.ReadDataset(rightDir)
	if err != nil {
		return Comparison{}, fmt.Errorf("right dataset: %w", err)
	}
	if !ignoreFlavor && leftManifest.Flavor != rightManifest.Flavor {
		return Comparison{}, fmt.Errorf("dataset flavors differ (%s and %s); use --cross-flavor to compare them", leftManifest.Flavor, rightManifest.Flavor)
	}
	comparison := Comparison{Left: leftDir, Right: rightDir}
	leftByKey := indexRecords(left, ignoreFlavor)
	rightByKey := indexRecords(right, ignoreFlavor)
	keySet := map[string]bool{}
	for key := range leftByKey {
		keySet[key] = true
	}
	for key := range rightByKey {
		keySet[key] = true
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		l, lok := leftByKey[key]
		r, rok := rightByKey[key]
		if !lok {
			comparison.Counts.RightOnly++
			comparison.Entities = append(comparison.Entities, EntityDiff{Flavor: r.Flavor, EntityType: r.EntityType, EntityID: r.EntityID, Status: "right-only"})
			continue
		}
		if !rok {
			comparison.Counts.LeftOnly++
			comparison.Entities = append(comparison.Entities, EntityDiff{Flavor: l.Flavor, EntityType: l.EntityType, EntityID: l.EntityID, Status: "left-only"})
			continue
		}
		changes, err := diffRecordsWithTolerance(l, r, coordinateTolerance)
		if err != nil {
			return Comparison{}, err
		}
		if len(changes) == 0 {
			comparison.Counts.Equal++
			comparison.Entities = append(comparison.Entities, EntityDiff{Flavor: l.Flavor, EntityType: l.EntityType, EntityID: l.EntityID, Status: "equal"})
		} else {
			comparison.Counts.Changed++
			comparison.Entities = append(comparison.Entities, EntityDiff{Flavor: l.Flavor, EntityType: l.EntityType, EntityID: l.EntityID, Status: "changed", Differences: changes})
		}
	}
	return comparison, nil
}

func indexRecords(rows []record.Record, ignoreFlavor bool) map[string]record.Record {
	out := make(map[string]record.Record, len(rows))
	for _, row := range rows {
		key := row.EntityType + "/" + row.EntityID
		if !ignoreFlavor {
			key = row.Flavor + "/" + key
		}
		out[key] = row
	}
	return out
}

func diffRecords(left, right record.Record) ([]Difference, error) {
	return diffRecordsWithTolerance(left, right, 0)
}

func diffRecordsWithTolerance(left, right record.Record, coordinateTolerance float64) ([]Difference, error) {
	leftAttrs := map[string]json.RawMessage{}
	rightAttrs := map[string]json.RawMessage{}
	for k, v := range left.Attributes {
		leftAttrs[k] = v
	}
	for k, v := range right.Attributes {
		rightAttrs[k] = v
	}
	leftState, err := json.Marshal(left.FieldStates)
	if err != nil {
		return nil, err
	}
	rightState, err := json.Marshal(right.FieldStates)
	if err != nil {
		return nil, err
	}
	leftAttrs["$fieldStates"], rightAttrs["$fieldStates"] = leftState, rightState
	keys := map[string]bool{}
	for k := range leftAttrs {
		keys[k] = true
	}
	for k := range rightAttrs {
		keys[k] = true
	}
	fields := make([]string, 0, len(keys))
	for k := range keys {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	var changes []Difference
	for _, field := range fields {
		l, lok := leftAttrs[field]
		r, rok := rightAttrs[field]
		if !lok || !rok {
			changes = append(changes, Difference{Path: field, Left: l, Right: r})
			continue
		}
		var lv, rv any
		if err := json.Unmarshal(l, &lv); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(r, &rv); err != nil {
			return nil, err
		}
		collectDifferencesWithTolerance(field, lv, rv, coordinateTolerance, &changes)
	}
	return changes, nil
}

func collectDifferences(path string, left, right any, out *[]Difference) {
	collectDifferencesWithTolerance(path, left, right, 0, out)
}

func collectDifferencesWithTolerance(path string, left, right any, tolerance float64, out *[]Difference) {
	leftMap, leftObject := left.(map[string]any)
	rightMap, rightObject := right.(map[string]any)
	if leftObject && rightObject {
		tolerated := toleratedCoordinates(leftMap, rightMap, tolerance)
		keys := map[string]bool{}
		for k := range leftMap {
			keys[k] = true
		}
		for k := range rightMap {
			keys[k] = true
		}
		ordered := make([]string, 0, len(keys))
		for k := range keys {
			ordered = append(ordered, k)
		}
		sort.Strings(ordered)
		for _, k := range ordered {
			if tolerated[k] {
				continue
			}
			l, lok := leftMap[k]
			r, rok := rightMap[k]
			child := path + "." + k
			if !lok || !rok {
				*out = append(*out, encodedDiff(child, l, lok, r, rok))
				continue
			}
			collectDifferencesWithTolerance(child, l, r, tolerance, out)
		}
		return
	}
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	if string(leftJSON) != string(rightJSON) {
		*out = append(*out, Difference{Path: path, Left: leftJSON, Right: rightJSON})
	}
}

func toleratedCoordinates(left, right map[string]any, tolerance float64) map[string]bool {
	if tolerance <= 0 {
		return nil
	}
	sharedFrame := false
	for _, key := range []string{"zone", "zoneId", "map", "mapId", "uiMapId"} {
		leftValue, leftOK := left[key]
		rightValue, rightOK := right[key]
		if leftOK && rightOK && reflect.DeepEqual(leftValue, rightValue) {
			sharedFrame = true
			break
		}
	}
	if !sharedFrame {
		return nil
	}
	result := map[string]bool{}
	for _, key := range []string{"x", "y"} {
		leftValue, leftOK := left[key].(float64)
		rightValue, rightOK := right[key].(float64)
		if leftOK && rightOK && math.Abs(leftValue-rightValue) <= tolerance {
			result[key] = true
		}
	}
	return result
}

func encodedDiff(path string, left any, hasLeft bool, right any, hasRight bool) Difference {
	diff := Difference{Path: path}
	if hasLeft {
		diff.Left, _ = json.Marshal(left)
	}
	if hasRight {
		diff.Right, _ = json.Marshal(right)
	}
	return diff
}

func WriteComparison(outDir string, comparison Comparison) error {
	if outDir == "" {
		return fmt.Errorf("comparison output path is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "comparison.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# Dataset comparison\n\nLeft: `%s`  \nRight: `%s`\n\n", comparison.Left, comparison.Right)
	fmt.Fprintf(&md, "- Left only: %d\n- Right only: %d\n- Changed: %d\n- Equal: %d\n", comparison.Counts.LeftOnly, comparison.Counts.RightOnly, comparison.Counts.Changed, comparison.Counts.Equal)
	for _, entity := range comparison.Entities {
		if entity.Status != "changed" {
			continue
		}
		fmt.Fprintf(&md, "\n## %s/%s/%s\n", entity.Flavor, entity.EntityType, entity.EntityID)
		for _, field := range entity.Differences {
			fmt.Fprintf(&md, "- `%s` differs\n", field.Path)
		}
	}
	return os.WriteFile(filepath.Join(outDir, "comparison.md"), []byte(md.String()), 0o644)
}
