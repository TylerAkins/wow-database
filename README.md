# WoW Database

Reusable World of Warcraft quest data gathered from Wowhead. The repository keeps canonical scrape artifacts separate from deterministic compiled outputs so consumers do not need to understand the scraper's storage layout.

The initial dataset contains the **Forever** game version and was migrated from [`TylerAkins/forever-quest-markers`](https://github.com/TylerAkins/forever-quest-markers) at commit `43f22e73ff3bf19f564d20d1e6846b0d57dbd3de`.

## Layout

| Path | Purpose |
|------|---------|
| `data/forever/raw/` | Canonical list snapshots, indexes, quest details, and object/item records |
| `data/forever/compiled/zones/` | Consumer-ready quest bundles grouped by Wowhead zone catalog |
| `tools/fetch_quest_pages.py` | Local/manual Wowhead ingestion and synchronization CLI |
| `tools/compile_zone_files.py` | Deterministic zone compiler and drift checker |
| `tools/quest_db/` | Parsing, classification, synchronization, and compilation modules |

## Compiled zone files

Generate every zone:

```bash
python3 tools/compile_zone_files.py
```

Generate only Durotar while preserving the other compiled files:

```bash
python3 tools/compile_zone_files.py --zone durotar
```

Verify that committed output is current without writing files:

```bash
python3 tools/compile_zone_files.py --check
```

Each file contains source metadata and a `quests` object keyed by quest ID. Every quest retains its complete global `index` record and full `detail` record. Zone membership comes only from the corresponding Wowhead zone catalog, not from coordinate inference.

## Development

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for tests, refresh commands, invariants, and the known quest-detail gap.

## License and attribution

Repository code is GPL-3.0. Wowhead and World of Warcraft data retain their respective ownership; see [ATTRIBUTION.md](ATTRIBUTION.md).
