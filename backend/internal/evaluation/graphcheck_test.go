// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package evaluation

import (
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue/cuecontext"

	"github.com/stratorys/cueto/backend/internal/diag"
)

// runChecks compiles src and walks it for @file/@uri failures against moduleDir,
// the unit under test for the Layer-2 graph checks.
func runChecks(t *testing.T, moduleDir, src string) []diag.Diagnostic {
	t.Helper()
	v := cuecontext.New().CompileString(src)
	if err := v.Err(); err != nil {
		t.Fatalf("compile: %v", err)
	}
	var out []diag.Diagnostic
	walkChecks(v, moduleDir, map[string]struct{}{}, &out)
	return out
}

func TestWalkChecks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		src   string
		wants int
	}{
		{"file present", `x: "README.md" @file()`, 0},
		{"file relative present", `x: "./README.md" @file()`, 0},
		{"file missing", `x: "nope.md" @file()`, 1},
		{"file escapes", `x: "../secret" @file()`, 1},
		{"file absolute rejected", `x: "/etc/passwd" @file()`, 1},
		{"non-concrete schema field skipped", `x: string @file()`, 0},
		{"uri relative present", `x: "docs/a.md" @uri()`, 0},
		{"uri relative missing", `x: "docs/missing.md" @uri()`, 1},
		{"uri file scheme present", `x: "file://README.md" @uri()`, 0},
		{"uri http syntactic ok, no network", `x: "https://example.com/x" @uri()`, 0},
		{"uri http invalid", `x: "https://" @uri()`, 1},
		{"uri cue resolves", `people: marty: {name: "M"}` + "\n" + `x: "cue://people/marty" @uri()`, 0},
		{"uri cue dangling", `people: marty: {name: "M"}` + "\n" + `x: "cue://people/nobody" @uri()`, 1},
		{"typo attribute ignored", `x: "nope.md" @fil()`, 0},
		{"pattern constraint propagates to member", `reg: [_]: {readme: string @file()}` + "\n" + `reg: a: {readme: "nope.md"}`, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runChecks(t, dir, tc.src)
			if len(got) != tc.wants {
				t.Fatalf("got %d diagnostics, want %d: %+v", len(got), tc.wants, got)
			}
			for _, d := range got {
				if d.Kind != diag.KindReference {
					t.Errorf("diagnostic kind = %q, want %q", d.Kind, diag.KindReference)
				}
			}
		})
	}
}

func TestResolveWithin(t *testing.T) {
	dir := "/mod"
	cases := []struct {
		rel string
		ok  bool
	}{
		{"README.md", true},
		{"docs/a.md", true},
		{"./a.md", true},
		{"../escape", false},
		{"docs/../../escape", false},
		{"/etc/passwd", false},
		{"", false},
	}
	for _, tc := range cases {
		if _, ok := resolveWithin(dir, tc.rel); ok != tc.ok {
			t.Errorf("resolveWithin(%q) ok = %v, want %v", tc.rel, ok, tc.ok)
		}
	}
}
