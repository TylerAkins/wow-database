package compile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TylerAkins/wow-database/internal/att"
)

func TestCompile_OmitsWowheadFields(t *testing.T) {
	dir := publish(t, dump(t, quest(1, map[string]any{"name": "A"})))
	body := readQuest(t, dir, 1)
	walkJSON(t, body, func(key string) {
		switch key {
		case "sourceUrl", "list", "html":
			t.Fatalf("published quest contains %s", key)
		}
	})
}

func TestCompile_IncludesEveryDumpId(t *testing.T) {
	err := Run(Options{
		Dump:   strings.NewReader(mustJSON(t, quest(1, nil)) + "\n" + summaryLine(t, 2)),
		OutDir: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("got %v", err)
	}
}

func TestCompile_StoresQuestOnceAndIndexesIdsOnly(t *testing.T) {
	previous := ShardSize
	ShardSize = 1
	t.Cleanup(func() { ShardSize = previous })
	dir := publish(t, dump(t,
		quest(1, map[string]any{"zoneOrSort": 14, "preQuestGroup": []any{9}}),
		quest(2, map[string]any{"zoneOrSort": 14}),
	))
	seen := map[int]int{}
	for _, path := range []string{"quests/0001.json", "quests/0002.json"} {
		var file map[string]any
		unmarshalFile(t, filepath.Join(dir, path), &file)
		quests := file["quests"].(map[string]any)
		if len(quests) != 1 {
			t.Fatalf("%s has %d quests", path, len(quests))
		}
		for key := range quests {
			id := int(key[0] - '0')
			seen[id]++
		}
	}
	if seen[1] != 1 || seen[2] != 1 {
		t.Fatalf("quest copies: %#v", seen)
	}
	var zones map[string]any
	unmarshalFile(t, filepath.Join(dir, "indexes/zones.json"), &zones)
	ids := zones["14"].([]any)
	if len(ids) != 2 {
		t.Fatalf("zone index %#v", zones["14"])
	}
	for _, id := range ids {
		if _, ok := id.(float64); !ok {
			t.Fatalf("zone index stored %T", id)
		}
	}
	var chains map[string]any
	unmarshalFile(t, filepath.Join(dir, "indexes/chains.json"), &chains)
	entry := chains["1"].(map[string]any)
	if _, ok := entry["name"]; ok {
		t.Fatal("chain index embeds a quest body")
	}
	if _, ok := entry["places"]; ok {
		t.Fatal("chain index embeds places")
	}
}

func TestCompile_ZoneIdOneSurvives(t *testing.T) {
	dir := publish(t, dump(t, quest(7, nil, map[string]any{
		"startedBy": []any{map[string]any{"type": "npc", "id": 1, "name": "A", "spawns": []any{[]any{1, 10, 20}}}},
		"places": []any{map[string]any{
			"role": "available", "type": "npc", "id": 1, "name": "A", "spawns": []any{[]any{1, 10, 20}},
		}},
	})))
	questBody := readQuest(t, dir, 7)
	spawns := questBody["places"].([]any)[0].(map[string]any)["spawns"].([]any)[0].([]any)
	if spawns[0].(float64) != 1 {
		t.Fatalf("zone id = %#v", spawns[0])
	}
}

func TestCompile_NamesObjectives(t *testing.T) {
	dir := publish(t, dump(t, quest(3, map[string]any{
		"objectives": map[string]any{"creature": []any{map[string]any{"id": 12, "text": "Boar"}}},
	})))
	objectives := readQuest(t, dir, 3)["objectives"].(map[string]any)
	if _, ok := objectives["creature"]; !ok {
		t.Fatalf("objectives %#v", objectives)
	}
	if _, ok := objectives["1"]; ok {
		t.Fatal("positional objective slot was published")
	}
}

func TestCompile_KeepsRequiredSourceItems(t *testing.T) {
	dir := publish(t, dump(t, quest(4, map[string]any{"requiredSourceItems": []any{190181}})))
	items := readQuest(t, dir, 4)["requiredSourceItems"].([]any)
	if len(items) != 1 || items[0].(float64) != 190181 {
		t.Fatalf("items %#v", items)
	}
}

func TestCompile_PlacesAvailableAndTurnIn(t *testing.T) {
	dir := publish(t, dump(t, quest(5, nil, map[string]any{
		"places": []any{
			map[string]any{"role": "available", "type": "npc", "id": 8, "name": "Giver", "spawns": []any{[]any{14, 1, 2}, []any{14, 3, 4}}},
			map[string]any{"role": "turnIn", "type": "npc", "id": 9, "name": "End", "spawns": []any{[]any{14, 5, 6}}},
		},
	})))
	places := readQuest(t, dir, 5)["places"].([]any)
	available := places[0].(map[string]any)
	turnIn := places[1].(map[string]any)
	if available["role"] != "available" || len(available["spawns"].([]any)) != 2 {
		t.Fatalf("available %#v", available)
	}
	if turnIn["role"] != "turnIn" {
		t.Fatalf("turnIn %#v", turnIn)
	}
}

func TestCompile_ObjectiveDropSpawns(t *testing.T) {
	dir := publish(t, dump(t, quest(6, nil, map[string]any{
		"places": []any{map[string]any{
			"role": "objective", "type": "npc", "id": 40, "name": "Wolf", "itemId": 99,
			"spawns": []any{[]any{12, 30.5, 40}},
		}},
	})))
	place := readQuest(t, dir, 6)["places"].([]any)[0].(map[string]any)
	if place["role"] != "objective" || place["itemId"].(float64) != 99 {
		t.Fatalf("place %#v", place)
	}
	spawn := place["spawns"].([]any)[0].([]any)
	if spawn[0].(float64) != 12 || spawn[1].(float64) != 30.5 || spawn[2].(float64) != 40 {
		t.Fatalf("spawn %#v", spawn)
	}
}

func TestCompile_PlaceWithoutSpawns(t *testing.T) {
	dir := publish(t, dump(t, quest(8, nil, map[string]any{
		"places": []any{map[string]any{"role": "available", "type": "item", "id": 5, "name": "Letter", "spawns": []any{}}},
	})))
	place := readQuest(t, dir, 8)["places"].([]any)[0].(map[string]any)
	if place["spawns"] == nil {
		t.Fatal("missing spawns array")
	}
	if len(place["spawns"].([]any)) != 0 {
		t.Fatalf("spawns %#v", place["spawns"])
	}
}

func TestCompile_QuestTypeFlags(t *testing.T) {
	dir := publish(t, dump(t, quest(9, map[string]any{"questFlags": 4096 + 64, "specialFlags": 1})))
	questType := readQuest(t, dir, 9)["questType"].(map[string]any)
	if questType["daily"] != true || questType["raid"] != true || questType["repeatable"] != true {
		t.Fatalf("questType %#v", questType)
	}
	eventDir := publish(t, dump(t, quest(19, map[string]any{"specialFlags": 2})))
	if readQuest(t, eventDir, 19)["questType"].(map[string]any)["event"] != true {
		t.Fatal("event bit was not published")
	}
	err := Run(Options{Dump: strings.NewReader(dump(t, quest(9, map[string]any{"questFlags": 8192}))), OutDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "unknown questFlags") {
		t.Fatalf("got %v", err)
	}
}

func TestCompile_QuestTypeProfession(t *testing.T) {
	dir := publish(t, dump(t,
		quest(10, map[string]any{"zoneOrSort": -304}),
		quest(11, map[string]any{"requiredSkill": []any{356, 1}}),
	))
	if readQuest(t, dir, 10)["questType"].(map[string]any)["sort"] != "COOKING" {
		t.Fatal("cooking sort missing")
	}
	if readQuest(t, dir, 11)["questType"].(map[string]any)["profession"] != "FISHING" {
		t.Fatal("fishing profession missing")
	}
}

func TestCompile_QuestTypeDungeon(t *testing.T) {
	dir := publish(t, dump(t,
		quest(12, map[string]any{"zoneOrSort": 1581}),
		quest(13, map[string]any{"zoneOrSort": 3277}),
	))
	deadmines := readQuest(t, dir, 12)["questType"].(map[string]any)
	gulch := readQuest(t, dir, 13)["questType"].(map[string]any)
	if deadmines["dungeon"] != true || deadmines["battleground"] == true {
		t.Fatalf("deadmines %#v", deadmines)
	}
	if gulch["battleground"] != true || gulch["dungeon"] == true {
		t.Fatalf("warsong %#v", gulch)
	}
}

func TestManifest_ProbeIsSufficient(t *testing.T) {
	dir := publish(t, dump(t, quest(1, map[string]any{"name": "A"})))
	var manifest map[string]any
	unmarshalFile(t, filepath.Join(dir, "manifest.json"), &manifest)
	shards := manifest["shards"].([]any)
	for _, row := range shards {
		shard := row.(map[string]any)
		body, err := os.ReadFile(filepath.Join(dir, shard["path"].(string)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != shard["sha256"] {
			t.Fatalf("sha256 mismatch for %s", shard["path"])
		}
	}
}

func TestCompile_ShardUnderLimit(t *testing.T) {
	if !ExceedsShardLimit(maxShardBytes + 1) {
		t.Fatal("oversized shard was accepted")
	}
	if ExceedsShardLimit(maxShardBytes) {
		t.Fatal("limit itself was rejected")
	}
}

func TestCompile_CheckRejectsStaleFile(t *testing.T) {
	raw := dump(t, quest(1, map[string]any{"name": "A"}))
	dir := publish(t, raw)
	stale := filepath.Join(dir, "quests", "0099.json")
	if err := os.WriteFile(stale, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run(Options{Dump: strings.NewReader(raw), OutDir: dir, Check: true})
	if err == nil || !strings.Contains(err.Error(), "stale file") {
		t.Fatalf("got %v", err)
	}
	if _, statErr := os.Stat(stale); statErr != nil {
		t.Fatal(statErr)
	}
}

func TestCompile_CheckIsDryRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	err := Run(Options{Dump: strings.NewReader(dump(t, quest(1, nil))), OutDir: dir, Check: true})
	if err == nil {
		t.Fatal("check succeeded without a tree")
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatal("check created the output directory")
	}
}

func TestCompile_SecondRunByteIdentical(t *testing.T) {
	raw := dump(t, quest(1, map[string]any{"name": "Same"}))
	first := publish(t, raw)
	second := publish(t, raw)
	sameFile(t, filepath.Join(first, "manifest.json"), filepath.Join(second, "manifest.json"))
	sameFile(t, filepath.Join(first, "quests/0001.json"), filepath.Join(second, "quests/0001.json"))
}

func TestCompile_MaskZeroIsBoth(t *testing.T) {
	dir := publish(t, dump(t, quest(1, map[string]any{"requiredRaces": 0})))
	if readQuest(t, dir, 1)["faction"] != "Both" {
		t.Fatal("mask 0 was not Both")
	}
	if _, ok := readQuest(t, dir, 1)["requiredRaces"]; ok {
		t.Fatal("zero race mask was published")
	}
}

func TestCompile_UnknownRaceBit(t *testing.T) {
	err := Run(Options{Dump: strings.NewReader(dump(t, quest(1, map[string]any{"requiredRaces": 512}))), OutDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "unknown race") {
		t.Fatalf("got %v", err)
	}
}

func TestCompile_PrerequisitesStayDistinct(t *testing.T) {
	dir := publish(t, dump(t, quest(1, map[string]any{
		"preQuestGroup":  []any{2, 3},
		"preQuestSingle": []any{4, 5},
	})))
	body := readQuest(t, dir, 1)
	if _, ok := body["prerequisiteQuestIds"]; ok {
		t.Fatal("collapsed prerequisite list was published")
	}
	if len(body["preQuestGroup"].([]any)) != 2 || len(body["preQuestSingle"].([]any)) != 2 {
		t.Fatalf("prerequisites %#v %#v", body["preQuestGroup"], body["preQuestSingle"])
	}
}

func TestCompile_TruncatedDumpKeepsExistingTree(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Run(Options{Dump: strings.NewReader("{\"id\":1}\n"), OutDir: dir})
	if err == nil {
		t.Fatal("truncated dump succeeded")
	}
	body, readErr := os.ReadFile(sentinel)
	if readErr != nil || string(body) != "keep" {
		t.Fatalf("existing tree changed: %v %q", readErr, body)
	}
}

func TestCompile_EmptyIDListFails(t *testing.T) {
	err := Run(Options{Dump: strings.NewReader(summaryLine(t, 0)), OutDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "empty quest list") {
		t.Fatalf("got %v", err)
	}
}

func TestCompile_UnmappedFieldFails(t *testing.T) {
	err := Run(Options{
		Dump:   strings.NewReader(dump(t, quest(1, map[string]any{"notAQuestField": 1}))),
		OutDir: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "unmapped field") {
		t.Fatalf("got %v", err)
	}
}

func TestMergeATT_FillsMatchingAvailablePlacesAndKeepsQuestieData(t *testing.T) {
	rows := []map[string]any{
		quest(333, map[string]any{"name": "Questie title"}, map[string]any{
			"places": []any{map[string]any{"role": "available", "type": "npc", "id": 1427, "name": "Harlan", "spawns": []any{}}},
		}),
		quest(399, map[string]any{"name": "Questie title"}, map[string]any{
			"places": []any{map[string]any{"role": "available", "type": "npc", "id": 1646, "spawns": []any{[]any{1519, 1.0, 2.0}}}},
		}),
		quest(9000, map[string]any{"name": "Questie only"}, map[string]any{"places": []any{map[string]any{"role": "available", "type": "npc", "id": 1, "spawns": []any{}}}}),
	}
	attData := &att.Result{Locations: []att.Location{
		{QuestID: 333, Kind: "npc", NPCID: 1427, Spawns: []att.Spawn{{Zone: 1519, X: 62.3, Y: 67.9}}},
		{QuestID: 399, Kind: "npc", NPCID: 1646, Spawns: []att.Spawn{{Zone: 1519, X: 57.7, Y: 47.9}}},
		{QuestID: 1, Kind: "npc", NPCID: 2, Spawns: []att.Spawn{{Zone: 1519, X: 3, Y: 4}}},
	}}
	dir := t.TempDir()
	if err := Run(Options{Dump: strings.NewReader(dump(t, rows...)), OutDir: dir, ATT: attData, ATTCommit: "def"}); err != nil {
		t.Fatal(err)
	}
	q333 := readQuest(t, dir, 333)
	spawn := q333["places"].([]any)[0].(map[string]any)["spawns"].([]any)[0].([]any)
	if spawn[0].(float64) != 1519 || spawn[1].(float64) != 62.3 || spawn[2].(float64) != 67.9 || q333["name"] != "Questie title" {
		t.Fatalf("merged quest 333 %#v", q333)
	}
	q399 := readQuest(t, dir, 399)
	spawn = q399["places"].([]any)[0].(map[string]any)["spawns"].([]any)[0].([]any)
	if spawn[1].(float64) != 1 || spawn[2].(float64) != 2 {
		t.Fatalf("Questie spawn was overwritten: %#v", spawn)
	}
	var manifest map[string]any
	unmarshalFile(t, filepath.Join(dir, "manifest.json"), &manifest)
	if manifest["sources"].(map[string]any)["att"] != "def" {
		t.Fatalf("source metadata %#v", manifest["sources"])
	}
	counts := manifest["merge"].(map[string]any)
	if counts["attFilledSpawns"].(float64) != 1 || counts["coordinateConflicts"].(float64) != 1 || counts["attOnlyQuestsExcluded"].(float64) != 1 {
		t.Fatalf("merge counts %#v", counts)
	}
	if len(counts["issues"].([]any)) != 1 {
		t.Fatalf("merge issues %#v", counts["issues"])
	}
	var shard map[string]any
	unmarshalFile(t, filepath.Join(dir, "quests/0001.json"), &shard)
	if _, included := shard["quests"].(map[string]any)["1"]; included {
		t.Fatal("ATT-only quest was added to the Questie quest set")
	}
}

func TestMergeATT_ReportsAmbiguousAvailablePlace(t *testing.T) {
	row := quest(10, nil, map[string]any{"places": []any{
		map[string]any{"role": "available", "type": "npc", "id": 4, "spawns": []any{}},
		map[string]any{"role": "available", "type": "npc", "id": 4, "spawns": []any{}},
	}})
	dir := t.TempDir()
	data := &att.Result{Locations: []att.Location{{QuestID: 10, Kind: "npc", NPCID: 4, Spawns: []att.Spawn{{Zone: 1519, X: 1, Y: 2}}}}}
	if err := Run(Options{Dump: strings.NewReader(dump(t, row)), OutDir: dir, ATT: data, ATTCommit: "att-sha"}); err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	unmarshalFile(t, filepath.Join(dir, "manifest.json"), &manifest)
	merge := manifest["merge"].(map[string]any)
	if merge["ambiguousLocations"].(float64) != 1 || len(merge["issues"].([]any)) != 1 {
		t.Fatalf("merge report %#v", merge)
	}
}

func TestCompile_ATTOutputIsDeterministic(t *testing.T) {
	attData := &att.Result{
		Locations: []att.Location{{QuestID: 3, Kind: "npc", NPCID: 4, Spawns: []att.Spawn{{Zone: 1519, X: 1.5, Y: 2.5}}}},
		Unmapped:  []att.Report{{QuestID: 9, NPCID: 10, File: "map.lua", Reason: "unknown map"}},
	}
	raw := dump(t, quest(3, map[string]any{"name": "A"}, map[string]any{"places": []any{map[string]any{
		"role": "available", "type": "npc", "id": 4, "spawns": []any{},
	}}}))
	first, second := t.TempDir(), t.TempDir()
	for _, dir := range []string{first, second} {
		if err := Run(Options{Dump: strings.NewReader(raw), OutDir: dir, ATT: attData, ATTCommit: "att-sha"}); err != nil {
			t.Fatal(err)
		}
	}
	var manifest map[string]any
	unmarshalFile(t, filepath.Join(first, "manifest.json"), &manifest)
	if manifest["merge"].(map[string]any)["unmappedLocations"].(float64) != 1 {
		t.Fatalf("manifest merge metadata %#v", manifest["merge"])
	}
	sameFile(t, filepath.Join(first, "manifest.json"), filepath.Join(second, "manifest.json"))
	sameFile(t, filepath.Join(first, "quests/0001.json"), filepath.Join(second, "quests/0001.json"))
}

func TestATTRegressionStormwindCoordinatesCompileForReportedAndRelatedQuests(t *testing.T) {
	cases := []struct {
		quest, npc int
		x, y       float64
	}{
		{333, 1427, 62.3, 67.9}, {399, 1646, 57.7, 47.9}, {353, 1416, 59.7, 33.8},
		{325, 1416, 59.7, 33.8}, {389, 1646, 57.7, 47.9}, {393, 1646, 57.7, 47.9}, {396, 1646, 57.7, 47.9},
	}
	var rows []map[string]any
	var locations []att.Location
	for _, test := range cases {
		rows = append(rows, quest(test.quest, map[string]any{"name": "Quest"}, map[string]any{"places": []any{map[string]any{
			"role": "available", "type": "npc", "id": test.npc, "spawns": []any{},
		}}}))
		locations = append(locations, att.Location{QuestID: test.quest, Kind: "npc", NPCID: test.npc, Spawns: []att.Spawn{{Zone: 1519, X: test.x, Y: test.y}}})
	}
	dir := t.TempDir()
	if err := Run(Options{Dump: strings.NewReader(dump(t, rows...)), OutDir: dir, ATT: &att.Result{Locations: locations}, ATTCommit: "att-sha"}); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		questBody := readQuest(t, dir, test.quest)
		place := questBody["places"].([]any)[0].(map[string]any)
		spawn := place["spawns"].([]any)[0].([]any)
		if spawn[0].(float64) != 1519 || spawn[1].(float64) != test.x || spawn[2].(float64) != test.y {
			t.Errorf("quest %d spawn = %#v", test.quest, spawn)
		}
	}
}

func quest(id int, fields map[string]any, extra ...map[string]any) map[string]any {
	if fields == nil {
		fields = map[string]any{}
	}
	row := map[string]any{"id": id, "fields": fields}
	for _, more := range extra {
		for key, value := range more {
			row[key] = value
		}
	}
	return row
}

func dump(t *testing.T, rows ...map[string]any) string {
	t.Helper()
	var buf strings.Builder
	for _, row := range rows {
		buf.WriteString(mustJSON(t, row))
		buf.WriteByte('\n')
	}
	buf.WriteString(summaryLine(t, len(rows)))
	return buf.String()
}

func summaryLine(t *testing.T, count int) string {
	t.Helper()
	return mustJSON(t, map[string]any{
		"kind": "summary", "questCount": count, "commit": "abc", "generatedAt": "2026-10-04T00:00:00Z",
	}) + "\n"
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func publish(t *testing.T, raw string) string {
	t.Helper()
	dir := t.TempDir()
	if err := Run(Options{Dump: strings.NewReader(raw), OutDir: dir}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readQuest(t *testing.T, dir string, id int) map[string]any {
	t.Helper()
	var file map[string]any
	unmarshalFile(t, filepath.Join(dir, "quests/0001.json"), &file)
	quests := file["quests"].(map[string]any)
	for _, value := range quests {
		body := value.(map[string]any)
		if int(body["id"].(float64)) == id {
			return body
		}
	}
	t.Fatalf("quest %d not in shard", id)
	return nil
}

func unmarshalFile(t *testing.T, path string, dest any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, dest); err != nil {
		t.Fatal(err)
	}
}

func walkJSON(t *testing.T, value any, visit func(string)) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			visit(key)
			walkJSON(t, child, visit)
		}
	case []any:
		for _, child := range typed {
			walkJSON(t, child, visit)
		}
	}
}

func sameFile(t *testing.T, left, right string) {
	t.Helper()
	a, err := os.ReadFile(left)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(right)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("files differ\n%s\n%s", a, b)
	}
}
