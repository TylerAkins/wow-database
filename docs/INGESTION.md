# Ingestion

Every adapter writes one versioned snapshot beneath `data/<flavor>/sources/<source>/`.
Adapters run offline against a local checkout or cache. They do not execute
upstream Lua, download DB2 definitions, or scrape Wowhead in automation.

## Daily update

`.github/workflows/database-update.yml` checks out AllTheThings, imports the
supported Forever ATT records, imports QuestieDB Era and Forever, and normalizes
the existing Wowhead Forever cache when it is present. It validates canonical snapshots, compiles Era and
Forever, and refreshes the legacy ATT export. One pull request contains
generated source snapshots and products. The workflow does not run the
client extractor or contact Wowhead.

## Wowhead Forever

Wowhead fetching remains local and manual. Conservative request pacing is the
default expectation for detail refreshes:

```sh
python3 tools/fetch_quest_pages.py sync-quests --browser \
  --delay 5 --batch-size 10 --batch-pause 15
```

Refresh a selected page with force enabled only when replacing its cached copy
is intentional:

```sh
python3 tools/fetch_quest_pages.py ingest --browser --force \
  --url 'https://www.wowhead.com/forever/quests/kalimdor/durotar'
```

After ingestion, normalize the local snapshot, validate it, and compile:

```sh
python3 tools/wowhead/import_snapshot.py
go run ./cmd/database validate --root data
go run ./cmd/database compile --flavor forever
```

The importer reads `data/forever/raw/` only and makes no network requests. Its
canonical dataset may contain the same licensed content as the input cache;
review source terms and repository data policy before publishing it. Missing
detail pages remain explicit as unavailable.

## AllTheThings and QuestieDB

The adapters accept local Git checkouts and record their exact revision. ATT
and QuestieDB both run in public automation:

```sh
go run ./cmd/ingest-att --att-root .cache/AllTheThings
python3 tools/questiedb/import_source.py --repo .cache/QuestieDB \
  --flavor era --out data/era/sources/questiedb
python3 tools/questiedb/import_source.py --repo .cache/QuestieDB \
  --flavor forever --out data/forever/sources/questiedb
```

ATT imports supported Forever quest declarations and selected explicit NPC,
item, object, and achievement entries. It retains source comments as labels,
literal names/descriptions/levels/flags, quest relationships, and nested entity
containment links. It does not evaluate arbitrary Lua or claim a complete ATT
entity catalog. QuestieDB imports generated Quest, NPC, Item, and Object tables
for Era and Forever. Era imports preserve Era and Shared static corrections.
Forever imports preserve its owned corrections, including converted legacy copies. Corrections are
preserved without applying them. Dynamic corrections, support data, and derived
runtime passes are reported as limitations in each snapshot manifest. Run the
QuestieDB adapter locally to reproduce or inspect the scheduled snapshots.

## Local Forever client

The Go extractor reads a local client install and local WoWDBDefs checkout. It
does not download either one and writes to `.local/` by default. Import the
supported DB2 tables into a canonical Forever snapshot with:

```sh
go run ./cmd/client-extract --dbd-dir .cache/WoWDBDefs
go run ./cmd/ingest-client --input .local/foreverclient \
  --out data/forever/sources/foreverclient
go run ./cmd/database validate --root data
go run ./cmd/database compile --flavor forever
```

The canonical adapter currently maps Achievement, AreaTable, Creature, Faction,
ItemSparse, Map, and SpellName. Other extracted tables stay in the local
extractor output and are listed as importer limitations. Partial encrypted
tables are marked on imported entities. Do not publish local client output
without reviewing the applicable redistribution terms. See
[CLIENT_EXTRACTION.md](CLIENT_EXTRACTION.md) for build-specific limitations.
