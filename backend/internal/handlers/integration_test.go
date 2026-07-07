// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/authoring"
	"github.com/stratorys/cueto/backend/internal/config"
	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/domain"
	"github.com/stratorys/cueto/backend/internal/evaluation"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// testProjectID is the repo's own cue dir seen as a project: rooting the projects
// dir at the cue dir's parent makes "cue" resolve to the real schema module, so
// eval overlays resolve the bundled diagram package.
const testProjectID = "cue"

// testConfig points at the repo's real ../cue schema dir with generous bounds and
// roots the projects dir at its parent, so the "cue" project resolves. Individual
// tests tighten a single bound or override ProjectsDir to exercise it.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	abs, err := filepath.Abs("../../../cue")
	if err != nil {
		t.Fatalf("abs cue dir: %v", err)
	}
	return config.Config{
		CueDir:         abs,
		ProjectsDir:    filepath.Dir(abs),
		MaxBodyBytes:   1 << 20,
		MaxOutputBytes: 4 << 20,
		EvalTimeout:    2 * time.Second,
		MaxConcurrent:  4,
	}
}

func realRouter(t *testing.T, cfg config.Config) *gin.Engine {
	t.Helper()
	return NewRouter(evaluation.New(cfg.CueDir, cfg.EvalTimeout, cfg.MaxOutputBytes), authoring.New(), cfg)
}

// pp builds a path scoped to the default test project. ppid does the same for an
// explicit project id (a scratch workspace).
func pp(op string) string { return ppid(testProjectID, op) }

func ppid(id, op string) string { return "/projects/" + id + op }

func postJSON(router *gin.Engine, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func getJSON(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func deleteJSON(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func evalBody(t *testing.T, data string) []byte {
	t.Helper()
	b, err := json.Marshal(dataRequest{Data: data})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return b
}

type diagResponse struct {
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

func decodeDiags(t *testing.T, rec *httptest.ResponseRecorder) []diag.Diagnostic {
	t.Helper()
	var r diagResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode diagnostics: %v (body %q)", err, rec.Body.String())
	}
	return r.Diagnostics
}

// tempWorkspace writes a scratch module (its own cue.mod for example.com/m plus the
// given files) as a project named "m" under a temp projects root, and returns that
// id and a router pointed at the root. The schema still comes from the repo
// CUE_DIR; only the module root moves.
func tempWorkspace(t *testing.T, files map[string]string) (string, *gin.Engine) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "m")
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
	cfg := testConfig(t)
	cfg.ProjectsDir = root
	return "m", realRouter(t, cfg)
}

// workspaceImportRoot derives its diagram from an on-disk subpackage. The import
// and the workspace cue.mod resolve only when Sources are rooted at the workspace,
// so a successful eval proves WORKSPACE_DIR moved the module root off CUE_DIR.
const workspaceImportRoot = `package main

import "example.com/m/sub"

diagram: {nodes: sub.nodes, edges: []}
`

const workspaceSub = "package sub\n\nnodes: {a: {type: \"entity\", label: \"A\"}}\n"

func TestWorkspaceEval(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"sub/sub.cue": workspaceSub})
	rec := postJSON(router, ppid(id, "/eval"), evalBody(t, workspaceImportRoot))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Diagram struct {
			Nodes map[string]any `json:"nodes"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body.Diagram.Nodes["a"]; !ok {
		t.Fatalf("nodes = %v, want the subpackage node a", body.Diagram.Nodes)
	}
}

func TestWorkspaceVet(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"sub/sub.cue": workspaceSub})
	rec := postJSON(router, ppid(id, "/vet"), evalBody(t, workspaceImportRoot))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Ok bool `json:"ok"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Ok {
		t.Fatalf("vet not ok: %s", rec.Body.String())
	}
}

func TestWorkspaceKeys(t *testing.T) {
	id, router := tempWorkspace(t, map[string]string{"sub/sub.cue": workspaceSub})
	body, err := json.Marshal(sourceRequest{Files: []domain.File{{Name: "data.cue", Content: workspaceImportRoot}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := postJSON(router, ppid(id, "/repl/keys"), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var hasDiagram bool
	for _, k := range out.Keys {
		if k == "diagram" {
			hasDiagram = true
		}
	}
	if !hasDiagram {
		t.Fatalf("keys = %v, want to include diagram", out.Keys)
	}
}

const validData = `package main

import d "github.com/stratorys/cueto/diagram"

diagram: d.#Diagram & {
	nodes: {
		a: {type: "process", x: 1, y: 1, label: "a"}
		b: {type: "process", x: 2, y: 2, label: "b"}
	}
	edges: [
		{id: "e1", source: "a", target: "b", kind: "arrow"},
		{id: "e2", source: "a", target: "b", kind: "arrow"},
		{id: "e3", source: "a", target: "b", kind: "arrow"},
	]
}
`

func TestEvalHappyPath(t *testing.T) {
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), evalBody(t, validData))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Diagram struct {
			Nodes map[string]any `json:"nodes"`
			Edges []struct {
				ID string `json:"id"`
			} `json:"edges"`
		} `json:"diagram"`
		Hints []evaluation.Hint `json:"hints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Diagram.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(out.Diagram.Nodes))
	}
	// Edge list order must be stable across a round-trip.
	got := []string{out.Diagram.Edges[0].ID, out.Diagram.Edges[1].ID, out.Diagram.Edges[2].ID}
	want := []string{"e1", "e2", "e3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("edge order = %v, want %v", got, want)
		}
	}
}

func TestEvalNoView(t *testing.T) {
	// A knowledge-only module has no diagram-shaped field: a valid success with an
	// empty view list and an empty diagram, distinct from a diagnostic.
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), evalBody(t, "package main\n\npeople: {george: {name: \"George\"}}\n"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Diagram json.RawMessage `json:"diagram"`
		Views   []string        `json:"views"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Views) != 0 {
		t.Fatalf("views = %v, want none", body.Views)
	}
	if string(body.Diagram) != "{}" {
		t.Fatalf("diagram = %s, want {}", body.Diagram)
	}
}

func TestEvalInferredLegend(t *testing.T) {
	// A schema-and-data module with no diagram field infers a view, and the response
	// carries the registry legend the frontend renders: one entry per registry, drawn
	// as a table in the default (model) view. This pins the wire contract end to end.
	data := `package main

#Person: {name: string}
people: [ID=string]: #Person
people: {
	george: {name: "George"}
	lorraine: {name: "Lorraine"}
}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), evalBody(t, data))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Views  []string `json:"views"`
		Legend []struct {
			Field string `json:"field"`
			Kind  string `json:"kind"`
			Count int    `json:"count"`
		} `json:"legend"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Views) != 2 {
		t.Fatalf("views = %v, want the two inferred views", body.Views)
	}
	if len(body.Legend) != 1 || body.Legend[0].Field != "people" ||
		body.Legend[0].Kind != "table" || body.Legend[0].Count != 1 {
		t.Fatalf("legend = %+v, want one people/table/1 entry", body.Legend)
	}
}

func TestEvalMultipleViews(t *testing.T) {
	// Two diagram-shaped fields are both discovered and listed; the default rendered
	// diagram is the one named diagram.
	data := `package main

alt: {nodes: {b: {type: "entity", label: "B"}}, edges: []}
diagram: {nodes: {a: {type: "entity", label: "A"}}, edges: []}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), evalBody(t, data))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Diagram struct {
			Nodes map[string]any `json:"nodes"`
		} `json:"diagram"`
		Views []string `json:"views"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Views) != 2 || body.Views[0] != "alt" || body.Views[1] != "diagram" {
		t.Fatalf("views = %v, want [alt diagram]", body.Views)
	}
	if _, ok := body.Diagram.Nodes["a"]; !ok || len(body.Diagram.Nodes) != 1 {
		t.Fatalf("default diagram nodes = %v, want {a}", body.Diagram.Nodes)
	}
}

func TestEvalSelectsView(t *testing.T) {
	// The same two-view module, but the request names the non-default view; eval
	// must render alt's node b, not the default diagram's a, and still list both.
	data := `package main

alt: {nodes: {b: {type: "entity", label: "B"}}, edges: []}
diagram: {nodes: {a: {type: "entity", label: "A"}}, edges: []}
`
	reqBody, err := json.Marshal(dataRequest{Data: data, View: "alt"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), reqBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Diagram struct {
			Nodes map[string]any `json:"nodes"`
		} `json:"diagram"`
		Views []string `json:"views"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body.Diagram.Nodes["b"]; !ok || len(body.Diagram.Nodes) != 1 {
		t.Fatalf("selected view nodes = %v, want alt's {b}", body.Diagram.Nodes)
	}
	if len(body.Views) != 2 {
		t.Fatalf("views = %v, want both listed", body.Views)
	}
}

func TestEvalSchemaViolation(t *testing.T) {
	data := `package main

import d "github.com/stratorys/cueto/diagram"

diagram: d.#Diagram & {
	nodes: {a: {type: "process", x: "nope", y: 1, label: "l"}}
	edges: []
}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), evalBody(t, data))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	cfg := testConfig(t)
	diags := decodeDiags(t, rec)
	if len(diags) == 0 {
		t.Fatal("want at least one diagnostic")
	}
	for _, d := range diags {
		if strings.Contains(d.Message, cfg.CueDir) {
			t.Fatalf("diagnostic leaks host path: %q", d.Message)
		}
	}
	if diags[0].Line == 0 {
		t.Fatalf("want a source line, got %+v", diags[0])
	}
	if diags[0].Kind != diag.KindSchema {
		t.Fatalf("kind = %q, want %q", diags[0].Kind, diag.KindSchema)
	}
}

func TestEvalNonConcrete(t *testing.T) {
	// Node missing a concrete label: valid against the schema but not concrete.
	// (x/y are optional, so a missing coordinate is concrete; label is required.)
	data := `package main

import d "github.com/stratorys/cueto/diagram"

diagram: d.#Diagram & {
	nodes: {a: {type: "process", x: 1, y: 1}}
	edges: []
}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), evalBody(t, data))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	diags := decodeDiags(t, rec)
	if len(diags) == 0 || diags[0].Kind != diag.KindIncomplete {
		t.Fatalf("diagnostics = %+v, want kind %q", diags, diag.KindIncomplete)
	}
}

func TestEvalConflictingID(t *testing.T) {
	// The node's id must equal its map key; a conflicting id is a hard error.
	data := `package main

import d "github.com/stratorys/cueto/diagram"

diagram: d.#Diagram & {
	nodes: {user: {id: "other", type: "process", x: 1, y: 1, label: "l"}}
	edges: []
}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), evalBody(t, data))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	diags := decodeDiags(t, rec)
	if len(diags) == 0 {
		t.Fatal("want a conflict diagnostic, got none")
	}
	if diags[0].Kind != diag.KindSchema {
		t.Fatalf("kind = %q, want %q (diags %+v)", diags[0].Kind, diag.KindSchema, diags)
	}
}

func TestBodyTooLarge(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxBodyBytes = 16
	router := realRouter(t, cfg)
	rec := postJSON(router, pp("/eval"), evalBody(t, validData))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestOutputTooLarge(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxOutputBytes = 5
	router := realRouter(t, cfg)
	rec := postJSON(router, pp("/eval"), evalBody(t, validData))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestEvalTimeout(t *testing.T) {
	cfg := testConfig(t)
	cfg.EvalTimeout = 5 * time.Millisecond
	// Forcing a concrete label forces the large list build, which blows the deadline.
	data := `package main

import (
	"list"

	d "github.com/stratorys/cueto/diagram"
)

diagram: d.#Diagram & {
	nodes: {a: {type: "process", x: 1, y: 1, label: "\(len([for i in list.Range(0, 1000000, 1) {i}]))"}}
	edges: []
}
`
	router := realRouter(t, cfg)
	start := time.Now()
	rec := postJSON(router, pp("/eval"), evalBody(t, data))
	elapsed := time.Since(start)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 (body %q)", rec.Code, rec.Body.String())
	}
	// The request must return without waiting on the runaway goroutine.
	if elapsed > time.Second {
		t.Fatalf("request blocked %v, want prompt return on timeout", elapsed)
	}
}

// blockingEval is an evalService fake that holds the concurrency slot until
// released, so the cap is testable deterministically without a real long
// evaluation. Only /eval is exercised, so the other concern services can be real.
type blockingEval struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingEval) Eval(ctx context.Context, src evaluation.Source) (json.RawMessage, []string, []evaluation.Hint, []evaluation.TraceEntry, []evaluation.LegendEntry, []diag.Diagnostic, error) {
	b.entered <- struct{}{}
	<-b.release
	return json.RawMessage(`{"nodes":{},"edges":[]}`), []string{"diagram"}, nil, nil, nil, nil, nil
}

func (b *blockingEval) EvalExpr(ctx context.Context, source string) (json.RawMessage, []diag.Diagnostic, error) {
	return json.RawMessage(`null`), nil, nil
}

func (b *blockingEval) EvalQuery(ctx context.Context, src evaluation.Source, expr string) (json.RawMessage, []diag.Diagnostic, error) {
	return json.RawMessage(`null`), nil, nil
}

func (b *blockingEval) Keys(ctx context.Context, src evaluation.Source) ([]string, []diag.Diagnostic, error) {
	return nil, nil, nil
}

func (b *blockingEval) Introspect() evaluation.CueMeta {
	return evaluation.CueMeta{}
}

func (b *blockingEval) Vet(ctx context.Context, src evaluation.Source) ([]diag.Diagnostic, error) {
	return nil, nil
}

func TestConcurrencyLimit(t *testing.T) {
	be := &blockingEval{entered: make(chan struct{}, 1), release: make(chan struct{})}
	cfg := testConfig(t)
	cfg.MaxConcurrent = 1
	router := NewRouter(be, authoring.New(), cfg)

	// First request occupies the only slot and blocks inside the handler.
	go func() {
		postJSON(router, pp("/eval"), evalBody(t, validData))
	}()
	<-be.entered

	// Second request finds the slot taken and must be rejected immediately.
	rec := postJSON(router, pp("/eval"), evalBody(t, validData))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	close(be.release)
}

func TestReplEvalExpr(t *testing.T) {
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{Source: "a: b: 3\nc: a.b + 1"})
	rec := postJSON(router, pp("/repl"), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Result struct {
			A struct {
				B int `json:"b"`
			} `json:"a"`
			C int `json:"c"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if out.Result.A.B != 3 || out.Result.C != 4 {
		t.Fatalf("result = %+v, want {a:{b:3}, c:4}", out.Result)
	}
}

func TestReplEvalExprError(t *testing.T) {
	// A conflict is reported as diagnostics (400), never persisted or 500'd.
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{Source: "a: 1\na: 2"})
	rec := postJSON(router, pp("/repl"), body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
	if len(decodeDiags(t, rec)) == 0 {
		t.Fatal("want a conflict diagnostic, got none")
	}
}

func TestReplQueryAgainstEditorData(t *testing.T) {
	// With files present, /repl evaluates the source as an expression against the
	// live diagram, so it can reference `diagram`.
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{
		Source: `diagram.nodes.a.label`,
		Files:  []domain.File{{Name: "data.cue", Content: validData}},
	})
	rec := postJSON(router, pp("/repl"), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if out.Result != "a" {
		t.Fatalf("result = %q, want %q", out.Result, "a")
	}
}

func TestReplQueryComprehension(t *testing.T) {
	// A list comprehension over the diagram returns a concrete list.
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{
		Source: `[for e in diagram.edges {e.id}]`,
		Files:  []domain.File{{Name: "data.cue", Content: validData}},
	})
	rec := postJSON(router, pp("/repl"), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Result []string `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	want := []string{"e1", "e2", "e3"}
	if len(out.Result) != len(want) {
		t.Fatalf("result = %v, want %v", out.Result, want)
	}
	for i := range want {
		if out.Result[i] != want[i] {
			t.Fatalf("result = %v, want %v", out.Result, want)
		}
	}
}

func TestReplQueryDoesNotAffectEval(t *testing.T) {
	// The query overlay must not leak into /eval output: the marshaled diagram
	// carries no replResult field.
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), evalBody(t, validData))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Diagram map[string]any `json:"diagram"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, present := out.Diagram["replResult"]; present {
		t.Fatal("replResult leaked into /eval diagram output")
	}
}

func TestReplQueryIncompleteExpr(t *testing.T) {
	// A non-concrete expression yields diagnostics (400), not a 500 or a value.
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{
		Source: `diagram.nodes.missing.label`,
		Files:  []domain.File{{Name: "data.cue", Content: validData}},
	})
	rec := postJSON(router, pp("/repl"), body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
	if len(decodeDiags(t, rec)) == 0 {
		t.Fatal("want a diagnostic for the missing field, got none")
	}
}

func TestReplQueryUsesStdlibPackage(t *testing.T) {
	// A query referencing a stdlib package works: the backend injects the import
	// for exactly the packages the expression uses.
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{
		Source: `strings.ToUpper(diagram.nodes.a.label)`,
		Files:  []domain.File{{Name: "data.cue", Content: validData}},
	})
	rec := postJSON(router, pp("/repl"), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if out.Result != "A" {
		t.Fatalf("result = %q, want %q", out.Result, "A")
	}
}

func TestReplQueryFieldNamedLikePackage(t *testing.T) {
	// A field access whose leaf shares a package name (diagram.nodes) must not
	// trigger an injected import, which would fail as "imported and not used".
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{
		Source: `len(diagram.nodes)`,
		Files:  []domain.File{{Name: "data.cue", Content: validData}},
	})
	rec := postJSON(router, pp("/repl"), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
}

func TestCueMetaLists(t *testing.T) {
	router := realRouter(t, testConfig(t))
	rec := getJSON(router, "/cue/meta")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var meta evaluation.CueMeta
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if len(meta.Builtins) == 0 || len(meta.Packages) == 0 {
		t.Fatalf("empty meta: %d builtins, %d packages", len(meta.Builtins), len(meta.Packages))
	}
}

func TestVetOk(t *testing.T) {
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/vet"), evalBody(t, validData))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.OK {
		t.Fatalf("ok = false, want true (body %q)", rec.Body.String())
	}
}

func TestVetInvalid(t *testing.T) {
	data := "package main\n\nimport d \"github.com/stratorys/cueto/diagram\"\n\ndiagram: d.#Diagram & {nodes: {a: {type: \"process\", x: \"nope\", y: 1, label: \"l\"}}, edges: []}\n"
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/vet"), evalBody(t, data))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		OK          bool              `json:"ok"`
		Diagnostics []diag.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OK || len(body.Diagnostics) == 0 {
		t.Fatalf("want ok:false with diagnostics, got %q", rec.Body.String())
	}
}

func TestFormatOk(t *testing.T) {
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{Source: "package diagram\ndiagram:{x:1}"})
	rec := postJSON(router, "/format", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		Formatted string `json:"formatted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(out.Formatted, "diagram: {x: 1}") {
		t.Fatalf("formatted = %q, want reflowed source", out.Formatted)
	}
}

func TestFormatError(t *testing.T) {
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{Source: "package diagram\ndiagram: {"})
	rec := postJSON(router, "/format", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(decodeDiags(t, rec)) == 0 {
		t.Fatal("want format diagnostics, got none")
	}
}

func TestConfigRequiresProjectsDir(t *testing.T) {
	t.Setenv("PROJECTS_DIR", "")
	if _, err := config.Load(); err == nil {
		t.Fatal("want error for missing PROJECTS_DIR, got nil")
	}
}

func TestConfigRejectsProjectsDirNotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Setenv("PROJECTS_DIR", file)
	if _, err := config.Load(); err == nil {
		t.Fatal("want error for PROJECTS_DIR pointing at a file, got nil")
	}
}

func filesBody(t *testing.T, files ...domain.File) []byte {
	t.Helper()
	b, err := json.Marshal(dataRequest{Files: files})
	if err != nil {
		t.Fatalf("marshal files body: %v", err)
	}
	return b
}

func TestEvalMultiFileUnifiesWithProvenance(t *testing.T) {
	// data.cue (shadowing the disk seed) holds node a and the edge list; extra.cue
	// contributes node b via path form. They unify into one diagram, and each node
	// is attributed to its authoring file.
	primary := domain.File{Name: "data.cue", Content: `package main
import d "github.com/stratorys/cueto/diagram"
diagram: d.#Diagram & {
	nodes: {a: {type: "process", x: 1, y: 1, label: "a"}}
	edges: [{id: "e1", source: "a", target: "b", kind: "arrow"}]
}
`}
	extra := domain.File{Name: "extra.cue", Content: `package main
diagram: nodes: b: {type: "process", x: 2, y: 2, label: "b"}
`}
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), filesBody(t, primary, extra))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Diagram struct {
			Nodes map[string]any `json:"nodes"`
		} `json:"diagram"`
		Provenance domain.Provenance `json:"provenance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Diagram.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2 (a+b unified)", len(out.Diagram.Nodes))
	}
	if out.Provenance.Nodes["a"] != "data.cue" || out.Provenance.Nodes["b"] != "extra.cue" {
		t.Fatalf("provenance nodes = %+v, want a->data.cue b->extra.cue", out.Provenance.Nodes)
	}
	if out.Provenance.Edges != "data.cue" {
		t.Fatalf("provenance edges = %q, want data.cue", out.Provenance.Edges)
	}
}

func TestEvalRejectsInvalidFilename(t *testing.T) {
	// A client must never be able to escape the module root with a traversal path.
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, pp("/eval"), filesBody(t, domain.File{Name: "../schema.cue", Content: "package main\n"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	diags := decodeDiags(t, rec)
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "../schema.cue") {
		t.Fatalf("diagnostics = %+v, want an invalid-file-name message", diags)
	}
}

func TestEvalRejectsTraversalFilename(t *testing.T) {
	router := realRouter(t, testConfig(t))
	for _, name := range []string{"../evil.cue", "sub/../evil.cue", "sub//dir.cue", "diagram/x.cue"} {
		rec := postJSON(router, pp("/eval"), filesBody(t, domain.File{Name: name, Content: "package main\n"}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("name %q status = %d, want 400", name, rec.Code)
		}
	}
}
