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
