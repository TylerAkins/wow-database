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

Run these steps from the repository root. Python, Go, and browser prerequisites
are listed in [DEVELOPMENT.md](DEVELOPMENT.md). Fetching is local and manual;
the scheduled workflow only imports the existing cache. Leave the Chrome window
alone while a browser command runs, and wait for each command to finish before
starting the next.

Check the current snapshot limitations in
[DEVELOPMENT.md](DEVELOPMENT.md#wowhead-snapshot-invariants). A successful fetch
can still return the wrong catalog. If zone compilation reports multiple zone
IDs for a single-zone source, inspect the fetched page's title and canonical URL
before replacing the previous snapshot or changing compiler expectations.

### 1. Refresh catalogs, then quest details

For a fresh download of the configured source catalogs and all indexed quest
pages, run:

```sh
python3 tools/fetch_quest_pages.py sync-sources --browser --force \
  --delay 5 --batch-size 10 --batch-pause 15
python3 tools/fetch_quest_pages.py sync-quests --browser --force \
  --delay 5 --batch-size 10 --batch-pause 15
```

`sync-sources` refreshes the configured list pages and merges their quest and
object IDs into the indexes. It does not remove IDs absent from a refreshed page.
`sync-quests` fetches details for the resulting quest index; it does not refresh
the catalogs itself. `--force` replaces cached pages. Omit it from `sync-quests`
when you only want to fill missing detail files.

After refreshing catalogs alone, run `sync-quests` without `--force` to fill
details for newly discovered IDs before rebuilding. The zone compiler rejects
missing details for quests in zone catalogs.

For a single catalog, use `ingest --browser --force --url '<catalog URL>'`
instead of `sync-sources`, then continue with quest details and the steps below.

Zone starters and object spawn pages are separate inputs. If those also need a
refresh, run these after quest details and before rebuilding:

```sh
python3 tools/fetch_quest_pages.py sync-zones --browser --force \
  --delay 5 --batch-size 10 --batch-pause 15
python3 tools/fetch_quest_pages.py sync-objects --browser --force \
  --delay 5 --batch-size 10 --batch-pause 15
```

Run `sync-zones` before `sync-objects` because zone starters can add object IDs.
Object synchronization attaches spawn information to quest detail files.
Standalone item pages require explicit `--item ID` arguments to `sync-objects`.
See [quest-database.md](quest-database.md#run-it-again) for targeted commands.

### 2. Check failures and retry affected pages

A catalog fetch failure stops `sync-sources`; earlier pages may already have been
updated. Detail synchronization logs fetch failures and continues, so a zero exit
status does not guarantee that every detail page refreshed.

Inspect the command output and `data/forever/raw/fetch_errors.jsonl`. The error
log is append-only and can include failures from earlier runs. For quest details,
`stats.questDetailFetchErrors` in `data/forever/raw/manifest.json` counts failures
from the latest `sync-quests` invocation, including a targeted retry. Existing
quest detail files survive failed fetches, so file counts alone do not prove a
successful refresh. Compare their `fetchedAt` values when needed.

Retry failed quest IDs with repeated `--quest` arguments, for example:

```sh
python3 tools/fetch_quest_pages.py sync-quests --browser --force \
  --quest 81738 --quest 95189 \
  --delay 5 --batch-size 10 --batch-pause 15
```

Use the IDs from your failed run. Retain `--force` when retrying a failed refresh
of an existing file. For blocked browser access, restore access and rerun the
failed command; the scraper stops rather than saving challenge pages.

### 3. Import and rebuild both products

After fetching and retries finish, run:

```sh
python3 tools/wowhead/import_snapshot.py
go run ./cmd/database validate --root data
go run ./cmd/database compile --flavor forever
go run ./cmd/database validate --root data
python3 tools/compile_zone_files.py
```

The importer reads `data/forever/raw/` without network requests. The Go compiler
writes the merged database to `data/forever/compilation/`. The Python compiler
separately writes Wowhead zone bundles to `data/forever/compiled/zones/`; both
products need rebuilding after a raw refresh. The legacy ATT export does not
need rebuilding for a Wowhead-only change.

### 4. Verify and review the refresh

```sh
python3 -m unittest discover -s tests -p 'test_*.py'
python3 tools/compile_zone_files.py --check
python3 tools/coverage_guard.py --base origin/main
GOCACHE=/private/tmp/wow-database-go-cache go test ./...
GOCACHE=/private/tmp/wow-database-go-cache go vet ./...
git diff --check
git diff --stat
```

The coverage guard compares source record counts with the local `origin/main`
reference. The QuestieDB runtime test needs Lua 5.1 and `.cache/QuestieDB`, as
explained in [DEVELOPMENT.md](DEVELOPMENT.md).

Review changes to the raw cache, canonical Wowhead snapshot, zone bundles, and
merged compilation together. Inspect `data/forever/compilation/conflicts.md` and
`reconciliation.json` for unexpected changes. If verified coverage changes, update
the snapshot expectations in `tests/test_compile_zones.py` and the invariants in
[DEVELOPMENT.md](DEVELOPMENT.md#wowhead-snapshot-invariants), then rerun the tests.
Investigate count changes before changing expectations.

Missing detail pages remain explicit as unavailable. Canonical snapshots can
contain the same licensed content as the input cache; review source terms and
repository data policy before publishing.

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
