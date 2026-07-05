// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

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
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// testConfig points at the repo's real ../cue schema dir with generous bounds.
// Individual tests tighten a single bound to exercise it.
func testConfig(t *testing.T) Config {
	t.Helper()
	abs, err := filepath.Abs("../cue")
	if err != nil {
		t.Fatalf("abs cue dir: %v", err)
	}
	return Config{
		CueDir:         abs,
		VersionsDir:    t.TempDir(),
		MaxBodyBytes:   1 << 20,
		MaxOutputBytes: 4 << 20,
		EvalTimeout:    2 * time.Second,
		MaxConcurrent:  4,
	}
}

func realRouter(t *testing.T, cfg Config) *gin.Engine {
	t.Helper()
	return newRouter(newCueEvaluator(cfg), cfg)
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
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func decodeDiags(t *testing.T, rec *httptest.ResponseRecorder) []Diagnostic {
	t.Helper()
	var r diagResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode diagnostics: %v (body %q)", err, rec.Body.String())
	}
	return r.Diagnostics
}

const validData = `package diagram

diagram: #Diagram & {
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
		Hints []Hint `json:"hints"`
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

func TestEvalUnaffectedByOptionalFields(t *testing.T) {
	// The optional domain metadata (policies, role/owner/region/zone, call/sync)
	// must not break eval, and must round-trip through the /eval output.
	data := `package diagram

diagram: #Diagram & {
	policies: ["security"]
	nodes: {
		api: {type: "process", role: "service", owner: "payments", region: "eu-west-1", zone: "public", x: 1, y: 1, label: "api"}
		db: {type: "process", role: "database", region: "eu-west-1", x: 2, y: 2, label: "db"}
	}
	edges: [
		{id: "e1", source: "api", target: "db", kind: "arrow", call: "reads", protocol: "sql", sync: true},
	]
}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/eval", evalBody(t, data))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Diagram struct {
			Nodes map[string]map[string]any `json:"nodes"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Diagram.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(out.Diagram.Nodes))
	}
	if out.Diagram.Nodes["api"]["role"] != "service" {
		t.Fatalf("api.role = %v, want service (domain field dropped?)", out.Diagram.Nodes["api"]["role"])
	}
}

func TestEvalMissingDiagram(t *testing.T) {
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/eval", evalBody(t, "package diagram\n"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	diags := decodeDiags(t, rec)
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "diagram") {
		t.Fatalf("diagnostics = %+v, want a missing-diagram message", diags)
	}
}

func TestEvalSchemaViolation(t *testing.T) {
	data := `package diagram

diagram: #Diagram & {
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
	if diags[0].Kind != kindSchema {
		t.Fatalf("kind = %q, want %q", diags[0].Kind, kindSchema)
	}
}

func TestEvalNonConcrete(t *testing.T) {
	// Node missing a concrete x: valid against the schema but not concrete.
	data := `package diagram

diagram: #Diagram & {
	nodes: {a: {type: "process", y: 1, label: "l"}}
	edges: []
}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/eval", evalBody(t, data))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	diags := decodeDiags(t, rec)
	if len(diags) == 0 || diags[0].Kind != kindIncomplete {
		t.Fatalf("diagnostics = %+v, want kind %q", diags, kindIncomplete)
	}
}

func TestEvalConflictingID(t *testing.T) {
	// The node's id must equal its map key; a conflicting id is a hard error.
	data := `package diagram

diagram: #Diagram & {
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
	if diags[0].Kind != kindSchema {
		t.Fatalf("kind = %q, want %q (diags %+v)", diags[0].Kind, kindSchema, diags)
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
	data := `package diagram

import "list"

diagram: #Diagram & {
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

// blockingEval holds the concurrency slot until released, so the cap is testable
// deterministically without a real long evaluation.
type blockingEval struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingEval) Eval(ctx context.Context, files []File) (json.RawMessage, []Hint, Provenance, []Diagnostic, error) {
	b.entered <- struct{}{}
	<-b.release
	return json.RawMessage(`{"nodes":{},"edges":[]}`), nil, Provenance{}, nil, nil
}

func (b *blockingEval) Vet(ctx context.Context, files []File, facts string) ([]Diagnostic, error) {
	return nil, nil
}

func (b *blockingEval) ImportCompose(source string) (string, []Diagnostic, error) {
	return "", nil, nil
}

func (b *blockingEval) Save(ctx context.Context, data string) (string, []Diagnostic, error) {
	return "v", nil, nil
}

func (b *blockingEval) ListVersions(ctx context.Context) ([]VersionMeta, error) { return nil, nil }

func (b *blockingEval) ReadVersion(ctx context.Context, id string) (string, error) { return "", nil }

func (b *blockingEval) Format(source string) (string, error) { return source, nil }

func (b *blockingEval) Rewrite(op RewriteOp) (string, []Diagnostic, error) {
	return op.Content, nil, nil
}

func TestConcurrencyLimit(t *testing.T) {
	be := &blockingEval{entered: make(chan struct{}, 1), release: make(chan struct{})}
	cfg := testConfig(t)
	cfg.MaxConcurrent = 1
	router := newRouter(be, cfg)

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
	data := "package diagram\n\ndiagram: #Diagram & {nodes: {a: {type: \"process\", x: \"nope\", y: 1, label: \"l\"}}, edges: []}\n"
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/vet", evalBody(t, data))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		OK          bool         `json:"ok"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OK || len(body.Diagnostics) == 0 {
		t.Fatalf("want ok:false with diagnostics, got %q", rec.Body.String())
	}
}

func TestVetPolicyClean(t *testing.T) {
	// Opts into the security pack and satisfies it: a db with an owner, no PCI
	// crossing, no cross-region sync.
	data := `package diagram

diagram: #Diagram & {
	policies: ["security"]
	nodes: {
		api: {type: "process", role: "service", region: "eu", zone: "public", x: 1, y: 1, label: "api"}
		db: {type: "process", role: "database", owner: "payments", region: "eu", zone: "public", x: 2, y: 2, label: "db"}
	}
	edges: [
		{id: "e1", source: "api", target: "db", kind: "arrow", call: "reads"},
	]
}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/vet", evalBody(t, data))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var body struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.OK {
		t.Fatalf("ok = false, want true (body %q)", rec.Body.String())
	}
}

func TestVetPolicyViolation(t *testing.T) {
	// db without an owner (db-needs-owner) and a PCI-boundary crossing edge.
	data := `package diagram

diagram: #Diagram & {
	policies: ["security"]
	nodes: {
		api: {type: "process", role: "service", zone: "public", x: 1, y: 1, label: "api"}
		db: {type: "process", role: "database", zone: "pci", x: 2, y: 2, label: "db"}
	}
	edges: [
		{id: "e1", source: "api", target: "db", kind: "arrow", call: "reads"},
	]
}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/vet", evalBody(t, data))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var body struct {
		OK          bool         `json:"ok"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OK || len(body.Diagnostics) == 0 {
		t.Fatalf("want ok:false with policy diagnostics, got %q", rec.Body.String())
	}
	rules := map[string]bool{}
	for _, d := range body.Diagnostics {
		if d.Kind != kindPolicy {
			t.Fatalf("diagnostic kind = %q, want %q (%+v)", d.Kind, kindPolicy, d)
		}
		rules[d.Rule] = true
	}
	for _, want := range []string{"db-needs-owner", "no-pci-crossing"} {
		if !rules[want] {
			t.Fatalf("missing rule %q in %+v", want, body.Diagnostics)
		}
	}
}

func TestVetPolicyNotOptedIn(t *testing.T) {
	// Same violating shape but no opt-in: the pack must not run, so vet is clean.
	data := `package diagram

diagram: #Diagram & {
	nodes: {
		db: {type: "process", role: "database", x: 1, y: 1, label: "db"}
	}
	edges: []
}
`
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/vet", evalBody(t, data))
	var body struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.OK {
		t.Fatalf("ok = false, want true when not opted in (body %q)", rec.Body.String())
	}
}

func TestImportComposeProducesFacts(t *testing.T) {
	compose := `services:
  web:
    image: nginx
    depends_on:
      - db
  db:
    image: postgres
`
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(sourceRequest{Source: compose})
	rec := postJSON(router, "/import/compose", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Facts string `json:"facts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var facts struct {
		Source string `json:"source"`
		Links  []struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(out.Facts), &facts); err != nil {
		t.Fatalf("facts is not valid JSON: %v (%q)", err, out.Facts)
	}
	if facts.Source != "compose" {
		t.Fatalf("source = %q, want compose", facts.Source)
	}
	if len(facts.Links) != 1 || facts.Links[0].Source != "web" || facts.Links[0].Target != "db" {
		t.Fatalf("links = %+v, want one web->db", facts.Links)
	}
}

func TestImportComposeMalformed(t *testing.T) {
	router := realRouter(t, testConfig(t))
	cfg := testConfig(t)
	body, _ := json.Marshal(sourceRequest{Source: "services: [1, 2"})
	rec := postJSON(router, "/import/compose", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
	diags := decodeDiags(t, rec)
	if len(diags) == 0 || diags[0].Kind != kindImport {
		t.Fatalf("diagnostics = %+v, want kind %q", diags, kindImport)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, cfg.CueDir) {
			t.Fatalf("import diagnostic leaks host path: %q", d.Message)
		}
	}
}

func TestVetDrift(t *testing.T) {
	// Diagram claims web->db; live infra shows web->cache instead.
	data := `package diagram

diagram: #Diagram & {
	nodes: {
		web: {type: "process", x: 1, y: 1, label: "web"}
		db: {type: "process", x: 2, y: 2, label: "db"}
	}
	edges: [
		{id: "e1", source: "web", target: "db", kind: "arrow"},
	]
}
`
	facts := `{"source":"compose","services":{"web":{"name":"web"},"cache":{"name":"cache"}},"links":[{"source":"web","target":"cache"}]}`

	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(dataRequest{Data: data, Facts: facts})
	rec := postJSON(router, "/vet", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		OK          bool         `json:"ok"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.OK || len(out.Diagnostics) == 0 {
		t.Fatalf("want ok:false with drift, got %q", rec.Body.String())
	}
	var missing, extra bool
	for _, d := range out.Diagnostics {
		if d.Kind != kindDrift {
			t.Fatalf("kind = %q, want %q (%+v)", d.Kind, kindDrift, d)
		}
		if strings.Contains(d.Message, "web->db") {
			missing = true
		}
		if strings.Contains(d.Message, "web->cache") {
			extra = true
		}
	}
	if !missing || !extra {
		t.Fatalf("want both missing web->db and extra web->cache, got %+v", out.Diagnostics)
	}
}

func TestVetDriftClean(t *testing.T) {
	// Diagram and infra agree: no drift.
	data := `package diagram

diagram: #Diagram & {
	nodes: {
		web: {type: "process", x: 1, y: 1, label: "web"}
		db: {type: "process", x: 2, y: 2, label: "db"}
	}
	edges: [
		{id: "e1", source: "web", target: "db", kind: "arrow"},
	]
}
`
	facts := `{"source":"compose","services":{"web":{"name":"web"},"db":{"name":"db"}},"links":[{"source":"web","target":"db"}]}`
	router := realRouter(t, testConfig(t))
	body, _ := json.Marshal(dataRequest{Data: data, Facts: facts})
	rec := postJSON(router, "/vet", body)
	var out struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.OK {
		t.Fatalf("ok = false, want true when diagram matches infra (body %q)", rec.Body.String())
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
	rec := postJSON(router, "/save", evalBody(t, validData))
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
	// The saved version file holds exactly the submitted text.
	saved, err := os.ReadFile(filepath.Join(cfg.VersionsDir, body.Version+".cue"))
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if string(saved) != validData {
		t.Fatalf("version content mismatch")
	}
	// The seed data.cue in the CUE dir must be untouched.
	if _, err := os.Stat(filepath.Join(cfg.VersionsDir, "data.cue")); !os.IsNotExist(err) {
		t.Fatalf("versions dir should not contain data.cue")
	}
}

func TestSaveInvalidNotWritten(t *testing.T) {
	cfg := testConfig(t)
	router := realRouter(t, cfg)
	data := "package diagram\n\ndiagram: #Diagram & {nodes: {a: {type: \"process\", x: \"nope\", y: 1, label: \"l\"}}, edges: []}\n"
	rec := postJSON(router, "/save", evalBody(t, data))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	entries, err := os.ReadDir(cfg.VersionsDir)
	if err != nil {
		t.Fatalf("read versions dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid data must not be persisted, found %d files", len(entries))
	}
}

func TestSaveIdempotent(t *testing.T) {
	cfg := testConfig(t)
	router := realRouter(t, cfg)
	first := postJSON(router, "/save", evalBody(t, validData))
	second := postJSON(router, "/save", evalBody(t, validData))
	if first.Body.String() != second.Body.String() {
		t.Fatalf("same content produced different versions: %q vs %q", first.Body.String(), second.Body.String())
	}
	entries, _ := os.ReadDir(cfg.VersionsDir)
	cueFiles := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".cue") {
			cueFiles++
		}
	}
	if cueFiles != 1 {
		t.Fatalf("want 1 version file, got %d", cueFiles)
	}
}

func TestListAndReadVersion(t *testing.T) {
	cfg := testConfig(t)
	router := realRouter(t, cfg)

	saveRec := postJSON(router, "/save", evalBody(t, validData))
	if saveRec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body %q", saveRec.Code, saveRec.Body.String())
	}
	var saved struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(saveRec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode save: %v", err)
	}

	listRec := getJSON(router, "/versions")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}
	var list struct {
		Versions []VersionMeta `json:"versions"`
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

	readRec := getJSON(router, "/versions/"+saved.Version)
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
		rec := getJSON(router, "/versions/"+id)
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
	rec := getJSON(router, "/versions/"+strings.Repeat("a", 64))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestVersionIndexNoDuplicateOnIdempotentSave(t *testing.T) {
	cfg := testConfig(t)
	router := realRouter(t, cfg)
	postJSON(router, "/save", evalBody(t, validData))
	postJSON(router, "/save", evalBody(t, validData)) // idempotent re-save

	// The index records the save exactly once (the re-save reused the file).
	index, err := os.ReadFile(filepath.Join(cfg.VersionsDir, "index.jsonl"))
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
	listRec := getJSON(router, "/versions")
	var list struct {
		Versions []VersionMeta `json:"versions"`
	}
	_ = json.Unmarshal(listRec.Body.Bytes(), &list)
	if len(list.Versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(list.Versions))
	}
}

func TestConfigRejectsVersionsInsideCueDir(t *testing.T) {
	cueDir, err := filepath.Abs("../cue")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CUE_DIR", cueDir)
	t.Setenv("VERSIONS_DIR", filepath.Join(cueDir, "versions"))
	if _, err := loadConfig(); err == nil {
		t.Fatal("want error for VERSIONS_DIR inside CUE_DIR, got nil")
	}
}

func TestConfigRequiresVersionsDir(t *testing.T) {
	t.Setenv("VERSIONS_DIR", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("want error for missing VERSIONS_DIR, got nil")
	}
}

func filesBody(t *testing.T, files ...File) []byte {
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
	primary := File{Name: "data.cue", Content: `package diagram
diagram: #Diagram & {
	nodes: {a: {type: "process", x: 1, y: 1, label: "a"}}
	edges: [{id: "e1", source: "a", target: "b", kind: "arrow"}]
}
`}
	extra := File{Name: "extra.cue", Content: `package diagram
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
		Provenance Provenance `json:"provenance"`
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
	rec := postJSON(router, "/eval", filesBody(t, File{Name: "schema.cue", Content: "package diagram\n"}))
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
		rec := postJSON(router, "/eval", filesBody(t, File{Name: name, Content: "package diagram\n"}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("name %q status = %d, want 400", name, rec.Code)
		}
	}
}
