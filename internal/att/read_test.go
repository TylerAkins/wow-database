package att

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadQuestLocationsAndMapConstants(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, ".contrib/.db/forever")
	if err := os.MkdirAll(filepath.Join(base, ".config/constants"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "zones"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(base, ".config/constants/maps.lua"), "MAP = {\n STORMWIND_CITY = 1453;\n WESTFALL = 1436;\n UNMAPPED_ZONE = 9999;\n}")
	write(filepath.Join(base, "zones/quests.lua"), `WARDEN_COORD = { { 59.7, 33.8, MAP.STORMWIND_CITY } }
	root({
	maproot(MAP.EASTERN_KINGDOMS, MAP.STORMWIND_CITY, { ["zone-text-areas"] = { 1519, 2918 } }),
	q(333, { ["qg"] = 1427, ["coord"] = { 62.3, 67.9, MAP.STORMWIND_CITY } }),
	q(399, { qg = 1646, coords = { { 57.7, 47.9, MAP.STORMWIND_CITY }, { 58.0, 48.0, MAP.STORMWIND_CITY } }, groups = { objective = { coord = { 1, 2, MAP.WESTFALL } } } }),
	q(353, { qg = 1416, coord = WARDEN_COORD }),
	q(3333, { qg = 1, coord = { 1, 2, MAP.UNMAPPED_ZONE } }),
})`)
	result, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Location{
		{QuestID: 333, Kind: "npc", NPCID: 1427, Spawns: []Spawn{{Zone: 1519, X: 62.3, Y: 67.9}}},
		{QuestID: 353, Kind: "npc", NPCID: 1416, Spawns: []Spawn{{Zone: 1519, X: 59.7, Y: 33.8}}},
		{QuestID: 399, Kind: "npc", NPCID: 1646, Spawns: []Spawn{{Zone: 1519, X: 57.7, Y: 47.9}, {Zone: 1519, X: 58, Y: 48}}},
	}
	if !reflect.DeepEqual(result.Locations, want) {
		t.Fatalf("locations:\n got %#v\nwant %#v", result.Locations, want)
	}
	if len(result.Unmapped) != 1 || result.Unmapped[0].QuestID != 3333 || !strings.Contains(result.Unmapped[0].Reason, "UNMAPPED_ZONE") {
		t.Fatalf("unmapped locations %#v", result.Unmapped)
	}
}

func TestReadRejectsMalformedQuestTable(t *testing.T) {
	_, _, err := readFile("broken.lua", []byte("q(4, { qg = 2, coord = { 1, 2, MAP.STORMWIND_CITY }"), map[string]int{"STORMWIND_CITY": 1453}, map[int]int{1453: 1519}, nil)
	if err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("got %v", err)
	}
}

func TestReadRejectsUnsupportedCoordinateSyntax(t *testing.T) {
	_, _, err := readFile("unsupported.lua", []byte("q(4, { qg = 2, coord = getCoord() })"), map[string]int{"STORMWIND_CITY": 1453}, map[int]int{1453: 1519}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported coordinate syntax") {
		t.Fatalf("got %v", err)
	}
}

func TestParsePointRejectsUnknownMapAndBadPercent(t *testing.T) {
	if _, mapName, err := parsePoint("1, 2, MAP.UNKNOWN", map[string]int{}, nil); err != nil || mapName != "UNKNOWN" {
		t.Fatalf("map result=%q err=%v", mapName, err)
	}
	spawn, mapName, err := parsePoint("1, 2, STORMWIND_CITY", map[string]int{"STORMWIND_CITY": 1453}, map[int]int{1453: 1519})
	if err != nil || mapName != "" || spawn.Zone != 1519 {
		t.Fatalf("unprefixed ATT map spawn=%+v map=%q err=%v", spawn, mapName, err)
	}
	if _, _, err := parsePoint("101, 2, MAP.STORMWIND_CITY", map[string]int{"STORMWIND_CITY": 1453}, map[int]int{1453: 1519}); err == nil {
		t.Fatal("accepted out-of-range coordinates")
	}
}

func TestLongCommentsHideQuestsAndPreserveTrailingCode(t *testing.T) {
	for _, equals := range []string{"", "=", "==", "=========="} {
		t.Run("level"+equals, func(t *testing.T) {
			source := "--[" + equals + "[\n--[[ nested note ]]\nq(1, {})\n--]" + equals + "]\nq(2, {})"
			rows, _, err := parseQuestFile("comments.lua", []byte(source), exportContext{})
			if err != nil {
				t.Fatal(err)
			}
			// At level zero the nested note closes the outer Lua comment.
			want := []int{2}
			if equals == "" {
				want = []int{1, 2}
			}
			var ids []int
			for _, row := range rows {
				ids = append(ids, row["id"].(int))
			}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("quest IDs = %v, want %v", ids, want)
			}
			masked := maskLua(source)
			if len(masked) != len(source) || strings.Count(masked, "\n") != strings.Count(source, "\n") {
				t.Fatal("mask changed source offsets or line count")
			}
		})
	}
}

func TestLongCommentsRequireMatchingClosers(t *testing.T) {
	for _, source := range []string{
		"--[=[\nq(1, {})\n--]]\nq(2, {})",
		"--[=[\nq(1, {})\n--]==]\nq(2, {})",
		"--[==[\nq(1, {})\n--]=]\nq(2, {})",
		"--[=[\nq(1, {})",
	} {
		rows, _, err := parseQuestFile("comments.lua", []byte(source), exportContext{})
		if err != nil || len(rows) != 0 {
			t.Fatalf("source %q: rows=%v err=%v", source, rows, err)
		}
	}
}

func TestTableEndIgnoresLongCommentBraces(t *testing.T) {
	for _, equals := range []string{"", "=", "==", "=========="} {
		source := "{ --[" + equals + "[\n} { --[[ note ]]\n--]" + equals + "]\n}"
		if equals == "" {
			source = "{ --[[\n} {\n--]]\n}"
		}
		end, err := luaTableEnd(source, 0)
		if err != nil || end != len(source)-1 {
			t.Fatalf("source %q: end=%d err=%v", source, end, err)
		}
	}
	for _, source := range []string{"{ --[=[\n--]]\n}", "{ --[=[\n}"} {
		if _, err := luaTableEnd(source, 0); err == nil {
			t.Fatalf("accepted unterminated comment in %q", source)
		}
	}
}
