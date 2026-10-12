# Local WoW client extraction

`client-extract` reads installed client data from local CASC archives. It does
not launch the game, inspect process memory, contact a game service, or modify
the installation. It currently decodes WDC5 DB2 tables and writes normalized
JSON Lines plus a manifest.

## Requirements

- macOS on Intel or Apple Silicon
- Go 1.27 or newer. The workflow pins Go 1.27.2, the latest stable patch
  verified for this project on 2026-10-11.
- A local WoW installation
- A local WoWDBDefs checkout containing `manifest.json` and `definitions/`

The extractor makes no network requests. Obtain or update WoWDBDefs separately
before extracting. Its `.dbd` definitions are distributed under CC BY-SA 4.0
and are not bundled in this repository. The extraction manifest records the
definition file hash and, when the definitions directory is a Git checkout,
its revision.

The implementation was compiled and tested locally with Go 1.27.1. Installing
Go 1.27.2 in the development environment was blocked because shell network
access is unavailable. CI is configured to validate with Go 1.27.2.

## Run

From the repository root, pass the path to the local definitions checkout:

```sh
go run ./cmd/client-extract \
  --dbd-dir "$HOME/src/WoWDBDefs" \
  --out .local/foreverclient
```

On macOS the command checks common installation paths. If the install is in a
different location, specify it and the exact product listed in its
`.build.info`:

```sh
go run ./cmd/client-extract \
  --client-dir "/Applications/World of Warcraft" \
  --product wow_classic_beta \
  --dbd-dir "$HOME/src/WoWDBDefs" \
  --out .local/foreverclient
```

Use `--table Map --table AreaTable` to select specific tables. With no table
filter, the command processes DB2 entries in the local WoWDBDefs manifest.
Run `go run ./cmd/client-extract --help` for the current flags. Ctrl-C stops
the extraction. A later run starts again from the beginning; table-level
checkpoint resume is not implemented. Existing output remains intact if a
run fails before publication.

The output directory contains `manifest.json` and one `<Table>.jsonl` file
per decoded table. The first line describes the table, followed by one JSON
object per record. Record fields preserve the names and values from the exact
matching `.dbd` version block. Files are written with owner-only permissions.
The default output path is `.local/foreverclient`, which is ignored by Git.
Output must remain outside the game installation.

## Supported data and limitations

The extractor selects the client build from `.build.info`, resolves DB2
FileDataIDs through the local CASC root and encoding tables, reads local
indexes and data archives, decompresses supported BLTE chunks, and validates
the selected DB2 layout against the complete four-part build in its `.dbd`
file.

The current parser supports WDC5 only. It skips encrypted WDC sections and
reports the number skipped. It does not obtain TACT keys or bypass encryption.
WDC6 and later, legacy DBC files, and DB2 files without a matching local
definition are unsupported or unavailable and are reported as such. A table
with skipped sections is marked `partial_encrypted`; its JSONL contains only
records from decoded sections. Do not treat that table as complete.

Client tables can provide static definitions such as maps, areas, factions,
quest fields, creatures, items, spells, and achievements when the installed
build contains those files and the table is not fully encrypted. They do not
reconstruct server-side quest availability, all prerequisite behavior, NPC
spawn positions, loot tables, or other server-only state. Map and area
definitions do not include complete world geometry or spawn placement data.
Those categories remain unavailable from this extractor unless another
permitted source supplies them.

For the local `wow_classic_beta` build `1.60.1.70338`, a manual extraction of
ten tables decoded all records from Faction and QuestInfo. Eight tables were
partial because the client marked some sections encrypted. This is a sample
run, not a guarantee that other client builds expose the same files or
sections.

## Packaging and importing data

Acquisition remains local. Package the extracted tables as a checked, compressed
snapshot, then import supported records and rebuild Forever:

```sh
python3 tools/client/package_snapshot.py --input .local/foreverclient \
  --out data/forever/client-extract
go run ./cmd/ingest-client --input data/forever/client-extract
go run ./cmd/database validate --root data
go run ./cmd/database compile --flavor forever
```

The packaged directory contains the original extraction manifest, one
`<Table>.jsonl.gz` per included table, and `files.json` with checksums of the
uncompressed bytes. Packaging validates headers, counts, and unique row IDs
before replacing an existing snapshot. `TactKey` is excluded because it contains
content-decryption keys; the index records that exclusion. No game executable,
archive, or definition files are included. Dataset rights remain with their
sources; the repository code license does not relicense client data.

The importer reads the extractor's flat JSONL layout or the compressed snapshot.
It verifies the snapshot index when present. Older inputs under `tables/` remain
readable. Field provenance uses the logical uncompressed table filename.

`QuestV2` maps row IDs to canonical quest IDs, so overlapping ATT and QuestieDB
quests merge into one record. It provides only client UI metadata and unique
bit flags. It does not provide titles, requirements, objectives, or rewards.
Encrypted sections can hide additional quest IDs; partial status is retained.

Quest-line, POI, objective, and package-item rows are preserved as relationship
records with IDs prefixed by their table name. QuestInfo, QuestSort, QuestLabel,
QuestLine, QuestXP, QuestMoneyReward, QuestFactionReward, and QuestFeedbackEffect
are quest metadata records, also with table-prefixed IDs. Their row IDs are not
quest IDs. POI points retain raw client coordinates; they are not NPC spawn
positions or normalized map percentages. Reward lookup tables are not interpreted
as per-quest rewards. Tables without a canonical mapping remain available in the
compressed extraction snapshot.

The extractor is not part of GitHub Actions. The daily compiler uses the
committed canonical client snapshot alongside the other Forever sources. See
[SOURCES.md](SOURCES.md) and [INGESTION.md](INGESTION.md).
