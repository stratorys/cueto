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
	"github.com/stratorys/cueto/backend/internal/workspace"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// testConfig points at the repo's real ../cue schema dir with generous bounds.
// Individual tests tighten a single bound to exercise it.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	abs, err := filepath.Abs("../../../cue")
	if err != nil {
		t.Fatalf("abs cue dir: %v", err)
	}
	return config.Config{
		CueDir:         abs,
		DataDir:        t.TempDir(),
		MaxBodyBytes:   1 << 20,
		MaxOutputBytes: 4 << 20,
		EvalTimeout:    2 * time.Second,
		MaxConcurrent:  4,
	}
}

func realRouter(t *testing.T, cfg config.Config) *gin.Engine {
	t.Helper()
	return NewRouter(evaluation.New(cfg.CueDir, cfg.EvalTimeout, cfg.MaxOutputBytes), workspace.New(cfg), authoring.New(), cfg)
}

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
	rec := postJSON(router, "/eval", evalBody(t, validData))
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

func TestEvalMissingDiagram(t *testing.T) {
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/eval", evalBody(t, "package main\n"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	diags := decodeDiags(t, rec)
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "diagram") {
		t.Fatalf("diagnostics = %+v, want a missing-diagram message", diags)
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
	rec := postJSON(router, "/eval", evalBody(t, data))
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
	rec := postJSON(router, "/eval", evalBody(t, data))
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
	rec := postJSON(router, "/eval", evalBody(t, data))
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
	rec := postJSON(router, "/eval", evalBody(t, validData))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestOutputTooLarge(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxOutputBytes = 5
	router := realRouter(t, cfg)
	rec := postJSON(router, "/eval", evalBody(t, validData))
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
	rec := postJSON(router, "/eval", evalBody(t, data))
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

func (b *blockingEval) Eval(ctx context.Context, src evaluation.Source) (json.RawMessage, []evaluation.Hint, []diag.Diagnostic, error) {
	b.entered <- struct{}{}
	<-b.release
	return json.RawMessage(`{"nodes":{},"edges":[]}`), nil, nil, nil
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
	router := NewRouter(be, workspace.New(cfg), authoring.New(), cfg)

	// First request occupies the only slot and blocks inside the handler.
	go func() {
		postJSON(router, "/eval", evalBody(t, validData))
	}()
	<-be.entered

	// Second request finds the slot taken and must be rejected immediately.
	rec := postJSON(router, "/eval", evalBody(t, validData))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	close(be.release)
}

func TestReplEvalExpr(t *testing.T) {
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{Source: "a: b: 3\nc: a.b + 1"})
	rec := postJSON(router, "/repl", body)
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
	rec := postJSON(router, "/repl", body)
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
	rec := postJSON(router, "/repl", body)
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
	rec := postJSON(router, "/repl", body)
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
	rec := postJSON(router, "/eval", evalBody(t, validData))
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
	rec := postJSON(router, "/repl", body)
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
	rec := postJSON(router, "/repl", body)
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
	rec := postJSON(router, "/repl", body)
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
	rec := postJSON(router, "/vet", evalBody(t, validData))
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
	rec := postJSON(router, "/vet", evalBody(t, data))
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

func TestSaveWritesVersion(t *testing.T) {
	cfg := testConfig(t)
	router := realRouter(t, cfg)
	rec := postJSON(router, "/projects/default/save", evalBody(t, validData))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.OK || body.Version == "" {
		t.Fatalf("want ok:true with a version, got %q", rec.Body.String())
	}
	// The submitted text is stored verbatim as the version's single blob, under the
	// project's version store.
	blobsDir := filepath.Join(cfg.DataDir, workspace.DefaultProjectID, "versions", "blobs")
	blobs, err := os.ReadDir(blobsDir)
	if err != nil {
		t.Fatalf("read blobs: %v", err)
	}
	if len(blobs) != 1 {
		t.Fatalf("want 1 blob, got %d", len(blobs))
	}
	saved, err := os.ReadFile(filepath.Join(blobsDir, blobs[0].Name()))
	if err != nil {
		t.Fatalf("read blob: %v", err)
	}
	if string(saved) != validData {
		t.Fatalf("version content mismatch")
	}
	// The data root holds the registry and project dirs, never a stray data.cue.
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "data.cue")); !os.IsNotExist(err) {
		t.Fatalf("data dir should not contain data.cue")
	}
}

func TestSaveInvalidNotWritten(t *testing.T) {
	cfg := testConfig(t)
	router := realRouter(t, cfg)
	data := "package main\n\nimport d \"github.com/stratorys/cueto/diagram\"\n\ndiagram: d.#Diagram & {nodes: {a: {type: \"process\", x: \"nope\", y: 1, label: \"l\"}}, edges: []}\n"
	rec := postJSON(router, "/projects/default/save", evalBody(t, data))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	// No manifest is written into the project's store for invalid data. The dir may
	// be absent (nothing ever created it), which is also "no versions".
	entries, err := os.ReadDir(filepath.Join(cfg.DataDir, workspace.DefaultProjectID, "versions", "manifests"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read project manifests dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid data must not be persisted, found %d manifests", len(entries))
	}
}

func TestSaveIdempotent(t *testing.T) {
	cfg := testConfig(t)
	router := realRouter(t, cfg)
	first := postJSON(router, "/projects/default/save", evalBody(t, validData))
	second := postJSON(router, "/projects/default/save", evalBody(t, validData))
	if first.Body.String() != second.Body.String() {
		t.Fatalf("same content produced different versions: %q vs %q", first.Body.String(), second.Body.String())
	}
	entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, workspace.DefaultProjectID, "versions", "manifests"))
	if len(entries) != 1 {
		t.Fatalf("want 1 manifest, got %d", len(entries))
	}
}

func TestListAndReadVersion(t *testing.T) {
	cfg := testConfig(t)
	router := realRouter(t, cfg)

	saveRec := postJSON(router, "/projects/default/save", evalBody(t, validData))
	if saveRec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body %q", saveRec.Code, saveRec.Body.String())
	}
	var saved struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(saveRec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode save: %v", err)
	}

	listRec := getJSON(router, "/projects/default/versions")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}
	var list struct {
		Versions []domain.Version `json:"versions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Versions) != 1 || list.Versions[0].Version != saved.Version {
		t.Fatalf("versions = %+v, want the saved hash %q", list.Versions, saved.Version)
	}
	if list.Versions[0].SavedAt.IsZero() {
		t.Fatalf("version is missing a savedAt timestamp: %+v", list.Versions[0])
	}

	readRec := getJSON(router, "/projects/default/versions/"+saved.Version)
	if readRec.Code != http.StatusOK {
		t.Fatalf("read status = %d, body %q", readRec.Code, readRec.Body.String())
	}
	var read struct {
		Version string `json:"version"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal(readRec.Body.Bytes(), &read); err != nil {
		t.Fatalf("decode read: %v", err)
	}
	if read.Data != validData {
		t.Fatalf("read data mismatch:\n got %q\nwant %q", read.Data, validData)
	}
}

func TestReadVersionBadID(t *testing.T) {
	router := realRouter(t, testConfig(t))
	// Well-formed single-segment but non-hex ids reach the handler and are 400.
	for _, id := range []string{"not-a-hash", strings.Repeat("g", 64), "abc"} {
		rec := getJSON(router, "/projects/default/versions/"+id)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("id %q status = %d, want 400", id, rec.Code)
		}
	}
	// An encoded path-traversal id is rejected by the router before the handler
	// (404); either way it must never reach the filesystem or return 200.
	rec := getJSON(router, "/versions/..%2f..%2fetc%2fpasswd")
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Fatalf("traversal id status = %d, want 400 or 404", rec.Code)
	}
}

func TestReadVersionNotFound(t *testing.T) {
	router := realRouter(t, testConfig(t))
	// Well-formed but absent hash -> 404.
	rec := getJSON(router, "/projects/default/versions/"+strings.Repeat("a", 64))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestVersionIndexNoDuplicateOnIdempotentSave(t *testing.T) {
	cfg := testConfig(t)
	router := realRouter(t, cfg)
	postJSON(router, "/projects/default/save", evalBody(t, validData))
	postJSON(router, "/projects/default/save", evalBody(t, validData)) // idempotent re-save

	// The index records the save exactly once (the re-save reused the manifest).
	index, err := os.ReadFile(filepath.Join(cfg.DataDir, workspace.DefaultProjectID, "versions", "index.jsonl"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	lines := 0
	for _, line := range strings.Split(strings.TrimSpace(string(index)), "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	if lines != 1 {
		t.Fatalf("index has %d lines, want 1 (idempotent save must not duplicate)", lines)
	}

	// And the listing still shows a single version.
	listRec := getJSON(router, "/projects/default/versions")
	var list struct {
		Versions []domain.Version `json:"versions"`
	}
	_ = json.Unmarshal(listRec.Body.Bytes(), &list)
	if len(list.Versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(list.Versions))
	}
}

func TestConfigRejectsDataDirInsideCueDir(t *testing.T) {
	cueDir, err := filepath.Abs("../../../cue")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CUE_DIR", cueDir)
	t.Setenv("DATA_DIR", filepath.Join(cueDir, "data"))
	if _, err := config.Load(); err == nil {
		t.Fatal("want error for DATA_DIR inside CUE_DIR, got nil")
	}
}

func TestConfigRequiresDataDir(t *testing.T) {
	t.Setenv("DATA_DIR", "")
	if _, err := config.Load(); err == nil {
		t.Fatal("want error for missing DATA_DIR, got nil")
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
	rec := postJSON(router, "/eval", filesBody(t, primary, extra))
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

func TestEvalRejectsSchemaFilename(t *testing.T) {
	// A client must never be able to supply schema.cue and shadow the hand-owned one.
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/eval", filesBody(t, domain.File{Name: "schema.cue", Content: "package diagram\n"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	diags := decodeDiags(t, rec)
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "schema.cue") {
		t.Fatalf("diagnostics = %+v, want an invalid-file-name message", diags)
	}
}

func TestEvalRejectsTraversalFilename(t *testing.T) {
	router := realRouter(t, testConfig(t))
	for _, name := range []string{"../evil.cue", "sub/dir.cue", "Schema.cue"} {
		rec := postJSON(router, "/eval", filesBody(t, domain.File{Name: name, Content: "package diagram\n"}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("name %q status = %d, want 400", name, rec.Code)
		}
	}
}
