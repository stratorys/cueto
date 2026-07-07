// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package domain

import "testing"

func TestValidEditableName(t *testing.T) {
	ok := []string{"data.cue", "nodes.cue", "my_file-2.cue", "A.cue"}
	for _, name := range ok {
		if !ValidEditableName(name) {
			t.Errorf("ValidEditableName(%q) = false, want true", name)
		}
	}
	bad := []string{
		"schema.cue",    // reserved
		"Schema.cue",    // reserved, case-insensitive (APFS)
		"SCHEMA.CUE",    // reserved, case-insensitive
		"../schema.cue", // traversal
		"a/b.cue",       // separator
		"a\\b.cue",      // backslash separator
		"data.txt",      // wrong suffix
		"data.cue.bak",  // extra dot
		".cue",          // no stem
		"data",          // no suffix
		"",              // empty
		".",             // dot
		"..",            // parent
		"/etc/data.cue", // absolute
	}
	for _, name := range bad {
		if ValidEditableName(name) {
			t.Errorf("ValidEditableName(%q) = true, want false", name)
		}
	}
}
