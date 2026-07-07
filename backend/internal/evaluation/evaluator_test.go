// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/domain"
)

func TestRecoverToResultCatchesPanic(t *testing.T) {
	result := recoverToResult(func() buildResult {
		panic("boom")
	})
	if !errors.Is(result.err, errEvalPanic) {
		t.Fatalf("err = %v, want errEvalPanic", result.err)
	}
}

func TestRecoverToResultPassesThrough(t *testing.T) {
	want := buildResult{diags: []diag.Diagnostic{{Message: "x", Kind: diag.KindIncomplete}}}
	result := recoverToResult(func() buildResult { return want })
	if len(result.diags) != 1 || result.diags[0].Message != "x" {
		t.Fatalf("diags = %+v, want pass-through of %+v", result.diags, want.diags)
	}
	if result.err != nil {
		t.Fatalf("err = %v, want nil", result.err)
	}
}

// realEngine builds an Engine against the repo's real cue/ package so tests that
// exercise the schema import resolve github.com/stratorys/cueto/diagram.
func realEngine(t *testing.T) *Engine {
	t.Helper()
	abs, err := filepath.Abs("../../../cue")
	if err != nil {
		t.Fatalf("abs cue dir: %v", err)
	}
	return New(abs, 5*time.Second, 4<<20)
}

// A user membrane (shape A per the knowledge-as-code doc): a `people` map whose
// #PersonKey disjunction is derived from the data, self-referential parent
// references constrained back to existing keys, and a `diagram` *derived* from the
// membrane by importing the schema package. Authored in-test, never committed as a
// cue/ package. This pins the whole derivation pipeline: clean data vets, and a
// parent reference to a non-existent person is a compile error, not a broken edge.
const familyMembrane = `package main

import d "github.com/stratorys/cueto/diagram"

#PersonKey: or([for k, _ in people {k}])

#Person: {
	name:   string
	mother: #PersonKey | *""
	father: #PersonKey | *""
	year:   int
}

people: [ID=string]: #Person
people: {
	george:   {name: "George McFly", year: 1938}
	lorraine: {name: "Lorraine Baines", year: 1937}
	marty:    {name: "Marty McFly", mother: "lorraine", father: "george", year: 1968}
	dave:     {name: "Dave McFly", mother: "lorraine", father: "george", year: 1960}
	linda:    {name: "Linda McFly", mother: "lorraine", father: "george", year: 1965}
	jennifer: {name: "Jennifer Parker", year: 1968}
}

diagram: d.#Diagram & {
	nodes: {for pid, p in people {(pid): {type: "entity", label: p.name}}}
	edges: [
		for pid, p in people if p.mother != "" {
			{id: "m_\(pid)", source: p.mother, target: pid, kind: "arrow", label: "mother"}
		},
		for pid, p in people if p.father != "" {
			{id: "f_\(pid)", source: p.father, target: pid, kind: "arrow", label: "father"}
		},
	]
}
`

func TestMembraneFamilyTreeVetsCleanAndDerives(t *testing.T) {
	e := realEngine(t)
	files := []domain.File{{Name: "data.cue", Content: familyMembrane}}

	diags, err := e.Vet(context.Background(), Source{Dir: e.cueDir, Overlay: files})
	if err != nil {
		t.Fatalf("vet: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("want clean vet, got %+v", diags)
	}

	out, _, evalDiags, err := e.Eval(context.Background(), Source{Dir: e.cueDir, Overlay: files})
	if err != nil || len(evalDiags) != 0 {
		t.Fatalf("eval err=%v diags=%+v", err, evalDiags)
	}
	var got struct {
		Nodes map[string]json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode diagram: %v", err)
	}
	// One node per person: the diagram is derived from the six-person membrane.
	if len(got.Nodes) != 6 {
		t.Fatalf("nodes = %d, want 6", len(got.Nodes))
	}
}

func TestMembraneFamilyTreeDanglingReferenceFails(t *testing.T) {
	e := realEngine(t)
	// Point Marty's father at a person key that does not exist. The #PersonKey
	// disjunction rejects it, so vet must fail at people.marty.father.
	dangling := strings.Replace(familyMembrane,
		`marty:    {name: "Marty McFly", mother: "lorraine", father: "george", year: 1968}`,
		`marty:    {name: "Marty McFly", mother: "lorraine", father: "ghost", year: 1968}`, 1)
	files := []domain.File{{Name: "data.cue", Content: dangling}}

	diags, err := e.Vet(context.Background(), Source{Dir: e.cueDir, Overlay: files})
	if err != nil {
		t.Fatalf("vet: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("want a diagnostic for the dangling parent reference, got none")
	}
	var anchored bool
	for _, d := range diags {
		if strings.Contains(d.Message, "people.marty.father") && d.Line > 0 {
			anchored = true
		}
	}
	if !anchored {
		t.Fatalf("want a diagnostic anchored at people.marty.father with a source line, got %+v", diags)
	}
}

// tempModule writes a throwaway CUE module (its own cue.mod plus the given files,
// keyed by module-relative path) and returns an Engine pointed at it. The fixtures'
// diagram field is a plain concrete struct, so no schema import is needed; hints
// degrade to a no-op because the temp module has no diagram package.
func tempModule(t *testing.T, files map[string]string) *Engine {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("cue.mod/module.cue", "module: \"example.com/m\"\nlanguage: version: \"v0.17.0\"\n")
	for rel, content := range files {
		write(rel, content)
	}
	return New(dir, 5*time.Second, 4<<20)
}

func decodeNodes(t *testing.T, out json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var got struct {
		Nodes map[string]json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode diagram: %v", err)
	}
	return got.Nodes
}

const subModuleRoot = `package main

import "example.com/m/sub"

diagram: {nodes: sub.nodes}
`

// Recursive load resolves an on-disk subpackage the root imports.
func TestBuildLoadsSubpackageFromDisk(t *testing.T) {
	e := tempModule(t, map[string]string{
		"data.cue":    subModuleRoot,
		"sub/sub.cue": "package sub\n\nnodes: {a: {type: \"entity\", label: \"A\"}}\n",
	})
	out, _, diags, err := e.Eval(context.Background(), Source{Dir: e.cueDir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("eval err=%v diags=%+v", err, diags)
	}
	if nodes := decodeNodes(t, out); len(nodes) != 1 || nodes["a"] == nil {
		t.Fatalf("nodes = %v, want exactly {a}", nodes)
	}
}

// A subdirectory overlay key lands in the right instance and unifies with the disk file.
func TestBuildSubdirOverlayLandsInSubpackage(t *testing.T) {
	e := tempModule(t, map[string]string{
		"data.cue":    subModuleRoot,
		"sub/sub.cue": "package sub\n\nnodes: {a: {type: \"entity\", label: \"A\"}}\n",
	})
	overlay := []domain.File{{Name: "sub/extra.cue", Content: "package sub\n\nnodes: {b: {type: \"entity\", label: \"B\"}}\n"}}
	out, _, diags, err := e.Eval(context.Background(), Source{Dir: e.cueDir, Overlay: overlay})
	if err != nil || len(diags) != 0 {
		t.Fatalf("eval err=%v diags=%+v", err, diags)
	}
	if nodes := decodeNodes(t, out); len(nodes) != 2 {
		t.Fatalf("nodes = %v, want a and b", nodes)
	}
}

// A broken sibling package the root does not import must not fail eval.
func TestBuildIgnoresBrokenSiblingPackage(t *testing.T) {
	e := tempModule(t, map[string]string{
		"data.cue":          "package main\n\ndiagram: {nodes: {a: {type: \"entity\", label: \"A\"}}}\n",
		"broken/broken.cue": "package broken\n\nthis is not valid cue !!!\n",
	})
	out, _, diags, err := e.Eval(context.Background(), Source{Dir: e.cueDir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("eval err=%v diags=%+v", err, diags)
	}
	if len(out) == 0 {
		t.Fatal("want diagram JSON, got empty")
	}
}
