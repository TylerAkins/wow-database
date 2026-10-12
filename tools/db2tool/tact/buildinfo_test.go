package tact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBuildInfoAndSelectActiveProduct(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".build.info")
	content := "Branch!STRING:0|Active!DEC:1|Build Key!HEX:16|Version!STRING:0|Product!STRING:0\n" +
		"us|0|1111111111111111|1.60.0.70000|wow_classic_beta\n" +
		"us|1|2222222222222222|1.60.1.70338|wow_classic_beta\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := ParseBuildInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectBuild(entries, "wow_classic_beta")
	if err != nil {
		t.Fatal(err)
	}
	if !selected.Active || selected.Version != "1.60.1.70338" || selected.BuildConfig != "2222222222222222" {
		t.Fatalf("selected build = %+v", selected)
	}
	if _, err := SelectBuild(entries, "wow"); err == nil {
		t.Fatal("unknown product should fail")
	}
}

func TestSelectBuildRejectsAmbiguousOrInactiveProduct(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []AvailableBuild
	}{
		{
			name:    "no active entry",
			entries: []AvailableBuild{{Product: "wow", Version: "1.2.3.4", Active: false}},
		},
		{
			name:    "multiple active entries",
			entries: []AvailableBuild{{Product: "wow", Version: "1.2.3.4", Active: true}, {Product: "wow", Version: "1.2.3.5", Active: true}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := SelectBuild(tc.entries, "wow"); err == nil {
				t.Fatal("ambiguous or inactive product should fail")
			}
		})
	}
}

func TestParseBuildInfoRejectsMalformedEntries(t *testing.T) {
	for _, content := range []string{
		"us|1|abc|1.60.1.70338|wow_classic_beta\n",
		"Branch!STRING:0|Active!DEC:1|Build Key!HEX:16|Version!STRING:0|Product!STRING:0\n" +
			"us|2|abc|1.60.1.70338|wow_classic_beta\n",
		"Branch!STRING:0|Active!DEC:1|Build Key!HEX:16|Version!STRING:0|Product!STRING:0\n" +
			"us|1|abc|1.60.1|wow_classic_beta\n",
	} {
		path := filepath.Join(t.TempDir(), ".build.info")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ParseBuildInfo(path); err == nil {
			t.Fatalf("ParseBuildInfo accepted malformed content %q", content)
		}
	}
}
