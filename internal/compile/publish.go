package compile

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

var knownFields = map[string]bool{
	"name": true, "startedBy": true, "finishedBy": true, "requiredLevel": true,
	"questLevel": true, "requiredRaces": true, "requiredClasses": true, "objectivesText": true,
	"triggerEnd": true, "objectives": true, "sourceItemId": true, "preQuestGroup": true,
	"preQuestSingle": true, "childQuests": true, "inGroupWith": true, "exclusiveTo": true,
	"zoneOrSort": true, "requiredSkill": true, "requiredMinRep": true, "requiredMaxRep": true,
	"requiredSourceItems": true, "nextQuestInChain": true, "questFlags": true, "specialFlags": true,
	"parentQuest": true, "reputationReward": true, "breadcrumbForQuestId": true, "breadcrumbs": true,
	"extraObjectives": true, "requiredSpell": true, "requiredSpecialization": true,
	"requiredMaxLevel": true, "availableUntilCompleted": true, "availableStartingWith": true,
	"requiredRanks": true, "disabledByQuest": true,
}

var placeRoles = map[string]bool{
	"available": true, "turnIn": true, "objective": true, "trigger": true, "extra": true,
}

func publishQuest(id int, row map[string]any) (map[string]any, error) {
	fields, _ := row["fields"].(map[string]any)
	if fields == nil {
		fields = map[string]any{}
	}
	for name, value := range fields {
		if !knownFields[name] && !emptyValue(value) {
			return nil, fmt.Errorf("quest %d: unmapped field %s", id, name)
		}
	}
	quest := map[string]any{"id": id}
	if text, ok := stringField(fields["name"]); ok {
		quest["name"] = text
	}
	if level, ok, err := optionalInt(fields["questLevel"]); err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	} else if ok && level != 0 {
		quest["questLevel"] = level
	}
	if level, ok, err := optionalInt(fields["requiredLevel"]); err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	} else if ok && level != 0 {
		quest["requiredLevel"] = level
	}
	races, err := decodeMask(fields["requiredRaces"], raceBits, "race")
	if err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	}
	classes, err := decodeMask(fields["requiredClasses"], classBits, "class")
	if err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	}
	if len(races) > 0 {
		quest["requiredRaces"] = races
	}
	if len(classes) > 0 {
		quest["requiredClasses"] = classes
	}
	quest["faction"] = factionFor(races)
	copyValue(quest, "objectivesText", fields["objectivesText"])
	copyValue(quest, "objectives", fields["objectives"])
	copyValue(quest, "triggerEnd", fields["triggerEnd"])
	copyValue(quest, "extraObjectives", fields["extraObjectives"])
	if _, err := copyOptionalInt(quest, "sourceItemId", fields["sourceItemId"]); err != nil {
		return nil, err
	}
	if err := copyIDList(quest, "preQuestGroup", fields["preQuestGroup"]); err != nil {
		return nil, err
	}
	if err := copyIDList(quest, "preQuestSingle", fields["preQuestSingle"]); err != nil {
		return nil, err
	}
	if err := copyIDList(quest, "childQuests", fields["childQuests"]); err != nil {
		return nil, err
	}
	if err := copyIDList(quest, "inGroupWith", fields["inGroupWith"]); err != nil {
		return nil, err
	}
	if err := copyIDList(quest, "exclusiveTo", fields["exclusiveTo"]); err != nil {
		return nil, err
	}
	if err := copyIDList(quest, "requiredSourceItems", fields["requiredSourceItems"]); err != nil {
		return nil, err
	}
	if err := copyIDList(quest, "breadcrumbs", fields["breadcrumbs"]); err != nil {
		return nil, err
	}
	for _, name := range []string{
		"nextQuestInChain", "parentQuest", "breadcrumbForQuestId", "requiredSpell",
		"requiredSpecialization", "requiredMaxLevel", "availableUntilCompleted",
		"availableStartingWith", "disabledByQuest",
	} {
		if _, err := copyOptionalInt(quest, name, fields[name]); err != nil {
			return nil, fmt.Errorf("quest %d: %w", id, err)
		}
	}
	zone, err := copyOptionalInt(quest, "zoneOrSort", fields["zoneOrSort"])
	if err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	}
	skill, err := copyPair(quest, "requiredSkill", fields["requiredSkill"], "skillId", "value")
	if err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	}
	if _, err := copyPair(quest, "requiredMinRep", fields["requiredMinRep"], "factionId", "value"); err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	}
	if _, err := copyPair(quest, "requiredMaxRep", fields["requiredMaxRep"], "factionId", "value"); err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	}
	ranks, err := copyPairs(quest, "requiredRanks", fields["requiredRanks"], "skillId", "value")
	if err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	}
	if _, err := copyPairs(quest, "reputationReward", fields["reputationReward"], "factionId", "value"); err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	}
	questType, err := buildQuestType(fields, zone, skill, ranks)
	if err != nil {
		return nil, fmt.Errorf("quest %d: %w", id, err)
	}
	quest["questType"] = questType
	started, err := copyProviders(row["startedBy"])
	if err != nil {
		return nil, fmt.Errorf("quest %d: startedBy: %w", id, err)
	}
	finished, err := copyProviders(row["finishedBy"])
	if err != nil {
		return nil, fmt.Errorf("quest %d: finishedBy: %w", id, err)
	}
	if len(started) > 0 {
		quest["startedBy"] = started
	}
	if len(finished) > 0 {
		quest["finishedBy"] = finished
	}
	places, err := copyPlaces(row["places"])
	if err != nil {
		return nil, fmt.Errorf("quest %d: places: %w", id, err)
	}
	if len(places) > 0 {
		quest["places"] = places
	}
	return quest, nil
}

func buildQuestType(fields map[string]any, zone int, skill []int, ranks [][]int) (map[string]any, error) {
	questFlags, _, err := optionalInt(fields["questFlags"])
	if err != nil {
		return nil, err
	}
	specialFlags, _, err := optionalInt(fields["specialFlags"])
	if err != nil {
		return nil, err
	}
	if questFlags < 0 || specialFlags < 0 {
		return nil, fmt.Errorf("negative flag value")
	}
	qMask := uint64(questFlags)
	sMask := uint64(specialFlags)
	if qMask&^knownQuestFlags != 0 {
		return nil, fmt.Errorf("unknown questFlags bits: %d", qMask&^knownQuestFlags)
	}
	const knownSpecialFlags = questSpecialRepeatable | questSpecialEvent
	if sMask&^uint64(knownSpecialFlags) != 0 {
		return nil, fmt.Errorf("unknown specialFlags bits: %d", sMask&^uint64(knownSpecialFlags))
	}
	questType := map[string]any{
		"stayAlive":     qMask&questFlagStayAlive != 0,
		"partyAccept":   qMask&questFlagPartyAccept != 0,
		"exploration":   qMask&questFlagExploration != 0,
		"sharable":      qMask&questFlagSharable != 0,
		"epic":          qMask&questFlagEpic != 0,
		"raid":          qMask&questFlagRaid != 0,
		"hiddenRewards": qMask&questFlagHiddenRewards != 0,
		"autoRewarded":  qMask&questFlagAutoRewarded != 0,
		"daily":         qMask&questFlagDaily != 0,
		"weekly":        qMask&questFlagWeekly != 0,
		"monthly":       qMask&questFlagMonthly != 0,
		"repeatable":    sMask&questSpecialRepeatable != 0,
		"event":         sMask&questSpecialEvent != 0,
		"dungeon":       false,
		"battleground":  false,
	}
	if zone < 0 {
		name, ok := sortKeys[zone]
		if !ok {
			return nil, fmt.Errorf("unknown quest sort %d", zone)
		}
		questType["sort"] = name
		if name == "BATTLEGROUNDS" {
			questType["battleground"] = true
		}
	}
	if zone == areaAlteracValley || zone == areaWarsongGulch || zone == areaArathiBasin {
		questType["battleground"] = true
	} else if zone > 0 && dungeonAreas[zone] {
		questType["dungeon"] = true
	}
	profession, err := professionName(skill, ranks)
	if err != nil {
		return nil, err
	}
	if profession != "" {
		questType["profession"] = profession
	}
	return questType, nil
}

func professionName(skill []int, ranks [][]int) (string, error) {
	var id int
	switch {
	case len(skill) == 2 && skill[0] != 0:
		id = skill[0]
	case len(ranks) > 0 && len(ranks[0]) == 2 && ranks[0][0] != 0:
		id = ranks[0][0]
	default:
		return "", nil
	}
	for _, candidate := range append([]int{id}, rankIDs(ranks)...) {
		if candidate == 0 {
			continue
		}
		if _, ok := professionKeys[candidate]; !ok {
			return "", fmt.Errorf("unknown profession skill %d", candidate)
		}
	}
	return professionKeys[id], nil
}

func rankIDs(ranks [][]int) []int {
	ids := make([]int, 0, len(ranks))
	for _, rank := range ranks {
		if len(rank) == 2 {
			ids = append(ids, rank[0])
		}
	}
	return ids
}

func factionFor(races []int) string {
	if len(races) == 0 {
		return "Both"
	}
	alliance, horde := true, true
	for _, id := range races {
		if !allianceRaces[id] {
			alliance = false
		}
		if !hordeRaces[id] {
			horde = false
		}
	}
	switch {
	case alliance && !horde:
		return "Alliance"
	case horde && !alliance:
		return "Horde"
	default:
		return "Both"
	}
}

func decodeMask(value any, bits []bitID, label string) ([]int, error) {
	number, present, err := optionalInt(value)
	if err != nil {
		return nil, err
	}
	if !present || number == 0 {
		return nil, nil
	}
	if number < 0 {
		return nil, fmt.Errorf("negative %s mask", label)
	}
	mask := uint64(number)
	var known uint64
	for _, bit := range bits {
		known |= bit.bit
	}
	if mask&^known != 0 {
		return nil, fmt.Errorf("unknown %s mask bits: %d", label, mask&^known)
	}
	var ids []int
	for _, bit := range bits {
		if mask&bit.bit != 0 {
			ids = append(ids, bit.id)
		}
	}
	return ids, nil
}

func copyValue(quest map[string]any, name string, value any) {
	if emptyValue(value) {
		return
	}
	quest[name] = normalize(value)
}

func copyOptionalInt(quest map[string]any, name string, value any) (int, error) {
	number, present, err := optionalInt(value)
	if err != nil || !present || number == 0 {
		return 0, err
	}
	quest[name] = number
	return number, nil
}

func copyIDList(quest map[string]any, name string, value any) error {
	if emptyValue(value) {
		return nil
	}
	ids, err := intList(value)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(ids) > 0 {
		quest[name] = ids
	}
	return nil
}

func copyPair(quest map[string]any, name string, value any, first, second string) ([]int, error) {
	if emptyValue(value) {
		return nil, nil
	}
	row, ok := value.([]any)
	if !ok || len(row) != 2 {
		return nil, fmt.Errorf("%s: expected a pair", name)
	}
	a, err := asInt(row[0])
	if err != nil {
		return nil, err
	}
	b, err := asInt(row[1])
	if err != nil {
		return nil, err
	}
	if a == 0 && b == 0 {
		return nil, nil
	}
	quest[name] = map[string]any{first: a, second: b}
	return []int{a, b}, nil
}

func copyPairs(quest map[string]any, name string, value any, first, second string) ([][]int, error) {
	if emptyValue(value) {
		return nil, nil
	}
	rows, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected a list", name)
	}
	var out []any
	var pairs [][]int
	for _, row := range rows {
		pair, ok := row.([]any)
		if !ok || len(pair) != 2 {
			return nil, fmt.Errorf("%s: expected pairs", name)
		}
		a, err := asInt(pair[0])
		if err != nil {
			return nil, err
		}
		b, err := asInt(pair[1])
		if err != nil {
			return nil, err
		}
		if a == 0 && b == 0 {
			continue
		}
		out = append(out, map[string]any{first: a, second: b})
		pairs = append(pairs, []int{a, b})
	}
	if len(out) > 0 {
		quest[name] = out
	}
	return pairs, nil
}

func copyProviders(value any) ([]any, error) {
	if emptyValue(value) {
		return nil, nil
	}
	rows, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a list")
	}
	var out []any
	for _, row := range rows {
		provider, ok := row.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected an object")
		}
		copied, err := copyPlace(provider, false)
		if err != nil {
			return nil, err
		}
		out = append(out, copied)
	}
	return out, nil
}

func copyPlaces(value any) ([]any, error) {
	if emptyValue(value) {
		return nil, nil
	}
	rows, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a list")
	}
	var out []any
	for _, row := range rows {
		place, ok := row.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected an object")
		}
		role, _ := place["role"].(string)
		if !placeRoles[role] {
			return nil, fmt.Errorf("unknown role %q", role)
		}
		copied, err := copyPlace(place, true)
		if err != nil {
			return nil, err
		}
		copied["role"] = role
		out = append(out, copied)
	}
	return out, nil
}

func copyPlace(row map[string]any, requireSpawns bool) (map[string]any, error) {
	kind, _ := row["type"].(string)
	if kind == "" {
		return nil, fmt.Errorf("missing type")
	}
	id, err := asInt(row["id"])
	if err != nil {
		return nil, err
	}
	copied := map[string]any{"type": kind, "id": id}
	if name, ok := row["name"].(string); ok && name != "" {
		copied["name"] = name
	}
	if itemID, present, err := optionalInt(row["itemId"]); err != nil {
		return nil, err
	} else if present && itemID != 0 {
		copied["itemId"] = itemID
	}
	spawns, err := copySpawns(row["spawns"])
	if err != nil {
		return nil, err
	}
	if requireSpawns || spawns != nil {
		if spawns == nil {
			spawns = []any{}
		}
		copied["spawns"] = spawns
	}
	return copied, nil
}

func copySpawns(value any) ([]any, error) {
	if value == nil {
		return nil, nil
	}
	rows, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("spawns must be a list")
	}
	var out []any
	for _, row := range rows {
		point, ok := row.([]any)
		if !ok || len(point) != 3 {
			return nil, fmt.Errorf("spawn must be [zoneId, x, y]")
		}
		zone, err := asInt(point[0])
		if err != nil {
			return nil, err
		}
		x, err := asFloat(point[1])
		if err != nil {
			return nil, err
		}
		y, err := asFloat(point[2])
		if err != nil {
			return nil, err
		}
		out = append(out, []any{zone, x, y})
	}
	if out == nil {
		out = []any{}
	}
	return out, nil
}

func chainEntry(quest map[string]any) map[string]any {
	entry := map[string]any{}
	for _, name := range []string{"preQuestGroup", "preQuestSingle", "exclusiveTo", "childQuests", "inGroupWith", "breadcrumbs"} {
		if value, ok := quest[name]; ok {
			entry[name] = value
		}
	}
	for _, name := range []string{"nextQuestInChain", "parentQuest", "breadcrumbForQuestId"} {
		if value, ok := quest[name]; ok {
			entry[name] = []any{value}
		}
	}
	if len(entry) == 0 {
		return nil
	}
	return entry
}

func indexIDs(index map[string][]int, key string, id int) {
	index[key] = append(index[key], id)
}

func providerKeys(rows []any) []string {
	var keys []string
	for _, row := range rows {
		provider := row.(map[string]any)
		keys = append(keys, fmt.Sprintf("%s|%d", provider["type"], provider["id"]))
	}
	return keys
}

func sortedIndex(index map[string][]int) map[string]any {
	keys := make([]string, 0, len(index))
	for key := range index {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := map[string]any{}
	for _, key := range keys {
		ids := index[key]
		sort.Ints(ids)
		copied := make([]any, len(ids))
		for i, id := range ids {
			copied[i] = id
		}
		out[key] = copied
	}
	return out
}

func optionalInt(value any) (int, bool, error) {
	if value == nil || emptyValue(value) {
		return 0, false, nil
	}
	number, err := asInt(value)
	if err != nil {
		return 0, false, err
	}
	return number, true, nil
}

func asInt(value any) (int, error) {
	switch typed := value.(type) {
	case json.Number:
		return asInt(string(typed))
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		if typed != float64(int64(typed)) {
			return 0, fmt.Errorf("expected an integer, got %v", typed)
		}
		return int(typed), nil
	case string:
		number, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			parsed, floatErr := strconv.ParseFloat(typed, 64)
			if floatErr != nil || parsed != float64(int64(parsed)) {
				return 0, fmt.Errorf("expected an integer: %w", err)
			}
			return int(parsed), nil
		}
		return int(number), nil
	default:
		return 0, fmt.Errorf("expected an integer, got %T", value)
	}
}

func asFloat(value any) (float64, error) {
	switch typed := value.(type) {
	case json.Number:
		return asFloat(string(typed))
	case float64:
		return typed, nil
	case int:
		return float64(typed), nil
	case string:
		number, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return 0, err
		}
		return number, nil
	default:
		return 0, fmt.Errorf("expected a number, got %T", value)
	}
}

func intList(value any) ([]any, error) {
	rows, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a list")
	}
	var ids []any
	for _, row := range rows {
		id, err := asInt(row)
		if err != nil {
			return nil, err
		}
		if id != 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func emptyValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func normalize(value any) any {
	switch typed := value.(type) {
	case json.Number:
		if number, err := typed.Int64(); err == nil {
			return int(number)
		}
		number, err := typed.Float64()
		if err != nil {
			return string(typed)
		}
		return number
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalize(item)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for key, item := range typed {
			out[key] = normalize(item)
		}
		return out
	case string:
		if number, err := strconv.ParseInt(typed, 10, 64); err == nil && strconv.FormatInt(number, 10) == typed {
			return int(number)
		}
		if _, err := strconv.ParseFloat(typed, 64); err == nil {
			number, _ := strconv.ParseFloat(typed, 64)
			return number
		}
		return typed
	default:
		return value
	}
}

func stringField(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok && text != ""
}
