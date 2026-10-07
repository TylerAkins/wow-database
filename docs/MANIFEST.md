# Manifest and consumer migration

The probe file is `export/forever/manifest.json`. Always fetch it before downloading shards.

## Current shape (ATT-only)

| Field | Meaning |
| --- | --- |
| `schemaVersion` | Always `2` for this tree |
| `gameVersion` | `forever` |
| `source` | `AllTheThings` |
| `commit` | Git SHA of the ATT revision used to build the export |
| `generatedAt` | RFC3339 time taken from ATT’s latest commit timestamp at build time |
| `locale` | `enUS` |
| `questCount` | Number of quests in the published set |
| `shards` | List of `{ path, idRange, questCount, sha256 }` under `export/forever/quests/` |
| `parse` | Optional. Present when coordinate parsing reported issues |
| `parse.unmappedLocations` | Count of unmapped map constant reports |
| `parse.issues` | Array of `{ kind, questId, npcId?, file, reason }` |

There is no `sources` object and no `merge` block.

## Removed fields (Questie + ATT merge era)

If your consumer still reads these, update it:

| Old field | Replacement |
| --- | --- |
| `source: "Questie/QuestieDB"` | `source: "AllTheThings"` |
| `commit` (Questie SHA) | `commit` (ATT SHA only) |
| `sources.questie` / `sources.att` | Single `commit` (ATT) |
| `merge.attFilledSpawns`, `merge.coordinateConflicts`, … | Dropped. Use `parse.issues` only for known ATT map gaps |
| ~5000 quests (full Questie Forever set) | ~2000+ quests (ATT Forever catalog only) |

## Quest body (unchanged schema version)

Shard files still use `{ "schemaVersion": 2, "quests": { "<id>": { ... } } }`.

Notable fields for automation:

- **`startedBy`** / **`finishedBy`**: `{ type, id, spawns? }` with `type` one of `npc`, `object`, or `item`.
- **`places`**: `{ role, type, id, spawns, itemId?, name? }` with `role` in `available`, `turnIn`, `objective`, `trigger`, `extra`.
- **`preQuestGroup`** / **`preQuestSingle`**: prerequisites.
- **`indexes/starters.json`**: keys like `npc|123`, `object|456`, `item|789` → quest id lists.

Item-started quests appear when ATT declares `provider = { "i", ... }` (or equivalent). The export does not resolve item drop sources the way the old QuestieDB pipeline did unless ATT encodes those objectives and locations.
