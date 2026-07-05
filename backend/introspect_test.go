// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import "testing"

func TestBuildCueMetaEnumeratesPackages(t *testing.T) {
	meta := buildCueMeta()

	if len(meta.Packages) != len(replPackages) {
		t.Fatalf("got %d packages, want %d", len(meta.Packages), len(replPackages))
	}
	if len(meta.Builtins) == 0 {
		t.Fatal("expected builtins to be non-empty")
	}

	byName := map[string]CuePackage{}
	for _, p := range meta.Packages {
		byName[p.Name] = p
	}

	strs, ok := byName["strings"]
	if !ok {
		t.Fatal("strings package missing")
	}
	if strs.Path != "strings" {
		t.Fatalf("strings path = %q, want strings", strs.Path)
	}
	if !hasFunc(strs.Members, "ToUpper") {
		t.Fatalf("strings.ToUpper not enumerated as a func: %+v", strs.Members)
	}

	// A path-qualified package binds to its last segment and still enumerates.
	json, ok := byName["json"]
	if !ok || json.Path != "encoding/json" {
		t.Fatalf("encoding/json not enumerated correctly: %+v", json)
	}
	if !hasFunc(json.Members, "Marshal") {
		t.Fatalf("json.Marshal missing: %+v", json.Members)
	}

	// math exposes value constants (Pi), which must not be flagged as funcs.
	math, ok := byName["math"]
	if !ok {
		t.Fatal("math package missing")
	}
	for _, m := range math.Members {
		if m.Name == "Pi" && m.IsFunc {
			t.Fatal("math.Pi wrongly flagged as a func")
		}
	}
}

func TestIntrospectMemoizes(t *testing.T) {
	e := &cueEvaluator{}
	first := e.Introspect()
	second := e.Introspect()
	if len(first.Packages) == 0 {
		t.Fatal("expected packages")
	}
	if len(first.Packages) != len(second.Packages) {
		t.Fatal("memoized result diverged")
	}
}

func hasFunc(members []CueMember, name string) bool {
	for _, m := range members {
		if m.Name == name && m.IsFunc {
			return true
		}
	}
	return false
}
