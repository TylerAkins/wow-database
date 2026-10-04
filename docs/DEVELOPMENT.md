# Development

## Requirements

- Python 3.12 or newer
- No third-party packages for compilation or offline tests
- Playwright and local Chrome only when using `fetch_quest_pages.py --browser`

## Tests and generated-data checks

```bash
python3 -m unittest discover -s tests -p 'test_*.py'
python3 tools/compile_zone_files.py --check
```

Regenerate all committed bundles after canonical data changes:

```bash
python3 tools/compile_zone_files.py
```

The compiler validates all selected inputs before replacing output. A full run replaces the complete zone directory, removing stale files. A targeted `--zone` run preserves bundles for other zones. `--check` never writes.

## Forever snapshot invariants

- 5,058 indexed quests
- 5,058 quest detail records
- 330 object records
- One item record
- 59 zone catalogs with complete detail coverage

## Refreshing raw data

The scraper defaults to `data/forever/raw/`. Network refreshes are intentionally local/manual and are not run by CI.

```bash
python3 tools/fetch_quest_pages.py ingest --browser --force \
  --url 'https://www.wowhead.com/forever/quests/kalimdor/durotar'
python3 tools/fetch_quest_pages.py sync-quests --browser --delay 5 \
  --batch-size 10 --batch-pause 15
```

Run the offline tests and regenerate the compiled files after any refresh. See [quest-database.md](quest-database.md) for the complete source catalog and specialized sync commands.

The pinned QuestieDB overlay is reapplied during list and detail ingestion. After a batch refresh, run `python3 tools/import_questie.py` and `python3 tools/compile_zone_files.py` to refresh both compiled collections and the zone bundles. `python3 tools/import_questie.py --dry-run` previews effective field changes without writing files.
