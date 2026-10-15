package record

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
)

// ValidateCompiledAttributes checks the shared shapes introduced by compilation format 2.
func ValidateCompiledAttributes(r Record) error {
	var audit map[string]struct {
		Revision    string                     `json:"revision"`
		Attributes  map[string]json.RawMessage `json:"attributes"`
		FieldStates map[string]string          `json:"fieldStates"`
		Provenance  map[string]FieldProvenance `json:"provenance"`
	}
	if err := json.Unmarshal(r.Attributes["sourceData"], &audit); err != nil || len(audit) == 0 {
		return fmt.Errorf("format 2 requires sourceData observations")
	}
	for source, a := range audit {
		original := Record{SchemaVersion: SchemaVersion, Flavor: r.Flavor, EntityType: r.EntityType, EntityID: r.EntityID, Source: source, SourceRevision: a.Revision, Attributes: a.Attributes, FieldStates: a.FieldStates, Provenance: a.Provenance}
		if source == "compilation" {
			return fmt.Errorf("sourceData cannot contain compilation")
		}
		if err := original.Validate(); err != nil {
			return fmt.Errorf("sourceData %s: %w", source, err)
		}
	}
	for field, raw := range r.Attributes {
		if r.FieldStates[field] == "empty" && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var value any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(&value); err != nil {
			return err
		}
		if compiledIntegerField(r.EntityType, field) {
			n, ok := value.(json.Number)
			if !ok {
				return fmt.Errorf("%s must be an integer", field)
			}
			v, ok := new(big.Int).SetString(string(n), 10)
			if !ok || (field != "questLevel" && field != "zoneOrSort" && field != "requiredSpell" && v.Sign() < 0) {
				return fmt.Errorf("invalid integer %s", field)
			}
		}
		if field == "name" {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("name must be text")
			}
		}
		if r.EntityType == "quest" && (field == "startedBy" || field == "finishedBy") {
			entries, ok := value.([]any)
			if !ok {
				return fmt.Errorf("%s must be an array", field)
			}
			for _, entry := range entries {
				m, ok := entry.(map[string]any)
				if !ok || len(m) != 2 {
					return fmt.Errorf("provider requires type and id")
				}
				if m["type"] != "npc" && m["type"] != "object" && m["type"] != "item" {
					return fmt.Errorf("invalid provider type")
				}
				if !compiledPositiveID(m["id"]) {
					return fmt.Errorf("invalid provider ID")
				}
			}
		}
		if r.EntityType == "quest" && field == "objectives" {
			groups, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("objectives must be named groups")
			}
			for kind, group := range groups {
				if kind != "creature" && kind != "object" && kind != "item" && kind != "reputation" && kind != "killCredit" && kind != "spell" {
					return fmt.Errorf("invalid objective kind")
				}
				entries, ok := group.([]any)
				if !ok {
					return fmt.Errorf("objective group must be an array")
				}
				for _, entry := range entries {
					m, ok := entry.(map[string]any)
					if !ok {
						return fmt.Errorf("objective must be an object")
					}
					if kind == "killCredit" {
						ids, ok := m["ids"].([]any)
						if !ok {
							return fmt.Errorf("kill credit requires IDs")
						}
						for _, id := range ids {
							if !compiledPositiveID(id) {
								return fmt.Errorf("invalid kill credit ID")
							}
						}
					} else if !compiledPositiveID(m["id"]) {
						return fmt.Errorf("invalid objective ID")
					}
				}
			}
		}
		if (r.EntityType == "npc" || r.EntityType == "object") && field == "spawns" {
			entries, ok := value.([]any)
			if !ok {
				return fmt.Errorf("spawns must be an array")
			}
			for _, entry := range entries {
				m, ok := entry.(map[string]any)
				if !ok || len(m) != 3 || !compiledPositiveID(m["zoneId"]) {
					return fmt.Errorf("spawn requires zoneId, x, y")
				}
				for _, axis := range []string{"x", "y"} {
					n, ok := m[axis].(json.Number)
					if !ok {
						return fmt.Errorf("coordinate must be numeric")
					}
					v, ok := new(big.Rat).SetString(string(n))
					if !ok || v.Sign() < 0 || v.Cmp(big.NewRat(100, 1)) > 0 {
						return fmt.Errorf("coordinate must be within 0..100")
					}
				}
			}
		}
	}
	return nil
}
func compiledPositiveID(value any) bool {
	n, ok := value.(json.Number)
	if !ok {
		return false
	}
	v, ok := new(big.Int).SetString(string(n), 10)
	return ok && v.Sign() > 0
}

func compiledIntegerField(entity, field string) bool {
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
	return false
}
