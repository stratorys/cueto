// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package evaluation

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"cuelang.org/go/cue/cuecontext"

	"github.com/stratorys/cueto/backend/internal/domain"
)

// gotDiagram is the decoded shape of an inferred diagram, enough to assert node and
// edge sets and the label/data of a node.
type gotDiagram struct {
	Nodes map[string]struct {
		Label string                 `json:"label"`
		Data  map[string]interface{} `json:"data"`
	} `json:"nodes"`
	Edges []struct {
		ID     string `json:"id"`
		Source string `json:"source"`
		Target string `json:"target"`
		Label  string `json:"label"`
	} `json:"edges"`
}

// inferFrom compiles src into a project value and runs inference against the engine's
// real diagram schema. It fails the test on a compile error or an inference diagnostic,
// returning the decoded diagram and the trace.
func inferFrom(t *testing.T, e *Engine, src string) (gotDiagram, []TraceEntry) {
	t.Helper()
	ctx := cuecontext.New()
	v := ctx.CompileString(src)
	if err := v.Err(); err != nil {
		t.Fatalf("compile fixture: %v", err)
	}
	diagram, trace, diags := e.inferDiagram(ctx, v)
	if len(diags) != 0 {
		t.Fatalf("inference diagnostics: %+v", diags)
	}
	if !diagram.Exists() {
		return gotDiagram{}, trace
	}
	raw, err := diagram.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal inferred diagram: %v", err)
	}
	var got gotDiagram
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode inferred diagram: %v", err)
	}
	return got, trace
}

func nodeIDs(g gotDiagram) []string {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func edgeIDs(g gotDiagram) []string {
	ids := make([]string, 0, len(g.Edges))
	for _, e := range g.Edges {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	return ids
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestInferFixtures is the table from .claude/docs/inference-fixtures.md: each small
// module maps to an exact node and edge set. Cases run against the pinned cue version
// through the real engine schema.
func TestInferFixtures(t *testing.T) {
	e := realEngine(t)
	cases := []struct {
		name  string
		src   string
		nodes []string
		edges []string
	}{
		{
			name: "1 one registry alone",
			src: `
people: [ID=string]: {name: string}
people: {a: {name: "A"}, b: {name: "B"}}
`,
			nodes: []string{"people/a", "people/b"},
			edges: nil,
		},
		{
			name: "2 two registries single reference",
			src: `
#TeamKey: or([for k, _ in teams {k}])
teams: [ID=string]: {}
teams: {red: {}}
people: [ID=string]: {team: #TeamKey}
people: {marty: {team: "red"}}
`,
			nodes: []string{"people/marty", "teams/red"},
			edges: []string{"people/marty--team-->teams/red"},
		},
		{
			name: "3 list of references",
			src: `
#PersonKey: or([for k, _ in people {k}])
people: [ID=string]: {friends: [...#PersonKey]}
people: {
	george:   {friends: []}
	lorraine: {friends: []}
	marty:    {friends: ["lorraine", "george"]}
}
`,
			nodes: []string{"people/george", "people/lorraine", "people/marty"},
			edges: []string{
				"people/marty--friends-->people/george",
				"people/marty--friends-->people/lorraine",
			},
		},
		{
			name: "4 optional reference absent",
			src: `
#PersonKey: or([for k, _ in people {k}])
people: [ID=string]: {name: string, mentor: #PersonKey | *""}
people: {a: {name: "A"}, b: {name: "B"}}
`,
			nodes: []string{"people/a", "people/b"},
			edges: nil,
		},
		{
			name: "5 explicit attribute reference",
			src: `
people: [ID=string]: {name: string, lead?: string @ref(people)}
people: {
	marty:    {name: "Marty", lead: "lorraine"}
	lorraine: {name: "Lorraine"}
}
`,
			nodes: []string{"people/lorraine", "people/marty"},
			edges: []string{"people/marty--lead-->people/lorraine"},
		},
		{
			name: "6 plain string not a reference",
			src: `
people: [ID=string]: {name: string, nickname: string}
people: {marty: {name: "Marty", nickname: "mac"}}
`,
			nodes: []string{"people/marty"},
			edges: nil,
		},
		{
			name: "7 key collision across registries",
			src: `
#PersonKey: or([for k, _ in people {k}])
people: [ID=string]: {name: string}
people: {x: {name: "Person X"}}
robots: [ID=string]: {owner: #PersonKey}
robots: {x: {owner: "x"}}
`,
			nodes: []string{"people/x", "robots/x"},
			edges: []string{"robots/x--owner-->people/x"},
		},
		{
			name:  "8 empty registry",
			src:   `services: [ID=string]: {}`,
			nodes: nil,
			edges: nil,
		},
		{
			name: "9 members with no name field",
			src: `
#PersonKey: or([for k, _ in people {k}])
people: [ID=string]: {year: int, mother: #PersonKey | *""}
people: {
	marty:    {year: 1968, mother: "lorraine"}
	lorraine: {year: 1937}
}
`,
			nodes: []string{"people/lorraine", "people/marty"},
			edges: []string{"people/marty--mother-->people/lorraine"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, trace := inferFrom(t, e, tc.src)
			if ids := nodeIDs(got); !eq(ids, tc.nodes) {
				t.Fatalf("nodes = %v, want %v", ids, tc.nodes)
			}
			if ids := edgeIDs(got); !eq(ids, tc.edges) {
				t.Fatalf("edges = %v, want %v", ids, tc.edges)
			}
			if want := len(tc.nodes) + len(tc.edges); len(trace) != want {
				t.Fatalf("trace entries = %d, want %d (one per element)", len(trace), want)
			}
		})
	}
}

// TestInferNodeLabelAndData checks the projection conventions: label from the first
// name-like field else the key, and remaining scalars on the data card.
func TestInferNodeLabelAndData(t *testing.T) {
	e := realEngine(t)
	got, _ := inferFrom(t, e, `
people: [ID=string]: {name: string, year: int}
people: {marty: {name: "Marty McFly", year: 1968}}
`)
	n, ok := got.Nodes["people/marty"]
	if !ok {
		t.Fatalf("missing node people/marty in %v", nodeIDs(got))
	}
	if n.Label != "Marty McFly" {
		t.Fatalf("label = %q, want %q", n.Label, "Marty McFly")
	}
	if got := n.Data["year"]; got == nil {
		t.Fatalf("data.year missing, data = %v", n.Data)
	}
	if _, present := n.Data["name"]; present {
		t.Fatalf("label source should not appear in data card, data = %v", n.Data)
	}
}

// TestInferNoRegistryIsEmpty confirms a module with no open-label field infers nothing:
// a plain record is not a registry, so there is no diagram to draw.
func TestInferNoRegistryIsEmpty(t *testing.T) {
	e := realEngine(t)
	ctx := cuecontext.New()
	v := ctx.CompileString(`config: {host: "localhost", port: 8080}`)
	if err := v.Err(); err != nil {
		t.Fatalf("compile: %v", err)
	}
	diagram, trace, diags := e.inferDiagram(ctx, v)
	if diagram.Exists() || trace != nil || diags != nil {
		t.Fatalf("want empty inference, got exists=%v trace=%v diags=%v", diagram.Exists(), trace, diags)
	}
}

// familyInferFixture is the family membrane with no diagram field and no schema import:
// the canonical end-to-end shape inference must reconstruct. Six people, mother/father
// references derived from the #PersonKey key set.
const familyInferFixture = `
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
`

// TestInferFamilyMembrane is the acceptance fixture: the schema-and-data family module
// infers six nodes and the mother/father edges, one trace entry per element.
func TestInferFamilyMembrane(t *testing.T) {
	e := realEngine(t)
	got, trace := inferFrom(t, e, familyInferFixture)

	if len(got.Nodes) != 6 {
		t.Fatalf("nodes = %d, want 6 (%v)", len(got.Nodes), nodeIDs(got))
	}
	// Four children each carry a mother and a father edge: eight edges total.
	wantEdges := []string{
		"people/dave--father-->people/george",
		"people/dave--mother-->people/lorraine",
		"people/linda--father-->people/george",
		"people/linda--mother-->people/lorraine",
		"people/marty--father-->people/george",
		"people/marty--mother-->people/lorraine",
	}
	ids := edgeIDs(got)
	for _, want := range wantEdges {
		found := false
		for _, id := range ids {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing edge %q in %v", want, ids)
		}
	}
	if len(trace) != len(got.Nodes)+len(got.Edges) {
		t.Fatalf("trace = %d, want %d", len(trace), len(got.Nodes)+len(got.Edges))
	}
	// The label is read from the name field, not the key.
	if got.Nodes["people/marty"].Label != "Marty McFly" {
		t.Fatalf("marty label = %q", got.Nodes["people/marty"].Label)
	}
}

// TestEvalInfersFamilyDiagram is the phase-4 done criterion end to end: a schema-and-
// data module with no diagram field, run through Eval, renders the inferred diagram
// under the synthetic "inferred" view, carries a trace, and skips inlay hints.
func TestEvalInfersFamilyDiagram(t *testing.T) {
	e := realEngine(t)
	files := []domain.File{{Name: "data.cue", Content: "package main\n" + familyInferFixture}}

	out, views, hints, trace, diags, err := e.Eval(context.Background(), Source{Dir: e.cueDir, Overlay: files})
	if err != nil || len(diags) != 0 {
		t.Fatalf("eval err=%v diags=%+v", err, diags)
	}
	if len(views) != 1 || views[0] != inferredViewName {
		t.Fatalf("views = %v, want [%s]", views, inferredViewName)
	}
	if nodes := decodeNodes(t, out); len(nodes) != 6 {
		t.Fatalf("nodes = %d, want 6", len(nodes))
	}
	if len(trace) == 0 {
		t.Fatal("want trace entries for inferred elements")
	}
	if hints != nil {
		t.Fatalf("inferred diagram carries no source, want no hints, got %d", len(hints))
	}
}

// TestEvalExplicitViewWinsOverInference confirms an explicit diagram-shaped field is
// rendered as its declared view and inference does not run (no synthetic view, no
// trace), so authored diagrams never regress.
func TestEvalExplicitViewWinsOverInference(t *testing.T) {
	e := realEngine(t)
	files := []domain.File{{Name: "data.cue", Content: familyMembrane}}

	_, views, _, trace, diags, err := e.Eval(context.Background(), Source{Dir: e.cueDir, Overlay: files})
	if err != nil || len(diags) != 0 {
		t.Fatalf("eval err=%v diags=%+v", err, diags)
	}
	if len(views) != 1 || views[0] != "diagram" {
		t.Fatalf("views = %v, want [diagram]", views)
	}
	if trace != nil {
		t.Fatalf("declared view must carry no inference trace, got %+v", trace)
	}
}
