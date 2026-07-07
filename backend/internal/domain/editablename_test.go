// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package domain

import "testing"

func TestValidEditableName(t *testing.T) {
	ok := []string{
		"data.cue", "nodes.cue", "my_file-2.cue", "A.cue",
		"sub/data.cue", // subdirectory
		"a/b/c.cue",    // nested subdirectories
		"team_1/nodes-2.cue",
		"diagram.cue", // a root file named diagram.cue collides with nothing
		"schema.cue",  // no longer reserved: the schema lives in diagram/
		"Schema.cue",  // no longer reserved
	}
	for _, name := range ok {
		if !ValidEditableName(name) {
			t.Errorf("ValidEditableName(%q) = false, want true", name)
		}
	}
	bad := []string{
		"../data.cue",        // traversal
		"sub/../data.cue",    // traversal mid-path
		"a/./b.cue",          // current-dir segment
		"sub//data.cue",      // doubled separator
		"/abs/data.cue",      // absolute
		"sub/",               // trailing separator, no filename
		"a\\b.cue",           // backslash separator
		"cue.mod/module.cue", // reserved module dir
		"diagram/x.cue",      // reserved diagram dir
		"Diagram/x.cue",      // reserved, case-insensitive (APFS)
		"a.b/c.cue",          // dotted directory segment
		"dａta.cue",           // unicode look-alike (fullwidth a)
		"data.txt",           // wrong suffix
		"data.cue.bak",       // extra dot
		".cue",               // no stem
		"data",               // no suffix
		"",                   // empty
		".",                  // dot
		"..",                 // parent
	}
	for _, name := range bad {
		if ValidEditableName(name) {
			t.Errorf("ValidEditableName(%q) = true, want false", name)
		}
	}
}

func TestLexRejectsIllegalBytes(t *testing.T) {
	for _, name := range []string{"a\\b.cue", "a:b.cue", "a b.cue", "a\tb.cue", "dａta.cue"} {
		if _, err := lex(name); err == nil {
			t.Errorf("lex(%q) = nil error, want error", name)
		}
	}
}
