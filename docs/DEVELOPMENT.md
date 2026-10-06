# Development

Refreshes run in GitHub Actions. A local run is only for debugging.

Optional tools:

- Go 1.22 or newer
- LuaJIT 2.1
- A QuestieDB checkout
- An AllTheThings checkout

```bash
cd /path/to/QuestieDB
luajit /path/to/wow-database/tools/export_questiedb.lua <commit> <generatedAt> > /tmp/quests.jsonl
cd /path/to/wow-database
go run ./cmd/compile --dump /tmp/quests.jsonl --att-root /path/to/AllTheThings \
  --att-commit "$ATT_COMMIT" --out export/forever
go run ./cmd/compile --dump /tmp/quests.jsonl --att-root /path/to/AllTheThings \
  --att-commit "$ATT_COMMIT" --out export/forever --check
go test ./...
gofmt -l .
```

`--check` compares the merged sources with the published tree and writes nothing. The compiler reads ATT's `.contrib/.db/forever` files and map constants. It fills only empty matching QuestieDB available NPC places. Unmapped ATT maps, ambiguous matches, and coordinate conflicts are counted and listed in the manifest's `merge.issues`; unsupported coordinate syntax stops the run with a source path and quest ID. Running compile twice on the same source revisions produces the same bytes.
