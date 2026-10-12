package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseWorkflowIsManualAndScheduledWithoutLocalScrapers(t *testing.T) {
	root := repoRoot(t)
	update := string(mustRead(t, filepath.Join(root, ".github/workflows/database-update.yml")))
	for _, needle := range []string{
		`cron: "0 11 * * *"`,
		"workflow_dispatch",
		"actions/checkout@v7.0.1",
		"actions/setup-go@v7.0.0",
		"cmd/compile",
		"go test ./...",
		"peter-evans/create-pull-request@v8.1.1",
	} {
		if !strings.Contains(update, needle) {
			t.Fatalf("database workflow missing %q", needle)
		}
	}
	for _, localOnly := range []string{"fetch_quest_pages.py", "client-extract"} {
		if strings.Contains(update, localOnly) {
			t.Fatalf("scheduled workflow must not invoke local-only source %q", localOnly)
		}
	}
}

func TestReadmeLinksCurrentSourceAndDevelopmentDocs(t *testing.T) {
	root := repoRoot(t)
	readme := string(mustRead(t, filepath.Join(root, "README.md")))
	for _, needle := range []string{
		"All The Things",
		"Wowhead scraping is local and manual",
		"docs/INGESTION.md",
		"docs/CLIENT_EXTRACTION.md",
		"docs/ARCHITECTURE.md",
		"ATTRIBUTION.md",
	} {
		if !strings.Contains(readme, needle) {
			t.Fatalf("README missing %q", needle)
		}
	}
}

func TestClientExtractionDocsMatchCLIAndLimitations(t *testing.T) {
	root := repoRoot(t)
	docs := string(mustRead(t, filepath.Join(root, "docs/CLIENT_EXTRACTION.md")))
	main := string(mustRead(t, filepath.Join(root, "cmd/client-extract/main.go")))
	for _, flag := range []string{"--client-dir", "--product", "--dbd-dir", "--out", "--table"} {
		if !strings.Contains(main, `String("`+strings.TrimPrefix(flag, "--")+`"`) && !strings.Contains(main, `Var(&tables, "table"`) {
			t.Errorf("CLI does not define documented flag %s", flag)
		}
		if !strings.Contains(docs, flag) {
			t.Errorf("client extraction docs omit flag %s", flag)
		}
	}
	for _, needle := range []string{
		"Only WDC5 is supported",
		"partial_encrypted",
		"Encrypted sections are skipped",
		"`TactKey` contains",
		"content-decryption keys and is excluded",
		"server-side",
		"rerunning begins again",
		"table-level resume is not implemented",
	} {
		if !strings.Contains(docs, needle) {
			t.Errorf("client extraction docs omit limitation %q", needle)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
