// Package att reads the location-bearing subset of ATT's Forever Lua data.
package att

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Spawn struct {
	Zone int     `json:"zone"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

type Location struct {
	QuestID int
	Kind    string
	NPCID   int
	Spawns  []Spawn
}

type Report struct {
	QuestID int    `json:"questId"`
	NPCID   int    `json:"npcId"`
	File    string `json:"file"`
	Reason  string `json:"reason"`
}

type Result struct {
	Locations []Location
	Unmapped  []Report
}

var (
	questCallRE     = regexp.MustCompile(`\bq\s*\(\s*(\d+)\s*,\s*\{`)
	mapConstRE      = regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]*)\s*=\s*(\d+)\s*[,;]?`)
	mapRootRE       = regexp.MustCompile(`\b(?:maproot|m)\s*\(\s*(?:MAP\.[A-Z][A-Z0-9_]*\s*,\s*)?MAP\.([A-Z][A-Z0-9_]*)\s*,?\s*\{`)
	constantTableRE = regexp.MustCompile(`(?m)\b([A-Z][A-Z0-9_]+)\s*=\s*\{`)
	areaFieldRE     = regexp.MustCompile(`(?:\["zone-text-areas"\]|\bzone-text-areas)\s*=\s*\{`)
	identifierRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)
	providerRE      = regexp.MustCompile(`(?:\["qg"\]|\bqg)\s*=\s*(\d+)`)
	providersRE     = regexp.MustCompile(`(?:\["qgs"\]|\bqgs)\s*=\s*\{([^}]*)\}`)
	coordFieldRE    = regexp.MustCompile(`(?:\["coords?"\]|\bcoords?)\s*=\s*`)
	pointRE         = regexp.MustCompile(`^\s*(-?\d+(?:\.\d*)?)\s*,\s*(-?\d+(?:\.\d*)?)\s*,\s*(?:(?:MAP\.)?([A-Z][A-Z0-9_]*)|(\d+))\s*$`)
	idRE            = regexp.MustCompile(`\d+`)
)

// Read loads the current ATT Forever database. It deliberately parses quest
// declarations and literal quest-giver coordinates only, not arbitrary Lua.
func Read(root string) (Result, error) {
	const dataRoot = ".contrib/.db/forever"
	mapPath := filepath.Join(root, dataRoot, ".config/constants/maps.lua")
	mapBody, err := os.ReadFile(mapPath)
	if err != nil {
		return Result{}, fmt.Errorf("ATT map constants %s: %w", mapPath, err)
	}
	maps := map[string]int{}
	for _, row := range mapConstRE.FindAllSubmatch(mapBody, -1) {
		id, err := strconv.Atoi(string(row[2]))
		if err != nil {
			return Result{}, fmt.Errorf("ATT map constant %s: %w", row[1], err)
		}
		maps[string(row[1])] = id
	}
	if len(maps) == 0 {
		return Result{}, fmt.Errorf("ATT map constants contain no supported assignments")
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
		return Result{}, fmt.Errorf("walk ATT Forever data: %w", err)
	}
	sort.Strings(files)
	mapAreas := map[int]int{}
	for mapID, areaID := range classicMapAreas {
		mapAreas[mapID] = areaID
	}
	for _, path := range files {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return Result{}, readErr
		}
		if err := readMapAreas(path, body, maps, mapAreas); err != nil {
			return Result{}, err
		}
	}
	coordinateConstants := map[string][]Spawn{}
	for _, path := range files {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return Result{}, readErr
		}
		if err := readCoordinateConstants(path, body, maps, mapAreas, coordinateConstants); err != nil {
			return Result{}, err
		}
	}
	var result Result
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			return Result{}, err
		}
		locations, reports, err := readFile(path, body, maps, mapAreas, coordinateConstants)
		if err != nil {
			return Result{}, err
		}
		result.Locations = append(result.Locations, locations...)
		result.Unmapped = append(result.Unmapped, reports...)
	}
	sort.Slice(result.Locations, func(i, j int) bool {
		a, b := result.Locations[i], result.Locations[j]
		if a.QuestID != b.QuestID {
			return a.QuestID < b.QuestID
		}
		return a.NPCID < b.NPCID
	})
	sort.Slice(result.Unmapped, func(i, j int) bool {
		a, b := result.Unmapped[i], result.Unmapped[j]
		if a.QuestID != b.QuestID {
			return a.QuestID < b.QuestID
		}
		if a.NPCID != b.NPCID {
			return a.NPCID < b.NPCID
		}
		return a.File < b.File
	})
	return result, nil
}

// Classic UiMapID to AreaTableID crosswalk for the public spawn tuple format.
var classicMapAreas = map[int]int{
	1411: 14, 1412: 215, 1413: 17, 1416: 36, 1417: 45,
	1418: 3, 1419: 4, 1420: 85, 1421: 130, 1422: 28, 1423: 139, 1424: 267,
	1425: 47, 1426: 1, 1427: 51, 1428: 46, 1429: 12, 1430: 41, 1431: 10,
	1432: 38, 1433: 44, 1434: 33, 1435: 8, 1436: 40, 1437: 11, 1438: 141,
	1439: 148, 1440: 331, 1441: 400, 1442: 406, 1443: 405, 1444: 357,
	1445: 15, 1446: 440, 1447: 16, 1448: 361, 1449: 490, 1450: 493,
	1451: 1377, 1452: 618, 1453: 1519, 1454: 1637, 1455: 1537, 1456: 1638,
	1457: 1657, 1458: 1497,
}

func readFile(path string, body []byte, maps map[string]int, mapAreas map[int]int, coordinateConstants map[string][]Spawn) ([]Location, []Report, error) {
	text := string(body)
	masked := maskLua(text)
	matches := questCallRE.FindAllStringSubmatchIndex(masked, -1)
	var locations []Location
	var reports []Report
	for _, match := range matches {
		id, _ := strconv.Atoi(text[match[2]:match[3]])
		open := match[1] - 1 // opening table brace captured by the q call pattern
		end, err := luaTableEnd(text, open)
		if err != nil {
			return nil, nil, fmt.Errorf("%s quest %d: %w", path, id, err)
		}
		block := text[open+1 : end]
		if !strings.Contains(block, "qg") && !strings.Contains(block, "coords") && !strings.Contains(block, "coord") {
			continue
		}
		providerIDs := []int{}
		blockDepth := braceDepths(masked[open+1 : end])
		for _, row := range topLevelSubmatches(providerRE, block, blockDepth) {
			npc, _ := strconv.Atoi(row[1])
			providerIDs = append(providerIDs, npc)
		}
		for _, row := range topLevelSubmatches(providersRE, block, blockDepth) {
			for _, idText := range idRE.FindAllString(row[1], -1) {
				npc, _ := strconv.Atoi(idText)
				providerIDs = append(providerIDs, npc)
			}
		}
		providerIDs = unique(providerIDs)
		if len(providerIDs) == 0 {
			continue
		}
		coordRows, err := coordinateTables(block)
		if err != nil {
			return nil, nil, fmt.Errorf("%s quest %d: %w", path, id, err)
		}
		var spawns []Spawn
		badMap := ""
		for _, coordBody := range coordRows {
			coordBody = strings.TrimSpace(coordBody)
			if isIdentifier(coordBody) {
				resolved, exists := coordinateConstants[coordBody]
				if !exists {
					return nil, nil, fmt.Errorf("%s quest %d: unsupported ATT coordinate reference %s", path, id, coordBody)
				} else {
					spawns = append(spawns, resolved...)
				}
				continue
			}
			if strings.Contains(coordBody, "{") || strings.Contains(coordBody, "}") {
				for _, point := range nestedTables(coordBody) {
					spawn, mapName, parseErr := parsePoint(point, maps, mapAreas)
					if parseErr != nil {
						return nil, nil, fmt.Errorf("%s quest %d coordinate %q: %w", path, id, point, parseErr)
					}
					if mapName != "" {
						badMap = "unmapped ATT map constant " + mapName
						continue
					}
					spawns = append(spawns, spawn)
				}
			} else {
				spawn, mapName, parseErr := parsePoint(coordBody, maps, mapAreas)
				if parseErr != nil {
					return nil, nil, fmt.Errorf("%s quest %d coordinate %q: %w", path, id, coordBody, parseErr)
				}
				if mapName != "" {
					badMap = "unmapped ATT map constant " + mapName
					continue
				}
				spawns = append(spawns, spawn)
			}
		}
		for _, npc := range providerIDs {
			if badMap != "" {
				reports = append(reports, Report{QuestID: id, NPCID: npc, File: filepath.Base(path), Reason: badMap})
				continue
			}
			if len(spawns) == 0 {
				continue
			}
			locations = append(locations, Location{QuestID: id, Kind: "npc", NPCID: npc, Spawns: dedupe(spawns)})
		}
	}
	return locations, reports, nil
}

func readCoordinateConstants(path string, body []byte, maps map[string]int, mapAreas map[int]int, constants map[string][]Spawn) error {
	text := string(body)
	masked := maskLua(text)
	for _, match := range constantTableRE.FindAllStringSubmatchIndex(masked, -1) {
		name := text[match[2]:match[3]]
		open := match[1] - 1
		end, err := luaTableEnd(text, open)
		if err != nil {
			return fmt.Errorf("%s constant %s: %w", path, name, err)
		}
		content := strings.TrimSpace(maskLua(text[open+1 : end]))
		var points []string
		if strings.Contains(content, "{") {
			points = nestedTables(content)
		} else {
			points = []string{content}
		}
		var spawns []Spawn
		valid := len(points) > 0
		for _, point := range points {
			spawn, mapName, parseErr := parsePoint(point, maps, mapAreas)
			if parseErr != nil || mapName != "" {
				valid = false
				break
			}
			spawns = append(spawns, spawn)
		}
		if valid {
			constants[name] = dedupe(spawns)
		}
	}
	return nil
}

func isIdentifier(value string) bool {
	if value == "" || !((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z') || value[0] == '_') {
		return false
	}
	for _, c := range value[1:] {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// luaTableEnd returns the byte offset of the matching closing brace, ignoring
// braces inside Lua strings and comments.
func luaTableEnd(text string, open int) (int, error) {
	depth, quote, lineComment, longComment := 0, byte(0), false, false
	for i := open; i < len(text); i++ {
		c := text[i]
		if lineComment {
			if c == '\n' {
				lineComment = false
			}
			continue
		}
		if longComment {
			if i+1 < len(text) && c == ']' && text[i+1] == ']' {
				longComment = false
				i++
			}
			continue
		}
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '-' && i+1 < len(text) && text[i+1] == '-' {
			if i+3 < len(text) && text[i+2] == '[' && text[i+3] == '[' {
				longComment = true
				i += 3
			} else {
				lineComment = true
				i++
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated quest table")
}

func parsePoint(text string, maps map[string]int, mapAreas map[int]int) (Spawn, string, error) {
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), ","))
	match := pointRE.FindStringSubmatch(text)
	if match == nil {
		return Spawn{}, "", fmt.Errorf("unsupported coordinate syntax %q", strings.TrimSpace(text))
	}
	x, _ := strconv.ParseFloat(match[1], 64)
	y, _ := strconv.ParseFloat(match[2], 64)
	zone := 0
	if match[3] != "" {
		mapID, ok := maps[match[3]]
		if !ok {
			return Spawn{}, match[3], nil
		}
		// ATT MAP ids are UiMapIDs. This crosswalk preserves the public
		// AreaTable-based coordinate format for locations currently consumed.
		zone, ok = mapAreas[mapID]
		if !ok {
			return Spawn{}, match[3], nil
		}
	} else {
		var err error
		zone, err = strconv.Atoi(match[4])
		if err != nil {
			return Spawn{}, "", err
		}
	}
	if x < 0 || x > 100 || y < 0 || y > 100 {
		return Spawn{}, "", fmt.Errorf("coordinate outside 0..100: %q", text)
	}
	return Spawn{Zone: zone, X: x, Y: y}, "", nil
}

func coordinateTables(block string) ([]string, error) {
	var out []string
	masked := maskLua(block)
	depth := braceDepths(masked)
	for _, field := range coordFieldRE.FindAllStringIndex(block, -1) {
		if depth[field[0]] != 0 {
			continue
		}
		open := field[1]
		for open < len(block) && (block[open] == ' ' || block[open] == '\t' || block[open] == '\r' || block[open] == '\n') {
			open++
		}
		if open >= len(block) || block[open] != '{' {
			if name := identifierRE.FindString(block[open:]); name != "" && (open+len(name) == len(block) || strings.ContainsRune(" \t\r\n,}", rune(block[open+len(name)]))) {
				out = append(out, name)
				continue
			}
			return nil, fmt.Errorf("unsupported coordinate syntax %q", strings.TrimSpace(block[open:]))
		}
		end, err := luaTableEnd(block, open)
		if err != nil {
			return nil, err
		}
		out = append(out, masked[open+1:end])
	}
	return out, nil
}

func topLevelSubmatches(re *regexp.Regexp, text string, depths []int) [][]string {
	var rows [][]string
	for _, indexes := range re.FindAllStringSubmatchIndex(text, -1) {
		if indexes[0] >= len(depths) || depths[indexes[0]] != 0 {
			continue
		}
		row := make([]string, len(indexes)/2)
		for i := range row {
			if indexes[2*i] >= 0 {
				row[i] = text[indexes[2*i]:indexes[2*i+1]]
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func maskLua(text string) string {
	masked := []byte(text)
	quote, lineComment, longComment := byte(0), false, false
	for i := 0; i < len(masked); i++ {
		c := masked[i]
		if lineComment {
			if c == '\n' {
				lineComment = false
			} else {
				masked[i] = ' '
			}
			continue
		}
		if longComment {
			if c == ']' && i+1 < len(masked) && masked[i+1] == ']' {
				masked[i], masked[i+1] = ' ', ' '
				i++
				longComment = false
			} else if c != '\n' {
				masked[i] = ' '
			}
			continue
		}
		if quote != 0 {
			if c != '\n' {
				masked[i] = ' '
			}
			if c == '\\' && i+1 < len(masked) {
				i++
				if masked[i] != '\n' {
					masked[i] = ' '
				}
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '-' && i+1 < len(masked) && masked[i+1] == '-' {
			masked[i], masked[i+1] = ' ', ' '
			if i+3 < len(masked) && masked[i+2] == '[' && masked[i+3] == '[' {
				masked[i+2], masked[i+3] = ' ', ' '
				i += 3
				longComment = true
			} else {
				i++
				lineComment = true
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			masked[i] = ' '
		}
	}
	return string(masked)
}

func braceDepths(text string) []int {
	depths := make([]int, len(text))
	depth := 0
	for i := range text {
		depths[i] = depth
		if text[i] == '{' {
			depth++
		}
		if text[i] == '}' {
			depth--
		}
	}
	return depths
}

func readMapAreas(path string, body []byte, maps map[string]int, areas map[int]int) error {
	text := string(body)
	masked := maskLua(text)
	values := map[int]map[int]bool{}
	for _, match := range mapRootRE.FindAllStringSubmatchIndex(masked, -1) {
		constant := text[match[2]:match[3]]
		mapID, exists := maps[constant]
		if !exists {
			continue
		}
		open := match[1] - 1
		end, err := luaTableEnd(text, open)
		if err != nil {
			return fmt.Errorf("%s map %s: %w", path, constant, err)
		}
		bodyText := text[open+1 : end]
		bodyMasked := masked[open+1 : end]
		depth := braceDepths(bodyMasked)
		for _, field := range areaFieldRE.FindAllStringIndex(bodyText, -1) {
			if depth[field[0]] != 0 {
				continue
			}
			listOpen := field[1] - 1
			for listOpen < len(bodyText) && (bodyText[listOpen] == ' ' || bodyText[listOpen] == '\t' || bodyText[listOpen] == '\r' || bodyText[listOpen] == '\n') {
				listOpen++
			}
			if listOpen >= len(bodyText) || bodyText[listOpen] != '{' {
				continue
			}
			listEnd, err := luaTableEnd(bodyText, listOpen)
			if err != nil {
				return fmt.Errorf("%s map %s zone-text-areas: %w", path, constant, err)
			}
			matchArea := idRE.FindString(bodyMasked[listOpen+1 : listEnd])
			if matchArea == "" {
				continue
			}
			areaID, _ := strconv.Atoi(matchArea)
			if values[mapID] == nil {
				values[mapID] = map[int]bool{}
			}
			values[mapID][areaID] = true
			break
		}
	}
	for mapID, candidates := range values {
		if len(candidates) != 1 {
			continue
		}
		for areaID := range candidates {
			areas[mapID] = areaID
		}
	}
	return nil
}

func nestedTables(text string) []string {
	var out []string
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		end, err := luaTableEnd(text, i)
		if err != nil {
			return out
		}
		out = append(out, text[i+1:end])
		i = end
	}
	return out
}

func unique(ids []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func dedupe(spawns []Spawn) []Spawn {
	seen := map[Spawn]bool{}
	out := make([]Spawn, 0, len(spawns))
	for _, spawn := range spawns {
		if !seen[spawn] {
			seen[spawn] = true
			out = append(out, spawn)
		}
	}
	return out
}
