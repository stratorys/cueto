// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package authoring

import (
	"testing"

	"github.com/stratorys/cueto/backend/internal/domain"
)

func TestProvenanceFromAllShapes(t *testing.T) {
	files := []domain.File{
		// Embedded #Diagram & {…} with a nodes struct and the edge list.
		{Name: "a.cue", Content: `package diagram
diagram: #Diagram & {
	nodes: {
		a: {type: "process", x: 1, y: 1, label: "a"}
		b: {type: "process", x: 2, y: 2, label: "b"}
	}
	edges: [{id: "e1", source: "a", target: "b", kind: "arrow"}]
}
`},
		// Path form: diagram: nodes: c: {…}
		{Name: "b.cue", Content: `package diagram
diagram: nodes: c: {type: "process", x: 3, y: 3, label: "c"}
`},
		// Plain struct: diagram: { nodes: { d: {…} } }
		{Name: "c.cue", Content: `package diagram
diagram: {
	nodes: {
		d: {type: "process", x: 4, y: 4, label: "d"}
	}
}
`},
	}
	prov := provenanceFrom(files)
	want := map[string]string{"a": "a.cue", "b": "a.cue", "c": "b.cue", "d": "c.cue"}
	if len(prov.Nodes) != len(want) {
		t.Fatalf("nodes = %+v, want %+v", prov.Nodes, want)
	}
	for id, file := range want {
		if prov.Nodes[id] != file {
			t.Errorf("node %q -> %q, want %q", id, prov.Nodes[id], file)
		}
	}
	if prov.Edges != "a.cue" {
		t.Errorf("edges owner = %q, want a.cue", prov.Edges)
	}
}

func TestProvenanceFirstDeclarationWins(t *testing.T) {
	// A node id declared in two files is attributed to the first file in order.
	files := []domain.File{
		{Name: "first.cue", Content: "package diagram\ndiagram: nodes: shared: {x: 1}\n"},
		{Name: "second.cue", Content: "package diagram\ndiagram: nodes: shared: {y: 2}\n"},
	}
	prov := provenanceFrom(files)
	if prov.Nodes["shared"] != "first.cue" {
		t.Errorf("shared -> %q, want first.cue", prov.Nodes["shared"])
	}
}

func TestProvenanceSkipsUnparseable(t *testing.T) {
	files := []domain.File{
		{Name: "broken.cue", Content: "package diagram\ndiagram: nodes: {"},
		{Name: "ok.cue", Content: "package diagram\ndiagram: nodes: e: {x: 1}\n"},
	}
	prov := provenanceFrom(files)
	if prov.Nodes["e"] != "ok.cue" {
		t.Errorf("e -> %q, want ok.cue", prov.Nodes["e"])
	}
}
