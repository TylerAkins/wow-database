# WoW Database

Published Forever quest data for other repositories to probe. [AllTheThings](https://github.com/ATTWoWAddon/AllTheThings) is the sole source: the Go compiler reads ATT's Forever quest declarations and map constants. The scheduled workflow refreshes from upstream ATT daily. You do not run a scraper locally.

`schemaVersion` is 2. The previous Wowhead tree is gone, including `data/forever/raw`, `data/forever/compiled`, `data/forever/questie`, and `data/forever/reports`. Start at `export/forever/manifest.json`.

## Layout

| Path | Purpose |
| --- | --- |
| `export/forever/manifest.json` | Probe document: ATT commit, quest count, shard paths, sha256, and parse issues |
| `export/forever/quests/` | Quest bodies, at most 500 quests per file |
| `export/forever/indexes/zones.json` | `zoneOrSort` to quest ids |
| `export/forever/indexes/starters.json` | `npc\|id`, `object\|id`, or `item\|id` to quests that start there |
| `export/forever/indexes/finishers.json` | Same key shape for turn-in providers |
| `export/forever/indexes/chains.json` | Prerequisite and follow-up ids |

Each quest is stored once. `places` says where it is available, where it turns in, and where its objectives are. Coordinates are `[zoneId, x, y]`. `preQuestGroup` means every listed quest is required. `preQuestSingle` means any one is enough. `questType` carries repeatable, event, daily, weekly, monthly, raid, dungeon, battleground, profession, and sort.

The published quest set is the set of quests ATT catalogs for Forever, not the full in-game quest list.

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

[`.github/workflows/database-update.yml`](.github/workflows/database-update.yml) refreshes the Forever export daily at 11:00 UTC and can be started manually with `workflow_dispatch`. Each run checks out the current AllTheThings revision, compiles `export/forever` with Go, runs `go test ./...`, and updates the existing `att-update` review pull request branch. The pull request records the ATT commit and any coordinate parse issues. The workflow does not merge its pull request.

Before the first scheduled pull request can open, the repository needs:

1. Settings, Actions, General, Workflow permissions: Read and write permissions.
2. The same page: Allow GitHub Actions to create and approve pull requests.

The workflow creates the pull request. It does not approve it.

## Development

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for the optional local commands.

## License and attribution

The schema, compiler, and arrangement are proprietary. See [LICENSE](LICENSE) and [ATTRIBUTION.md](ATTRIBUTION.md).
