package database

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/TylerAkins/wow-database/internal/record"
	"golang.org/x/text/unicode/norm"
)

const (
	CompilationFormatVersion = 2
	NormalizationVersion     = 2
)

type sourceAudit struct {
	ExcludedStates map[string]string                 `json:"excludedStates,omitempty"`
	Revision       string                            `json:"revision"`
	Attributes     map[string]json.RawMessage        `json:"attributes"`
	FieldStates    map[string]string                 `json:"fieldStates,omitempty"`
	Provenance     map[string]record.FieldProvenance `json:"provenance,omitempty"`
	Issues         map[string]string                 `json:"normalizationIssues,omitempty"`
}

func normalizeRecord(row record.Record) (record.Record, sourceAudit, error) {
	audit := sourceAudit{Revision: row.SourceRevision, Attributes: row.Attributes, FieldStates: row.FieldStates, Provenance: row.Provenance, Issues: map[string]string{}}
	out := row
	out.Attributes = map[string]json.RawMessage{}
	out.FieldStates = map[string]string{}
	out.Provenance = map[string]record.FieldProvenance{}
	for k, v := range row.FieldStates {
		out.FieldStates[k] = v
	}
	for k, v := range row.Provenance {
		out.Provenance[k] = v
	}
	for field, raw := range row.Attributes {
		if field == "sourceData" {
			return out, audit, fmt.Errorf("sourceData is reserved for compiled observations")
		}
		if state := row.FieldStates[field]; state != "" && state != "empty" {
			audit.Issues[field] = "source marks observation " + state
			continue
		}
		if row.FieldStates[field] == "empty" && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			out.Attributes[field] = raw
			continue
		}
		value, state, reason := normalizeField(row, field, raw)
		if state != "" {
			out.FieldStates[field] = state
			audit.Issues[field] = reason
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return out, audit, err
		}
		out.Attributes[field] = encoded
		delete(out.FieldStates, field)
		if isExplicitEmpty(encoded) {
			out.FieldStates[field] = "empty"
		}
	}
	if row.Source == "questiedb" && (row.EntityType == "npc" || row.EntityType == "object") && row.Attributes["spawns"] != nil && out.Attributes["spawns"] != nil {
		var v any
		d := json.NewDecoder(bytes.NewReader(row.Attributes["spawns"]))
		d.UseNumber()
		_ = d.Decode(&v)
		presence := presenceZones(v)
		if len(presence) > 0 {
			encoded, _ := json.Marshal(presence)
			out.Attributes["spawnPresence"] = encoded
			out.Provenance["spawnPresence"] = row.Provenance["spawns"]
			if isExplicitEmpty(out.Attributes["spawns"]) {
				delete(out.Attributes, "spawns")
				out.FieldStates["spawns"] = "unavailable"
				audit.Issues["spawns"] = "instance presence provides no coordinate point"
			}
		}
	}
	// DB2 names are table-specific definitions, not translations of every similar field.
	if row.Source == "foreverclient" {
		aliases := clientAliases(row.EntityType)
		for dest, source := range aliases {
			if raw, ok := row.Attributes[source]; ok {
				value, state, reason := normalizeField(row, dest, raw)
				if row.FieldStates[source] != "" && row.FieldStates[source] != "empty" {
					state = row.FieldStates[source]
					reason = "source marks observation " + state
				}
				if state != "" {
					out.FieldStates[dest] = state
					audit.Issues[dest] = reason
				} else {
					encoded, _ := json.Marshal(value)
					out.Attributes[dest] = encoded
					if isExplicitEmpty(encoded) {
						out.FieldStates[dest] = "empty"
					}
				}
				out.Provenance[dest] = row.Provenance[source]
			} else if state := row.FieldStates[source]; state != "" {
				out.FieldStates[dest] = state
				out.Provenance[dest] = row.Provenance[source]
			}
		}
	}
	audit.ExcludedStates = map[string]string{}
	for field, state := range out.FieldStates {
		if state == "invalid" || state == "unsupported" || state == "unavailable" {
			audit.ExcludedStates[field] = state
		}
	}
	return out, audit, nil
}

func normalizeField(row record.Record, field string, raw json.RawMessage) (any, string, string) {
	if row.Source == "allthethings" && row.EntityType == "quest" {
		switch field {
		case "places":
			return nil, "unsupported", "ATT legacy places include inferred turn-in providers; use normalized providers and objectives"
		case "finishedBy":
			return nil, "unsupported", "ATT providers were copied to finishers without an explicit completion declaration"
		case "questLevel":
			return nil, "unsupported", "ATT lvl does not establish canonical quest level"
		case "objectivesText":
			return nil, "unsupported", "ATT description is catalog commentary, not quest objective text"
		case "zoneOrSort":
			return nil, "unsupported", "ATT containment location does not establish quest area or sort"
		}
	}
	if row.Source == "allthethings" && row.EntityType == "relationship" && field == "relationship" {
		var link struct {
			Kind string `json:"kind"`
			Role string `json:"role"`
		}
		if err := json.Unmarshal(raw, &link); err == nil && link.Kind == "quest-provider" && link.Role == "finishedBy" {
			return nil, "unsupported", "ATT provider relationship infers completion from a starting provider"
		}
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, "invalid", err.Error()
	}
	var err error
	switch {
	case field == "name":
		name, ok := value.(string)
		if !ok || !utf8.ValidString(name) {
			err = fmt.Errorf("name must be valid Unicode text")
		} else {
			value = norm.NFC.String(name)
		}
	case row.EntityType == "quest" && (field == "startedBy" || field == "finishedBy"):
		value, err = normalizeProviders(row.Source, value, field)
	case row.EntityType == "quest" && field == "objectives":
		value, err = normalizeObjectives(row.Source, value)
	case (row.EntityType == "npc" || row.EntityType == "object") && field == "spawns":
		value, err = normalizeSpawns(row.Source, value)
	case integerField(row.EntityType, field):
		number, ok := value.(json.Number)
		if !ok {
			err = fmt.Errorf("field must be an exact integer")
		} else {
			n := new(big.Int)
			if _, ok := n.SetString(string(number), 10); !ok {
				err = fmt.Errorf("field must be an exact integer")
			} else if field != "questLevel" && field != "zoneOrSort" && field != "requiredSpell" && n.Sign() < 0 {
				err = fmt.Errorf("field must be nonnegative")
			}
		}
	case row.EntityType == "quest" && field == "preQuestGroup":
		value, err = normalizeIDSet(value, true)
	case unorderedIDs(row.EntityType, field):
		value, err = normalizeIDs(value)
	case row.EntityType == "quest" && field == "objectivesText":
		entries, ok := value.([]any)
		if !ok {
			err = fmt.Errorf("objectivesText must be an ordered text array")
		} else {
			for _, entry := range entries {
				if _, ok := entry.(string); !ok {
					err = fmt.Errorf("objectivesText entry must be text")
					break
				}
			}
		}
	}
	if err != nil {
		return nil, "invalid", err.Error()
	}
	return value, "", ""
}

func integerField(entity, field string) bool {
	if entity == "npc" {
		switch field {
		case "minLevel", "maxLevel", "minLevelHealth", "maxLevelHealth", "rank", "npcFlags", "factionID", "zoneID":
			return true
		}
	}
	if entity == "object" {
		switch field {
		case "factionID", "zoneID":
			return true
		}
	}
	if entity == "quest" {
		switch field {
		case "questLevel", "requiredLevel", "requiredRaces", "requiredClasses", "zoneOrSort", "sourceItemId", "questFlags", "uniqueBitFlag", "questUiThemeId", "specialFlags", "nextQuestInChain", "parentQuest", "requiredMaxLevel", "requiredSpell", "requiredSpecialization", "availableUntilCompleted", "availableStartingWith", "disabledByQuest", "breadcrumbForQuestId":
			return true
		}
	}
	if entity == "item" {
		switch field {
		case "itemLevel", "requiredLevel", "class", "subClass", "ammoType", "flags", "startQuest":
			return true
		}
	}
	return false
}
func unorderedIDs(entity, field string) bool {
	if entity == "quest" {
		switch field {
		case "preQuestSingle", "preQuestGroup", "exclusiveTo", "childQuests", "inGroupWith", "requiredSourceItems", "breadcrumbs":
			return true
		}
	}
	if entity == "item" {
		switch field {
		case "npcDrops", "objectDrops", "itemDrops", "questRewards", "vendors", "relatedQuests":
			return true
		}
	}
	if entity == "npc" || entity == "object" {
		switch field {
		case "questStarts", "questEnds", "startsQuestIds", "containedItemIds":
			return true
		}
	}
	return false
}
func positiveID(value any) (json.Number, error) {
	number, ok := value.(json.Number)
	if !ok {
		return "", fmt.Errorf("ID must be an integer")
	}
	n := new(big.Int)
	if _, ok := n.SetString(string(number), 10); !ok || n.Sign() <= 0 {
		return "", fmt.Errorf("ID must be positive")
	}
	return json.Number(n.String()), nil
}
func normalizeIDs(value any) (any, error) {
	return normalizeIDSet(value, false)
}

func normalizeIDSet(value any, signed bool) (any, error) {
	entries, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("IDs must be an array")
	}
	ids := []any{}
	for _, v := range entries {
		n, err := positiveID(v)
		if signed {
			if number, ok := v.(json.Number); ok {
				integer, valid := new(big.Int).SetString(string(number), 10)
				if valid && integer.Sign() != 0 {
					n, err = json.Number(integer.String()), nil
				}
			}
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, n)
	}
	return sortedUnique(ids), nil
}
func sortedUnique(values []any) []any {
	keyed := map[string]any{}
	for _, v := range values {
		v = canonicalNumbers(v)
		b, _ := json.Marshal(v)
		keyed[string(b)] = v
	}
	keys := make([]string, 0, len(keyed))
	for k := range keyed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]any, 0, len(keys))
	for _, k := range keys {
		result = append(result, keyed[k])
	}
	return result
}
func normalizeProviders(source string, value any, field string) (any, error) {
	entries, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("providers must be an array")
	}
	out := []any{}
	if source == "questiedb" {
		kinds := []string{"npc", "object", "item"}
		limit := 3
		if field == "finishedBy" {
			limit = 2
		}
		if len(entries) > limit {
			return nil, fmt.Errorf("unexpected provider slots")
		}
		for i, slot := range entries {
			if slot == nil {
				continue
			}
			ids, ok := slot.([]any)
			if !ok {
				return nil, fmt.Errorf("provider slot must be an ID array")
			}
			for _, id := range ids {
				n, err := positiveID(id)
				if err != nil {
					return nil, err
				}
				out = append(out, map[string]any{"type": kinds[i], "id": n})
			}
		}
	} else {
		for _, entry := range entries {
			m, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("provider must be an object")
			}
			kind, ok := m["type"].(string)
			if !ok || (kind != "npc" && kind != "object" && kind != "item") {
				return nil, fmt.Errorf("invalid provider type")
			}
			id, err := positiveID(m["id"])
			if err != nil {
				return nil, err
			}
			out = append(out, map[string]any{"type": kind, "id": id})
		}
	}
	return sortedUnique(out), nil
}
func normalizeObjectives(source string, value any) (any, error) {
	groups := map[string]any{}
	if entries, ok := value.([]any); ok && len(entries) == 0 {
		return groups, nil
	}
	if source != "questiedb" {
		input, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("objectives must be named groups")
		}
		for kind, raw := range input {
			if kind != "creature" && kind != "object" && kind != "item" {
				return nil, fmt.Errorf("unsupported objective group %s", kind)
			}
			entries, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("objective group must be an array")
			}
			out := []any{}
			for _, entry := range entries {
				m, ok := entry.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("objective entry must be an object")
				}
				if _, err := positiveID(m["id"]); err != nil {
					return nil, err
				}
				out = append(out, m)
			}
			groups[kind] = out
		}
		return groups, nil
	}
	slots, ok := value.([]any)
	if !ok || len(slots) > 6 {
		return nil, fmt.Errorf("objectives must contain at most six slots")
	}
	kinds := []string{"creature", "object", "item", "reputation", "killCredit", "spell"}
	for i, slot := range slots {
		if slot == nil {
			continue
		}
		entries, ok := slot.([]any)
		if !ok {
			return nil, fmt.Errorf("objective slot must be an array")
		}
		out := []any{}
		if i == 3 {
			if len(entries) != 2 {
				return nil, fmt.Errorf("reputation objective requires faction and value")
			}
			id, err := positiveID(entries[0])
			if err != nil {
				return nil, err
			}
			if _, ok := entries[1].(json.Number); !ok {
				return nil, fmt.Errorf("reputation value must be numeric")
			}
			out = append(out, map[string]any{"id": id, "value": entries[1]})
		} else {
			for _, entry := range entries {
				parts, ok := entry.([]any)
				if !ok || len(parts) == 0 {
					return nil, fmt.Errorf("objective entry must be a nonempty array")
				}
				m := map[string]any{}
				if i == 4 {
					if len(parts) > 4 {
						return nil, fmt.Errorf("unexpected kill credit fields")
					}
					ids, err := normalizeIDs(parts[0])
					if err != nil {
						return nil, err
					}
					m["ids"] = ids
					if len(parts) > 1 && parts[1] != nil {
						id, err := positiveID(parts[1])
						if err != nil {
							return nil, err
						}
						m["baseId"] = id
					}
					if len(parts) > 2 && parts[2] != nil {
						if _, ok := parts[2].(string); !ok {
							return nil, fmt.Errorf("objective text must be text")
						}
						m["text"] = parts[2]
					}
					if len(parts) > 3 && parts[3] != nil {
						m["iconFile"] = parts[3]
					}
				} else {
					if len(parts) > 3 {
						return nil, fmt.Errorf("unexpected objective fields")
					}
					id, err := positiveID(parts[0])
					if err != nil {
						return nil, err
					}
					m["id"] = id
					if len(parts) > 1 && parts[1] != nil {
						if _, ok := parts[1].(string); !ok {
							return nil, fmt.Errorf("objective text must be text")
						}
						m["text"] = parts[1]
					}
					if len(parts) > 2 && parts[2] != nil {
						key := "iconFile"
						if i == 5 {
							key = "itemId"
							if _, err := positiveID(parts[2]); err != nil {
								return nil, err
							}
						}
						m[key] = parts[2]
					}
				}
				out = append(out, m)
			}
		}
		groups[kinds[i]] = out
	}
	return groups, nil
}
func normalizeSpawns(source string, value any) (any, error) {
	out := []any{}
	add := func(zone, x, y any) error {
		id, err := positiveID(zone)
		if err != nil {
			return err
		}
		for _, v := range []any{x, y} {
			n, ok := v.(json.Number)
			if !ok {
				return fmt.Errorf("coordinate must be numeric")
			}
			r, ok := new(big.Rat).SetString(string(n))
			if !ok || r.Sign() < 0 || r.Cmp(big.NewRat(100, 1)) > 0 {
				return fmt.Errorf("coordinate must be within 0..100")
			}
		}
		out = append(out, map[string]any{"zoneId": id, "x": x, "y": y})
		return nil
	}
	switch input := value.(type) {
	case map[string]any:
		for zone, raw := range input {
			entries, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("spawn zone must contain coordinate pairs")
			}
			for _, entry := range entries {
				pair, ok := entry.([]any)
				if !ok || len(pair) != 2 {
					return nil, fmt.Errorf("spawn must be a coordinate pair")
				}
				if source == "questiedb" && isPresence(pair[0], pair[1]) {
					continue
				}
				if err := add(json.Number(zone), pair[0], pair[1]); err != nil {
					return nil, err
				}
			}
		}
	case []any:
		for _, entry := range input {
			m, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("spawn must be an object")
			}
			if err := add(m["zoneId"], m["x"], m["y"]); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("spawns must be a zone map or array")
	}
	return sortedUnique(out), nil
}

// decimalNumber makes numeric representations comparable without rounding.
func decimalNumber(number json.Number) json.Number {
	s := string(number)
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	exponent := 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil || n > 10000 || n < -10000 {
			return number
		}
		exponent = n
		s = s[:i]
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		exponent -= len(s) - i - 1
		s = s[:i] + s[i+1:]
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	for strings.HasSuffix(s, "0") {
		s = strings.TrimSuffix(s, "0")
		exponent++
	}
	if exponent >= 0 {
		s += strings.Repeat("0", exponent)
	} else if len(s)+exponent > 0 {
		i := len(s) + exponent
		s = s[:i] + "." + s[i:]
	} else {
		s = "0." + strings.Repeat("0", -exponent-len(s)) + s
	}
	if negative {
		s = "-" + s
	}
	return json.Number(s)
}
func canonicalNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		return decimalNumber(v)
	case []any:
		for i, x := range v {
			v[i] = canonicalNumbers(x)
		}
	case map[string]any:
		for k, x := range v {
			v[k] = canonicalNumbers(x)
		}
	}
	return value
}

func isPresence(x, y any) bool { return x == json.Number("-1") && y == json.Number("-1") }
func presenceZones(value any) []any {
	zones, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := []any{}
	for zone, v := range zones {
		pairs, ok := v.([]any)
		if !ok {
			continue
		}
		for _, entry := range pairs {
			pair, ok := entry.([]any)
			if ok && len(pair) == 2 && isPresence(pair[0], pair[1]) {
				id, err := positiveID(json.Number(zone))
				if err == nil {
					out = append(out, id)
				}
			}
		}
	}
	return sortedUnique(out)
}

func clientAliases(entity string) map[string]string {
	names := map[string]string{"npc": "Name_lang", "item": "Display_lang", "zone": "AreaName_lang", "map": "MapName_lang", "spell": "Name_lang", "faction": "Name_lang", "achievement": "Title_lang"}
	aliases := map[string]string{}
	if entity == "quest" {
		return map[string]string{"questUiThemeId": "UiQuestDetailsThemeID", "uniqueBitFlag": "UniqueBitFlag"}
	}
	if name := names[entity]; name != "" {
		aliases["name"] = name
	}
	if entity == "item" {
		aliases["itemLevel"] = "ItemLevel"
		aliases["requiredLevel"] = "RequiredLevel"
	}
	return aliases
}
