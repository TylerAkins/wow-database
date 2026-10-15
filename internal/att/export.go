package att

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	questNameRE        = regexp.MustCompile(`(?m)^\s*q\s*\(\s*(\d+)\s*,\s*\{[^\n]*--\s*(.+?)\s*$`)
	sourceQuestRE      = regexp.MustCompile(`(?:\["sourceQuest"\]|\bsourceQuest)\s*=\s*(\d+)`)
	sourceQuestsRE     = regexp.MustCompile(`(?:\["sourceQuests"\]|\bsourceQuests)\s*=\s*\{([^}]*)\}`)
	sourceQuestNumRE   = regexp.MustCompile(`(?:\["sourceQuestNumRequired"\]|\bsourceQuestNumRequired)\s*=\s*(\d+)`)
	altQuestsRE        = regexp.MustCompile(`(?:\["altQuests"\]|\baltQuests)\s*=\s*\{([^}]*)\}`)
	lvlRE              = regexp.MustCompile(`(?:\["lvl"\]|\blvl)\s*=\s*(\d+)`)
	descRE             = regexp.MustCompile(`(?:\["description"\]|\bdescription)\s*=\s*"((?:\\.|[^"\\])*)"`)
	nameFieldRE        = regexp.MustCompile(`(?:\["name"\]|\bname)\s*=\s*"((?:\\.|[^"\\])*)"`)
	qiRE               = regexp.MustCompile(`(?:\["qi"\]|\bqi)\s*=\s*(\d+)`)
	boolFieldRE        = regexp.MustCompile(`(?:\["(isBreadcrumb|isDaily|isWeekly|isMonthly|repeatable|isYearly)"\]|(isBreadcrumb|isDaily|isWeekly|isMonthly|repeatable|isYearly))\s*=\s*true`)
	racesFieldRE       = regexp.MustCompile(`(?:\["races"\]|\braces)\s*=\s*(ALLIANCE_ONLY|HORDE_ONLY|\{([^}]*)\})`)
	classesFieldRE     = regexp.MustCompile(`(?:\["classes"\]|\bclasses)\s*=\s*\{([^}]*)\}`)
	providerSingleRE   = regexp.MustCompile(`(?:\["provider"\]|\bprovider)\s*=\s*\{\s*"([noi])"\s*,\s*(\d+)\s*\}`)
	providersMultiRE   = regexp.MustCompile(`(?:\["providers"\]|\bproviders)\s*=\s*\{`)
	questDeclarationRE = regexp.MustCompile(`\bq\s*\(\s*(\d+)\s*([,)])`)
	objectiveCallRE    = regexp.MustCompile(`objective\s*\(\s*\d+\s*,\s*\{`)
	objectiveTextRE    = regexp.MustCompile(`objective\s*\(\s*\d+\s*,\s*\{[^\n]*--\s*(.+?)\s*$`)
	crRE               = regexp.MustCompile(`(?:\["cr"\]|\bcr)\s*=\s*(\d+)`)
	crsRE              = regexp.MustCompile(`(?:\["crs"\]|\bcrs)\s*=\s*\{([^}]*)\}`)
)

var foreverAllianceRaces = []int{1, 3, 4, 7, 95}
var foreverHordeRaces = []int{2, 5, 6, 8, 96}

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
	var objectiveRoutes []map[string]any
	for _, path := range ctx.files {
		body, readErr := readSource(path, ctx.config)
		if readErr != nil {
			return nil, ExportMeta{}, readErr
		}
		rows, reports, parseErr := parseQuestFile(path, body, ctx)
		if parseErr != nil {
			return nil, ExportMeta{}, parseErr
		}
		ctx.meta.Unmapped = append(ctx.meta.Unmapped, reports...)
		for _, row := range rows {
			if row["_objectiveRoute"] == true {
				objectiveRoutes = append(objectiveRoutes, row)
				continue
			}
			id, _ := row["id"].(int)
			if existing, ok := byID[id]; ok {
				mergeDumpRow(existing, row)
			} else {
				byID[id] = row
			}
		}
	}
	for _, route := range objectiveRoutes {
		mergeObjectiveRoute(byID, route)
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
	root                string
	config              sourceConfig
	constants           map[string]int
	factions            map[string][]int
}

func loadExportContext(root string) (exportContext, error) {
	const dataRoot = ".contrib/.db/forever"
	mapPath := filepath.Join(root, dataRoot, ".config/constants/maps.lua")
	mapBody, err := readSource(mapPath, readSourceConfig(root))
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
	files, err := sourceFiles(root)
	if err != nil {
		return exportContext{}, err
	}
	config := readSourceConfig(root)
	mapAreas := map[int]int{}
	for mapID, areaID := range classicMapAreas {
		mapAreas[mapID] = areaID
	}
	ctx := exportContext{
		files: files, root: root, config: config, constants: map[string]int{}, factions: map[string][]int{},
		maps:                maps,
		mapAreas:            mapAreas,
		coordinateConstants: map[string][]Spawn{},
	}
	for _, path := range files {
		body, readErr := readSource(path, ctx.config)
		if readErr != nil {
			return exportContext{}, readErr
		}
		if strings.Contains(filepath.ToSlash(path), "/shared/constants/") {
			for _, match := range mapConstRE.FindAllSubmatch(body, -1) {
				value, _ := strconv.Atoi(string(match[2]))
				ctx.constants[string(match[1])] = value
			}
		}
		if strings.HasSuffix(path, "raceIDs.lua") {
			for _, name := range []string{"ALLIANCE_ONLY", "HORDE_ONLY"} {
				re := regexp.MustCompile(name + `\s*=\s*\{([^}]*)\}`)
				if match := re.FindSubmatch(body); match != nil {
					ids, parseErr := parseSymbolList(stripLineComments(string(match[1])), ctx.constants)
					if parseErr == nil {
						ctx.factions[name] = ids
					}
				}
			}
		}
		if err := readMapAreas(path, body, maps, mapAreas); err != nil {
			return exportContext{}, err
		}
	}
	for _, path := range files {
		body, readErr := readSource(path, ctx.config)
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
	matches := questDeclarationRE.FindAllStringSubmatchIndex(masked, -1)
	var rows []map[string]any
	var reports []Report
	for _, match := range matches {
		id, _ := strconv.Atoi(text[match[2]:match[3]])
		open, end := match[1], match[1]
		block, wrapper := "", ""
		labelStart := match[1]
		if masked[match[4]] == ',' {
			for open < len(masked) && strings.ContainsRune(" \t\r\n", rune(masked[open])) {
				open++
			}
			if open < len(masked) && masked[open] == '{' {
				var err error
				end, err = luaTableEnd(text, open)
				if err != nil {
					return nil, nil, fmt.Errorf("%s quest %d: %w", path, id, err)
				}
				block = text[open+1 : end]
				labelStart = open + 1
			} else {
				close := open
				depth := 1
				for close < len(masked) {
					if masked[close] == '(' {
						depth++
					}
					if masked[close] == ')' {
						depth--
						if depth == 0 {
							break
						}
					}
					close++
				}
				if close >= len(masked) {
					return nil, nil, fmt.Errorf("%s quest %d: unterminated quest expression", path, id)
				}
				wrapper = strings.TrimSpace(text[open:close])
			}
		}
		own := topFields(block)
		inherited := topFields(inheritedFieldBlock(text, match[0]))
		for key, value := range inherited {
			if _, exists := own[key]; !exists {
				block += fmt.Sprintf("\n[\"%s\"] = %s,\n", key, value)
			}
		}
		row, fileReports, err := buildDumpQuest(path, id, text, block, ctx)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := own["name"]; !exists {
			line := strings.SplitN(text[labelStart:], "\n", 2)[0]
			if comment := strings.Index(line, "--"); comment >= 0 && strings.Trim(strings.TrimSpace(line[:comment]), ",") == "" {
				row["fields"].(map[string]any)["name"] = strings.TrimSpace(line[comment+2:])
			}
		}
		if wrapper != "" {
			evidence, _ := row["unresolved"].([]any)
			row["unresolved"] = append(evidence, map[string]any{"field": "declaration", "expression": wrapper, "reason": "quest wrapper expression was not evaluated"})
		}
		row["declarations"] = []any{map[string]any{"path": relativeATTPath(ctx.root, path), "line": strings.Count(text[:match[0]], "\n") + 1, "fields": copyFields(row["fields"].(map[string]any)), "rawFields": own, "inheritedFields": inherited, "expression": wrapper}}
		row["fieldPaths"] = map[string]string{}
		for key := range row["fields"].(map[string]any) {
			row["fieldPaths"].(map[string]string)[key] = fmt.Sprintf("%s:%d", relativeATTPath(ctx.root, path), strings.Count(text[:match[0]], "\n")+1)
		}
		reports = append(reports, fileReports...)
		rows = append(rows, row)
	}
	for _, indexes := range objectiveCallRE.FindAllStringIndex(masked, -1) {
		open := indexes[1] - 1
		end, err := luaTableEnd(text, open)
		if err != nil {
			continue
		}
		raw := topFields(text[open+1 : end])
		explicit, exists := raw["questID"]
		if !exists {
			continue
		}
		owner, err := strconv.Atoi(explicit)
		if err != nil || owner <= 0 {
			continue
		}
		objectives, places, objectiveErr := parseObjectives(path, owner, text[indexes[0]:end+1], ctx)
		route := map[string]any{"id": owner, "fields": map[string]any{}, "fieldPaths": map[string]string{}, "_objectiveRoute": true}
		if len(objectives) > 0 {
			route["fields"].(map[string]any)["objectives"] = objectives
		}
		if len(places) > 0 {
			route["places"] = mapsToAny(places)
		}
		if objectiveErr != nil {
			route["unresolved"] = []any{map[string]any{"field": "objectives", "reason": objectiveErr.Error()}}
		}
		line := strings.Count(text[:indexes[0]], "\n") + 1
		route["declarations"] = []any{map[string]any{"path": relativeATTPath(ctx.root, path), "line": line, "kind": "objective", "questID": owner, "fields": copyFields(route["fields"].(map[string]any)), "rawFields": raw}}
		route["fieldPaths"].(map[string]string)["objectives"] = fmt.Sprintf("%s:%d", relativeATTPath(ctx.root, path), line)
		rows = append(rows, route)
	}
	return rows, reports, nil
}

func buildDumpQuest(path string, id int, fileText, block string, ctx exportContext) (map[string]any, []Report, error) {
	fields := map[string]any{}
	fullBlock := block
	block = topFieldBlock(block)
	if match := nameFieldRE.FindStringSubmatch(block); len(match) == 2 {
		fields["name"] = unescapeLuaString(match[1])
	}
	if match := descRE.FindStringSubmatch(block); len(match) == 2 {
		fields["description"] = unescapeLuaString(match[1])
	}
	if match := lvlRE.FindStringSubmatch(block); len(match) == 2 {
		level, _ := strconv.Atoi(match[1])
		fields["requiredLevel"] = level
	}
	if match := qiRE.FindStringSubmatch(block); len(match) == 2 {
		item, _ := strconv.Atoi(match[1])
		fields["providedItemId"] = item
	}
	_ = applyPrereqs(fields, block)
	_ = applyAltQuests(fields, block)
	_ = applyRaceClass(fields, block)
	applyQuestFlags(fields, block)

	providers, err := questProviders(block)
	if err != nil {
		return nil, nil, fmt.Errorf("%s quest %d: %w", path, id, err)
	}
	startedBy := providersToRows(providers)

	questCoords, reports, err := questLevelCoords(path, id, block, ctx)
	var unresolved []any
	if err != nil {
		unresolved = append(unresolved, map[string]any{"field": "coordinates", "expression": topFields(block)["coord"], "reason": err.Error()})
		questCoords = map[string][]Spawn{}
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

	objectives, objectivePlaces, err := parseObjectives(path, id, fullBlock, ctx)
	if err != nil {
		unresolved = append(unresolved, map[string]any{"field": "objectives", "reason": err.Error()})
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

	row := map[string]any{"id": id, "fields": fields}
	if len(unresolved) > 0 {
		row["unresolved"] = unresolved
	}
	if len(startedBy) > 0 {
		row["startedBy"] = mapsToAny(startedBy)
	}
	if len(places) > 0 {
		row["places"] = mapsToAny(places)
	}
	applyNormalizedEvidence(row, block, ctx)
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
			if strings.TrimSpace(match[0][strings.Index(match[0], "{")+1:strings.Index(match[0], "--")]) == "" {
				return strings.TrimSpace(match[2])
			}
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
	fields["altQuests"] = list
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
		case "isBreadcrumb":
			fields["isBreadcrumb"] = true
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
	out := []byte(text)
	quote := byte(0)
	for i := 0; i < len(text); i++ {
		if quote != 0 {
			if text[i] == '\\' {
				i++
				continue
			}
			if text[i] == quote {
				quote = 0
			}
			continue
		}
		if text[i] == '"' || text[i] == '\'' {
			quote = text[i]
			continue
		}
		if i+1 < len(text) && text[i:i+2] == "--" {
			end := i + 2
			if closer, width := luaLongComment(text, i); width > 0 {
				if index := strings.Index(text[i+width:], closer); index >= 0 {
					end = i + width + index + len(closer)
				} else {
					end = len(text)
				}
			} else {
				for end < len(text) && text[end] != '\n' {
					end++
				}
			}
			for j := i; j < end; j++ {
				if out[j] != '\n' && out[j] != '\r' {
					out[j] = ' '
				}
			}
			i = end - 1
		}
	}
	return string(out)
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
	raw := topFields(block)
	if value, err := strconv.Atoi(raw["qg"]); err == nil && value > 0 {
		add("n", value)
	}
	if values, ok := literalLuaList(raw["qgs"], nil); ok {
		for _, value := range values {
			if id, ok := value.(int); ok && id > 0 {
				add("n", id)
			}
		}
	}
	for _, name := range []string{"provider", "providers"} {
		expression, exists := raw[name]
		if !exists {
			continue
		}
		rows := []string{expression}
		if name == "providers" {
			rows, _ = luaListExpressions(expression)
		}
		for _, row := range rows {
			values, ok := literalLuaList(row, nil)
			if !ok || len(values) != 2 {
				continue
			}
			kind, kindOK := values[0].(string)
			id, idOK := values[1].(int)
			if kindOK && idOK && id > 0 && (kind == "i" || kind == "o" || kind == "n") {
				add(kind, id)
			}
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
	coordRows, err := coordinateTables(block)
	if err != nil {
		return nil, nil, err
	}
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
	var unsupported []string
	masked := maskLua(block)
	for _, indexes := range objectiveCallRE.FindAllStringIndex(masked, -1) {
		open := indexes[1] - 1
		end, err := luaTableEnd(block, open)
		if err != nil {
			unsupported = append(unsupported, err.Error())
			continue
		}
		objBlock := topFieldBlock(block[open+1 : end])
		raw := topFields(objBlock)
		if explicit, exists := raw["questID"]; exists {
			owner, err := strconv.Atoi(explicit)
			if err != nil || owner <= 0 {
				unsupported = append(unsupported, "unsupported objective questID = "+explicit)
				continue
			}
			if owner != id {
				continue
			}
		} else if owner, found := enclosingQuestID(block, indexes[0]); found && owner != id {
			continue
		}
		label := ""
		line := strings.SplitN(block[open+1:end], "\n", 2)[0]
		if comment := strings.Index(line, "--"); comment >= 0 && strings.TrimSpace(line[:comment]) == "" {
			label = strings.TrimSpace(line[comment+2:])
		}
		refs, err := questProviders(objBlock)
		if err != nil {
			unsupported = append(unsupported, err.Error())
		}
		for _, name := range []string{"provider", "providers"} {
			expression, exists := raw[name]
			if !exists {
				continue
			}
			rows := []string{expression}
			if name == "providers" {
				var ok bool
				rows, ok = luaListExpressions(expression)
				if !ok {
					unsupported = append(unsupported, fmt.Sprintf("objective %s = %s: unsupported provider list", name, expression))
					continue
				}
			}
			for _, row := range rows {
				values, ok := literalLuaList(row, ctx.constants)
				if !ok || len(values) != 2 {
					unsupported = append(unsupported, fmt.Sprintf("objective %s = %s: unsupported provider expression", name, row))
					continue
				}
				kind, kindOK := values[0].(string)
				providerID, idOK := values[1].(int)
				if !kindOK || !idOK || providerID <= 0 || (kind != "i" && kind != "o" && kind != "n") {
					unsupported = append(unsupported, fmt.Sprintf("objective %s = %s: unsupported provider type or ID", name, row))
				} else {
					refs = append(refs, providerRef{kind: attProviderKind(kind), id: providerID})
				}
			}
		}
		for _, name := range []string{"cr", "crs"} {
			expression, exists := raw[name]
			if !exists {
				continue
			}
			values := []any{}
			if name == "cr" {
				value, err := strconv.Atoi(expression)
				if err != nil {
					unsupported = append(unsupported, "objective cr = "+expression)
					continue
				}
				values = append(values, value)
			} else {
				var ok bool
				values, ok = literalLuaList(expression, ctx.constants)
				if !ok {
					unsupported = append(unsupported, "objective crs = "+expression)
					continue
				}
			}
			for _, value := range values {
				providerID, ok := value.(int)
				if ok && providerID > 0 {
					refs = append(refs, providerRef{kind: "npc", id: providerID})
				} else {
					unsupported = append(unsupported, "objective "+name+" = "+expression)
				}
			}
		}
		var spawns []Spawn
		coordRows, err := coordinateTables(objBlock)
		if err != nil {
			unsupported = append(unsupported, err.Error())
		} else {
			var badMap string
			spawns, badMap, err = resolveCoordRows(path, id, coordRows, ctx)
			if err != nil {
				unsupported = append(unsupported, err.Error())
			}
			if badMap != "" {
				unsupported = append(unsupported, badMap)
			}
		}
		objectiveSeen := map[string]bool{}
		for _, ref := range refs {
			if objectiveSeen[providerKey(ref)] {
				continue
			}
			objectiveSeen[providerKey(ref)] = true
			group := ref.kind
			if group == "npc" {
				group = "creature"
			}
			entry := map[string]any{"id": ref.id}
			if label != "" {
				entry["text"] = label
			}
			objectives[group] = appendSliceObjective(objectives[group], entry)
			place := map[string]any{"role": "objective", "type": ref.kind, "id": ref.id, "spawns": spawnsToAny(spawns)}
			if ref.kind == "item" {
				place["itemId"] = ref.id
			}
			if label != "" {
				place["name"] = label
			}
			places = addPlace(places, seen, place)
		}
	}
	if len(unsupported) > 0 {
		return objectives, places, fmt.Errorf("%s quest %d: %s", path, id, strings.Join(unsupported, "; "))
	}
	return objectives, places, nil
}

func luaListExpressions(expression string) ([]string, bool) {
	expression = strings.TrimSpace(expression)
	if !strings.HasPrefix(expression, "{") || !strings.HasSuffix(expression, "}") {
		return nil, false
	}
	content := expression[1 : len(expression)-1]
	masked := maskLua(content)
	depth := 0
	start := 0
	var rows []string
	for index := 0; index < len(masked); index++ {
		switch masked[index] {
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		}
		if masked[index] == ',' && depth == 0 {
			if value := strings.TrimSpace(content[start:index]); value != "" {
				rows = append(rows, value)
			}
			start = index + 1
		}
		if depth < 0 {
			return nil, false
		}
	}
	if depth != 0 {
		return nil, false
	}
	if value := strings.TrimSpace(content[start:]); value != "" {
		rows = append(rows, value)
	}
	return rows, true
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
	for _, key := range []string{"declarations", "unresolved"} {
		incomingRows, _ := incoming[key].([]any)
		if len(incomingRows) > 0 {
			existingRows, _ := existing[key].([]any)
			existing[key] = append(existingRows, incomingRows...)
		}
	}
	if inFields == nil {
		return
	}
	if exFields == nil {
		existing["fields"] = inFields
	} else {
		keys := make([]string, 0, len(inFields))
		for key := range inFields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := inFields[key]
			if existingValue, exists := exFields[key]; exists && !reflect.DeepEqual(existingValue, value) {
				conflicts, _ := existing["unresolved"].([]any)
				conflicts = append(conflicts, map[string]any{"field": key, "reason": "conflicting declarations", "values": []any{existingValue, value}})
				existing["unresolved"] = conflicts
				delete(exFields, key)
			} else if !exists && !hasFieldConflict(existing, key) {
				exFields[key] = value
				if paths, ok := incoming["fieldPaths"].(map[string]string); ok {
					existing["fieldPaths"].(map[string]string)[key] = paths[key]
				}
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

func enclosingQuestID(block string, position int) (int, bool) {
	masked := maskLua(block)
	owner := 0
	found := false
	for _, match := range questDeclarationRE.FindAllStringSubmatchIndex(masked[:position], -1) {
		open := strings.Index(masked[match[0]:match[1]], "(") + match[0]
		depth := 1
		close := open + 1
		for ; close < len(masked); close++ {
			if masked[close] == '(' {
				depth++
			}
			if masked[close] == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if close > position {
			owner, _ = strconv.Atoi(masked[match[2]:match[3]])
			found = true
		}
	}
	return owner, found
}

func mergeObjectiveRoute(byID map[int]map[string]any, route map[string]any) {
	id := route["id"].(int)
	target, exists := byID[id]
	if !exists {
		target = map[string]any{"id": id, "fields": map[string]any{}, "fieldPaths": map[string]string{}}
		byID[id] = target
	}
	fields := target["fields"].(map[string]any)
	routeFields := route["fields"].(map[string]any)
	incoming, _ := routeFields["objectives"].(map[string]any)
	if len(incoming) > 0 && !hasFieldConflict(target, "objectives") {
		groups, _ := fields["objectives"].(map[string]any)
		if groups == nil {
			groups = map[string]any{}
		}
		for kind, raw := range incoming {
			entries, _ := groups[kind].([]any)
			for _, entry := range raw.([]any) {
				duplicate := false
				for _, existing := range entries {
					if reflect.DeepEqual(existing, entry) {
						duplicate = true
						break
					}
				}
				if !duplicate {
					entries = append(entries, entry)
				}
			}
			groups[kind] = entries
		}
		fields["objectives"] = groups
		paths := target["fieldPaths"].(map[string]string)
		if paths["objectives"] == "" {
			paths["objectives"] = route["fieldPaths"].(map[string]string)["objectives"]
		}
	}
	for _, key := range []string{"declarations", "unresolved"} {
		incoming, _ := route[key].([]any)
		existing, _ := target[key].([]any)
		if len(incoming) > 0 {
			target[key] = append(existing, incoming...)
		}
	}
	mergePlaceList(target, route)
}
