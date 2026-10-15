package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/TylerAkins/wow-database/internal/att"
	"github.com/TylerAkins/wow-database/internal/record"
)

func TestNormalizeRetainsUnlocatedQuestFieldsAndRelationships(t *testing.T) {
	rows := []map[string]any{
		{
			"id":        101,
			"fields":    map[string]any{"name": "No map pin", "preQuestSingle": []any{100}, "nextQuestInChain": 102, "objectives": []any{}},
			"startedBy": []any{map[string]any{"type": "npc", "id": 500, "spawns": []any{}}},
		},
	}
	unmapped := []att.Report{{QuestID: 101, NPCID: 500, File: "quests.lua", Reason: "unknown map"}}
	got, err := normalize(rows, unmapped, "0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("record count = %d, want quest, 3 links, and issue", len(got))
	}
	var questFound, providerFound, prereqFound, chainFound, issueFound bool
	for _, row := range got {
		if err := row.Validate(); err != nil {
			t.Fatalf("invalid normalized record %s: %v", row.Key(), err)
		}
		switch row.EntityType {
		case "quest":
			questFound = string(row.Attributes["name"]) == `"No map pin"`
			if string(row.Attributes["objectives"]) != "[]" {
				t.Fatalf("empty objectives lost: %s", row.Attributes["objectives"])
			}
		case "relationship":
			if row.Attributes["issue"] != nil {
				issueFound = true
				continue
			}
			var relationship map[string]any
			if err := json.Unmarshal(row.Attributes["relationship"], &relationship); err != nil {
				t.Fatal(err)
			}
			switch relationship["kind"] {
			case "quest-provider":
				providerFound = relationship["toId"] == "500"
			case "preQuestSingle":
				prereqFound = relationship["toId"] == "100"
			case "nextQuestInChain":
				chainFound = relationship["toId"] == "102"
			}
		}
	}
	if !questFound || !providerFound || !prereqFound || !chainFound || !issueFound {
		t.Fatalf("records lost data: quest=%t provider=%t prerequisite=%t chain=%t issue=%t", questFound, providerFound, prereqFound, chainFound, issueFound)
	}
	out := filepath.Join(t.TempDir(), "dataset")
	if err := record.WriteDataset(out, got, record.Manifest{Source: "allthethings", Flavor: "forever", SourceRevision: "0123456789abcdef0123456789abcdef01234567"}); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeRetainsDeclarationAndUnresolvedEvidence(t *testing.T) {
	rows := []map[string]any{{"id": 1, "fields": map[string]any{"requiredLevel": 5}, "fieldPaths": map[string]string{"requiredLevel": "forever/quests.lua:12"}, "declarations": []any{map[string]any{"path": "forever/quests.lua", "line": 12}, map[string]any{"path": "forever/zzOLD/quests.lua", "line": 2}}, "unresolved": []any{map[string]any{"field": "requireSkill", "expression": "Unknown()"}}}}
	got, err := normalize(rows, nil, "0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Attributes["declarations"] == nil || got[0].Attributes["unresolved"] == nil {
		t.Fatalf("declaration evidence lost: %#v", got[0])
	}
	if got[0].Provenance["requiredLevel"].Path != "forever/quests.lua:12" {
		t.Fatalf("field provenance = %#v", got[0].Provenance)
	}
}

func TestNormalizeMarksConflictingFields(t *testing.T) {
	rows := []map[string]any{{"id": 1, "fields": map[string]any{}, "fieldPaths": map[string]string{"requiredLevel": "quests.lua:1"}, "unresolved": []any{map[string]any{"field": "requiredLevel", "reason": "conflicting declarations", "values": []any{4, 5}}}}}
	got, err := normalize(rows, nil, "0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].FieldStates["requiredLevel"] != "conflicting" {
		t.Fatalf("conflict disappeared: %#v", got[0])
	}
	if err := got[0].Validate(); err != nil {
		t.Fatal(err)
	}
}
