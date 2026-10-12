// Version selection for .dbd definitions.
// Derived from DBDefsLib types (https://github.com/wowdev/WoWDBDefs).
// Copyright 2022 WoWDBDefs Contributors. BSD-3-Clause — see tools/db2tool/NOTICES.md.

package dbd

import "fmt"

// SelectVersion returns the last version block that lists the complete
// four-part client build. Build ranges and layout hashes are deliberately not
// consulted: an unlisted build must fail rather than use a near-match layout.
func SelectVersion(def DBDefinition, build Build) (VersionDefinitions, error) {
	for i := len(def.VersionDefinitions) - 1; i >= 0; i-- {
		for _, b := range def.VersionDefinitions[i].Builds {
			if b == build {
				return def.VersionDefinitions[i], nil
			}
		}
	}
	return VersionDefinitions{}, fmt.Errorf(
		"build %s not found in the .dbd definition; WoWDBDefs may not contain this build yet", build.String())
}
