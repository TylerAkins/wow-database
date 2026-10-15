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

// ATT loads shared definitions, flavor configuration, then every flavor data file.
func sourceFiles(root string) ([]string, error) {
	var shared, config, data []string
	for _, flavor := range []string{"shared", "forever"} {
		base := filepath.Join(root, ".contrib/.db", flavor)
		if _, err := os.Stat(base); os.IsNotExist(err) && flavor == "shared" {
			continue
		}
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".lua") {
				return nil
			}
			if flavor == "shared" {
				shared = append(shared, path)
			} else if strings.Contains(filepath.ToSlash(path), "/.config/") {
				config = append(config, path)
			} else {
				data = append(data, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(shared)
	sort.Strings(config)
	sort.Strings(data)
	return append(append(shared, config...), data...), nil
}

var sourceDirectiveRE = regexp.MustCompile(`(?i)^\s*--\s*#(if|elseif|elif|else|endif)\b\s*(.*)`)

type sourceConfig struct {
	tags    map[string]bool
	version []int
}

func readSourceConfig(root string) sourceConfig {
	config := sourceConfig{tags: map[string]bool{}, version: []int{1, 60, 1, 69893}}
	body, err := os.ReadFile(filepath.Join(root, ".contrib/.db/forever/.config/forever.config"))
	if err != nil {
		body = []byte(`"PreProcessorTags": ["ANYCLASSIC","FOREVER","CAMELOT","CLASSIC","CRIEVE","EXPLORATION","OBJECTIVES","NOSIMPLIFY"]`)
	}
	if match := regexp.MustCompile(`"PreProcessorTags"\s*:\s*\[([^]]*)\]`).FindSubmatch(body); match != nil {
		for _, tag := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(match[1], -1) {
			config.tags[strings.ToUpper(string(tag[1]))] = true
		}
	}
	if match := regexp.MustCompile(`"DataPatch"\s*:\s*\[([^]]*)\]`).FindSubmatch(body); match != nil {
		config.version = nil
		for _, value := range strings.Split(string(match[1]), ",") {
			n, _ := strconv.Atoi(strings.TrimSpace(value))
			config.version = append(config.version, n)
		}
	}
	return config
}

func (config sourceConfig) condition(expression string) bool {
	words := strings.Fields(strings.ToUpper(expression))
	if len(words) == 0 {
		return false
	}
	if words[0] == "NOT" {
		return !config.condition(strings.Join(words[1:], " "))
	}
	if config.tags[words[0]] {
		return true
	}
	if len(words) == 2 && (words[0] == "AFTER" || words[0] == "BEFORE") {
		// Forever is a Classic phase, preceding the first expansion phase.
		expansion := map[string]bool{"TBC": true, "WRATH": true, "CATA": true, "MOP": true, "WOD": true, "LEGION": true, "BFA": true, "SL": true, "DF": true, "TWW": true, "MID": true, "TRANSMOG": true, "TLT": true}
		less := expansion[words[1]]
		if words[1] == "CLASSIC" || words[1] == "FOREVER" || words[1] == "SEASON_OF_DISCOVERY" {
			if words[0] == "AFTER" {
				return true
			}
			return false
		}
		if !less {
			version := strings.Split(words[1], ".")
			for i, part := range version {
				other, err := strconv.Atoi(part)
				if err != nil {
					return false
				}
				current := 0
				if i < len(config.version) {
					current = config.version[i]
				}
				if current != other {
					less = current < other
					break
				}
			}
		}
		if words[0] == "BEFORE" {
			return less
		}
		return !less
	}
	return false
}

// Blank excluded branches without changing byte offsets used by provenance.
func preprocessSource(body []byte, config sourceConfig) ([]byte, error) {
	type branch struct{ parent, taken bool }
	stack := []branch{}
	active := true
	out := append([]byte(nil), body...)
	offset := 0
	for _, line := range strings.SplitAfter(string(body), "\n") {
		directive := sourceDirectiveRE.FindStringSubmatch(strings.TrimPrefix(line, "\ufeff"))
		if directive != nil {
			switch strings.ToLower(directive[1]) {
			case "if":
				value := config.condition(strings.TrimSpace(directive[2]))
				stack = append(stack, branch{active, value})
				active = active && value
			case "elseif", "elif", "else":
				if len(stack) == 0 {
					return nil, fmt.Errorf("unmatched preprocessor branch")
				}
				current := &stack[len(stack)-1]
				value := strings.EqualFold(directive[1], "else") || config.condition(strings.TrimSpace(directive[2]))
				active = current.parent && !current.taken && value
				current.taken = current.taken || value
			case "endif":
				if len(stack) == 0 {
					return nil, fmt.Errorf("unmatched preprocessor endif")
				}
				active = stack[len(stack)-1].parent
				stack = stack[:len(stack)-1]
			}
		}
		if directive != nil || !active {
			for i := offset; i < offset+len(line); i++ {
				if out[i] != '\n' && out[i] != '\r' {
					out[i] = ' '
				}
			}
		}
		offset += len(line)
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unterminated preprocessor conditional")
	}
	return out, nil
}

func readSource(path string, config sourceConfig) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	processed, err := preprocessSource(body, config)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return processed, nil
}

var literalFieldRE = regexp.MustCompile(`(?:\["([A-Za-z_][A-Za-z0-9_]*)"\]|\b([A-Za-z_][A-Za-z0-9_]*))\s*=`)

// topFields retains complete expressions and ignores child groups and comments.
func topFields(block string) map[string]string {
	masked := maskLua(block)
	depths := braceDepths(masked)
	fields := map[string]string{}
	for _, indexes := range literalFieldRE.FindAllStringSubmatchIndex(block, -1) {
		start := indexes[0]
		if start >= len(depths) || depths[start] != 0 || strings.TrimSpace(masked[start:indexes[1]]) == "" {
			continue
		}
		name := ""
		if indexes[2] >= 0 {
			name = block[indexes[2]:indexes[3]]
		} else {
			name = block[indexes[4]:indexes[5]]
		}
		expressionStart := indexes[1]
		commentsRemoved := stripLineComments(block)
		for expressionStart < len(commentsRemoved) && strings.ContainsRune(" \t\r\n", rune(commentsRemoved[expressionStart])) {
			expressionStart++
		}
		end := expressionStart
		braces, parens, brackets := 0, 0, 0
		for ; end < len(masked); end++ {
			c := masked[end]
			if (c == ',' || c == ';' || c == '\n' || c == '\r') && braces == 0 && parens == 0 && brackets == 0 {
				break
			}
			switch c {
			case '{':
				braces++
			case '}':
				braces--
			case '(':
				parens++
			case ')':
				parens--
			case '[':
				brackets++
			case ']':
				brackets--
			}
		}
		value := strings.TrimSpace(stripLineComments(block[indexes[1]:end]))
		fields[name] = value
	}
	return fields
}

func topFieldBlock(block string) string {
	values := topFields(block)
	names := make([]string, 0, len(values))
	for name := range values {
		if name != "groups" && name != "g" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var result strings.Builder
	for _, name := range names {
		fmt.Fprintf(&result, "[\"%s\"] = %s,\n", name, values[name])
	}
	return result.String()
}

func inheritedFieldBlock(text string, position int) string {
	masked := maskLua(text)
	stack := []int{}
	for i := 0; i < position; i++ {
		if masked[i] == '{' {
			stack = append(stack, i)
		} else if masked[i] == '}' && len(stack) > 0 {
			stack = stack[:len(stack)-1]
		}
	}
	values := map[string]string{}
	for _, open := range stack {
		end, err := luaTableEnd(text, open)
		if err != nil {
			continue
		}
		for key, value := range topFields(text[open+1 : end]) {
			switch key {
			case "races", "classes", "lvl", "requireSkill":
				values[key] = value
			}
		}
	}
	for _, match := range regexp.MustCompile(`\b(?:sharedData|bubbleDown|bubbleDownSelf)\s*\(\s*\{`).FindAllStringIndex(masked[:position], -1) {
		open := match[1] - 1
		end, err := luaTableEnd(text, open)
		if err != nil || end >= position {
			continue
		}
		next := end + 1
		for next < len(masked) && strings.ContainsRune(" \t\r\n,", rune(masked[next])) {
			next++
		}
		if next >= len(masked) || masked[next] != '{' {
			continue
		}
		childrenEnd, err := luaTableEnd(text, next)
		if err != nil || childrenEnd < position {
			continue
		}
		for key, value := range topFields(text[open+1 : end]) {
			switch key {
			case "races", "classes", "lvl", "requireSkill":
				values[key] = value
			}
		}
	}
	var result strings.Builder
	for _, key := range []string{"races", "classes", "lvl", "requireSkill"} {
		if value, ok := values[key]; ok {
			fmt.Fprintf(&result, "[\"%s\"] = %s,\n", key, value)
		}
	}
	return result.String()
}

func applyNormalizedEvidence(row map[string]any, block string, ctx exportContext) {
	fields := row["fields"].(map[string]any)
	raw := topFields(block)
	unresolved := []any{}
	unresolvedField := func(name, expression, reason string) {
		unresolved = append(unresolved, map[string]any{"field": name, "expression": expression, "reason": reason})
	}
	for _, name := range []string{"races", "classes", "requireSkill", "sourceQuest", "sourceQuests", "altQuests", "lockCriteria", "lvl", "qi", "qis", "isBreadcrumb", "coord", "coords", "qg", "qgs", "provider", "providers", "sourceQuestNumRequired"} {
		expression, exists := raw[name]
		if !exists {
			continue
		}
		clean := strings.TrimSpace(expression)
		switch name {
		case "sourceQuestNumRequired":
			count, err := strconv.Atoi(clean)
			if err != nil {
				unresolvedField(name, expression, "unsupported prerequisite threshold")
			} else {
				fields[name] = count
				ids, valid := literalLuaList(raw["sourceQuests"], ctx.constants)
				if valid && count > 1 && count < len(ids) {
					delete(fields, "preQuestGroup")
					delete(fields, "preQuestSingle")
					unresolvedField(name, expression, "partial prerequisite threshold is preserved without mapping to all/any prerequisites")
				}
			}
		case "coord", "coords":
			_, err := coordinateTables(block)
			if err != nil {
				unresolvedField(name, expression, err.Error())
			} else if isIdentifier(clean) {
				if _, ok := ctx.coordinateConstants[clean]; !ok {
					unresolvedField(name, expression, "unresolved coordinate constant")
				}
			}
		case "qg":
			if _, err := strconv.Atoi(clean); err != nil {
				unresolvedField(name, expression, "unsupported provider expression")
				delete(row, "startedBy")
			}
		case "qgs":
			if _, ok := literalLuaList(clean, ctx.constants); !ok {
				unresolvedField(name, expression, "unsupported provider list expression")
				delete(row, "startedBy")
			}
		case "provider", "providers":
			if !strings.HasPrefix(clean, "{") || !strings.HasSuffix(clean, "}") {
				unresolvedField(name, expression, "unsupported provider expression")
				delete(row, "startedBy")
			}
		case "races":
			ids, known := ctx.factions[clean]
			if !known {
				switch clean {
				case "HORDE_ONLY":
					ids = foreverHordeRaces
					known = true
				case "ALLIANCE_ONLY":
					ids = foreverAllianceRaces
					known = true
				}
			}
			if !known {
				var err error
				ids, err = parseSymbolList(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(clean, "{"), "}")), raceConstants)
				known = err == nil
			}
			if known {
				fields["requiredRaces"] = raceMask(ids)
			} else {
				unresolvedField(name, expression, "unsupported race expression")
			}
		case "requireSkill":
			value, err := strconv.Atoi(clean)
			if err != nil {
				value = ctx.constants[clean]
			}
			if value > 0 {
				fields["requiredSkill"] = []any{value, 1}
			} else {
				unresolvedField(name, expression, "unsupported profession expression")
			}
		case "isBreadcrumb":
			if clean == "true" || clean == "false" {
				fields["isBreadcrumb"] = clean == "true"
			} else {
				unresolvedField(name, expression, "unsupported boolean expression")
			}
		case "lockCriteria":
			// ATT locks completion after the threshold is met. It is not a prerequisite.
			if values, ok := literalLuaList(clean, ctx.constants); ok {
				fields["lockCriteria"] = values
			} else {
				unresolvedField(name, expression, "unsupported lock expression")
			}
		case "qis":
			if values, ok := literalLuaList(clean, ctx.constants); ok {
				fields["providedItemIds"] = values
			} else {
				unresolvedField(name, expression, "unsupported provided item expression")
			}
		case "sourceQuests", "altQuests":
			if _, ok := literalLuaList(clean, ctx.constants); !ok {
				unresolvedField(name, expression, "unsupported quest list expression")
			}
		case "classes":
			if _, err := classMaskFromList(strings.TrimSuffix(strings.TrimPrefix(clean, "{"), "}")); err != nil {
				unresolvedField(name, expression, "unsupported class expression")
			}
		default:
			if _, err := strconv.Atoi(clean); err != nil {
				unresolvedField(name, expression, "unsupported integer expression")
				switch name {
				case "lvl":
					delete(fields, "requiredLevel")
				case "qi":
					delete(fields, "providedItemId")
				case "sourceQuest":
					if _, exists := raw["sourceQuests"]; !exists {
						delete(fields, "preQuestSingle")
					}
				}
			}
		}
	}
	if providers, ok := row["startedBy"].([]any); ok {
		for _, value := range providers {
			provider := value.(map[string]any)
			if provider["type"] == "item" {
				fields["sourceItemId"] = provider["id"]
				break
			}
		}
	}
	if len(unresolved) > 0 {
		existing, _ := row["unresolved"].([]any)
		row["unresolved"] = append(existing, unresolved...)
	}
}

func literalLuaList(expression string, constants map[string]int) ([]any, bool) {
	if !strings.HasPrefix(expression, "{") || !strings.HasSuffix(expression, "}") {
		return nil, false
	}
	var values []any
	for _, token := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(expression, "{"), "}"), ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "\"") {
			value, err := strconv.Unquote(token)
			if err != nil {
				return nil, false
			}
			values = append(values, value)
			continue
		}
		value, err := strconv.Atoi(token)
		if err != nil {
			var ok bool
			value, ok = constants[token]
			if !ok {
				return nil, false
			}
		}
		values = append(values, value)
	}
	return values, true
}

func copyFields(fields map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range fields {
		switch nested := value.(type) {
		case map[string]any:
			result[key] = copyFields(nested)
		case []any:
			copied := make([]any, len(nested))
			for index, entry := range nested {
				if object, ok := entry.(map[string]any); ok {
					copied[index] = copyFields(object)
				} else {
					copied[index] = entry
				}
			}
			result[key] = copied
		default:
			result[key] = value
		}
	}
	return result
}
func hasFieldConflict(row map[string]any, field string) bool {
	unresolved, _ := row["unresolved"].([]any)
	for _, value := range unresolved {
		evidence, ok := value.(map[string]any)
		if ok && evidence["field"] == field && evidence["reason"] == "conflicting declarations" {
			return true
		}
	}
	return false
}
