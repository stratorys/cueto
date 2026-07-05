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
		Nodes map[string]any `json:"nodes"`
		Edges []struct {
			ID string `json:"id"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(out.Nodes))
	}
	// Edge list order must be stable across a round-trip.
	got := []string{out.Edges[0].ID, out.Edges[1].ID, out.Edges[2].ID}
	want := []string{"e1", "e2", "e3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("edge order = %v, want %v", got, want)
		}
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

func (b *blockingEval) Eval(ctx context.Context, data string) (json.RawMessage, []Diagnostic, error) {
	b.entered <- struct{}{}
	<-b.release
	return json.RawMessage(`{"nodes":{},"edges":[]}`), nil, nil
}

func (b *blockingEval) Vet(ctx context.Context, data string) ([]Diagnostic, error) {
	return nil, nil
}

func (b *blockingEval) Save(ctx context.Context, data string) (string, []Diagnostic, error) {
	return "v", nil, nil
}

func (b *blockingEval) Format(source string) (string, error) { return source, nil }

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
	if len(entries) != 1 {
		t.Fatalf("want 1 version file, got %d", len(entries))
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
