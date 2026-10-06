package compile

import (
	"fmt"

	"github.com/TylerAkins/wow-database/internal/att"
)

func mergeATT(records []questRecord, source att.Result) mergeCounts {
	counts := mergeCounts{unmapped: len(source.Unmapped), issues: []map[string]any{}}
	for _, report := range source.Unmapped {
		counts.issues = append(counts.issues, map[string]any{
			"kind": "unmapped", "questId": report.QuestID, "npcId": report.NPCID,
			"file": report.File, "reason": report.Reason,
		})
	}
	byQuest := map[int][]att.Location{}
	attQuestIDs := map[int]bool{}
	for _, location := range source.Locations {
		byQuest[location.QuestID] = append(byQuest[location.QuestID], location)
		attQuestIDs[location.QuestID] = true
	}
	questIDs := map[int]bool{}
	for _, record := range records {
		questIDs[record.id] = true
	}
	for id := range attQuestIDs {
		if !questIDs[id] {
			counts.attOnly++
		}
	}

	for i := range records {
		row := records[i].raw
		places, ok := row["places"].([]any)
		if !ok {
			continue
		}
		for _, location := range byQuest[records[i].id] {
			matches := []map[string]any{}
			for _, value := range places {
				place, ok := value.(map[string]any)
				if !ok || place["role"] != "available" {
					continue
				}
				kind, _ := place["type"].(string)
				npcID, err := asInt(place["id"])
				if err == nil && kind == location.Kind && npcID == location.NPCID {
					matches = append(matches, place)
				}
			}
			if len(matches) != 1 {
				counts.ambiguous++
				counts.issues = append(counts.issues, map[string]any{
					"kind": "ambiguous", "questId": location.QuestID, "npcId": location.NPCID,
					"reason": fmt.Sprintf("expected one matching Questie available place, found %d", len(matches)),
				})
				continue
			}
			place := matches[0]
			questSpawns := usableSpawns(place["spawns"])
			attSpawns := make([]any, 0, len(location.Spawns))
			for _, spawn := range location.Spawns {
				attSpawns = append(attSpawns, []any{spawn.Zone, spawn.X, spawn.Y})
			}
			if len(questSpawns) > 0 {
				if !sameSpawns(questSpawns, attSpawns) {
					counts.conflicts++
					counts.issues = append(counts.issues, map[string]any{
						"kind": "conflict", "questId": location.QuestID, "npcId": location.NPCID,
						"reason": "Questie coordinates retained over differing ATT coordinates",
					})
				}
				continue
			}
			if len(attSpawns) == 0 {
				continue
			}
			place["spawns"] = attSpawns
			counts.filled++
		}
	}
	return counts
}

func usableSpawns(value any) []any {
	rows, ok := value.([]any)
	if !ok {
		return nil
	}
	var out []any
	for _, value := range rows {
		point, ok := value.([]any)
		if !ok || len(point) != 3 {
			continue
		}
		zone, err := asInt(point[0])
		if err != nil || zone <= 0 {
			continue
		}
		x, err := asFloat(point[1])
		if err != nil || x < 0 || x > 100 {
			continue
		}
		y, err := asFloat(point[2])
		if err != nil || y < 0 || y > 100 {
			continue
		}
		out = append(out, []any{zone, x, y})
	}
	return out
}

func sameSpawns(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		left, lok := a[i].([]any)
		right, rok := b[i].([]any)
		if !lok || !rok || len(left) != 3 || len(right) != 3 {
			return false
		}
		for j := 0; j < 3; j++ {
			if fmt.Sprint(left[j]) != fmt.Sprint(right[j]) {
				return false
			}
		}
	}
	return true
}
