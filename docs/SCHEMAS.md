# Data formats and schemas

## Canonical source records

Every source adapter writes `records.jsonl` (or `records.jsonl.gz` for datasets
at least 50 MiB) and `manifest.json` to
`data/forever/sources/<source>/`. The JSON Schema contract is in
`schemas/source-record.schema.json`; `schemas/field-resolution.json` defines
optional conflict priorities for compilation.

Records identify a flavor, entity type and ID, source revision, normalized
attributes, optional field states, and field-level provenance. A known zero is
stored as a value. A known empty value remains explicit and can be marked `empty`. Missing,
unsupported, unavailable, invalid, and conflicting values use `fieldStates` so
they cannot be mistaken for zero or empty data. Unknown source-specific
content stays in attributes with source provenance.

The Go reader validates record contracts and manifest checksums. The shared
Python writer emits the same record shape and deterministic checksum. Snapshot
manifests include record counts, source revision, generation time when known,
status, and limitations.

## Compilation

`data/forever/compilation/` contains merged canonical records,
`manifest.json`, and machine-readable and Markdown conflict reports. Complementary
fields merge with source provenance. Equal values retain all observations.
Conflicting values stay unresolved unless an explicit field policy selects a
source. The legacy `export/forever/` ATT product remains available separately.

## Source-specific formats

The original Wowhead cache under `data/forever/raw/` and its zone products under
`data/forever/compiled/zones/` retain their existing source-specific formats.
The Wowhead adapter normalizes an existing cache; it does not fetch data.

The local client extractor writes one table JSONL file plus `manifest.json` to
`.local/foreverclient/`. The local `cmd/ingest-client` adapter maps a supported
subset of those tables into canonical records. The checked compressed table
snapshot is retained under `data/forever/client-extract/`, with its original
manifest and `files.json` checksums of uncompressed table bytes. The temporary
export can be purged after verification. No game executable or CASC archive is
bundled; see [CLIENT_EXTRACTION.md](CLIENT_EXTRACTION.md) for the full workflow.

Large source and compilation datasets use deterministic gzip compression to
stay below GitHub file limits. The manifest checksum always covers the
uncompressed JSONL bytes. Validation, compilation, comparison, and semantic
change detection read either form. Consumers reading files directly must
decompress `records.jsonl.gz` when `records.jsonl` is absent.

The only supported `flavor` is `forever`. Classic Era baseline quests and
Forever additions use the same quest ID namespace. Source snapshots retain
independent observations; compilation emits one record per entity type and ID.

Client quest lookup tables use `entityType=questmetadata` with an ID such as
`QuestInfo:1`. Quest-line and POI links use `relationship` with table-prefixed
IDs. Only QuestV2 row IDs map directly to `quest` IDs. Raw client field names and
values are preserved with provenance; support-table row IDs never become quests.

## Compiled format 2

Compilation manifests identify `compilationFormatVersion: 2` independently of
record envelope version 1. Readers retain support for unmarked older compilations
and reject unsupported explicit versions. Shared attributes, original observations,
and migration instructions are in [RECONCILIATION.md](RECONCILIATION.md).
