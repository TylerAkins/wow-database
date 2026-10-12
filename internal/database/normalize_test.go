package database

import (
	"encoding/json"
	"github.com/TylerAkins/wow-database/internal/record"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizationFixtures(t *testing.T) {
	tests := []struct{ name, source, entity, field, input, want, state string }{
		{"provider slots", "questiedb", "quest", "startedBy", `[[9,3,9],null,[20]]`, `[{"id":20,"type":"item"},{"id":3,"type":"npc"},{"id":9,"type":"npc"}]`, ""},
		{"provider coordinates separate", "allthethings", "quest", "startedBy", `[{"id":9,"type":"npc","spawns":[[130,20,30]]}]`, `[{"id":9,"type":"npc"}]`, ""},
		{"inferred finisher", "allthethings", "quest", "finishedBy", `[{"id":9,"type":"npc"}]`, "", "unsupported"},
		{"ATT level", "allthethings", "quest", "questLevel", "20", "", "unsupported"},
		{"ATT location", "allthethings", "quest", "zoneOrSort", "130", "", "unsupported"},
		{"different finisher", "questiedb", "quest", "finishedBy", `[[11]]`, `[{"id":11,"type":"npc"}]`, ""},
		{"objectives", "questiedb", "quest", "objectives", `[[[9,"Slay",123]],null,[[20]], [72,250], [[[9,3],9,"Credits",456]],[[10,"Cast",20]]]`, `{"creature":[{"iconFile":123,"id":9,"text":"Slay"}],"item":[{"id":20}],"killCredit":[{"baseId":9,"iconFile":456,"ids":[3,9],"text":"Credits"}],"reputation":[{"id":72,"value":250}],"spell":[{"id":10,"itemId":20,"text":"Cast"}]}`, ""},
		{"empty ATT objectives", "allthethings", "quest", "objectives", `[]`, `{}`, ""},
		{"negative sort", "questiedb", "quest", "zoneOrSort", "-82", "-82", ""},
		{"signed level", "questiedb", "quest", "questLevel", "-1", "-1", ""},
		{"large mask", "questiedb", "quest", "requiredRaces", "18446744073709551615", "18446744073709551615", ""},
		{"set ordering", "questiedb", "quest", "preQuestGroup", "[9,3,9]", "[3,9]", ""},
		{"text ordering", "questiedb", "quest", "objectivesText", `["Z","A"]`, `["Z","A"]`, ""},
		{"Unicode", "wowhead", "quest", "name", `"Cafe\u0301"`, `"Café"`, ""},
		{"HTML literal", "questiedb", "quest", "name", `"A &amp; B"`, `"A &amp; B"`, ""},
		{"spawn precision", "questiedb", "npc", "spawns", `{"130":[[65.36,24.81],[65.360,24.810]]}`, `[{"x":65.36,"y":24.81,"zoneId":130}]`, ""},
		{"UI metadata separate", "wowhead", "object", "spawns", `[{"zoneId":130,"x":65.3,"y":24.8,"uiMapId":1421,"uiMapName":"Silverpine"}]`, `[{"x":65.3,"y":24.8,"zoneId":130}]`, ""},
		{"bad providers", "questiedb", "quest", "startedBy", `[["9"]]`, "", "invalid"},
		{"bad spawn", "questiedb", "npc", "spawns", `{"130":[[101,20]]}`, "", "invalid"},
		{"breadcrumb ID", "questiedb", "quest", "breadcrumbForQuestId", `"7"`, "", "invalid"},
		{"npc scalar", "questiedb", "npc", "npcFlags", `1.5`, "", "invalid"},
		{"object scalar", "questiedb", "object", "factionID", `"7"`, "", "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := sourceRecord(tt.source, "rev", map[string]string{tt.field: tt.input}, nil)
			row.EntityType = tt.entity
			n, a, err := normalizeRecord(row)
			if err != nil {
				t.Fatal(err)
			}
			if tt.state != "" {
				if n.FieldStates[tt.field] != tt.state || n.Attributes[tt.field] != nil || a.Issues[tt.field] == "" {
					t.Fatalf("state/value/audit = %+v %+v", n, a)
				}
				return
			}
			got, err := canonicalJSON(n.Attributes[tt.field])
			if err != nil {
				t.Fatal(err)
			}
			want, err := canonicalJSON(json.RawMessage(tt.want))
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("got %s want %s", got, want)
			}
		})
	}
}

func TestSelectionExcludesInvalidAndRetainsEveryOriginal(t *testing.T) {
	q := sourceRecord("questiedb", "q", map[string]string{"requiredLevel": `"wrong"`, "name": `"Cafe\u0301"`}, nil)
	w := sourceRecord("wowhead", "w", map[string]string{"requiredLevel": "20", "name": `"Café"`}, nil)
	row, c, err := mergeRecords([]record.Record{q, w}, ResolutionPolicy{Fields: map[string][]string{"quest.requiredLevel": {"questiedb", "wowhead"}}})
	if err != nil {
		t.Fatal(err)
	}
	if string(row.Attributes["requiredLevel"]) != "20" || len(c) != 0 {
		t.Fatalf("invalid preferred selected: %+v %+v", row, c)
	}
	if len(row.Provenance["name"].Sources) != 2 {
		t.Fatal("equal normalization lost provenance")
	}
	var audits map[string]sourceAudit
	if err := json.Unmarshal(row.Attributes["sourceData"], &audits); err != nil {
		t.Fatal(err)
	}
	if string(audits["questiedb"].Attributes["name"]) != `"Cafe\u0301"` || audits["questiedb"].Issues["requiredLevel"] == "" {
		t.Fatal("original value or issue lost")
	}
	sole, _, err := mergeRecords([]record.Record{q}, ResolutionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if sole.Attributes["requiredLevel"] != nil || sole.FieldStates["requiredLevel"] != "invalid" {
		t.Fatal("sole invalid observation selected")
	}
	marked := sourceRecord("questiedb", "q", map[string]string{"requiredLevel": "99"}, map[string]string{"requiredLevel": "invalid"})
	sole, _, err = mergeRecords([]record.Record{marked}, ResolutionPolicy{})
	if err != nil || sole.Attributes["requiredLevel"] != nil {
		t.Fatal("source-invalid value selected")
	}
}
func TestGenuineDisagreementAndUnknownFields(t *testing.T) {
	a := sourceRecord("questiedb", "q", map[string]string{"name": `"A"`, "mystery": "1"}, nil)
	b := sourceRecord("wowhead", "w", map[string]string{"name": `"B"`, "mystery": "2"}, nil)
	row, c, err := mergeRecords([]record.Record{a, b}, ResolutionPolicy{Fields: map[string][]string{"quest.name": {"wowhead", "questiedb"}}})
	if err != nil {
		t.Fatal(err)
	}
	if string(row.Attributes["name"]) != `"B"` || row.FieldStates["mystery"] != "conflicting" || len(c) != 2 {
		t.Fatalf("selection = %+v %+v", row, c)
	}
	for _, conflict := range c {
		if len(conflict.Observations) != 2 {
			t.Fatal("original conflict observations missing")
		}
	}
	report := reconciliationReport([]record.Record{row}, c)
	if report.Totals.Selected != 1 || report.Totals.Unresolved != 1 {
		t.Fatalf("report %+v", report)
	}
}
func TestPolicyChangesRevisionAndFailedPolicyPreservesOutput(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sources")
	writeSource(t, root, "questiedb", sourceRecord("questiedb", "rev-questiedb", map[string]string{"name": `"Q"`}, nil))
	writeSource(t, root, "wowhead", sourceRecord("wowhead", "rev-wowhead", map[string]string{"name": `"W"`}, nil))
	policy := filepath.Join(t.TempDir(), "policy.json")
	out := filepath.Join(t.TempDir(), "compiled")
	revisions := []string{}
	for _, source := range []string{"questiedb", "wowhead"} {
		if err := os.WriteFile(policy, []byte(`{"fields":{"quest.name":["`+source+`"]}}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := Compile("forever", root, out, policy); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		revisions = append(revisions, m["sourceRevision"].(string))
		if m["compilationFormatVersion"] != float64(2) {
			t.Fatal("missing format version")
		}
	}
	if revisions[0] == revisions[1] {
		t.Fatal("policy did not change revision")
	}
	before, _ := os.ReadFile(filepath.Join(out, "records.jsonl"))
	_ = os.WriteFile(policy, []byte(`{"fields":{"quest.name":["questiedb","questiedb"]}}`), 0600)
	if err := Compile("forever", root, out, policy); err == nil {
		t.Fatal("invalid policy accepted")
	}
	after, _ := os.ReadFile(filepath.Join(out, "records.jsonl"))
	if string(before) != string(after) {
		t.Fatal("failure replaced output")
	}
}

func TestInstancePresenceAndAmbiguousRelationship(t *testing.T) {
	row := sourceRecord("questiedb", "q", map[string]string{"spawns": `{"209":[[-1,-1]],"130":[[0,0]]}`}, nil)
	row.EntityType = "npc"
	n, _, err := normalizeRecord(row)
	if err != nil {
		t.Fatal(err)
	}
	if string(n.Attributes["spawnPresence"]) != "[209]" || string(n.Attributes["spawns"]) != `[{"x":0,"y":0,"zoneId":130}]` {
		t.Fatalf("presence/real origin %+v", n.Attributes)
	}
	row.Attributes["spawns"] = json.RawMessage(`{"209":[[-1,-1]]}`)
	n, _, err = normalizeRecord(row)
	if err != nil || n.FieldStates["spawns"] != "unavailable" || n.Attributes["spawns"] != nil {
		t.Fatal("presence was presented as empty coordinate coverage")
	}
	row.Attributes["spawns"] = json.RawMessage(`{"209":[[-1,20]]}`)
	n, _, err = normalizeRecord(row)
	if err != nil || n.FieldStates["spawns"] != "invalid" {
		t.Fatal("partial sentinel accepted")
	}
	row = sourceRecord("allthethings", "a", map[string]string{"relationship": `{"kind":"quest-provider","role":"finishedBy","fromType":"quest","fromId":"1","toType":"npc","toId":"9"}`}, nil)
	row.EntityType = "relationship"
	n, _, err = normalizeRecord(row)
	if err != nil || n.Attributes["relationship"] != nil || n.FieldStates["relationship"] != "unsupported" {
		t.Fatal("inferred finisher remained accessible as a relationship")
	}
}

func TestClientQuestMetadataIntegers(t *testing.T) {
	row := sourceRecord("foreverclient", "client", map[string]string{"UiQuestDetailsThemeID": `1.5`}, nil)
	row.EntityType = "quest"
	n, audit, err := normalizeRecord(row)
	if err != nil {
		t.Fatal(err)
	}
	if n.Attributes["questUiThemeId"] != nil || n.FieldStates["questUiThemeId"] != "invalid" || audit.Issues["questUiThemeId"] == "" {
		t.Fatalf("invalid quest UI metadata remained eligible: %+v %+v", n, audit)
	}
}
