# Compilation

## Canonical multi-source products

Compile source datasets for a flavor with:

```sh
go run ./cmd/database compile --flavor forever
```

The compiler reads `data/<flavor>/sources/*/`, merges complementary attributes,
preserves source observations in provenance, and writes
`data/<flavor>/compilation/`. Equal values combine provenance. Contradictory
values remain unresolved with a `conflicting` state unless
`schemas/field-resolution.json` selects a source for that entity field. Every
conflict is retained in `conflicts.json` and `conflicts.md`, including conflicts
resolved by policy.

Validate canonical snapshots before or after compilation:

```sh
go run ./cmd/database validate --root data
```

The validator verifies schema shape and checksums and reports dangling links.
Partial source snapshots may legitimately reference records outside their
own coverage; those links are counted, not rejected. Raw scraper manifests are
validated by their source tools and are not treated as canonical manifests.

## Existing source products

`python3 tools/compile_zone_files.py` continues to build Wowhead Forever zone
bundles. `go run ./cmd/compile` continues to build the legacy ATT Forever
export. They remain separate migration products and do not replace the
canonical merged datasets.

## Known coverage limits

ATT ingestion normalizes supported Forever quest declarations and parsed
links, plus selected explicit NPC, item, object, and achievement entries. It
does not cover the complete ATT catalog or standalone map and zone definitions.
QuestieDB's generated entity tables can be imported
daily or locally for Forever, but its dynamic corrections, support
bundles, and
derived runtime passes are not evaluated. The local
client adapter maps only the subset listed in [INGESTION.md](INGESTION.md).
The compiled union is therefore a union of records supplied by these
adapters, not a guarantee of complete game data.

Forever is the only compiled flavor. Its QuestieDB input contains the converted
Classic baseline; the importer verifies that all upstream Classic quest IDs are
present. The compiler merges the union of source records by entity type and ID,
so a shared Classic quest appears once alongside Forever additions. Multiple
source observations and conflicting values remain in provenance and reports.
