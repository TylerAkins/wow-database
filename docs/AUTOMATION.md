# Automation

`.github/workflows/database-update.yml` is the single daily and manually
dispatchable database workflow. It imports ATT and QuestieDB Forever records
and the existing Wowhead snapshot when that cache is present. It validates
canonical data, compiles the Forever database, and refreshes the legacy
ATT export. It also includes the committed canonical client snapshot in the
compilation. QuestieDB imports are also available locally. The workflow never runs the Wowhead scraper or
local client extractor.

One stable `takins/database-update` branch is reused for the generated pull
request. Before opening or updating it, `tools/semantic_changes.py` compares
canonical snapshots, compilations, and the legacy ATT export with `origin/main`,
ignoring revision-only metadata. If data values are unchanged, the job does not
open a pull request and closes an existing update PR that has become empty. The
workflow does not merge its pull request.

Before compiling, `tools/coverage_guard.py` checks each existing source
snapshot against its baseline manifest. The minimum retained record share is
configured in `schemas/validation.json`; larger drops fail the job for review.

Local source refresh commands are in [INGESTION.md](INGESTION.md). Client
acquisition, packaging, and import run locally; the scheduled job does not refresh
the client snapshot. Removing the temporary export after verification does not
affect scheduled compilation. See [CLIENT_EXTRACTION.md](CLIENT_EXTRACTION.md)
for setup and cleanup.
