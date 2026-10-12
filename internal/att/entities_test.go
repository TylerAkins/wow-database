package att

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadEntityCatalogKeepsTypedEntriesAndContainment(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, ".contrib", ".db", "forever")
	if err := os.MkdirAll(filepath.Join(base, ".config", "constants"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "zones"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, ".config", "constants", "maps.lua"), []byte("MAP = {\n  TEST_ZONE = 1,\n}"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `root({
	n(100, { -- Test NPC
		["lvl"] = 60,
		["isRaid"] = true,
		["groups"] = {
			i(200, { -- Test item
				["name"] = "Explicit Item Name",
			}),
			o(300), -- Test object
			ach(400, { -- Test achievement
				["description"] = "Achievement description",
			}),
		},
	}),
})`
	if err := os.WriteFile(filepath.Join(base, "zones", "entities.lua"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := ReadEntityCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Declarations) != 4 {
		t.Fatalf("got %d declarations, want 4: %#v", len(catalog.Declarations), catalog.Declarations)
	}
	if len(catalog.Containments) != 3 {
		t.Fatalf("got %d containment links, want 3: %#v", len(catalog.Containments), catalog.Containments)
	}
	var npc, item EntityDeclaration
	for _, declaration := range catalog.Declarations {
		if declaration.EntityType == "npc" {
			npc = declaration
		}
		if declaration.EntityType == "item" {
			item = declaration
		}
	}
	if npc.Label != "Test NPC" || npc.Fields["level"] != 60 || npc.Fields["isRaid"] != true {
		t.Fatalf("NPC declaration lost its literal fields: %#v", npc)
	}
	if item.Fields["name"] != "Explicit Item Name" || item.SourcePath != filepath.ToSlash(filepath.Join(".contrib", ".db", "forever", "zones", "entities.lua")) {
		t.Fatalf("item declaration lost name or source path: %#v", item)
	}
}
