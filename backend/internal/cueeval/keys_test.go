// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package cueeval

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestWalkKeysFlattensTopLevelFields(t *testing.T) {
	v := cuecontext.New().CompileString(`
people: {
	george: {name: "George", year: 1938}
	lorraine: {name: "Lorraine"}
}
diagram: {
	nodes: {a: {label: "a"}}
	edges: [{id: "e1"}]
}
"quoted-key": {inner: 1}
#Def: {field: 1}
_hidden: {x: 1}
`)
	if err := v.Err(); err != nil {
		t.Fatalf("compile: %v", err)
	}

	var keys []string
	walkKeys(v, "", 0, &keys)
	got := map[string]bool{}
	for _, k := range keys {
		got[k] = true
	}

	// Every top-level data field is flattened, not just the diagram.
	for _, want := range []string{
		"people", "people.george", "people.george.name", "people.george.year",
		"people.lorraine", "people.lorraine.name",
		"diagram", "diagram.nodes", "diagram.nodes.a", "diagram.nodes.a.label",
		"diagram.edges",
	} {
		if !got[want] {
			t.Errorf("missing key %q (got %v)", want, keys)
		}
	}

	// Lists carry their own path but are not descended.
	for _, k := range keys {
		if strings.Contains(k, "e1") || k == "diagram.edges.0" {
			t.Errorf("descended into a list: %q", k)
		}
	}

	// Definitions, hidden fields, and non-identifier keys are skipped.
	for _, bad := range []string{"#Def", "Def", "_hidden", "quoted-key"} {
		if got[bad] {
			t.Errorf("emitted skipped field %q", bad)
		}
	}
}

func TestWalkKeysBoundsDepth(t *testing.T) {
	// A struct nested deeper than keyDepthMax stops contributing paths past the bound.
	var b strings.Builder
	for i := 0; i < keyDepthMax+3; i++ {
		b.WriteString("a: {")
	}
	b.WriteString("leaf: 1")
	for i := 0; i < keyDepthMax+3; i++ {
		b.WriteString("}")
	}
	v := cuecontext.New().CompileString(b.String())
	if err := v.Err(); err != nil {
		t.Fatalf("compile: %v", err)
	}

	var keys []string
	walkKeys(v, "", 0, &keys)
	deepest := strings.Repeat("a.", keyDepthMax-1) + "a"
	for _, k := range keys {
		if strings.Count(k, "a") > keyDepthMax {
			t.Errorf("key exceeds depth bound: %q", k)
		}
	}
	if !contains(keys, deepest) {
		t.Errorf("expected a key at the depth bound %q (got %v)", deepest, keys)
	}
}

func contains(keys []string, target string) bool {
	for _, k := range keys {
		if k == target {
			return true
		}
	}
	return false
}
