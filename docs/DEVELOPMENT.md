# Development

Production refreshes run in [`.github/workflows/database-update.yml`](../.github/workflows/database-update.yml). Local commands are for debugging the compiler or validating an ATT revision before merge.

## Requirements

- Go 1.22 or newer (see `go.mod`)
- A checkout of [AllTheThings](https://github.com/ATTWoWAddon/AllTheThings) with `.contrib/.db/forever`

LuaJIT and QuestieDB are **not** used.

## Repository layout

| Path | Role |
| --- | --- |
| `cmd/compile` | CLI: read ATT (or a test JSONL dump) and write `export/forever` |
| `internal/att` | Parse ATT Forever Lua (`q(...)`, coords, map constants) and build export rows |
| `internal/compile` | Publish shards, indexes, and `manifest.json` (schema version 2) |
| `export/forever` | Published tree consumed by other repos |

## Compile from ATT

```bash
cd /path/to/wow-database
ATT_COMMIT=$(git -C /path/to/AllTheThings rev-parse HEAD)
GENERATED_AT=$(git -C /path/to/AllTheThings log -1 --format=%cI)

go run ./cmd/compile \
  --att-root /path/to/AllTheThings \
  --att-commit "$ATT_COMMIT" \
  --generated-at "$GENERATED_AT" \
  --out export/forever

go run ./cmd/compile \
  --att-root /path/to/AllTheThings \
  --att-commit "$ATT_COMMIT" \
  --generated-at "$GENERATED_AT" \
  --out export/forever \
  --check

go test ./...
gofmt -l .
```

### Flags

| Flag | Required | Meaning |
| --- | --- | --- |
| `--att-root` | Yes (unless `--dump`) | ATT repo root |
| `--att-commit` | With `--att-root` | SHA recorded in `manifest.json` as `commit` |
| `--generated-at` | With `--att-root` | RFC3339 timestamp for `manifest.generatedAt` |
| `--out` | No (default `export/forever`) | Output directory |
| `--check` | No | Recompute export and byte-compare with `--out`; do not write |
| `--dump` | Test only | JSONL intermediate format (see below); skips ATT |

`--check` must match the published tree exactly, including stale-file detection (extra files under `export/forever` fail the check).

## What the ATT reader extracts

The exporter walks `.contrib/.db/forever` (skipping `zzOLD`) and parses literal `q(questId, { ... })` tables. It does **not** execute ATT’s Lua preprocessor.

| ATT (examples) | Published quest / index |
| --- | --- |
| Comment on `q(id, { -- Name` or `name = "..."` | `name` |
| `qg` / `qgs` / `provider` / `providers` | `startedBy`, `places` (`available`), usually same ids for `finishedBy` / `turnIn` |
| `provider = { "i", itemId }` | `startedBy` / `places` with `type: "item"` |
| `provider = { "o", objectId }` | `type: "object"` |
| `coord` / `coords` | Spawn tuples `[zoneId, x, y]` on matching providers |
| `sourceQuest` / `sourceQuests` | `preQuestSingle` / `preQuestGroup` |
| `altQuests` | `exclusiveTo` |
| `lvl` | `questLevel` |
| `races` / `classes` | `requiredRaces` / `requiredClasses` |
| `isDaily`, `isWeekly`, `repeatable`, … | `questType` flags |
| `groups` → `objective(...)` with `cr` / `provider` | `objectives`, `places` (`objective`) |
| `description` | `objectivesText` (single string in an array) |

Quest names usually come from the `-- Title` comment on the `q(...)` line. ATT does not ship full client quest text for every id the way QuestieDB did.

Non-literal coordinates (for example `coord = appendGroups(...)`) are skipped for that quest block instead of aborting the whole export. Unsupported syntax inside a literal coordinate table still fails with file and quest id.

Unmapped `MAP.*` constants produce `manifest.parse.issues` and empty spawns for that NPC; they do not stop the run.

## Test JSONL dump (`--dump`)

Unit tests feed the publisher without cloning ATT. Each line is one JSON object; the last line must be a summary:

```json
{"id":1,"fields":{"name":"Example"},"startedBy":[{"type":"npc","id":2,"spawns":[]}],"places":[]}
{"kind":"summary","questCount":1,"commit":"test","generatedAt":"2026-10-07T00:00:00Z"}
```

This path exists only for `go test` and local publisher checks. Production exports always use `--att-root`.

## Determinism

Two runs with the same ATT commit, `generatedAt`, and compiler revision must produce identical bytes under `export/forever`.
