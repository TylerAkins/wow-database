package clientextract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadManifestSortsAndFiltersByTable(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "manifest.json")
	raw := []byte(`[
		{"tableName":"Spell","db2FileDataID":42},
		{"tableName":"Achievement","db2FileDataID":0},
		{"tableName":"AreaTable","db2FileDataID":12}
	]`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadManifest(path, []string{"Spell", "AreaTable"})
	if err != nil {
		t.Fatal(err)
	}
	want := []tableInput{{name: "AreaTable", fileID: 12}, {name: "Spell", fileID: 42}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadManifest() = %#v, want %#v", got, want)
	}
}

func TestLoadManifestRejectsUnsafeAndDuplicateNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{name: "path traversal", data: `[{"tableName":"../Escape","db2FileDataID":1}]`},
		{name: "duplicate", data: `[{"tableName":"Spell","db2FileDataID":1},{"tableName":"Spell","db2FileDataID":2}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadManifest(path, nil); err == nil {
				t.Fatal("expected manifest validation error")
			}
		})
	}
}

func TestOutputPreservesZeroAndEmptyValues(t *testing.T) {
	var got record
	raw := []byte(`{"kind":"record","id":0,"fields":{"empty":"","zero":0,"list":[]}}`)
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != 0 || got.Fields["empty"] != "" || got.Fields["zero"] != float64(0) {
		t.Fatalf("decoded record lost explicit values: %#v", got)
	}
	if values, ok := got.Fields["list"].([]any); !ok || len(values) != 0 {
		t.Fatalf("decoded empty list = %#v", got.Fields["list"])
	}
}

func TestEnsureOutputOutsideClient(t *testing.T) {
	client := filepath.Join(t.TempDir(), "World of Warcraft")
	if err := os.MkdirAll(client, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureOutputOutsideClient(client, filepath.Join(client, ".local", "dump")); err == nil {
		t.Fatal("output inside game installation should be rejected")
	}
	if err := ensureOutputOutsideClient(client, filepath.Join(filepath.Dir(client), "export")); err != nil {
		t.Fatalf("external output path rejected: %v", err)
	}
}

func TestEnsureOutputOutsideClientResolvesSymlinks(t *testing.T) {
	root := t.TempDir()
	client := filepath.Join(root, "World of Warcraft")
	if err := os.MkdirAll(filepath.Join(client, "Data"), 0o700); err != nil {
		t.Fatal(err)
	}
	clientAlias := filepath.Join(root, "client-link")
	if err := os.Symlink(client, clientAlias); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := ensureOutputOutsideClient(client, filepath.Join(clientAlias, "Data", "extract")); err == nil {
		t.Fatal("output through a symlink into the client should be rejected")
	}

	outputAlias := filepath.Join(root, "output-link")
	if err := os.Symlink(filepath.Join(client, "Data"), outputAlias); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := ensureOutputOutsideClient(client, outputAlias); err == nil {
		t.Fatal("output symlink resolving into the client should be rejected")
	}
}

func TestReplaceOutputPublishesDirectoryAndRejectsFile(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "snapshot")
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "old.json"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(root, "staged")
	if err := os.Mkdir(staged, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "new.json"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceOutput(staged, output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "old.json")); !os.IsNotExist(err) {
		t.Fatalf("old output still exists: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(output, "new.json")); err != nil || string(got) != "new" {
		t.Fatalf("new output = %q, %v", got, err)
	}

	fileOutput := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(fileOutput, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceOutput(staged, fileOutput); err == nil {
		t.Fatal("replacing an existing file should fail")
	}
	if got, err := os.ReadFile(fileOutput); err != nil || string(got) != "preserve" {
		t.Fatalf("failed replace damaged existing file: %q, %v", got, err)
	}
}
