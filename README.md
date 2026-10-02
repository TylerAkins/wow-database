# WoW Database

Reusable World of Warcraft quest data gathered from Wowhead. The repository keeps canonical scrape artifacts separate from deterministic compiled outputs so consumers do not need to understand the scraper's storage layout.

The initial dataset contains the **Forever** game version and was migrated from [`TylerAkins/forever-quest-markers`](https://github.com/TylerAkins/forever-quest-markers) at commit `43f22e73ff3bf19f564d20d1e6846b0d57dbd3de`.

## Layout

| Path | Purpose |
|------|---------|
| `data/forever/raw/` | Canonical list snapshots, indexes, quest details, and object/item records |
| `data/forever/questie/quests.json` | Pinned effective QuestieDB records for quests first seen in Forever |
| `data/forever/compiled/zones/` | Consumer-ready quest bundles grouped by Wowhead zone catalog |
| `data/forever/compiled/collections/` | Zephras Isle and New in Forever quest collections |
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

## QuestieDB Forever overlay

The 820 quests with `list.firstseenpatch == 16001` include all 114 Zephras Isle quests. Their effective QuestieDB fields are pinned in `data/forever/questie/quests.json` at commit `cac1eff815923f896d082764d023cf812454a023`. The importer keeps the original Wowhead `list` and quest-page markup intact, puts every available QuestieDB quest field and starter/finisher spawn in `detail.questie`, and updates matching effective index/detail fields. QuestieDB has no record for 85 of the 820 IDs; those retain their existing data.

QuestieDB returns `0` for missing numeric fields on a known quest. The snapshot retains those zeros for an exact API record; the importer does not use them to erase an existing nonzero level or eligibility restriction.

`detail.requirements.allOf` holds grouped prerequisites. `detail.requirements.anyOf` holds alternative prerequisites. The legacy `prerequisiteQuestIds` field is updated only when it can express the QuestieDB requirement without losing that distinction. The [field-level CSV](data/forever/reports/questie-forever-changes.csv) records old and new values plus likely ATT fields; the [summary](data/forever/reports/questie-forever-summary.md) lists missing records and QuestieDB verification comments. These are review inputs, not edits to ATT.

Reapply the committed snapshot after a Wowhead refresh:

```bash
python3 tools/import_questie.py --dry-run
python3 tools/import_questie.py
python3 tools/compile_zone_files.py
```

To reproduce the snapshot from a checkout at the pinned QuestieDB commit, run `python3 tools/import_questie.py --questiedb /path/to/QuestieDB`. This requires LuaJIT or another Lua 5.1-compatible interpreter. Normal snapshot reapplication and compilation use only Python's standard library.

## Development

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for tests, refresh commands, invariants, and the known quest-detail gap.

## License and attribution

The repository's original code, documentation, schema, and database compilation are proprietary and **All Rights Reserved**. No permission is granted to use, copy, modify, or redistribute them. Wowhead, Blizzard, and other third-party data retain their respective ownership; see [LICENSE](LICENSE) and [ATTRIBUTION.md](ATTRIBUTION.md).
