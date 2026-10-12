# Local WoW client extraction

The client workflow reads an installed Forever client, packages the decoded DB2
tables, imports supported records, and rebuilds the combined Forever database.
It does not launch the game or modify its installation. Acquisition runs locally;
GitHub Actions compiles the committed canonical snapshot.

## Requirements

- macOS on Intel or Apple Silicon
- Go 1.27 or newer; CI uses Go 1.27.2
- Python 3.12 or newer for packaging
- Git for obtaining WoWDBDefs
- A local Forever installation with `.build.info` and its CASC data

Run every command below from the `wow-database` repository root. Confirm the
installed tools with `go version`, `python3 --version`, and `git --version`.

## 1. Obtain the DB2 definitions

Clone WoWDBDefs once:

```sh
git clone --depth 1 https://github.com/wowdev/WoWDBDefs.git .cache/WoWDBDefs
```

If that checkout already exists, update it instead:

```sh
git -C .cache/WoWDBDefs pull --ff-only
```

The extractor requires the checkout's `manifest.json` and `definitions/`
directory. It does not download definitions itself. The definition data is
CC BY-SA 4.0 and is supplied separately; the extraction manifest records its
revision and each selected definition's hash.

## 2. Extract the installed Forever client

The committed snapshot was extracted from `wow_classic_beta` build
`1.60.1.70338`. Use the product that identifies your installed Forever client:

```sh
go run ./cmd/client-extract \
  --client-dir "/Applications/World of Warcraft" \
  --product wow_classic_beta \
  --dbd-dir .cache/WoWDBDefs \
  --out .local/foreverclient
```

Replace `--client-dir` if the installation is elsewhere. Product identifiers
come from the installation's `.build.info`; they are not canonical database
flavors. The output always uses the Forever flavor. To inspect the installed
products without extracting, run:

```sh
python3 - <<'PY'
import csv
from pathlib import Path
path = Path("/Applications/World of Warcraft/.build.info")
with path.open() as stream:
    reader = csv.DictReader(stream, delimiter="|")
    columns = {key.split("!", 1)[0]: key for key in reader.fieldnames}
    for row in reader:
        print(row[columns["Product"]], row[columns["Version"]])
PY
```

The extractor detects common macOS installation paths when `--client-dir` is
omitted. It selects the product automatically only when the installation has
one available product; otherwise `--product` is required.

With no table filter, all DB2 entries in the definitions manifest are attempted.
For a smaller run, add repeated flags such as `--table QuestV2 --table QuestInfo`.
Use `go run ./cmd/client-extract --help` for all flags. Ctrl-C cancels a run;
rerunning begins again because table-level resume is not implemented. Failed
extraction does not replace the previous completed output.

The output is the hidden, Git-ignored `.local/foreverclient/` directory. It
contains `manifest.json` and one `<Table>.jsonl` file per decoded table, directly
inside that directory. The first line describes the table; later lines contain
records. Fields retain the matching DBD names and values. Files have owner-only
permissions, and output must remain outside the game installation.

## 3. Package, import, and compile

Review `.local/foreverclient/manifest.json` for table status and skipped encrypted
sections, then run these commands in order. Stop if any command fails:

```sh
python3 tools/client/package_snapshot.py --input .local/foreverclient \
  --out data/forever/client-extract
go run ./cmd/ingest-client --input data/forever/client-extract
go run ./cmd/database validate --root data
go run ./cmd/database compile --flavor forever
go run ./cmd/database validate --root data
```

| Output | Contents |
|---|---|
| `.local/foreverclient/` | Temporary uncompressed extraction |
| `data/forever/client-extract/` | Retained compressed DB2 tables, original manifest, and checksum index |
| `data/forever/sources/foreverclient/` | Canonical client observations and provenance |
| `data/forever/compilation/` | Combined ATT, QuestieDB, Wowhead, and client records, plus conflict reports |

Packaging validates headers, counts, and unique row IDs before replacing an
existing snapshot. Each retained table becomes `<Table>.jsonl.gz`; `files.json`
records its uncompressed SHA-256 and record count. `TactKey` contains
content-decryption keys and is excluded, with the exclusion recorded in the
index. No game executable, CASC archive, or DBD files are bundled. Dataset rights
remain with their sources; the code license does not relicense client data.

The importer accepts flat JSONL files or this compressed snapshot and verifies
the index when present. Older `tables/` inputs remain readable. Provenance uses
the logical uncompressed table filename. Canonical records use `records.jsonl.gz`
when the uncompressed dataset reaches 50 MiB; checksums cover uncompressed bytes.

The committed snapshot contains 1,160 retained tables and 1,931,668 records from
build `1.60.1.70338`. Its canonical client dataset has 60,713 records, including
6,610 QuestV2 quest IDs. These counts describe that snapshot; another build or a
table-filtered run can differ.

## 4. Verify and purge the temporary export

Keep the temporary export until packaging, import, compilation, and validation
have succeeded. Before deleting it, verify every retained table against the
packaged checksum and verify that the manifests match:

```sh
python3 - <<'PY'
import gzip
import hashlib
import json
from pathlib import Path
raw = Path(".local/foreverclient")
snapshot = Path("data/forever/client-extract")
index = json.loads((snapshot / "files.json").read_text())
assert (raw / "manifest.json").read_bytes() == (snapshot / "manifest.json").read_bytes()
for name, entry in index["files"].items():
    source = raw / name.removesuffix(".gz")
    assert hashlib.sha256(source.read_bytes()).hexdigest() == entry["recordsSha256"], name
    retained = gzip.decompress((snapshot / name).read_bytes())
    assert hashlib.sha256(retained).hexdigest() == entry["recordsSha256"], name
print(f"Verified {len(index['files'])} retained tables; temporary export can be removed.")
PY
```

Commit the retained snapshot, canonical client dataset, and compilation on the
work branch before cleanup. The excluded `TactKey` table is deliberately not
retained; deleting the temporary export also removes that local copy.

After verification succeeds, delete only the temporary export:

```sh
rm -r .local/foreverclient
```

This leaves the compressed snapshot, normalized records, compilation, game
installation, and `.cache/WoWDBDefs` checkout available. The definitions cache
can be reused for future extraction. If the temporary export has already been
removed, skip verification and deletion; it can be recreated by extracting again.

## Rebuild after cleanup

The retained snapshot is sufficient for import and compilation. No installed
game or temporary export is needed:

```sh
go run ./cmd/ingest-client --input data/forever/client-extract
go run ./cmd/database compile --flavor forever
go run ./cmd/database validate --root data
```

## Quest data and supported tables

The adapter maps Achievement, AreaTable, Creature, Faction, ItemSparse, Map,
SpellName, QuestV2, and selected quest support tables. Tables without canonical
mappings remain in the compressed snapshot and are listed as import limitations.

QuestV2 maps row IDs to canonical quest IDs, so overlapping source observations
merge into one quest. It supplies client UI metadata and unique bit flags,
not titles, requirements, objectives, rewards, or server availability rules.
Encrypted sections can hide additional quest IDs; partial status is retained.
ATT, QuestieDB, and Wowhead remain the sources for supported quest details.

QuestLineXQuest, QuestPOIBlob, QuestPOIPoint, QuestObjective, and QuestPackageItem
use relationship records with table-prefixed IDs. QuestInfo, QuestSort,
QuestLabel, QuestLine, QuestXP, QuestMoneyReward, QuestFactionReward, and
QuestFeedbackEffect use `questmetadata`, also with table-prefixed IDs. Their row
IDs do not become quest IDs. POI points retain raw client coordinates; they are
not NPC spawn positions or normalized map percentages. Reward lookup tables are
not interpreted as per-quest rewards.

## Extraction limits and troubleshooting

Only WDC5 is supported. WDC6 and later, legacy DBC files, missing client files,
and tables without a matching DBD version are reported as unsupported or
unavailable. Encrypted sections are skipped. `partial_encrypted` means only
decoded sections are present. The full committed extraction reported 1,078 written tables, 83 partial
tables, and 105 unavailable entries before the TactKey packaging exclusion.

Client data does not reconstruct server-side quest availability, all
prerequisite rules, NPC spawns, loot, or complete world geometry. A successful
import or validation does not establish complete coverage. Unresolved source
conflicts remain in `compilation/conflicts.json` and `conflicts.md`.

If extraction cannot find the installation or product, set `--client-dir` and
`--product` explicitly. If a definition is missing, update WoWDBDefs and inspect
the manifest's table reason. Do not delete an earlier valid snapshot after a
failed run. Acquisition is local and is not part of GitHub Actions; the daily
compiler reads the committed canonical client snapshot. See
[SOURCES.md](SOURCES.md), [INGESTION.md](INGESTION.md), and
[COMPILATION.md](COMPILATION.md).
