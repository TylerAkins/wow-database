package att

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func attFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["forever/.config/constants/maps.lua"] = "MAP = {\n TEST = 1429;\n}"
	for path, body := range files {
		target := filepath.Join(root, ".contrib/.db", path)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestExportLoadedDeclarationsAndFieldSemantics(t *testing.T) {
	root := attFixture(t, map[string]string{
		"shared/constants/professionIDs.lua": "ALCHEMY = 171;",
		"shared/quests.lua":                  `q(101, { races = HUMAN, lvl = 5 })`,
		"forever/zzOLD/quests.lua":           `q(102, { races = HORDE_ONLY, requireSkill = ALCHEMY, lvl = 12, isBreadcrumb = true, qi = 99, provider = { "i", 88 }, groups = { objective(1, { provider = { "i", 77 } }) } })`,
		"forever/zones/quests.lua":           `q(102, { sourceQuest = 100 })`,
	})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("loaded quests = %d, want shared and zzOLD declarations: %#v", len(rows), rows)
	}
	shared := rows[0]["fields"].(map[string]any)
	if shared["requiredRaces"] != 1 {
		t.Errorf("scalar HUMAN race = %#v", shared["requiredRaces"])
	}
	fields := rows[1]["fields"].(map[string]any)
	if fields["requiredLevel"] != 12 || fields["questLevel"] != nil {
		t.Errorf("lvl must be minimum required level: %#v", fields)
	}
	if !reflect.DeepEqual(fields["requiredSkill"], []any{171, 1}) {
		t.Errorf("profession gate = %#v", fields["requiredSkill"])
	}
	if fields["requiredRaces"] != int(uint64(2+16+32+128)+8589934592) {
		t.Errorf("Forever Horde races = %#v", fields["requiredRaces"])
	}
	if fields["isBreadcrumb"] != true {
		t.Errorf("breadcrumb lost: %#v", fields)
	}
	if fields["providedItemId"] != 99 || fields["sourceItemId"] != 88 {
		t.Errorf("provided and starting items conflated: %#v", fields)
	}
	if got := rows[1]["startedBy"].([]any); len(got) != 1 || got[0].(map[string]any)["id"] != 88 {
		t.Errorf("objective provider leaked into starters: %#v", got)
	}
	if got, ok := rows[1]["declarations"].([]any); !ok || len(got) != 2 {
		t.Errorf("duplicate declarations lost: %#v", rows[1]["declarations"])
	}
}

func TestExportInheritedRestrictionsAndUnresolvedEvidence(t *testing.T) {
	root := attFixture(t, map[string]string{"forever/quests.lua": `n(1, { races = HORDE_ONLY, classes = { WARRIOR }, lvl = 20, groups = {
 q(201, { groups = { i(9, { lvl = 99, races = HUMAN }) } }),
 q(202, { races = HUMAN, requireSkill = UnknownSkill(), sourceQuests = MakePrereqs() }),
} })`})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	fields := rows[0]["fields"].(map[string]any)
	if fields["requiredLevel"] != 20 || fields["requiredClasses"] != 1 || fields["requiredRaces"] != int(uint64(2+16+32+128)+8589934592) {
		t.Errorf("inherited restrictions = %#v", fields)
	}
	own := rows[1]["fields"].(map[string]any)
	if own["requiredRaces"] != 1 {
		t.Errorf("child override = %#v", own)
	}
	if got, ok := rows[1]["unresolved"].([]any); !ok || len(got) != 2 {
		t.Errorf("unsupported expressions vanished: %#v", rows[1]["unresolved"])
	}
}

func TestExportPreprocessorSharedDataAndConflictingDeclarations(t *testing.T) {
	root := attFixture(t, map[string]string{
		"shared/constants/raceIDs.lua": `HUMAN = 1;
ORC = 2;
GOBLIN = 9;
SKYBORNE_HORDE = 96;
HORDE_ONLY = { ORC,
-- #if AFTER CATA
GOBLIN,
-- #endif
-- #if FOREVER
SKYBORNE_HORDE,
-- #endif
};`,
		"forever/quests.lua": `-- #if AFTER TBC
q(300, { lvl = 80 })
-- #elseif FOREVER
n(1, sharedData({ lvl = 40, requireSkill = 171 }, {
 q(301, { races = HORDE_ONLY, description = "text -- still text" }),
}))
-- #endif
q(301, { lvl = 41 })`,
	})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("inactive branch exported: %#v", rows)
	}
	fields := rows[0]["fields"].(map[string]any)
	if fields["requiredLevel"] != nil {
		t.Errorf("conflicting minimum level selected silently: %#v", fields)
	}
	if !reflect.DeepEqual(fields["requiredSkill"], []any{171, 1}) {
		t.Errorf("sharedData inheritance missing: %#v", fields)
	}
	if fields["requiredRaces"] != int(uint64(2)+8589934592) {
		t.Errorf("race constant preprocessing = %#v", fields)
	}
	if fields["description"] != "text -- still text" {
		t.Errorf("comment marker inside literal lost: %#v", fields)
	}
	declarations := rows[0]["declarations"].([]any)
	if declarations[0].(map[string]any)["fields"].(map[string]any)["requiredLevel"] != 40 {
		t.Errorf("first observation changed or inherited level missing: %#v", declarations)
	}
	if got, ok := rows[0]["unresolved"].([]any); !ok || len(got) == 0 {
		t.Errorf("conflict lacks evidence: %#v", rows[0])
	}
}

func TestExportBareQuestDeclarationsRemainObservations(t *testing.T) {
	root := attFixture(t, map[string]string{"forever/quests.lua": `n(1, { races = HORDE_ONLY, groups = { q(501), q(501, { lvl = 7 }) } })`})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%#v", rows)
	}
	declarations := rows[0]["declarations"].([]any)
	if len(declarations) != 2 {
		t.Fatalf("bare declaration missing: %#v", declarations)
	}
	if declarations[0].(map[string]any)["fields"].(map[string]any)["requiredLevel"] != nil {
		t.Fatalf("later declaration mutated first: %#v", declarations)
	}
}

func TestExportKeepsUnsupportedCoordinatesAndPrerequisiteThreshold(t *testing.T) {
	root := attFixture(t, map[string]string{"forever/quests.lua": `q(601, { qg = 1, coord = CoordinatesForQuest(), sourceQuests = { 2,3,4 }, sourceQuestNumRequired = 2, altQuests = { 5,6 }, lockCriteria = { 1, "spellID", 123 } })`})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	fields := rows[0]["fields"].(map[string]any)
	if fields["preQuestGroup"] != nil || fields["exclusiveTo"] != nil {
		t.Errorf("ATT-specific semantics guessed as another contract: %#v", fields)
	}
	if !reflect.DeepEqual(fields["altQuests"], []any{5, 6}) || !reflect.DeepEqual(fields["lockCriteria"], []any{1, "spellID", 123}) {
		t.Errorf("source-native semantics lost: %#v", fields)
	}
	if fields["sourceQuestNumRequired"] != 2 {
		t.Errorf("prerequisite threshold lost: %#v", fields)
	}
	if got, ok := rows[0]["unresolved"].([]any); !ok || len(got) < 2 {
		t.Errorf("unsupported coordinates/threshold evidence missing: %#v", rows[0])
	}
	if rows[0]["finishedBy"] != nil {
		t.Errorf("turn-in provider invented: %#v", rows[0])
	}
}

func TestExportObjectiveProviderListsKeepEveryTypeAndUnresolvedEvidence(t *testing.T) {
	root := attFixture(t, map[string]string{"forever/quests.lua": `q(5213, { groups = {
 objective(1, { -- 0/1 Scourge Data
  providers = { { "i",13176 }, { "o",176249 } },
 }),
 objective(2, { provider = { "n",10516 }, coord = CoordinatesForObjective() }),
 objective(3, { providers = { { "i",777 }, BuildProvider() }, crs = { 888,889 } }),
} })`})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	objectives := rows[0]["fields"].(map[string]any)["objectives"].(map[string]any)
	ids := func(kind string) []int {
		var result []int
		entries, _ := objectives[kind].([]any)
		for _, value := range entries {
			result = append(result, value.(map[string]any)["id"].(int))
		}
		return result
	}
	if !reflect.DeepEqual(ids("item"), []int{13176, 777}) || !reflect.DeepEqual(ids("object"), []int{176249}) || !reflect.DeepEqual(ids("creature"), []int{10516, 888, 889}) {
		t.Errorf("objective providers missing: %#v", objectives)
	}
	if unresolved, _ := rows[0]["unresolved"].([]any); len(unresolved) == 0 {
		t.Errorf("unsupported objective expressions disappeared: %#v", rows[0])
	}
	if got := rows[0]["startedBy"]; got != nil {
		t.Errorf("objective providers leaked into quest starters: %#v", got)
	}
	places, _ := rows[0]["places"].([]any)
	if len(places) != 6 {
		t.Errorf("objective places=%#v", places)
	}
}

func TestExportMultilineValuesPreserveDirectAndInheritedLevels(t *testing.T) {
	root := attFixture(t, map[string]string{"forever/quests.lua": `n(1, { lvl =
-- #if AFTER TBC
60,
-- #else
48,
-- #endif
 groups = { q(5122, {}) },
})
q(14355, { lvl =
-- #if FOREVER
 25,
-- #else
 80,
-- #endif
 races =
 HORDE_ONLY,
})`})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%#v", rows)
	}
	if fields := rows[0]["fields"].(map[string]any); fields["requiredLevel"] != 48 {
		t.Errorf("inherited multiline level=%#v", fields)
	}
	if fields := rows[1]["fields"].(map[string]any); fields["requiredLevel"] != 25 || fields["requiredRaces"] != int(uint64(2+16+32+128)+8589934592) {
		t.Errorf("direct multiline values=%#v", fields)
	}
}

func TestExportWrappedQuestDeclarationsRemainUnresolvedEvidence(t *testing.T) {
	root := attFixture(t, map[string]string{"forever/quests.lua": `q(76249, name(HEADERS.Achievement,18368,{ lvl = 48, groups = { i(10) } }))
q(700, {})`})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1]["id"] != 76249 {
		t.Fatalf("wrapped quest declaration omitted: %#v", rows)
	}
	if declarations, _ := rows[1]["declarations"].([]any); len(declarations) != 1 {
		t.Errorf("wrapped declaration provenance missing: %#v", rows[1])
	}
	if unresolved, _ := rows[1]["unresolved"].([]any); len(unresolved) == 0 {
		t.Errorf("wrapper expression disappeared: %#v", rows[1])
	}
}

func TestExportQuestProviderListsUseTheSameLiteralProviderSemantics(t *testing.T) {
	root := attFixture(t, map[string]string{"forever/quests.lua": `q(5862, { providers = { { "n",11872 }, { "i",14872 } } })`})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	providers, _ := rows[0]["startedBy"].([]any)
	if len(providers) != 2 {
		t.Fatalf("quest provider list omitted: %#v", rows[0])
	}
	if rows[0]["fields"].(map[string]any)["sourceItemId"] != 14872 {
		t.Fatalf("starting item omitted: %#v", rows[0])
	}
}

func TestExportObjectiveOwnershipUsesNearestQuestAndExplicitQuestID(t *testing.T) {
	root := attFixture(t, map[string]string{"forever/quests.lua": `q(1654, { groups = {
 q(1442, { groups = {
  objective(1, { provider = { "i",6995 }, crs = { 4803,4805,4802 } }),
  objective(2, { questID = 1654, provider = { "i",7083 } }),
 } }),
} })
q(254, { groups = { objective(1, { questID = 253, provider = { "i",2382 } }) } })
q(253, { groups = { objective(1, { provider = { "i",999 } }) } })`})
	rows, _, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[int][]int{1654: {7083}, 1442: {6995}, 253: {999, 2382}, 254: nil}
	for _, row := range rows {
		fields := row["fields"].(map[string]any)
		objectives, _ := fields["objectives"].(map[string]any)
		var items []int
		entries, _ := objectives["item"].([]any)
		for _, entry := range entries {
			items = append(items, entry.(map[string]any)["id"].(int))
		}
		id := row["id"].(int)
		if !reflect.DeepEqual(items, expected[id]) {
			t.Errorf("quest%d item objectives=%#v, want %#v", id, items, expected[id])
		}
		if id == 253 {
			declarations := row["declarations"].([]any)
			first := declarations[0].(map[string]any)["fields"].(map[string]any)["objectives"].(map[string]any)
			if len(first["item"].([]any)) != 1 {
				t.Errorf("routed objectives mutated declaration evidence: %#v", first)
			}
		}
		if id == 1654 && objectives["creature"] != nil {
			t.Errorf("child creature objectives leaked: %#v", objectives)
		}
	}
}
