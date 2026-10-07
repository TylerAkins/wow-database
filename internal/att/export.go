package att

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	questNameRE      = regexp.MustCompile(`(?m)^\s*q\s*\(\s*(\d+)\s*,\s*\{[^\n]*--\s*(.+?)\s*$`)
	sourceQuestRE    = regexp.MustCompile(`(?:\["sourceQuest"\]|\bsourceQuest)\s*=\s*(\d+)`)
	sourceQuestsRE   = regexp.MustCompile(`(?:\["sourceQuests"\]|\bsourceQuests)\s*=\s*\{([^}]*)\}`)
	sourceQuestNumRE = regexp.MustCompile(`(?:\["sourceQuestNumRequired"\]|\bsourceQuestNumRequired)\s*=\s*(\d+)`)
	altQuestsRE      = regexp.MustCompile(`(?:\["altQuests"\]|\baltQuests)\s*=\s*\{([^}]*)\}`)
	lvlRE            = regexp.MustCompile(`(?:\["lvl"\]|\blvl)\s*=\s*(\d+)`)
	descRE           = regexp.MustCompile(`(?:\["description"\]|\bdescription)\s*=\s*"((?:\\.|[^"\\])*)"`)
	nameFieldRE      = regexp.MustCompile(`(?:\["name"\]|\bname)\s*=\s*"((?:\\.|[^"\\])*)"`)
	qiRE             = regexp.MustCompile(`(?:\["qi"\]|\bqi)\s*=\s*(\d+)`)
	boolFieldRE      = regexp.MustCompile(`(?:\["(isBreadcrumb|isDaily|isWeekly|isMonthly|repeatable|isYearly)"\]|(isBreadcrumb|isDaily|isWeekly|isMonthly|repeatable|isYearly))\s*=\s*true`)
	racesFieldRE     = regexp.MustCompile(`(?:\["races"\]|\braces)\s*=\s*(ALLIANCE_ONLY|HORDE_ONLY|\{([^}]*)\})`)
	classesFieldRE   = regexp.MustCompile(`(?:\["classes"\]|\bclasses)\s*=\s*\{([^}]*)\}`)
	providerSingleRE = regexp.MustCompile(`(?:\["provider"\]|\bprovider)\s*=\s*\{\s*"([noi])"\s*,\s*(\d+)\s*\}`)
	providersMultiRE = regexp.MustCompile(`(?:\["providers"\]|\bproviders)\s*=\s*\{`)
	objectiveCallRE  = regexp.MustCompile(`objective\s*\(\s*\d+\s*,\s*\{`)
	objectiveTextRE  = regexp.MustCompile(`objective\s*\(\s*\d+\s*,\s*\{[^\n]*--\s*(.+?)\s*$`)
	crRE             = regexp.MustCompile(`(?:\["cr"\]|\bcr)\s*=\s*(\d+)`)
	crsRE            = regexp.MustCompile(`(?:\["crs"\]|\bcrs)\s*=\s*\{([^}]*)\}`)
)

var foreverAllianceRaces = []int{1, 3, 4, 7, 95}
var foreverHordeRaces = []int{2, 5, 6, 8, 9, 96}

var raceConstants = map[string]int{
	"HUMAN": 1, "DWARF": 3, "NIGHTELF": 4, "GNOME": 7, "ORC": 2, "UNDEAD": 5,
	"TAUREN": 6, "TROLL": 8, "GOBLIN": 9, "SKYBORNE_ALLIANCE": 95, "SKYBORNE_HORDE": 96,
}

var classConstants = map[string]uint64{
	"WARRIOR": 1, "PALADIN": 2, "HUNTER": 4, "ROGUE": 8, "PRIEST": 16, "DEATH_KNIGHT": 32,
	"SHAMAN": 64, "MAGE": 128, "WARLOCK": 256, "MONK": 512, "DRUID": 1024,
}

// ExportMeta holds non-fatal parse outcomes from an ATT export.
type ExportMeta struct {
	Unmapped []Report
}

// Export reads ATT Forever quest declarations and returns JSONL-shaped quest rows.
func Export(root string) ([]map[string]any, ExportMeta, error) {
	ctx, err := loadExportContext(root)
	if err != nil {
		return nil, ExportMeta{}, err
	}
	byID := map[int]map[string]any{}
	for _, path := range ctx.files {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, ExportMeta{}, readErr
		}
		rows, reports, parseErr := parseQuestFile(path, body, ctx)
		if parseErr != nil {
			return nil, ExportMeta{}, parseErr
		}
		ctx.meta.Unmapped = append(ctx.meta.Unmapped, reports...)
		for _, row := range rows {
			id, _ := row["id"].(int)
			if existing, ok := byID[id]; ok {
				mergeDumpRow(existing, row)
			} else {
				byID[id] = row
			}
		}
	}
	ids := make([]int, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	sort.Slice(ctx.meta.Unmapped, func(i, j int) bool {
		a, b := ctx.meta.Unmapped[i], ctx.meta.Unmapped[j]
		if a.QuestID != b.QuestID {
			return a.QuestID < b.QuestID
		}
		if a.NPCID != b.NPCID {
			return a.NPCID < b.NPCID
		}
		return a.File < b.File
	})
	return out, ctx.meta, nil
}

// WriteJSONL encodes quest rows and a summary line for the compile pipeline.
func WriteJSONL(rows []map[string]any, commit, generatedAt string, w *bytes.Buffer) error {
	for _, row := range rows {
		body, err := json.Marshal(row)
		if err != nil {
			return err
		}
		w.Write(body)
		w.WriteByte('\n')
	}
	summary, err := json.Marshal(map[string]any{
		"kind":        "summary",
		"questCount":  len(rows),
		"commit":      commit,
		"generatedAt": generatedAt,
	})
	if err != nil {
		return err
	}
	w.Write(summary)
	w.WriteByte('\n')
	return nil
}

type exportContext struct {
	files               []string
	maps                map[string]int
	mapAreas            map[int]int
	coordinateConstants map[string][]Spawn
	meta                ExportMeta
}

func loadExportContext(root string) (exportContext, error) {
	const dataRoot = ".contrib/.db/forever"
	mapPath := filepath.Join(root, dataRoot, ".config/constants/maps.lua")
	mapBody, err := os.ReadFile(mapPath)
	if err != nil {
		return exportContext{}, fmt.Errorf("ATT map constants %s: %w", mapPath, err)
	}
	maps := map[string]int{}
	for _, row := range mapConstRE.FindAllSubmatch(mapBody, -1) {
		id, err := strconv.Atoi(string(row[2]))
		if err != nil {
			return exportContext{}, fmt.Errorf("ATT map constant %s: %w", row[1], err)
		}
		maps[string(row[1])] = id
	}
	if len(maps) == 0 {
		return exportContext{}, fmt.Errorf("ATT map constants contain no supported assignments")
	}
	var files []string
	err = filepath.WalkDir(filepath.Join(root, dataRoot), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "zzOLD" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".lua") && !strings.Contains(path, string(filepath.Separator)+".config"+string(filepath.Separator)) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return exportContext{}, fmt.Errorf("walk ATT Forever data: %w", err)
	}
	sort.Strings(files)
	mapAreas := map[int]int{}
	for mapID, areaID := range classicMapAreas {
		mapAreas[mapID] = areaID
	}
	ctx := exportContext{
		files:               files,
		maps:                maps,
		mapAreas:            mapAreas,
		coordinateConstants: map[string][]Spawn{},
	}
	for _, path := range files {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return exportContext{}, readErr
		}
		if err := readMapAreas(path, body, maps, mapAreas); err != nil {
			return exportContext{}, err
		}
	}
	for _, path := range files {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return exportContext{}, readErr
		}
		if err := readCoordinateConstants(path, body, maps, mapAreas, ctx.coordinateConstants); err != nil {
			return exportContext{}, err
		}
	}
	return ctx, nil
}

func parseQuestFile(path string, body []byte, ctx exportContext) ([]map[string]any, []Report, error) {
	text := string(body)
	masked := maskLua(text)
	matches := questCallRE.FindAllStringSubmatchIndex(masked, -1)
	var rows []map[string]any
	var reports []Report
	for _, match := range matches {
		id, _ := strconv.Atoi(text[match[2]:match[3]])
		open := match[1] - 1
		end, err := luaTableEnd(text, open)
		if err != nil {
			return nil, nil, fmt.Errorf("%s quest %d: %w", path, id, err)
		}
		block := text[open+1 : end]
		row, fileReports, err := buildDumpQuest(path, id, text, block, ctx)
		if err != nil {
			return nil, nil, err
		}
		reports = append(reports, fileReports...)
		rows = append(rows, row)
	}
	return rows, reports, nil
}

func buildDumpQuest(path string, id int, fileText, block string, ctx exportContext) (map[string]any, []Report, error) {
	fields := map[string]any{}
	if name := questNameFromSource(fileText, id); name != "" {
		fields["name"] = name
	}
	if match := nameFieldRE.FindStringSubmatch(block); len(match) == 2 {
		fields["name"] = unescapeLuaString(match[1])
	}
	if match := descRE.FindStringSubmatch(block); len(match) == 2 {
		fields["objectivesText"] = []any{unescapeLuaString(match[1])}
	}
	if match := lvlRE.FindStringSubmatch(block); len(match) == 2 {
		level, _ := strconv.Atoi(match[1])
		fields["questLevel"] = level
	}
	if match := qiRE.FindStringSubmatch(block); len(match) == 2 {
		item, _ := strconv.Atoi(match[1])
		fields["sourceItemId"] = item
	}
	if err := applyPrereqs(fields, block); err != nil {
		return nil, nil, fmt.Errorf("%s quest %d: %w", path, id, err)
	}
	if err := applyAltQuests(fields, block); err != nil {
		return nil, nil, fmt.Errorf("%s quest %d: %w", path, id, err)
	}
	if err := applyRaceClass(fields, block); err != nil {
		return nil, nil, fmt.Errorf("%s quest %d: %w", path, id, err)
	}
	applyQuestFlags(fields, block)

	providers, err := questProviders(block)
	if err != nil {
		return nil, nil, fmt.Errorf("%s quest %d: %w", path, id, err)
	}
	startedBy := providersToRows(providers)
	finishedBy := startedBy

	questCoords, reports, err := questLevelCoords(path, id, block, ctx)
	if err != nil {
		return nil, nil, err
	}

	var places []map[string]any
	seen := map[string]bool{}
	for _, provider := range providers {
		spawns := questCoords[providerKey(provider)]
		place := map[string]any{
			"role": "available", "type": provider.kind, "id": provider.id, "spawns": spawnsToAny(spawns),
		}
		places = addPlace(places, seen, place)
	}
	for _, provider := range providers {
		spawns := questCoords[providerKey(provider)]
		place := map[string]any{
			"role": "turnIn", "type": provider.kind, "id": provider.id, "spawns": spawnsToAny(spawns),
		}
		places = addPlace(places, seen, place)
	}

	objectives, objectivePlaces, err := parseObjectives(path, id, block, ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(objectives) > 0 {
		fields["objectives"] = objectives
	}
	for _, place := range objectivePlaces {
		places = addPlace(places, seen, place)
	}

	if len(startedBy) > 0 {
		for i, row := range startedBy {
			key := providerKey(providerRef{kind: row["type"].(string), id: row["id"].(int)})
			if spawns, ok := questCoords[key]; ok {
				startedBy[i]["spawns"] = spawnsToAny(spawns)
			} else {
				startedBy[i]["spawns"] = []any{}
			}
		}
	}
	if len(finishedBy) > 0 {
		for i, row := range finishedBy {
			key := providerKey(providerRef{kind: row["type"].(string), id: row["id"].(int)})
			if spawns, ok := questCoords[key]; ok {
				finishedBy[i]["spawns"] = spawnsToAny(spawns)
			} else {
				finishedBy[i]["spawns"] = []any{}
			}
		}
	}

	if zone := inferZoneOrSort(places); zone != 0 {
		fields["zoneOrSort"] = zone
	}

	row := map[string]any{"id": id, "fields": fields}
	if len(startedBy) > 0 {
		row["startedBy"] = mapsToAny(startedBy)
	}
	if len(finishedBy) > 0 {
		row["finishedBy"] = mapsToAny(finishedBy)
	}
	if len(places) > 0 {
		row["places"] = mapsToAny(places)
	}
	return row, reports, nil
}

func mapsToAny(rows []map[string]any) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

type providerRef struct {
	kind string
	id   int
}

func providerKey(ref providerRef) string {
	return ref.kind + "\x00" + strconv.Itoa(ref.id)
}

func questNameFromSource(fileText string, id int) string {
	for _, match := range questNameRE.FindAllStringSubmatch(fileText, -1) {
		questID, _ := strconv.Atoi(match[1])
		if questID == id {
			return strings.TrimSpace(match[2])
		}
	}
	return ""
}

func unescapeLuaString(value string) string {
	value = strings.ReplaceAll(value, `\"`, `"`)
	value = strings.ReplaceAll(value, `\\`, `\`)
	value = strings.ReplaceAll(value, `\n`, "\n")
	value = strings.ReplaceAll(value, `\r`, "\r")
	value = strings.ReplaceAll(value, `\t`, "\t")
	return value
}

func applyPrereqs(fields map[string]any, block string) error {
	if match := sourceQuestRE.FindStringSubmatch(block); len(match) == 2 {
		id, _ := strconv.Atoi(match[1])
		fields["preQuestSingle"] = []any{id}
	}
	if match := sourceQuestsRE.FindStringSubmatch(block); len(match) == 2 {
		ids, err := parseIDList(stripLineComments(match[1]))
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		required := 0
		if numMatch := sourceQuestNumRE.FindStringSubmatch(block); len(numMatch) == 2 {
			required, _ = strconv.Atoi(numMatch[1])
		}
		if required > 1 || (required == 0 && len(ids) > 1) {
			list := make([]any, 0, len(ids))
			for _, id := range ids {
				list = append(list, id)
			}
			fields["preQuestGroup"] = list
		} else {
			list := make([]any, 0, len(ids))
			for _, id := range ids {
				list = append(list, id)
			}
			fields["preQuestSingle"] = list
		}
	}
	return nil
}

func applyAltQuests(fields map[string]any, block string) error {
	match := altQuestsRE.FindStringSubmatch(block)
	if len(match) != 2 {
		return nil
	}
	ids, err := parseIDList(match[1])
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	list := make([]any, 0, len(ids))
	for _, id := range ids {
		list = append(list, id)
	}
	fields["exclusiveTo"] = list
	return nil
}

func applyRaceClass(fields map[string]any, block string) error {
	if match := racesFieldRE.FindStringSubmatch(block); len(match) == 3 {
		switch match[1] {
		case "ALLIANCE_ONLY":
			fields["requiredRaces"] = raceMask(foreverAllianceRaces)
		case "HORDE_ONLY":
			fields["requiredRaces"] = raceMask(foreverHordeRaces)
		default:
			ids, err := parseSymbolList(stripLineComments(match[2]), raceConstants)
			if err != nil {
				return err
			}
			fields["requiredRaces"] = raceMask(ids)
		}
	}
	if match := classesFieldRE.FindStringSubmatch(block); len(match) == 2 {
		mask, err := classMaskFromList(stripLineComments(match[1]))
		if err != nil {
			return err
		}
		if mask != 0 {
			fields["requiredClasses"] = mask
		}
	}
	return nil
}

func applyQuestFlags(fields map[string]any, block string) {
	var questFlags int
	var specialFlags int
	for _, match := range boolFieldRE.FindAllStringSubmatch(block, -1) {
		name := match[1]
		if name == "" {
			name = match[2]
		}
		switch name {
		case "isDaily":
			questFlags |= 4096
		case "isWeekly":
			questFlags |= 32768
		case "isMonthly":
			questFlags |= 65536
		case "repeatable", "isYearly":
			specialFlags |= 1
		}
	}
	if questFlags != 0 {
		fields["questFlags"] = questFlags
	}
	if specialFlags != 0 {
		fields["specialFlags"] = specialFlags
	}
}

func raceMask(ids []int) int {
	mask := uint64(0)
	for _, id := range ids {
		for _, bit := range raceBitsForExport() {
			if bit.id == id {
				mask |= bit.bit
			}
		}
	}
	return int(mask)
}

func raceBitsForExport() []struct {
	bit uint64
	id  int
} {
	return []struct {
		bit uint64
		id  int
	}{
		{1, 1}, {2, 2}, {4, 3}, {8, 4}, {16, 5}, {32, 6}, {64, 7}, {128, 8},
		{256, 9}, {4294967296, 95}, {8589934592, 96},
	}
}

func classMaskFromList(body string) (int, error) {
	var mask uint64
	for _, token := range strings.Split(stripLineComments(body), ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if bit, ok := classConstants[token]; ok {
			mask |= bit
			continue
		}
		return 0, fmt.Errorf("unknown class constant %s", token)
	}
	return int(mask), nil
}

func parseSymbolList(body string, constants map[string]int) ([]int, error) {
	var ids []int
	for _, token := range strings.Split(body, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if constants != nil {
			if id, ok := constants[token]; ok {
				ids = append(ids, id)
				continue
			}
		}
		id, err := strconv.Atoi(token)
		if err != nil {
			return nil, fmt.Errorf("unknown symbol %s", token)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func parseIDList(body string) ([]int, error) {
	return parseSymbolList(stripLineComments(body), nil)
}

func stripLineComments(text string) string {
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func questProviders(block string) ([]providerRef, error) {
	refs := []providerRef{}
	seen := map[string]bool{}
	add := func(kind string, id int) {
		ref := providerRef{kind: attProviderKind(kind), id: id}
		key := providerKey(ref)
		if seen[key] {
			return
		}
		seen[key] = true
		refs = append(refs, ref)
	}
	for _, match := range providerRE.FindAllStringSubmatch(block, -1) {
		id, _ := strconv.Atoi(match[1])
		add("n", id)
	}
	for _, match := range providersRE.FindAllStringSubmatch(block, -1) {
		for _, idText := range idRE.FindAllString(match[1], -1) {
			id, _ := strconv.Atoi(idText)
			add("n", id)
		}
	}
	for _, match := range providerSingleRE.FindAllStringSubmatch(block, -1) {
		id, _ := strconv.Atoi(match[2])
		add(match[1], id)
	}
	masked := maskLua(block)
	for _, indexes := range providersMultiRE.FindAllStringIndex(masked, -1) {
		open := indexes[1]
		for open < len(block) && block[open] != '{' {
			open++
		}
		if open >= len(block) {
			continue
		}
		end, err := luaTableEnd(block, open)
		if err != nil {
			return nil, err
		}
		inner := block[open+1 : end]
		for _, row := range regexp.MustCompile(`\{\s*"([noi])"\s*,\s*(\d+)\s*\}`).FindAllStringSubmatch(inner, -1) {
			id, _ := strconv.Atoi(row[2])
			add(row[1], id)
		}
	}
	return refs, nil
}

func attProviderKind(code string) string {
	switch code {
	case "o":
		return "object"
	case "i":
		return "item"
	default:
		return "npc"
	}
}

func providersToRows(refs []providerRef) []map[string]any {
	rows := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		rows = append(rows, map[string]any{"type": ref.kind, "id": ref.id})
	}
	return rows
}

func questLevelCoords(path string, id int, block string, ctx exportContext) (map[string][]Spawn, []Report, error) {
	coordsByProvider := map[string][]Spawn{}
	var reports []Report
	coordRows := questCoordinateTables(block)
	spawns, badMap, err := resolveCoordRows(path, id, coordRows, ctx)
	if err != nil {
		return nil, nil, err
	}
	providers, err := questProviders(block)
	if err != nil {
		return nil, nil, err
	}
	if len(providers) == 1 && len(spawns) > 0 {
		coordsByProvider[providerKey(providers[0])] = spawns
	} else if len(spawns) > 0 && len(providers) > 0 {
		coordsByProvider[providerKey(providers[0])] = spawns
	}
	for _, provider := range providers {
		if badMap != "" {
			if provider.kind == "npc" {
				reports = append(reports, Report{QuestID: id, NPCID: provider.id, File: filepath.Base(path), Reason: badMap})
			}
			continue
		}
	}
	return coordsByProvider, reports, nil
}

func resolveCoordRows(path string, id int, coordRows []string, ctx exportContext) ([]Spawn, string, error) {
	var spawns []Spawn
	badMap := ""
	for _, coordBody := range coordRows {
		coordBody = strings.TrimSpace(coordBody)
		if isIdentifier(coordBody) {
			resolved, exists := ctx.coordinateConstants[coordBody]
			if !exists {
				return nil, "", fmt.Errorf("%s quest %d: unsupported ATT coordinate reference %s", path, id, coordBody)
			}
			spawns = append(spawns, resolved...)
			continue
		}
		if strings.Contains(coordBody, "{") || strings.Contains(coordBody, "}") {
			for _, point := range nestedTables(coordBody) {
				spawn, mapName, parseErr := parsePoint(point, ctx.maps, ctx.mapAreas)
				if parseErr != nil {
					return nil, "", fmt.Errorf("%s quest %d coordinate %q: %w", path, id, point, parseErr)
				}
				if mapName != "" {
					badMap = "unmapped ATT map constant " + mapName
					continue
				}
				spawns = append(spawns, spawn)
			}
		} else {
			spawn, mapName, parseErr := parsePoint(coordBody, ctx.maps, ctx.mapAreas)
			if parseErr != nil {
				return nil, "", fmt.Errorf("%s quest %d coordinate %q: %w", path, id, coordBody, parseErr)
			}
			if mapName != "" {
				badMap = "unmapped ATT map constant " + mapName
				continue
			}
			spawns = append(spawns, spawn)
		}
	}
	return dedupe(spawns), badMap, nil
}

func parseObjectives(path string, id int, block string, ctx exportContext) (map[string]any, []map[string]any, error) {
	objectives := map[string]any{}
	places := []map[string]any{}
	seen := map[string]bool{}
	masked := maskLua(block)
	for _, indexes := range objectiveCallRE.FindAllStringIndex(masked, -1) {
		open := indexes[0]
		for open < len(block) && block[open] != '{' {
			open++
		}
		end, err := luaTableEnd(block, open)
		if err != nil {
			return nil, nil, fmt.Errorf("%s quest %d objective: %w", path, id, err)
		}
		objBlock := block[open+1 : end]
		text := ""
		if match := objectiveTextRE.FindStringSubmatch(block[indexes[0]:]); len(match) == 2 {
			text = strings.TrimSpace(match[1])
		}
		var itemID int
		var creatureIDs []int
		if match := providerSingleRE.FindStringSubmatch(objBlock); len(match) == 3 {
			providerID, _ := strconv.Atoi(match[2])
			switch match[1] {
			case "i":
				itemID = providerID
			case "n":
				creatureIDs = append(creatureIDs, providerID)
			case "o":
				spawns, _, err := resolveCoordRows(path, id, coordRowsFromBlock(objBlock), ctx)
				if err != nil {
					return nil, nil, err
				}
				place := map[string]any{
					"role": "objective", "type": "object", "id": providerID, "spawns": spawnsToAny(spawns),
				}
				if text != "" {
					place["name"] = text
				}
				places = addPlace(places, seen, place)
			}
		}
		if match := crRE.FindStringSubmatch(objBlock); len(match) == 2 {
			creatureID, _ := strconv.Atoi(match[1])
			creatureIDs = append(creatureIDs, creatureID)
		}
		if match := crsRE.FindStringSubmatch(objBlock); len(match) == 2 {
			for _, idText := range idRE.FindAllString(match[1], -1) {
				creatureID, _ := strconv.Atoi(idText)
				creatureIDs = append(creatureIDs, creatureID)
			}
		}
		spawns, _, err := resolveCoordRows(path, id, coordRowsFromBlock(objBlock), ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, creatureID := range unique(creatureIDs) {
			entry := map[string]any{"id": creatureID}
			if text != "" {
				entry["text"] = text
			}
			objectives["creature"] = appendSliceObjective(objectives["creature"], entry)
			place := map[string]any{
				"role": "objective", "type": "npc", "id": creatureID, "spawns": spawnsToAny(spawns),
			}
			if text != "" {
				place["name"] = text
			}
			places = addPlace(places, seen, place)
		}
		if itemID != 0 {
			entry := map[string]any{"id": itemID}
			if text != "" {
				entry["text"] = text
			}
			objectives["item"] = appendSliceObjective(objectives["item"], entry)
			place := map[string]any{
				"role": "objective", "type": "item", "id": itemID, "itemId": itemID, "spawns": spawnsToAny(spawns),
			}
			if text != "" {
				place["name"] = text
			}
			places = addPlace(places, seen, place)
		}
	}
	if len(objectives) == 0 {
		return nil, places, nil
	}
	return objectives, places, nil
}

func coordRowsFromBlock(block string) []string {
	return questCoordinateTables(block)
}

func questCoordinateTables(block string) []string {
	rows, err := coordinateTables(block)
	if err != nil {
		return nil
	}
	return rows
}

func appendSliceObjective(current any, entry map[string]any) []any {
	if current == nil {
		return []any{entry}
	}
	if slice, ok := current.([]any); ok {
		return append(slice, entry)
	}
	return []any{entry}
}

func spawnsToAny(spawns []Spawn) []any {
	out := make([]any, 0, len(spawns))
	for _, spawn := range spawns {
		out = append(out, []any{spawn.Zone, spawn.X, spawn.Y})
	}
	return out
}

func inferZoneOrSort(places []map[string]any) int {
	for _, place := range places {
		if place["role"] != "available" {
			continue
		}
		spawns, ok := place["spawns"].([]any)
		if !ok || len(spawns) == 0 {
			continue
		}
		tuple, ok := spawns[0].([]any)
		if !ok || len(tuple) == 0 {
			continue
		}
		switch zone := tuple[0].(type) {
		case int:
			return zone
		case float64:
			return int(zone)
		}
	}
	return 0
}

func addPlace(places []map[string]any, seen map[string]bool, place map[string]any) []map[string]any {
	key := fmt.Sprintf("%s\x00%s\x00%v\x00%v",
		place["role"], place["type"], place["id"], place["itemId"])
	if seen[key] {
		return places
	}
	seen[key] = true
	if place["spawns"] == nil {
		place["spawns"] = []any{}
	}
	return append(places, place)
}

func mergeDumpRow(existing, incoming map[string]any) {
	inFields, _ := incoming["fields"].(map[string]any)
	exFields, _ := existing["fields"].(map[string]any)
	if inFields == nil {
		return
	}
	if exFields == nil {
		existing["fields"] = inFields
	} else {
		for key, value := range inFields {
			if exFields[key] == nil || exFields[key] == "" || exFields[key] == 0 {
				exFields[key] = value
			}
		}
	}
	mergeProviders(existing, incoming, "startedBy")
	mergeProviders(existing, incoming, "finishedBy")
	mergePlaceList(existing, incoming)
}

func mergeProviders(existing, incoming map[string]any, key string) {
	inRows, _ := incoming[key].([]any)
	if len(inRows) == 0 {
		return
	}
	exRows, _ := existing[key].([]any)
	seen := map[string]bool{}
	for _, row := range exRows {
		if m, ok := row.(map[string]any); ok {
			seen[providerRowKey(m)] = true
		}
	}
	for _, row := range inRows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		k := providerRowKey(m)
		if !seen[k] {
			exRows = append(exRows, m)
			seen[k] = true
		}
	}
	if len(exRows) > 0 {
		existing[key] = exRows
	}
}

func providerRowKey(row map[string]any) string {
	id, _ := row["id"].(int)
	if id == 0 {
		if f, ok := row["id"].(float64); ok {
			id = int(f)
		}
	}
	return row["type"].(string) + "\x00" + strconv.Itoa(id)
}

func mergePlaceList(existing, incoming map[string]any) {
	inPlaces, _ := incoming["places"].([]any)
	if len(inPlaces) == 0 {
		return
	}
	exPlaces, _ := existing["places"].([]any)
	seen := map[string]bool{}
	for _, place := range exPlaces {
		if m, ok := place.(map[string]any); ok {
			seen[placeKey(m)] = true
		}
	}
	for _, place := range inPlaces {
		m, ok := place.(map[string]any)
		if !ok {
			continue
		}
		if !seen[placeKey(m)] {
			exPlaces = append(exPlaces, m)
			seen[placeKey(m)] = true
		}
	}
	if len(exPlaces) > 0 {
		existing["places"] = exPlaces
	}
}

func placeKey(place map[string]any) string {
	return fmt.Sprintf("%s\x00%s\x00%v\x00%v", place["role"], place["type"], place["id"], place["itemId"])
}
