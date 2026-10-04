# WoW Database

Published Forever quest data for other repositories to probe. QuestieDB is the only source. The scheduled workflow refreshes the export. You do not run a scraper locally.

`schemaVersion` is 2. The previous Wowhead tree is gone, including `data/forever/raw`, `data/forever/compiled`, `data/forever/questie`, and `data/forever/reports`. Start at `export/forever/manifest.json`.

## Layout

| Path | Purpose |
| --- | --- |
| `export/forever/manifest.json` | Probe document: QuestieDB commit, quest count, shard paths, and sha256 |
| `export/forever/quests/` | Quest bodies, at most 500 quests per file |
| `export/forever/indexes/zones.json` | `zoneOrSort` to quest ids |
| `export/forever/indexes/starters.json` | `npc\|id`, `object\|id`, or `item\|id` to quests that start there |
| `export/forever/indexes/finishers.json` | Same key shape for turn-in providers |
| `export/forever/indexes/chains.json` | Prerequisite and follow-up ids |

Each quest is stored once. `places` says where it is available, where it turns in, and where its objectives are. Coordinates are `[zoneId, x, y]`. `preQuestGroup` means every listed quest is required. `preQuestSingle` means any one is enough. `questType` carries repeatable, event, daily, weekly, monthly, raid, dungeon, battleground, profession, and sort.

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

[`.github/workflows/update-questiedb.yml`](.github/workflows/update-questiedb.yml) runs every day at 11:00 UTC (5:00am CST, 6:00am CDT) and can be started with `workflow_dispatch`. It checks out QuestieDB `master`, exports the Forever flavor with LuaJIT, compiles `export/forever`, and opens a pull request when the tree changes. The pull request records the manifest commit and quest count from before the compile.

Before the first scheduled pull request can open, the repository needs:

1. Settings, Actions, General, Workflow permissions: Read and write permissions.
2. The same page: Allow GitHub Actions to create and approve pull requests.

The workflow creates the pull request. It does not approve it.

## Development

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for the optional local commands.

## License and attribution

The schema, compiler, and arrangement are proprietary. See [LICENSE](LICENSE) and [ATTRIBUTION.md](ATTRIBUTION.md).
