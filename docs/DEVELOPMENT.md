# Development

## Requirements

- Python 3.12 or newer
- Go 1.27 or newer for canonical ingestion, compilation, and local client extraction
- Lua 5.1 for QuestieDB static-provider import and integration tests (`lua5.1` on PATH, or `LUA51` set to its executable)
- No third-party Python packages for compilation or offline tests
- Playwright and local Chrome only when using `fetch_quest_pages.py --browser`

## Tests and generated-data checks

The QuestieDB runtime integration test uses `.cache/QuestieDB` and skips when that
checkout is absent. CI checks it out before running the suite. Local runs need Lua
5.1 plus the checkout to exercise provider order, operations, and effective values.

```bash
python3 -m unittest discover -s tests -p 'test_*.py'
python3 tools/compile_zone_files.py --check
GOCACHE=/private/tmp/wow-database-go-cache go test ./...
GOCACHE=/private/tmp/wow-database-go-cache go vet ./...
```

Regenerate all committed bundles after canonical data changes:

```bash
python3 tools/compile_zone_files.py
```

The compiler validates all selected inputs before replacing output. A full run replaces the complete zone directory, removing stale files. A targeted `--zone` run preserves bundles for other zones. `--check` never writes.

## Wowhead snapshot invariants

- 5,058 indexed quests
- 5,057 quest detail records
- 330 object records
- One item record
- 59 zone catalogs with complete detail coverage
- Quest `7507` (`Nostro's Compendium`) is the documented non-zone gap because its old Wowhead name redirect-loops

## Refreshing raw data

The scraper defaults to `data/forever/raw/`. Network refreshes are intentionally local/manual and are not run by CI.

```bash
python3 tools/fetch_quest_pages.py ingest --browser --force \
  --url 'https://www.wowhead.com/forever/quests/kalimdor/durotar'
python3 tools/fetch_quest_pages.py sync-quests --browser --delay 5 \
  --batch-size 10 --batch-pause 15
```

Run the offline tests and regenerate the compiled files after any refresh. See [quest-database.md](quest-database.md) for the complete source catalog and specialized sync commands.

The scheduled workflow uses Go 1.27.2. See [CLIENT_EXTRACTION.md](CLIENT_EXTRACTION.md)
for the local client workflow and its current format limits.

## Local client workflow

[CLIENT_EXTRACTION.md](CLIENT_EXTRACTION.md) covers definitions setup, extraction,
packaging, import, validation, compilation, and purging the temporary export.
Use the retained `data/forever/client-extract/` snapshot for offline rebuilds.
The client packager and importer tests use small fixtures and do not require
a game installation. Tables skipped because of encryption remain explicit
coverage limits; source conflicts are preserved by compilation.
