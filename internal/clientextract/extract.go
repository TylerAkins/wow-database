package clientextract

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/TylerAkins/wow-database/tools/db2tool/dbd"
	"github.com/TylerAkins/wow-database/tools/db2tool/tact"
	"github.com/TylerAkins/wow-database/tools/db2tool/wdc"
)

const schemaVersion = 1

const maxManifestBytes = 128 << 20
const maxDefinitionBytes = 8 << 20
const maxDB2Bytes = 512 << 20

var validTableName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Options struct {
	ClientDir string
	Product   string
	DBDDir    string
	OutputDir string
	Tables    []string
}

type ClientInfo struct {
	Product     string `json:"product"`
	Version     string `json:"version"`
	BuildNumber uint32 `json:"buildNumber"`
	BuildConfig string `json:"buildConfig"`
}

type DefinitionsInfo struct {
	RepositoryRevision string `json:"repositoryRevision,omitempty"`
	Source             string `json:"source"`
}

type TableResult struct {
	Name             string `json:"name"`
	FileDataID       uint32 `json:"fileDataId"`
	Status           string `json:"status"`
	RecordCount      int    `json:"recordCount,omitempty"`
	SkippedSections  int    `json:"skippedEncryptedSections,omitempty"`
	Reason           string `json:"reason,omitempty"`
	DefinitionSHA256 string `json:"definitionSha256,omitempty"`
}

type Counts struct {
	Written     int `json:"written"`
	Partial     int `json:"partial"`
	Unavailable int `json:"unavailable"`
	Unsupported int `json:"unsupported"`
}

type Report struct {
	SchemaVersion int             `json:"schemaVersion"`
	GameVersion   string          `json:"gameVersion"`
	Source        string          `json:"source"`
	Client        ClientInfo      `json:"client"`
	Definitions   DefinitionsInfo `json:"definitions"`
	Counts        Counts          `json:"counts"`
	Tables        []TableResult   `json:"tables"`
}

type db2ManifestEntry struct {
	TableName     string `json:"tableName"`
	DB2FileDataID uint32 `json:"db2FileDataID"`
}

type tableInput struct {
	name   string
	fileID uint32
}

type fileOpener interface {
	OpenFileByFDID(uint32) ([]byte, error)
}

func Extract(ctx context.Context, opts Options, progress io.Writer) (Report, error) {
	clientDir, err := findClientDirectory(opts.ClientDir)
	if err != nil {
		return Report{}, err
	}
	dbdDir, err := filepath.Abs(opts.DBDDir)
	if err != nil {
		return Report{}, fmt.Errorf("resolve --dbd-dir: %w", err)
	}
	outputDir, err := filepath.Abs(opts.OutputDir)
	if err != nil {
		return Report{}, fmt.Errorf("resolve --out: %w", err)
	}
	if err := ensureOutputOutsideClient(clientDir, outputDir); err != nil {
		return Report{}, err
	}

	entries, err := tact.ParseBuildInfo(filepath.Join(clientDir, ".build.info"))
	if err != nil {
		return Report{}, fmt.Errorf("read client build metadata: %w", err)
	}
	product := opts.Product
	if product == "" {
		product, err = selectProduct(entries)
		if err != nil {
			return Report{}, err
		}
	}
	build, err := tact.Open(clientDir, product)
	if err != nil {
		return Report{}, fmt.Errorf("open local CASC product %q: %w", product, err)
	}

	inputs, err := loadManifest(filepath.Join(dbdDir, "manifest.json"), opts.Tables)
	if err != nil {
		return Report{}, err
	}
	revision := repositoryRevision(ctx, dbdDir)

	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Report{}, fmt.Errorf("create output parent: %w", err)
	}
	tempDir, err := os.MkdirTemp(parent, filepath.Base(outputDir)+".pending-*")
	if err != nil {
		return Report{}, fmt.Errorf("create temporary output: %w", err)
	}
	defer os.RemoveAll(tempDir)

	report := Report{
		SchemaVersion: schemaVersion,
		GameVersion:   "forever",
		Source:        "foreverclient",
		Client: ClientInfo{
			Product:     build.Entry.Product,
			Version:     build.Entry.Version,
			BuildNumber: build.BuildNumber,
			BuildConfig: build.Entry.BuildConfig,
		},
		Definitions: DefinitionsInfo{
			RepositoryRevision: revision,
			Source:             "local WoWDBDefs checkout (DBD definitions are not bundled)",
		},
		Tables: make([]TableResult, 0, len(inputs)),
	}

	var fatal []string
	for _, input := range inputs {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		fmt.Fprintf(progress, "Reading %s (FileDataID %d)\n", input.name, input.fileID)
		result, fatalErr := extractTable(ctx, build, dbdDir, input, build.Entry.Version, build.BuildNumber, tempDir)
		report.Tables = append(report.Tables, result)
		switch result.Status {
		case "written":
			report.Counts.Written++
		case "partial_encrypted":
			report.Counts.Partial++
		case "unsupported_format":
			report.Counts.Unsupported++
		default:
			report.Counts.Unavailable++
		}
		if fatalErr != nil {
			fatal = append(fatal, fmt.Sprintf("%s: %v", input.name, fatalErr))
		}
	}
	if len(fatal) > 0 {
		return report, fmt.Errorf("extraction failed; previous output was preserved: %s", strings.Join(fatal, "; "))
	}
	if report.Counts.Written+report.Counts.Partial == 0 {
		return report, errors.New("no DB2 table could be decoded; previous output was preserved")
	}
	if err := writeJSON(filepath.Join(tempDir, "manifest.json"), report); err != nil {
		return report, fmt.Errorf("write extraction manifest: %w", err)
	}
	if err := replaceOutput(tempDir, outputDir); err != nil {
		return report, fmt.Errorf("publish extraction output: %w", err)
	}
	return report, nil
}

func extractTable(ctx context.Context, build fileOpener, dbdDir string, input tableInput, buildVersion string, buildNumber uint32, outputDir string) (TableResult, error) {
	result := TableResult{Name: input.name, FileDataID: input.fileID}
	definitionPath := filepath.Join(dbdDir, "definitions", input.name+".dbd")
	definitionInfo, err := os.Stat(definitionPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Status = "definition_missing"
			result.Reason = "no local .dbd definition"
			return result, nil
		}
		return result, fmt.Errorf("read definition: %w", err)
	}
	if definitionInfo.Size() > maxDefinitionBytes {
		return result, fmt.Errorf("definition exceeds %d-byte limit", maxDefinitionBytes)
	}
	definitionBytes, err := os.ReadFile(definitionPath)
	if err != nil {
		return result, fmt.Errorf("read definition: %w", err)
	}
	hash := sha256.Sum256(definitionBytes)
	result.DefinitionSHA256 = hex.EncodeToString(hash[:])

	definition, err := dbd.Read(strings.NewReader(string(definitionBytes)), true)
	if err != nil {
		return result, fmt.Errorf("parse definition: %w", err)
	}
	clientBuild, err := dbd.ParseBuild(buildVersion)
	if err != nil {
		return result, fmt.Errorf("parse client build %q: %w", buildVersion, err)
	}
	version, err := dbd.SelectVersion(definition, clientBuild)
	if err != nil {
		result.Status = "definition_build_missing"
		result.Reason = err.Error()
		return result, nil
	}
	data, err := build.OpenFileByFDID(input.fileID)
	if err != nil {
		if strings.Contains(err.Error(), "not found in root") {
			result.Status = "client_file_unavailable"
			result.Reason = err.Error()
			return result, nil
		}
		return result, fmt.Errorf("read local CASC record: %w", err)
	}
	if len(data) > maxDB2Bytes {
		return result, fmt.Errorf("DB2 file exceeds %d-byte limit", maxDB2Bytes)
	}
	table, err := wdc.ReadBytes(data)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported DB2 format") {
			result.Status = "unsupported_format"
			result.Reason = err.Error()
			return result, nil
		}
		return result, fmt.Errorf("decode DB2: %w", err)
	}
	decoded, err := table.DecodeRows(definition, version, buildNumber)
	if err != nil {
		return result, fmt.Errorf("decode rows: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	status := "written"
	if table.SkippedSections > 0 {
		status = "partial_encrypted"
	}
	result.Status = status
	result.RecordCount = len(decoded.Rows)
	result.SkippedSections = table.SkippedSections
	path := filepath.Join(outputDir, input.name+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return result, fmt.Errorf("create output: %w", err)
	}
	writer := bufio.NewWriter(file)
	header := tableHeader{
		Kind:             "table",
		SchemaVersion:    schemaVersion,
		GameVersion:      "forever",
		Source:           "foreverclient",
		Table:            input.name,
		FileDataID:       input.fileID,
		ClientBuild:      buildNumber,
		DefinitionSHA256: result.DefinitionSHA256,
	}
	if err := writeJSONLine(writer, header); err != nil {
		file.Close()
		return result, fmt.Errorf("write table header: %w", err)
	}
	for _, row := range decoded.Rows {
		if err := ctx.Err(); err != nil {
			file.Close()
			return result, err
		}
		if len(row.Values) != len(version.Definitions) {
			file.Close()
			return result, fmt.Errorf("row %d has %d values for %d columns", row.ID, len(row.Values), len(version.Definitions))
		}
		fields := make(map[string]any, len(row.Values))
		for index, column := range version.Definitions {
			fields[column.Name] = row.Values[index]
		}
		if err := writeJSONLine(writer, record{Kind: "record", ID: row.ID, Fields: fields}); err != nil {
			file.Close()
			return result, fmt.Errorf("write row %d: %w", row.ID, err)
		}
	}
	if err := writer.Flush(); err != nil {
		file.Close()
		return result, fmt.Errorf("flush table output: %w", err)
	}
	if err := file.Close(); err != nil {
		return result, fmt.Errorf("close table output: %w", err)
	}
	return result, nil
}

type tableHeader struct {
	Kind             string `json:"kind"`
	SchemaVersion    int    `json:"schemaVersion"`
	GameVersion      string `json:"gameVersion"`
	Source           string `json:"source"`
	Table            string `json:"table"`
	FileDataID       uint32 `json:"fileDataId"`
	ClientBuild      uint32 `json:"clientBuild"`
	DefinitionSHA256 string `json:"definitionSha256"`
}

type record struct {
	Kind   string         `json:"kind"`
	ID     int32          `json:"id"`
	Fields map[string]any `json:"fields"`
}

func loadManifest(path string, requested []string) ([]tableInput, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read local WoWDBDefs manifest %s: %w", path, err)
	}
	if info.Size() > maxManifestBytes {
		return nil, fmt.Errorf("local WoWDBDefs manifest exceeds %d-byte limit", maxManifestBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read local WoWDBDefs manifest %s: %w", path, err)
	}
	var entries []db2ManifestEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("parse local WoWDBDefs manifest %s: %w", path, err)
	}
	wanted := make(map[string]bool, len(requested))
	for _, name := range requested {
		if !validTableName.MatchString(name) {
			return nil, fmt.Errorf("invalid table name %q", name)
		}
		wanted[name] = true
	}
	found := make(map[string]bool)
	inputs := make([]tableInput, 0, len(entries))
	for _, entry := range entries {
		if entry.DB2FileDataID == 0 {
			continue
		}
		if !validTableName.MatchString(entry.TableName) {
			return nil, fmt.Errorf("unsafe table name in local manifest: %q", entry.TableName)
		}
		if len(wanted) > 0 && !wanted[entry.TableName] {
			continue
		}
		if found[entry.TableName] {
			return nil, fmt.Errorf("duplicate table name in local manifest: %s", entry.TableName)
		}
		found[entry.TableName] = true
		inputs = append(inputs, tableInput{name: entry.TableName, fileID: entry.DB2FileDataID})
	}
	if len(wanted) > 0 {
		for name := range wanted {
			if !found[name] {
				return nil, fmt.Errorf("table %s has no DB2 FileDataID in local manifest", name)
			}
		}
	}
	if len(inputs) == 0 {
		return nil, errors.New("local manifest contains no selected DB2 tables")
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].name < inputs[j].name })
	return inputs, nil
}

func findClientDirectory(explicit string) (string, error) {
	candidates := []string{}
	if explicit != "" {
		candidates = append(candidates, explicit)
	} else {
		candidates = append(candidates, "/Applications/World of Warcraft")
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates,
				filepath.Join(home, "Applications", "World of Warcraft"),
				filepath.Join(home, "Games", "World of Warcraft"),
			)
		}
	}
	var found []string
	for _, candidate := range candidates {
		path, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(filepath.Join(path, ".build.info"))
		if err == nil && !info.IsDir() {
			found = append(found, path)
		}
	}
	if len(found) == 0 {
		if explicit != "" {
			return "", fmt.Errorf("no .build.info file found under --client-dir %s", explicit)
		}
		return "", errors.New("no WoW installation found; pass --client-dir")
	}
	if len(found) > 1 {
		return "", fmt.Errorf("multiple WoW installations found; pass --client-dir: %s", strings.Join(found, ", "))
	}
	return found[0], nil
}

func selectProduct(entries []tact.AvailableBuild) (string, error) {
	if len(entries) == 1 {
		return entries[0].Product, nil
	}
	return "", errors.New("client has multiple products in .build.info; pass --product")
}

func ensureOutputOutsideClient(clientDir, outputDir string) error {
	resolvedClient, err := resolveExistingPath(clientDir)
	if err != nil {
		return fmt.Errorf("resolve client directory: %w", err)
	}
	resolvedOutput, err := resolveExistingPath(outputDir)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	relative, err := filepath.Rel(resolvedClient, resolvedOutput)
	if err != nil {
		return fmt.Errorf("compare client and output paths: %w", err)
	}
	if relative == "." || (!filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return errors.New("output directory must be outside the WoW installation")
	}
	return nil
}

func resolveExistingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var missing []string
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %s", path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, missing[i])
	}
	return filepath.Clean(resolved), nil
}

func repositoryRevision(ctx context.Context, dir string) string {
	command := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func replaceOutput(staged, output string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	if info, err := os.Stat(output); err == nil && !info.IsDir() {
		return fmt.Errorf("output path exists and is not a directory: %s", output)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	backup := ""
	if _, err := os.Stat(output); err == nil {
		backup, err = os.MkdirTemp(filepath.Dir(output), filepath.Base(output)+".previous-*")
		if err != nil {
			return err
		}
		if err := os.Remove(backup); err != nil {
			return err
		}
		if err := os.Rename(output, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(staged, output); err != nil {
		if backup != "" {
			if restoreErr := os.Rename(backup, output); restoreErr != nil {
				return fmt.Errorf("%w; restoring previous output also failed: %v", err, restoreErr)
			}
		}
		return err
	}
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("new output installed but failed to remove prior output %s: %w", backup, err)
		}
	}
	return nil
}

func writeJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func writeJSONLine(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
