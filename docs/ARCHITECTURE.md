# Architecture

## Current data flow

Source acquisition remains independent, then adapters converge the supported
records into canonical source snapshots:

```text
Wowhead Forever --local Python scraper--> data/forever/raw/
                 --offline adapter-------> data/forever/sources/wowhead/

All The Things --Go adapter-------------> data/forever/sources/allthethings/
QuestieDB ------Python adapter-----------> data/forever/sources/questiedb/
Installed client + local WoWDBDefs ------> .local/foreverclient/
                 --Python packager-----> data/forever/client-extract/
                 --Go adapter----------> data/forever/sources/foreverclient/

data/forever/sources/* --Go compiler--> data/forever/compilation/
All The Things -------------------------> export/forever/ (legacy product)
```

The Wowhead scraper and zone compiler remain from the restored baseline. The
offline adapter normalizes the existing cache. ATT exports supported Forever
quest fields and relationships plus selected explicit entity declarations and
containment links. QuestieDB imports its Forever generated entities, including the converted
Classic baseline, while preserving applicable unapplied corrections. Forever
is the only flavor imported by the daily workflow. The local client extractor and
importer cover a supported table subset. Source outputs remain separate until
the multi-source compiler merges them with provenance. Classic baseline quests
and Forever additions share one namespace; each compiled quest ID appears once.

## Restoration

The rebuild began with a restoration commit whose tree matches
`1f3ee24823421db94b1439478f3c503adcfccb61`. The ATT exporter and workflow were
restored on top as new work. The changes landed through pull requests while
preserving Git history.

## Compilation status

The canonical compiler merges complementary values, applies only explicit
field-resolution policy, and reports unresolved conflicts. The comparison
command produces JSON and Markdown. Neither output is an authoritative
replacement for its source; source-specific limitations remain in the
manifests. See [COMPILATION.md](COMPILATION.md), [SCHEMAS.md](SCHEMAS.md), and
[COMPARISON.md](COMPARISON.md) for the contracts and remaining gaps.

## Data integrity

The Wowhead compiler validates inputs before replacing generated zone files.
The Go client extractor writes into a temporary sibling directory and swaps
it into place only after successful decoding and manifest serialization. It
keeps an existing output if extraction fails. ATT export generation is
deterministic and validated by Go tests.

## Licensing and source isolation

Code licenses and third-party dataset rights are tracked separately. Temporary client extracts are ignored by Git and purged after verification.
The checked compressed snapshot and canonical client data are retained. Client
acquisition is not part of automation; the daily compiler reads the retained
canonical snapshot. See
[SOURCES.md](SOURCES.md) and [ATTRIBUTION.md](../ATTRIBUTION.md).
