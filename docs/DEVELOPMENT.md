# Development

Refreshes run in GitHub Actions. A local run is only for debugging.

Optional tools:

- Go 1.22 or newer
- An AllTheThings checkout

```bash
cd /path/to/wow-database
ATT_COMMIT=$(git -C /path/to/AllTheThings rev-parse HEAD)
GENERATED_AT=$(git -C /path/to/AllTheThings log -1 --format=%cI)
go run ./cmd/compile --att-root /path/to/AllTheThings \
  --att-commit "$ATT_COMMIT" --generated-at "$GENERATED_AT" --out export/forever
go run ./cmd/compile --att-root /path/to/AllTheThings \
  --att-commit "$ATT_COMMIT" --generated-at "$GENERATED_AT" --out export/forever --check
go test ./...
gofmt -l .
```

`--check` compares the ATT export with the published tree and writes nothing. The compiler parses ATT's `.contrib/.db/forever` Lua quest declarations and map constants. Unsupported coordinate syntax stops the run with a source path and quest ID. Unmapped ATT map constants are counted and listed in the manifest's `parse.issues`. Running compile twice on the same ATT revision produces the same bytes.

For unit tests that exercise the publisher without a full ATT tree, you can still pass a JSONL `--dump` shaped like the legacy Questie export.
