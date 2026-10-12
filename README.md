# WoW Database

Reusable World of Warcraft quest data gathered from Wowhead. The repository keeps canonical scrape artifacts separate from deterministic compiled outputs so consumers do not need to understand the scraper's storage layout.

The initial Wowhead dataset contains the **Forever** game version and was migrated from [`TylerAkins/forever-quest-markers`](https://github.com/TylerAkins/forever-quest-markers) at commit `43f22e73ff3bf19f564d20d1e6846b0d57dbd3de`. This repository is being rebuilt as a multi-source data platform. Original source snapshots remain separate from generated exports.

## Layout

| Path | Purpose |
|------|---------|
| `data/forever/raw/` | Canonical list snapshots, indexes, quest details, and object/item records |
| `data/forever/compiled/zones/` | Consumer-ready quest bundles grouped by Wowhead zone catalog |
| `tools/fetch_quest_pages.py` | Local/manual Wowhead ingestion and synchronization CLI |
| `tools/compile_zone_files.py` | Deterministic zone compiler and drift checker |
| `tools/quest_db/` | Parsing, classification, synchronization, and compilation modules |
| `cmd/client-extract/` | Local-only Go extractor for client CASC and WDC5 DB2 data |
| `cmd/ingest-client/` | Local adapter from supported client tables to canonical records |
| `cmd/database/` | Canonical dataset validation, compilation, and comparison |
| `data/forever/client-extract/` | Checked compressed client DB2 tables and checksum index |
| `data/<flavor>/sources/` | Versioned canonical source snapshots |
| `data/<flavor>/compilation/` | Multi-source merged datasets and conflict reports |
| `cmd/compile/` | Go exporter for supported All The Things records |
| `export/forever/` | Deterministic ATT-derived Forever quest export |

## Sources and workflows

Wowhead scraping is local and manual. Client extraction is local and manual.
Forever is the only game flavor. Its database includes the Classic Era baseline
and Forever additions, merged by entity type and ID. The scheduled workflow
imports ATT and QuestieDB Forever. It
normalizes the existing Wowhead cache when present, validates canonical
snapshots, compiles Forever, and
refreshes the legacy ATT export. QuestieDB imports are also available locally. See
[docs/INGESTION.md](docs/INGESTION.md) for commands and
[docs/CLIENT_EXTRACTION.md](docs/CLIENT_EXTRACTION.md) for local DB2 use.

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

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for tests, refresh commands, invariants, and known coverage gaps. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the rebuild plan and current data flow.

## License and attribution

Repository code is GPL-3.0. Wowhead and World of Warcraft data retain their respective ownership; see [ATTRIBUTION.md](ATTRIBUTION.md).
