# Forever raw quest database

Canonical scrape artifacts for the Forever version. These files are the inputs to `tools/compile_zone_files.py` and should not be edited by hand.

| Path | Purpose |
|------|---------|
| `manifest.json` | Schema version, source timestamps, and aggregate statistics |
| `quest_index.json` | Global record keyed by quest ID and merged across source catalogs |
| `sources/*.json` | Raw list snapshots for zones, classes, instances, events, professions, PvP, and objects |
| `details/<id>.json` | Per-quest map data, flags, eligibility, start pins, and prerequisites |
| `object_index.json` | Object names and source locations |
| `object/<id>.json` | Object spawn coordinates and linked quest IDs |
| `item/<id>.json` | Item spawn coordinates and linked quest IDs |
| `zone_starters.json` | Zone item/object starters and map quest givers |
| `zone_ui_map_ids.json` | Zone ID to Blizzard UiMapID reference |
| `attunement_quest_ids.json` | Legacy attunement classification seed retained with the snapshot |
| `pin_categories.json` | Pin classification reference |

The snapshot contains 5,058 indexed quests and 5,057 detail files. Quest `7507` is the known non-zone gap because its old Wowhead name redirect-loops. Every quest in the 59 zone catalogs has a detail record.

See `docs/quest-database.md` for refresh commands and the complete source list.
