# Repository instructions

Use the matching shared skills for implementation, debugging, testing, review,
and Git delivery. Read anti-ai-writing before drafting human-facing text.

## Follow the source workflow

- For Wowhead changes, follow [INGESTION.md](docs/INGESTION.md#wowhead-forever),
  including retries, canonical compilation, and the separate zone bundles.
- For client extraction, packaging, import, and cleanup, follow
  [CLIENT_EXTRACTION.md](docs/CLIENT_EXTRACTION.md). Preserve retained snapshots.
- Run applicable checks from [DEVELOPMENT.md](docs/DEVELOPMENT.md). Report missing
  prerequisites and incomplete checks; do not describe a partial suite as passing.

Keep commands, snapshot counts, and temporary source limitations in those docs.
Regenerate derived products through their tools rather than editing them directly.

## Preserve data evidence

- Preserve source provenance, explicit field states, and partial-extraction
  limitations. Follow [SCHEMAS.md](docs/SCHEMAS.md) and
  [RECONCILIATION.md](docs/RECONCILIATION.md) when changing canonical data.
- Distinguish saved-page coverage, successful refreshes, API-resolved IDs, and
  observed in-game availability. Missing records and failed lookups do not prove
  deletion; title resolution does not prove availability.
- Check fetch failures and stale records before declaring a refresh complete.
  File counts and exit status alone are insufficient.
- Investigate unexpected catalog identity, coverage changes, and validation
  failures before changing snapshot expectations. Do not weaken validation to
  accept a response for the wrong catalog. Document any retained older source.

## Keep changes scoped

Preserve unrelated work. Commit, push, or publish only with explicit authorization.
