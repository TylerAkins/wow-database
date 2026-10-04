# Development

Refreshes run in GitHub Actions. A local run is only for debugging.

Optional tools:

- Go 1.22 or newer
- LuaJIT 2.1
- A QuestieDB checkout

```bash
cd /path/to/QuestieDB
luajit /path/to/wow-database/tools/export_questiedb.lua <commit> <generatedAt> > /tmp/quests.jsonl
cd /path/to/wow-database
go run ./cmd/compile --dump /tmp/quests.jsonl --out export/forever
go run ./cmd/compile --dump /tmp/quests.jsonl --out export/forever --check
go test ./...
gofmt -l .
```

`--check` compares the dump with the published tree and writes nothing. Running compile twice on the same dump produces the same bytes.
