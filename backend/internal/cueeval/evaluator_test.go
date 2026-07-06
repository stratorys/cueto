// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package cueeval

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stratorys/cueto/backend/internal/config"
	"github.com/stratorys/cueto/backend/internal/diag"
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

// realEvaluator builds an Evaluator against the repo's real cue/ package so tests
// that exercise the schema import resolve github.com/stratorys/cueto/diagram.
func realEvaluator(t *testing.T) Evaluator {
	t.Helper()
	abs, err := filepath.Abs("../../../cue")
	if err != nil {
		t.Fatalf("abs cue dir: %v", err)
	}
	return New(config.Config{
		CueDir:         abs,
		VersionsDir:    t.TempDir(),
		MaxOutputBytes: 4 << 20,
		EvalTimeout:    5 * time.Second,
	})
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
	e := realEvaluator(t)
	files := []File{{Name: "data.cue", Content: familyMembrane}}

	diags, err := e.Vet(context.Background(), files)
	if err != nil {
		t.Fatalf("vet: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("want clean vet, got %+v", diags)
	}

	out, _, _, evalDiags, err := e.Eval(context.Background(), files)
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
	e := realEvaluator(t)
	// Point Marty's father at a person key that does not exist. The #PersonKey
	// disjunction rejects it, so vet must fail at people.marty.father.
	dangling := strings.Replace(familyMembrane,
		`marty:    {name: "Marty McFly", mother: "lorraine", father: "george", year: 1968}`,
		`marty:    {name: "Marty McFly", mother: "lorraine", father: "ghost", year: 1968}`, 1)
	files := []File{{Name: "data.cue", Content: dangling}}

	diags, err := e.Vet(context.Background(), files)
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
