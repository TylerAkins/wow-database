package compile

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/TylerAkins/wow-database/internal/att"
)

const (
	schemaVersion = 2
	maxShardBytes = 20 * 1024 * 1024
)

// ShardSize is the maximum number of quests in one published shard.
var ShardSize = 500

// ExceedsShardLimit reports whether a shard of n bytes is too large to publish.
func ExceedsShardLimit(n int) bool {
	return n > maxShardBytes
}

// Options selects the dump and the published directory.
type Options struct {
	Dump        io.Reader
	ATTRoot     string
	ATTCommit   string
	GeneratedAt string
	OutDir      string
	Check       bool
}

type summary struct {
	questCount  int
	commit      string
	generatedAt string
	parse       parseCounts
}

type parseCounts struct {
	unmapped int
	issues   []map[string]any
}

type questRecord struct {
	id    int
	quest map[string]any
	raw   map[string]any
}

// Run publishes a dump, or compares it with the existing tree when Check is set.
func Run(opts Options) error {
	if opts.Dump == nil && opts.ATTRoot == "" {
		return fmt.Errorf("either dump or att-root is required")
	}
	if opts.ATTRoot != "" && opts.ATTCommit == "" {
		return fmt.Errorf("ATT commit is required when att-root is provided")
	}
	if opts.OutDir == "" {
		return fmt.Errorf("output directory is required")
	}
	var records []questRecord
	var meta summary
	var err error
	if opts.ATTRoot != "" {
		records, meta, err = readATT(opts.ATTRoot, opts.ATTCommit, opts.GeneratedAt)
	} else {
		records, meta, err = readDump(opts.Dump)
	}
	if err != nil {
		return err
	}
	files, err := render(records, meta)
	if err != nil {
		return err
	}
	if opts.Check {
		return compare(opts.OutDir, files)
	}
	return replace(opts.OutDir, files)
}

func readATT(root, commit, generatedAt string) ([]questRecord, summary, error) {
	if generatedAt == "" {
		return nil, summary{}, fmt.Errorf("generatedAt is required for ATT export")
	}
	rows, exportMeta, err := att.Export(root)
	if err != nil {
		return nil, summary{}, err
	}
	meta := summary{
		questCount:  len(rows),
		commit:      commit,
		generatedAt: generatedAt,
	}
	for _, report := range exportMeta.Unmapped {
		meta.parse.unmapped++
		meta.parse.issues = append(meta.parse.issues, map[string]any{
			"kind":    "unmapped",
			"questId": report.QuestID,
			"npcId":   report.NPCID,
			"file":    report.File,
			"reason":  report.Reason,
		})
	}
	sort.Slice(meta.parse.issues, func(i, j int) bool {
		a, b := meta.parse.issues[i], meta.parse.issues[j]
		ai, _ := a["questId"].(int)
		bi, _ := b["questId"].(int)
		if ai != bi {
			return ai < bi
		}
		af, _ := a["file"].(string)
		bf, _ := b["file"].(string)
		return af < bf
	})
	return materializeRows(rows, meta)
}

func materializeRows(rows []map[string]any, meta summary) ([]questRecord, summary, error) {
	records := make([]questRecord, 0, len(rows))
	for _, row := range rows {
		id, err := asInt(row["id"])
		if err != nil {
			return nil, summary{}, fmt.Errorf("quest id: %w", err)
		}
		quest, err := publishQuest(id, row)
		if err != nil {
			return nil, summary{}, err
		}
		records = append(records, questRecord{id: id, quest: quest, raw: row})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].id < records[j].id })
	if meta.questCount != len(records) {
		return nil, summary{}, fmt.Errorf("summary questCount %d does not match %d rows", meta.questCount, len(records))
	}
	return records, meta, nil
}

func readDump(reader io.Reader) ([]questRecord, summary, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	var records []questRecord
	var meta summary
	var sawSummary bool
	seen := map[int]bool{}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		row, err := decodeObject(line)
		if err != nil {
			return nil, summary{}, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if row["kind"] == "summary" {
			if sawSummary {
				return nil, summary{}, fmt.Errorf("line %d: duplicate summary", lineNo)
			}
			sawSummary = true
			meta, err = readSummary(row)
			if err != nil {
				return nil, summary{}, err
			}
			continue
		}
		id, err := asInt(row["id"])
		if err != nil {
			return nil, summary{}, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if seen[id] {
			return nil, summary{}, fmt.Errorf("duplicate quest %d", id)
		}
		seen[id] = true
		records = append(records, questRecord{id: id, raw: row})
	}
	if err := scanner.Err(); err != nil {
		return nil, summary{}, err
	}
	if !sawSummary {
		return nil, summary{}, fmt.Errorf("dump is missing the summary line")
	}
	if meta.questCount == 0 || len(records) == 0 {
		return nil, summary{}, fmt.Errorf("empty quest list")
	}
	if meta.questCount != len(records) {
		return nil, summary{}, fmt.Errorf("summary questCount %d does not match %d rows", meta.questCount, len(records))
	}
	sort.Slice(records, func(i, j int) bool { return records[i].id < records[j].id })
	for i := range records {
		quest, err := publishQuest(records[i].id, records[i].raw)
		if err != nil {
			return nil, summary{}, err
		}
		records[i].quest = quest
	}
	return records, meta, nil
}

func readSummary(row map[string]any) (summary, error) {
	count, err := asInt(row["questCount"])
	if err != nil {
		return summary{}, fmt.Errorf("summary questCount: %w", err)
	}
	commit, _ := row["commit"].(string)
	generatedAt, _ := row["generatedAt"].(string)
	if commit == "" || generatedAt == "" {
		return summary{}, fmt.Errorf("summary requires commit and generatedAt")
	}
	return summary{questCount: count, commit: commit, generatedAt: generatedAt}, nil
}

func render(records []questRecord, meta summary) (map[string][]byte, error) {
	if ShardSize < 1 {
		return nil, fmt.Errorf("shard size must be positive")
	}
	zones := map[string][]int{}
	starters := map[string][]int{}
	finishers := map[string][]int{}
	chains := map[string]any{}
	var shards []map[string]any
	files := map[string][]byte{}
	for start := 0; start < len(records); start += ShardSize {
		end := start + ShardSize
		if end > len(records) {
			end = len(records)
		}
		chunk := records[start:end]
		quests := map[string]any{}
		for _, record := range chunk {
			key := strconv.Itoa(record.id)
			quests[key] = record.quest
			if zone, ok := record.quest["zoneOrSort"].(int); ok {
				indexIDs(zones, strconv.Itoa(zone), record.id)
			}
			if rows, ok := record.quest["startedBy"].([]any); ok {
				for _, provider := range providerKeys(rows) {
					indexIDs(starters, provider, record.id)
				}
			}
			if rows, ok := record.quest["finishedBy"].([]any); ok {
				for _, provider := range providerKeys(rows) {
					indexIDs(finishers, provider, record.id)
				}
			}
			if entry := chainEntry(record.quest); entry != nil {
				chains[key] = entry
			}
		}
		body, err := marshal(map[string]any{"schemaVersion": schemaVersion, "quests": quests})
		if err != nil {
			return nil, err
		}
		if ExceedsShardLimit(len(body)) {
			return nil, fmt.Errorf("shard %d is %d bytes", len(shards)+1, len(body))
		}
		path := fmt.Sprintf("quests/%04d.json", len(shards)+1)
		files[path] = body
		sum := sha256.Sum256(body)
		shards = append(shards, map[string]any{
			"path":       path,
			"idRange":    []any{chunk[0].id, chunk[len(chunk)-1].id},
			"questCount": len(chunk),
			"sha256":     hex.EncodeToString(sum[:]),
		})
	}
	manifestData := map[string]any{
		"schemaVersion": schemaVersion,
		"gameVersion":   "forever",
		"source":        "AllTheThings",
		"commit":        meta.commit,
		"locale":        "enUS",
		"generatedAt":   meta.generatedAt,
		"questCount":    meta.questCount,
		"shards":        shards,
	}
	if meta.parse.unmapped > 0 || len(meta.parse.issues) > 0 {
		manifestData["parse"] = map[string]any{
			"unmappedLocations": meta.parse.unmapped,
			"issues":            meta.parse.issues,
		}
	}
	manifest, err := marshal(manifestData)
	if err != nil {
		return nil, err
	}
	files["manifest.json"] = manifest
	for name, index := range map[string]map[string][]int{
		"indexes/zones.json":     zones,
		"indexes/starters.json":  starters,
		"indexes/finishers.json": finishers,
	} {
		body, err := marshal(sortedIndex(index))
		if err != nil {
			return nil, err
		}
		files[name] = body
	}
	chainBody, err := marshal(chains)
	if err != nil {
		return nil, err
	}
	files["indexes/chains.json"] = chainBody
	return files, nil
}

func marshal(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeObject(line []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected an object")
	}
	return object, nil
}

func compare(dir string, files map[string][]byte) error {
	for path, body := range files {
		existing, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			return fmt.Errorf("check %s: %w", path, err)
		}
		if !bytes.Equal(existing, body) {
			return fmt.Errorf("check %s: published bytes differ", path)
		}
	}
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, ok := files[rel]; !ok {
			return fmt.Errorf("check %s: stale file", rel)
		}
		return nil
	})
}

func replace(dir string, files map[string][]byte) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, filepath.Base(dir)+"-")
	if err != nil {
		return err
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.RemoveAll(temp)
		}
	}()
	for path, body := range files {
		target := filepath.Join(temp, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return err
		}
	}
	backup := dir + ".bak"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(temp, dir); err != nil {
		_ = os.Rename(backup, dir)
		return err
	}
	cleanupTemp = false
	return os.RemoveAll(backup)
}
