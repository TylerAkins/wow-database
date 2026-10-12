# Data formats and schemas

## Canonical source records

Every source adapter writes `records.jsonl` and `manifest.json` to
`data/<flavor>/sources/<source>/`. The JSON Schema contract is in
`schemas/source-record.schema.json`; `schemas/field-resolution.json` defines
optional conflict priorities for compilation.

Records identify a flavor, entity type and ID, source revision, normalized
attributes, optional field states, and field-level provenance. A known zero is
stored as a value. A known empty value is retained and marked `empty`. Missing,
unsupported, unavailable, invalid, and conflicting values use `fieldStates` so
they cannot be mistaken for zero or empty data. Unknown source-specific
content stays under source-named attributes.

The Go reader validates record contracts and manifest checksums. The shared
Python writer emits the same record shape and deterministic checksum. Snapshot
manifests include record counts, source revision, generation time when known,
status, and limitations.

## Compilation

`data/<flavor>/compilation/` contains merged canonical records,
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
subset of those tables into canonical records. It does not copy raw game assets.
