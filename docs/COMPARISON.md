# Dataset comparison

Compare two canonical datasets with a JSON and Markdown report:

```sh
go run ./cmd/database compare \
  --left data/forever/sources/allthethings \
  --right data/forever/sources/questiedb \
  --out .local/comparisons/att-questiedb
```

All datasets use the Forever flavor. Reports count
left-only, right-only, changed, and equal entities. Changed records include
field-level paths and before/after JSON values, including differences in
missing-value states.

Comparison is structural. `--coordinate-tolerance` optionally ignores nearby
numeric `x` and `y` differences only when both values share a matching `zone`,
`zoneId`, `map`, `mapId`, or `uiMapId` in the same coordinate object. The
default tolerance is zero. Localization normalization and source-specific
semantic equivalence are not applied.
