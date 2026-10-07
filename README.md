# WoW Database

Published Forever quest data for other repositories to probe. [AllTheThings](https://github.com/ATTWoWAddon/AllTheThings) is the **only** source: a Go compiler reads ATT’s `.contrib/.db/forever` quest declarations and map constants. The daily workflow checks out upstream ATT, rebuilds `export/forever`, and opens an `att-update` pull request. You do not run a scraper or QuestieDB locally.

`schemaVersion` is 2. Legacy trees under `data/forever/*` are removed. Start at `export/forever/manifest.json`.

**Consumer note:** The manifest no longer has `sources` or `merge` from the old QuestieDB + ATT merge. See [docs/MANIFEST.md](docs/MANIFEST.md) for field definitions and migration.

## What is in the export

- **Quest set:** Quests ATT catalogs for Forever (~2k ids), not the full in-game quest list (~5k+ on the old QuestieDB export).
- **Names:** Mostly from ATT `q(id, { -- Title` comments (or rare `name =` overrides).
- **Starters:** `startedBy` and `indexes/starters.json` use `npc`, `object`, or `item` when ATT sets `qg`, `provider`, etc. Item-start quests are included when ATT marks them; drop/vendor resolution is only what ATT encodes in objectives and places.
- **Coordinates:** `[zoneId, x, y]` with zone ids mapped from ATT `MAP.*` constants where possible. Gaps are listed in `manifest.parse.issues`.

## Layout

| Path | Purpose |
| --- | --- |
| `export/forever/manifest.json` | Probe: ATT `commit`, `questCount`, shards, sha256, optional `parse.issues` |
| `export/forever/quests/` | Quest bodies, at most 500 quests per file |
| `export/forever/indexes/zones.json` | `zoneOrSort` → quest ids |
| `export/forever/indexes/starters.json` | `npc\|id`, `object\|id`, or `item\|id` → quests that start there |
| `export/forever/indexes/finishers.json` | Same key shape for turn-in providers |
| `export/forever/indexes/chains.json` | Prerequisite and follow-up ids |

Each quest is stored once. `places` covers available, turn-in, and objective locations. `preQuestGroup` means every listed quest is required; `preQuestSingle` means any one is enough. `questType` carries repeatable, event, daily, weekly, monthly, raid, dungeon, battleground, profession, and sort.

## Probe

Fetch the manifest first. Use its shard list and sha256 values to decide what else to download.

Public checkout of the manifest and one shard:

```yaml
- uses: actions/checkout@v7.0.1
  with:
    repository: TylerAkins/wow-database
    sparse-checkout: |
      export/forever/manifest.json
      export/forever/quests/0001.json
    sparse-checkout-cone-mode: false
```

Private checkout uses a fine-grained personal access token with contents read on this repository, stored as a secret in the consumer repository:

```yaml
- uses: actions/checkout@v7.0.1
  with:
    repository: TylerAkins/wow-database
    token: ${{ secrets.WOW_DATABASE_TOKEN }}
    sparse-checkout: |
      export/forever/manifest.json
    sparse-checkout-cone-mode: false
```

`raw.githubusercontent.com` does not serve a private repository without that token. This workflow does not change repository visibility.

## Updates

[`.github/workflows/database-update.yml`](.github/workflows/database-update.yml) runs daily at 11:00 UTC and on `workflow_dispatch`. It checks out AllTheThings, runs `go run ./cmd/compile` with `--att-root`, runs `go test ./...`, and updates the `att-update` branch. The pull request records the ATT commit and any `parse.issues`. The workflow does not merge its pull request.

Repository settings required before the first PR:

1. Actions → General → Workflow permissions: **Read and write**
2. Same page: **Allow GitHub Actions to create and approve pull requests**

## Development

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for local compile commands and ATT field mapping. See [docs/MANIFEST.md](docs/MANIFEST.md) for manifest and migration details.

## License and attribution

The schema, compiler, and arrangement are proprietary. Quest and location data come from AllTheThings (MIT). See [LICENSE](LICENSE) and [ATTRIBUTION.md](ATTRIBUTION.md).
