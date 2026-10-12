# WoW Database

Reusable WoW Forever data from All The Things, QuestieDB, Wowhead, and the local
client. Quests, NPCs, items, objects, maps, spells, and other supported records
keep source provenance and compile into one Forever database.

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
| `data/forever/sources/` | Versioned canonical source snapshots |
| `data/forever/compilation/` | Multi-source merged datasets and conflict reports |
| `cmd/compile/` | Go exporter for supported All The Things records |
| `export/forever/` | Deterministic ATT-derived Forever quest export |

## Sources and workflows

Wowhead scraping is local and manual. Client extraction is local and manual.
Forever is the only game flavor. Its database includes the Classic Era baseline
and Forever additions, merged by entity type and ID. The scheduled workflow
imports ATT and QuestieDB Forever, normalizes the existing Wowhead cache when
present, validates canonical snapshots, compiles Forever, and refreshes the
legacy ATT export. The compilation also uses the committed client snapshot.
QuestieDB imports are also available locally. See
[docs/INGESTION.md](docs/INGESTION.md) for commands and
[docs/CLIENT_EXTRACTION.md](docs/CLIENT_EXTRACTION.md) for local DB2 use.

## Local client data

Follow [CLIENT_EXTRACTION.md](docs/CLIENT_EXTRACTION.md) to obtain WoWDBDefs,
extract the installed client, package and verify its tables, import supported
records, compile Forever, and purge the temporary export. The retained compressed
snapshot is under `data/forever/client-extract/`; rebuilding it does not require
the game installation:

```sh
go run ./cmd/ingest-client --input data/forever/client-extract
go run ./cmd/database compile --flavor forever
go run ./cmd/database validate --root data
```

QuestV2 contributes quest IDs and client UI metadata. Its IDs merge with other
Forever sources without duplicates; quest details and source conflicts remain
subject to the limits documented in [COMPILATION.md](docs/COMPILATION.md).

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
