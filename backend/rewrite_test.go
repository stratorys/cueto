package main

import (
	"strings"
	"testing"
)

// mustRewrite runs a rewrite and fails on any diagnostic or error.
func mustRewrite(t *testing.T, op RewriteOp) string {
	t.Helper()
	out, diags, err := rewriteFile(op)
	if err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	if len(diags) > 0 {
		t.Fatalf("rewrite diagnostics: %+v", diags)
	}
	return out
}

func TestRewriteUpsertEmbeddedForm(t *testing.T) {
	src := `package diagram

diagram: #Diagram & {
	nodes: {
		a: {type: "process", x: 1, y: 1, label: "a"}
	}
	edges: []
}
`
	out := mustRewrite(t, RewriteOp{
		Name:    "data.cue",
		Content: src,
		Nodes: map[string]string{
			"a": `{type: "process", x: 9, y: 9, label: "moved"}`, // update
			"b": `{type: "process", x: 2, y: 2, label: "b"}`,     // add
		},
	})
	if !strings.Contains(out, `label: "moved"`) {
		t.Fatalf("update not applied:\n%s", out)
	}
	if !strings.Contains(out, "b: {") && !strings.Contains(out, "b: {type") {
		t.Fatalf("new node b not added:\n%s", out)
	}
	// The #Diagram conjunction and edges must survive.
	if !strings.Contains(out, "#Diagram &") || !strings.Contains(out, "edges:") {
		t.Fatalf("structure not preserved:\n%s", out)
	}
}

func TestRewritePathForm(t *testing.T) {
	src := "package diagram\n\ndiagram: nodes: c: {type: \"process\", x: 3, y: 3, label: \"c\"}\n"
	out := mustRewrite(t, RewriteOp{
		Name:    "b.cue",
		Content: src,
		Nodes:   map[string]string{"d": `{type: "process", x: 4, y: 4, label: "d"}`},
	})
	// Both the pre-existing path-form node and the new one must be present.
	if !strings.Contains(out, "c:") || !strings.Contains(out, "d:") {
		t.Fatalf("path-form upsert lost a node:\n%s", out)
	}
}

func TestRewritePlainStructForm(t *testing.T) {
	src := `package diagram

diagram: {
	nodes: {
		a: {type: "process", x: 1, y: 1, label: "a"}
	}
}
`
	out := mustRewrite(t, RewriteOp{
		Name:    "c.cue",
		Content: src,
		Nodes:   map[string]string{"z": `{type: "process", x: 5, y: 5, label: "z"}`},
	})
	if !strings.Contains(out, "z:") || !strings.Contains(out, "a:") {
		t.Fatalf("plain-struct upsert lost a node:\n%s", out)
	}
}

func TestRewritePreservesHandWrittenCUE(t *testing.T) {
	// A hand-written helper def and comments living alongside canvas nodes must
	// survive a rewrite untouched.
	src := `package diagram

// A reusable colour used by hand.
#Accent: "#f59e0b"

diagram: #Diagram & {
	// nodes are canvas-managed
	nodes: {
		a: {type: "process", x: 1, y: 1, label: "a", fill: #Accent}
	}
	edges: []
}
`
	out := mustRewrite(t, RewriteOp{
		Name:    "data.cue",
		Content: src,
		Nodes:   map[string]string{"b": `{type: "process", x: 2, y: 2, label: "b"}`},
	})
	if !strings.Contains(out, "#Accent:") {
		t.Fatalf("hand-written def dropped:\n%s", out)
	}
	if !strings.Contains(out, "A reusable colour used by hand.") {
		t.Fatalf("comment dropped:\n%s", out)
	}
	if !strings.Contains(out, "b:") {
		t.Fatalf("new node not added:\n%s", out)
	}
}

func TestRewriteDeleteAndEdges(t *testing.T) {
	src := `package diagram

diagram: #Diagram & {
	nodes: {
		a: {type: "process", x: 1, y: 1, label: "a"}
		b: {type: "process", x: 2, y: 2, label: "b"}
	}
	edges: [{id: "e1", source: "a", target: "b", kind: "arrow"}]
}
`
	newEdges := `[{id: "e2", source: "b", target: "a", kind: "arrow"}]`
	out := mustRewrite(t, RewriteOp{
		Name:    "data.cue",
		Content: src,
		Deletes: []string{"b"},
		Edges:   &newEdges,
	})
	if strings.Contains(out, "b: {") {
		t.Fatalf("node b was not deleted:\n%s", out)
	}
	if !strings.Contains(out, "e2") || strings.Contains(out, "e1") {
		t.Fatalf("edge list not replaced:\n%s", out)
	}
}

func TestRewriteCreatesMissingPath(t *testing.T) {
	// A brand-new file with only a package clause: rewrite must create diagram.nodes.
	out := mustRewrite(t, RewriteOp{
		Name:    "new.cue",
		Content: "package diagram\n",
		Nodes:   map[string]string{"a": `{type: "process", x: 1, y: 1, label: "a"}`},
	})
	if !strings.Contains(out, "diagram:") || !strings.Contains(out, "nodes:") || !strings.Contains(out, "a:") {
		t.Fatalf("path not created:\n%s", out)
	}
}

func TestRewriteIdempotentReprint(t *testing.T) {
	// Applying the same rewrite twice yields identical text (stable formatting).
	src := "package diagram\n\ndiagram: #Diagram & {\n\tnodes: {}\n\tedges: []\n}\n"
	op := RewriteOp{Name: "data.cue", Content: src, Nodes: map[string]string{"a": `{type: "process", x: 1, y: 1, label: "a"}`}}
	first := mustRewrite(t, op)
	op.Content = first
	second := mustRewrite(t, op)
	if first != second {
		t.Fatalf("reprint not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestRewriteSyntaxErrorDiagnostics(t *testing.T) {
	_, diags, err := rewriteFile(RewriteOp{Name: "data.cue", Content: "package diagram\ndiagram: {"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("want diagnostics for unparseable source, got none")
	}
}

func TestRewriteRejectsSchemaName(t *testing.T) {
	e := &cueEvaluator{}
	_, diags, err := e.Rewrite(RewriteOp{Name: "schema.cue", Content: "package diagram\n"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("want diagnostics rejecting schema.cue, got none")
	}
}
