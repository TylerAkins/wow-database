package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowScheduleParses(t *testing.T) {
	root := repoRoot(t)
	ci := string(mustRead(t, filepath.Join(root, ".github/workflows/ci.yml")))
	update := string(mustRead(t, filepath.Join(root, ".github/workflows/database-update.yml")))
	for _, body := range []string{ci, update} {
		if !strings.Contains(body, "actions/checkout@v7.0.1") {
			t.Fatal("checkout is not pinned to v7.0.1")
		}
		if !strings.Contains(body, "actions/setup-go@v7.0.0") {
			t.Fatal("setup-go is not pinned to v7.0.0")
		}
	}
	for _, needle := range []string{
		`cron: "0 11 * * *"`,
		"workflow_dispatch",
		"contents: write",
		"pull-requests: write",
		"forever-database-update",
		"cmd/compile",
		"go test ./...",
		"peter-evans/create-pull-request@v8.1.1",
		".cache/AllTheThings",
		"--att-root",
	} {
		if !strings.Contains(update, needle) {
			t.Fatalf("update workflow missing %s", needle)
		}
	}
	for _, path := range []string{
		".github/workflows/update-att.yml",
		".github/workflows/update-questiedb.yml",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err == nil {
			t.Fatalf("legacy workflow %s should be removed", path)
		}
	}
}

func TestReadme_PrivateCheckout(t *testing.T) {
	body := string(mustRead(t, filepath.Join(repoRoot(t), "README.md")))
	if !strings.Contains(body, "secrets.WOW_DATABASE_TOKEN") {
		t.Fatal("README has no private checkout token example")
	}
	if !strings.Contains(body, "does not change repository visibility") {
		t.Fatal("README does not say the workflow leaves visibility alone")
	}
}

func TestDocs_DescribeATTOnlyPipeline(t *testing.T) {
	root := repoRoot(t)
	readme := string(mustRead(t, filepath.Join(root, "README.md")))
	dev := string(mustRead(t, filepath.Join(root, "docs/DEVELOPMENT.md")))
	manifest := string(mustRead(t, filepath.Join(root, "docs/MANIFEST.md")))
	for _, needle := range []string{
		"AllTheThings",
		"--att-root",
		"docs/MANIFEST.md",
		"att-update",
	} {
		if !strings.Contains(readme, needle) {
			t.Fatalf("README missing %q", needle)
		}
	}
	if strings.Contains(readme, "QuestieDB defines") || strings.Contains(readme, "merge pipeline") {
		t.Fatal("README still describes the Questie merge pipeline")
	}
	if !strings.Contains(dev, "internal/att") {
		t.Fatal("DEVELOPMENT.md should document internal/att")
	}
	if !strings.Contains(dev, "LuaJIT and QuestieDB are **not** used") {
		t.Fatal("DEVELOPMENT.md should state LuaJIT and QuestieDB are not used")
	}
	for _, needle := range []string{"parse.issues", "sources", "merge"} {
		if !strings.Contains(manifest, needle) {
			t.Fatalf("MANIFEST.md missing %q", needle)
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
