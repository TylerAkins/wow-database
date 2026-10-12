// Go translation of TACTSharp's BuildInfo (https://github.com/wowdev/TACTSharp,
// v0.0.13-alpha, commit d0ab516eb98b5db35682467b6e4977d88955046d).
// Copyright (c) 2024 Martin Benjamins. MIT License — see tools/db2tool/NOTICES.md.

package tact

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// AvailableBuild is one .build.info product entry, reduced to the fields this
// local-only reader consumes.
type AvailableBuild struct {
	Active      bool
	BuildConfig string
	Version     string
	Product     string
}

// ParseBuildInfo reads <BaseDir>/.build.info (typed pipe format: the header
// line names columns as "Name!TYPE:len") and returns all product entries.
func ParseBuildInfo(path string) ([]AvailableBuild, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []AvailableBuild
	headerMap := map[string]int{}
	headerSeen := false
	for line := range strings.SplitSeq(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}
		split := strings.Split(line, "|")
		if strings.HasPrefix(split[0], "Branch!") {
			headerSeen = true
			for i, header := range split {
				headerMap[strings.Split(header, "!")[0]] = i
			}
			continue
		}
		if !headerSeen {
			return nil, fmt.Errorf("%s: build entry appears before header", path)
		}
		col := func(name string) string {
			idx, ok := headerMap[name]
			if !ok || idx >= len(split) {
				return ""
			}
			return split[idx]
		}
		activeText := col("Active")
		active, err := strconv.Atoi(activeText)
		if err != nil || (active != 0 && active != 1) {
			return nil, fmt.Errorf("%s: invalid Active value %q", path, activeText)
		}
		entry := AvailableBuild{
			Active:      active == 1,
			BuildConfig: col("Build Key"),
			Version:     col("Version"),
			Product:     col("Product"),
		}
		if entry.BuildConfig == "" || entry.Version == "" || entry.Product == "" {
			return nil, fmt.Errorf("%s: build entry is missing build key, version, or product", path)
		}
		if _, err := BuildNumber(entry.Version); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		entries = append(entries, entry)
	}
	if !headerSeen || len(entries) == 0 {
		return nil, fmt.Errorf("%s: no build entries found", path)
	}
	return entries, nil
}

// SelectBuild returns the first entry for the given product.
func SelectBuild(entries []AvailableBuild, product string) (AvailableBuild, error) {
	var active []AvailableBuild
	found := false
	for _, e := range entries {
		if e.Product == product {
			found = true
			if e.Active {
				active = append(active, e)
			}
		}
	}
	if !found {
		return AvailableBuild{}, fmt.Errorf("product %q not found in .build.info", product)
	}
	if len(active) != 1 {
		return AvailableBuild{}, fmt.Errorf("product %q has %d active builds in .build.info; cannot select one safely", product, len(active))
	}
	return active[0], nil
}

// BuildNumber extracts the trailing build number from a 4-part version
// string.
func BuildNumber(version string) (uint32, error) {
	split := strings.Split(version, ".")
	if len(split) != 4 {
		return 0, fmt.Errorf("invalid build %q", version)
	}
	n, err := strconv.ParseUint(split[3], 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid build %q: %w", version, err)
	}
	return uint32(n), nil
}
