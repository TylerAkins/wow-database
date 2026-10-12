# Automation

`.github/workflows/database-update.yml` is the single daily and manually
dispatchable database workflow. It imports ATT Forever records, QuestieDB Era
and Forever records, and the existing Wowhead snapshot when that cache is
present. It validates canonical
data, compiles each flavor that has a source directory, and refreshes the legacy
ATT export. Flavors without source datasets are skipped. QuestieDB imports
are also available locally. The workflow never runs the Wowhead scraper or
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

Local source refresh commands are in [INGESTION.md](INGESTION.md). The local
client extract is not copied into the automated branch.
