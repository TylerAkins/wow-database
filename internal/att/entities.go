package att

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// EntityDeclaration is one literal, source-located ATT catalog entry. ATT uses
// nested Lua groups rather than a single flat entity table, so repeated
// declarations are retained as observations and never silently overwrite one
// another.
type EntityDeclaration struct {
	EntityType string
	EntityID   int
	SourcePath string
	Label      string
	Fields     map[string]any
	Start      int
	End        int
}

type EntityContainment struct {
	ParentType string
	ParentID   int
	ChildType  string
	ChildID    int
	SourcePath string
}

type EntityCatalog struct {
	Declarations []EntityDeclaration
	Containments []EntityContainment
}

var (
	entityCallRE = regexp.MustCompile(`\b(ach|n|i|o)\s*\(\s*(\d+)\s*(?:,\s*(\{))?`)
	entityFlagRE = regexp.MustCompile(`(?:\["(isRaid|isDungeon|isZone|isWorldBoss|isRare)"\]|\b(isRaid|isDungeon|isZone|isWorldBoss|isRare))\s*=\s*true`)
)

// ReadEntityCatalog reads explicit ATT NPC, item, object, and achievement
// declarations and the containment edges created by nested groups. It only
// recognizes literal identifiers and selected literal fields; it never runs
// upstream Lua.
func ReadEntityCatalog(root string) (EntityCatalog, error) {
	ctx, err := loadExportContext(root)
	if err != nil {
		return EntityCatalog{}, err
	}
	var catalog EntityCatalog
	for _, path := range ctx.files {
		body, err := readSource(path, ctx.config)
		if err != nil {
			return EntityCatalog{}, err
		}
		text := string(body)
		masked := maskLua(text)
		for _, match := range entityCallRE.FindAllStringSubmatchIndex(masked, -1) {
			kind := text[match[2]:match[3]]
			id, err := strconv.Atoi(text[match[4]:match[5]])
			if err != nil || id <= 0 {
				continue
			}
			entityType := map[string]string{"n": "npc", "i": "item", "o": "object", "ach": "achievement"}[kind]
			declaration := EntityDeclaration{
				EntityType: entityType,
				EntityID:   id,
				SourcePath: relativeATTPath(root, path),
				Label:      inlineLabel(text[match[1]:]),
				Fields:     map[string]any{},
				Start:      match[0],
				End:        match[1],
			}
			if match[6] >= 0 {
				open := match[6]
				end, err := luaTableEnd(text, open)
				if err != nil {
					return EntityCatalog{}, fmt.Errorf("%s %d: %w", path, id, err)
				}
				declaration.End = end
				declaration.Fields, err = parseEntityFields(text[open+1 : end])
				if err != nil {
					return EntityCatalog{}, fmt.Errorf("%s %d: %w", path, id, err)
				}
			}
			catalog.Declarations = append(catalog.Declarations, declaration)
		}
	}
	for _, child := range catalog.Declarations {
		parentIndex := -1
		parentSpan := int(^uint(0) >> 1)
		for index, candidate := range catalog.Declarations {
			if candidate.SourcePath != child.SourcePath || candidate.Start >= child.Start || candidate.End <= child.Start {
				continue
			}
			span := candidate.End - candidate.Start
			if span < parentSpan {
				parentIndex, parentSpan = index, span
			}
		}
		if parentIndex < 0 {
			continue
		}
		parent := catalog.Declarations[parentIndex]
		if parent.SourcePath != child.SourcePath || parent.EntityType == child.EntityType && parent.EntityID == child.EntityID {
			continue
		}
		catalog.Containments = append(catalog.Containments, EntityContainment{
			ParentType: parent.EntityType,
			ParentID:   parent.EntityID,
			ChildType:  child.EntityType,
			ChildID:    child.EntityID,
			SourcePath: child.SourcePath,
		})
	}
	return catalog, nil
}

func parseEntityFields(block string) (map[string]any, error) {
	fields := map[string]any{}
	masked := maskLua(block)
	depths := braceDepths(masked)
	for _, field := range topLevelSubmatches(nameFieldRE, block, depths) {
		value, err := strconv.Unquote("\"" + field[1] + "\"")
		if err != nil {
			return nil, fmt.Errorf("decode name: %w", err)
		}
		fields["name"] = value
		break
	}
	for _, field := range topLevelSubmatches(descRE, block, depths) {
		value, err := strconv.Unquote("\"" + field[1] + "\"")
		if err != nil {
			return nil, fmt.Errorf("decode description: %w", err)
		}
		fields["description"] = value
		break
	}
	for _, field := range topLevelSubmatches(lvlRE, block, depths) {
		value, err := strconv.Atoi(field[1])
		if err != nil {
			return nil, fmt.Errorf("decode level: %w", err)
		}
		fields["level"] = value
		break
	}
	for _, field := range topLevelSubmatches(entityFlagRE, block, depths) {
		name := field[1]
		if name == "" {
			name = field[2]
		}
		fields[name] = true
	}
	return fields, nil
}

func inlineLabel(text string) string {
	lineEnd := strings.IndexByte(text, '\n')
	if lineEnd >= 0 {
		text = text[:lineEnd]
	}
	comment := strings.Index(text, "--")
	if comment < 0 {
		return ""
	}
	return strings.TrimSpace(text[comment+2:])
}

func relativeATTPath(root, path string) string {
	if relative, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}
