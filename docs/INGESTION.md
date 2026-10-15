# Ingestion

Every adapter writes one versioned snapshot beneath `data/forever/sources/<source>/`.
Adapters run offline against a local checkout or cache. The QuestieDB adapter
executes the checkout’s registered static providers with Lua 5.1. Adapters do not
download DB2 definitions or scrape Wowhead in automation.

## Daily update

`.github/workflows/database-update.yml` checks out AllTheThings, imports the
supported Forever ATT records, imports QuestieDB Forever, and normalizes
the existing Wowhead Forever cache when it is present. It validates canonical snapshots, compiles Forever, and refreshes the legacy ATT export. One pull request contains
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
  --flavor forever --out data/forever/sources/questiedb
```

ATT follows the loader’s shared definitions, Forever configuration, and Forever
data files, including `zzOLD`, with Forever preprocessor branches applied. It
imports supported quest declarations and selected explicit NPC, item, object,
and achievement entries. It retains source comments as labels,
literal names/descriptions/levels/flags, quest relationships, and nested entity
containment links. It does not evaluate arbitrary Lua or claim a complete ATT
entity catalog. Repeated quest declarations retain paths, lines, and observations;
unsupported expressions remain unresolved evidence. Literal ancestor restrictions
are inherited. `lvl` maps to minimum `requiredLevel`, `requireSkill` to a profession
gate, and `qi`/`qis` remain separate from item starters and objective items.
QuestieDB imports generated Quest, NPC, Item, and Object tables
for Forever. Its generated tables include the converted Classic baseline. The
importer checks that every upstream Classic quest ID is retained. It applies
registered static corrections in registry order, including generated Forever
delta-base and trace providers and converted legacy copies. Field provenance
identifies the provider that supplied the effective static value. Explicit clears
remain distinct from missing fields. Dynamic Corrections, Derived Passes,
localization, and getter defaults are not evaluated. These stages remain limitations
in the snapshot manifest. Lua 5.1 must be available as `lua5.1` or through `LUA51`;
other Lua versions are rejected. Run the adapter locally to reproduce the scheduled
snapshot.

## Local Forever client

The client workflow is local acquisition followed by a retained compressed
snapshot and canonical import. The complete sequence, installation/product
selection, WoWDBDefs setup, output paths, verification, and temporary-export
cleanup are documented in [CLIENT_EXTRACTION.md](CLIENT_EXTRACTION.md).

Once the compressed snapshot exists, rebuild without extracting again:

```sh
go run ./cmd/ingest-client --input data/forever/client-extract
go run ./cmd/database compile --flavor forever
go run ./cmd/database validate --root data
```

The canonical adapter maps selected entity and quest support tables. QuestV2
supplies quest IDs and UI metadata; support row IDs retain separate table-prefixed
identities. Other tables remain in the compressed snapshot. Encrypted sections
remain partial, and unresolved source conflicts are preserved. Only the temporary
`.local/foreverclient/` export is purged after successful verification; retained
snapshots and compiled data remain available.

## Compilation normalization

Source snapshots retain their source representations. Shared consumer fields are
normalized during compilation, with every original observation retained in
`attributes.sourceData`. Refreshing a source does not change the normalization
contract. See [RECONCILIATION.md](RECONCILIATION.md) before consuming format 2.
