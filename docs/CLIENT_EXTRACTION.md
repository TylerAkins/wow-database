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

## Importing data

Do not commit the local output automatically. Review its manifest and the
applicable client-data redistribution terms before importing records into a
published dataset. The extractor is intentionally local-only and is not part
of GitHub Actions. See [SOURCES.md](SOURCES.md) and
[INGESTION.md](INGESTION.md) for source policy and repository workflows.
