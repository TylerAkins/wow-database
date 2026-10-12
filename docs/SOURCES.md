# Data sources

Source identity stays separate from game flavor. Canonical snapshots live at
`data/<flavor>/sources/<source>/` and record their source revision and limits.

| Source | Ingestion | Current supported data | Revision and limits |
|---|---|---|---|
| Wowhead Forever | Local, manual Python scraper plus offline adapter | Existing quest indexes/details, object and item details, zone-to-map crosswalks | The adapter is offline and preserves missing detail status. The scheduled job does not scrape Wowhead. |
| All The Things | Daily workflow or local Go adapter | Supported Forever quest declarations plus explicit NPC, item, object, and achievement declarations, selected literal fields, quest providers/prerequisites/chains, and nested containment links | Records exact upstream commit. ATT is a nested catalog, not a complete entity database. Unrecognized fields/expressions, entity coordinates, standalone map/zone definitions, and unsupported quest-provider coordinates remain outside the adapter. |
| QuestieDB | Daily workflow or local Python adapter | Generated Forever Quest, NPC, Item, and Object tables, including the converted Classic baseline; Forever-owned static correction entries are preserved | Forever is the only flavor; each compiled entity type and ID appears once. Corrections are not applied; dynamic corrections, support bundles, and derived passes are not imported. Owner permission to proceed was confirmed by the repository maintainer on 2026-10-11; daily automation imports Forever. |
| Forever client | Local Go extractor and canonical adapter | Selected Achievement, AreaTable, Creature, Faction, ItemSparse, Map, and SpellName DB2 tables | Exact local client build and definition hashes are recorded. Encrypted sections are skipped or marked partial. Local outputs are not automatically published. |

## Client categories

Client tables may contain static quest, map, area, NPC, item, spell, faction,
and achievement definitions. Availability depends on the installed build, file
presence, local `.dbd` definitions, and encryption. These are not equivalent
to complete server-side entities. NPC spawn positions, loot tables, and all
live quest-availability rules cannot be inferred from the client extractor
alone.

## Licensing

Repository code remains GPL-3.0 because the available history does not
establish relicensing rights for all existing contributions. The copied Go
parsers retain their upstream MIT and BSD-3-Clause notices in
`tools/db2tool/NOTICES.md`. WoWDBDefs definitions are not bundled; its
definition data has separate CC BY-SA terms. Blizzard, Wowhead, ATT, and
QuestieDB data remain subject to their own rights and terms. See
[ATTRIBUTION.md](../ATTRIBUTION.md) and [LICENSE](../LICENSE).
