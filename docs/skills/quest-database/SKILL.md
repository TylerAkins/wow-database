---
name: quest-database
description: Refresh and compile the Forever quest database from the URL list in docs/quest-database.md.
---

# Forever quest database

Canonical data lives in `data/forever/raw/`; consumer bundles live in `data/forever/compiled/zones/`. Quest detail and object pages should be fetched in local Chrome with `--browser`. Do not start a bulk browser download from a cloud agent.

## Look for changes

1. Read `data/forever/raw/manifest.json` and note `lastCheckedForChanges`.
2. Re-ingest the relevant list URLs from `docs/quest-database.md`.
3. Review changes under `data/forever/raw/`.
4. Fetch detail pages for new quest or object IDs.
5. Run the offline tests and regenerate compiled zone files.

```bash
python3 tools/fetch_quest_pages.py ingest --browser --force \
  --url 'https://www.wowhead.com/forever/quests/...'
python3 tools/fetch_quest_pages.py sync-quests --browser --force --quest 7507
python3 -m unittest discover -s tests -p 'test_*.py'
python3 tools/compile_zone_files.py
```

CI validates the committed compilation with `python3 tools/compile_zone_files.py --check` and performs no network requests.
