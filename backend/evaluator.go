// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
)

// Evaluator is the diagram evaluation contract the handlers depend on. Keeping
// it an interface isolates the cuelang library behind one seam so the transport
// stays library-agnostic and tests can substitute a fake.
type Evaluator interface {
	// Eval unifies the editable file set into one diagram and returns its concrete
	// JSON, inlay hints, and per-element provenance (which file authored each
	// node/edge), or input diagnostics when the data is invalid/incomplete. Hints
	// and provenance are non-empty only on success. The error is non-nil only for
	// operational failures (timeout, output too large) not tied to a source
	// position.
	Eval(ctx context.Context, files []File) (json.RawMessage, []Hint, Provenance, []Diagnostic, error)
	// EvalExpr evaluates a standalone CUE snippet in a fresh context - no schema, no
	// package overlay - and returns its concrete value as JSON. It backs the REPL
	// scratchpad: the input is ephemeral and never joins the file set, the schema,
	// saved versions, or the diagram. Diagnostics carry compile/concreteness errors.
	EvalExpr(ctx context.Context, source string) (json.RawMessage, []Diagnostic, error)
	// Vet unifies the file set and validates it against the schema and any opted-in
	// policy packs. When facts (imported #Actual, as JSON/CUE) is non-empty, it also
	// reports drift between the diagram and the live topology. Diagnostics are empty
	// when clean.
	Vet(ctx context.Context, files []File, facts string) ([]Diagnostic, error)
	// ImportCompose parses docker-compose YAML into #Actual facts (JSON), or
	// returns kindImport diagnostics on a parse failure.
	ImportCompose(source string) (string, []Diagnostic, error)
	// Save validates data and, only when valid, stores it as an immutable version
	// keyed by its content hash, returning the version id. Diagnostics are returned
	// (and nothing is written) when the data is invalid. The hand-owned schema.cue
	// and the seed data.cue are never touched.
	Save(ctx context.Context, projectID, data string) (string, []Diagnostic, error)
	// ListVersions returns a project's saved versions newest-first, for the
	// history/diff view.
	ListVersions(ctx context.Context, projectID string) ([]VersionMeta, error)
	// ReadVersion returns the stored data.cue text of one of a project's versions by
	// its content hash. Both ids are validated before any filesystem access.
	ReadVersion(ctx context.Context, projectID, id string) (string, error)
	// ReadSeed returns the on-disk seed data.cue text (the fallback used to seed a
	// "from sample" project). It is a static fixture, never a saved version.
	ReadSeed(ctx context.Context) (string, error)
	// ListProjects returns the registered projects (newest-updated first). The first
	// call bootstraps the registry, migrating any legacy flat version store into a
	// "default" project.
	ListProjects(ctx context.Context) ([]ProjectMeta, error)
	// CreateProject registers a new project seeded either "blank" or "sample" (a copy
	// of the seed data.cue as its first version), returning its metadata.
	CreateProject(ctx context.Context, name, seed string) (ProjectMeta, error)
	// RenameProject changes a project's display name.
	RenameProject(ctx context.Context, id, name string) (ProjectMeta, error)
	// DeleteProject removes a project and its version store. The last project cannot
	// be deleted.
	DeleteProject(ctx context.Context, id string) error
	// Format runs `cue fmt` over arbitrary source text.
	Format(source string) (string, error)
	// Rewrite splices canvas edits (node upserts/deletes and an optional edge list)
	// into one editable file's source, preserving its hand-written CUE and comments.
	// Diagnostics are returned on a syntax error; nothing is otherwise validated.
	Rewrite(op RewriteOp) (string, []Diagnostic, error)
}

// VersionMeta identifies one saved version and when it was first saved. SavedAt
// comes from the append-only index when present, else the file mtime.
type VersionMeta struct {
	Version string    `json:"version"`
	SavedAt time.Time `json:"savedAt"`
}

// versionIDPattern is the exact shape of a content-hash id (sha256 hex). Reads
// are rejected unless the id matches, so a version id from the URL can never
// escape the versions dir via path traversal.
var versionIDPattern = regexp.MustCompile("^[a-f0-9]{64}$")

// Operational errors, distinct from user-input diagnostics. Handlers map these
// to HTTP status codes; they never carry CUE positions or host paths.
var (
	errTimeout          = errors.New("evaluation timed out")
	errEvalPanic        = errors.New("evaluation failed")
	errOutputTooLarge   = errors.New("evaluation output too large")
	errNoVersionsDir    = errors.New("versions directory is not configured")
	errInvalidVersionID = errors.New("invalid version id")
	errVersionNotFound  = errors.New("version not found")
	errSeedNotFound     = errors.New("seed data.cue not found")
	errInvalidProjectID = errors.New("invalid project id")
	errProjectNotFound  = errors.New("project not found")
	errLastProject      = errors.New("cannot delete the last project")
)

// cueEvaluator evaluates schema.cue + a per-request data.cue in-process.
//
// Round-trip fidelity: evaluation concretizes references and if-guards and drops
// comments by design. Diagram logic lives in schema.cue, so data.cue is expected
// to be flat concrete data; the graph->CUE regeneration flattens it the same way.
type cueEvaluator struct {
	cueDir         string
	versionsDir    string
	timeout        time.Duration
	maxOutputBytes int
	// Guards the project registry (projects.json) read-modify-write. Per-version
	// files are content-addressed and written atomically, so only registry mutations
	// need serializing.
	mu sync.Mutex
}

func newCueEvaluator(cfg Config) *cueEvaluator {
	return &cueEvaluator{
		cueDir:         cfg.CueDir,
		versionsDir:    cfg.VersionsDir,
		timeout:        cfg.EvalTimeout,
		maxOutputBytes: cfg.MaxOutputBytes,
	}
}

// Eval implements Evaluator.
func (e *cueEvaluator) Eval(ctx context.Context, files []File) (json.RawMessage, []Hint, Provenance, []Diagnostic, error) {
	root, diagram, diags, err := e.evaluate(ctx, files, "")
	if err != nil || len(diags) > 0 {
		return nil, nil, Provenance{}, diags, err
	}
	out, merr := diagram.MarshalJSON()
	if merr != nil {
		return nil, nil, Provenance{}, diagnosticsFrom(merr, e.cueDir, kindIncomplete), nil
	}
	if len(out) > e.maxOutputBytes {
		return nil, nil, Provenance{}, nil, errOutputTooLarge
	}
	return out, hintsFrom(root, diagram), provenanceFrom(files), nil, nil
}

// Vet implements Evaluator. Beyond schema validation it reports the findings of
// any opted-in policy pack (from `policyReport`) and, when facts are supplied,
// drift between the diagram and the live topology (from `driftReport`).
func (e *cueEvaluator) Vet(ctx context.Context, files []File, facts string) ([]Diagnostic, error) {
	root, _, diags, err := e.evaluate(ctx, files, facts)
	if err != nil || len(diags) > 0 {
		return diags, err
	}
	out := e.policyDiagnostics(root)
	if facts != "" {
		out = append(out, e.driftDiagnostics(root)...)
	}
	return out, nil
}

// driftDiagnostics reads the `driftReport` produced by the drift harness (present
// only when facts were overlaid) and reports edges that disagree between the
// diagram and the live topology. missing = the diagram claims an edge the infra
// lacks; extra = the infra has an edge the diagram omits.
func (e *cueEvaluator) driftDiagnostics(root cue.Value) []Diagnostic {
	report := root.LookupPath(cue.ParsePath("driftReport"))
	if !report.Exists() {
		return nil
	}
	sections := []struct {
		field   string
		message string
	}{
		{"missing", "diagram edge %s is not present in the live infra"},
		{"extra", "live infra has %s, missing from the diagram"},
	}
	var out []Diagnostic
	for _, section := range sections {
		items, err := report.LookupPath(cue.ParsePath(section.field)).List()
		if err != nil {
			continue
		}
		for items.Next() {
			edge, err := items.Value().String()
			if err != nil {
				continue
			}
			out = append(out, Diagnostic{Kind: kindDrift, Message: fmt.Sprintf(section.message, edge)})
		}
	}
	return out
}

// ImportCompose implements Evaluator (parser lives in importer.go).
func (e *cueEvaluator) ImportCompose(source string) (string, []Diagnostic, error) {
	return e.importCompose(source)
}

// policyDiagnostics reads the `policyReport` sibling produced by the policy
// harness and flattens each pack's violations into kind:"policy" diagnostics,
// anchored to the offending node/edge. A missing or empty report yields none.
func (e *cueEvaluator) policyDiagnostics(root cue.Value) []Diagnostic {
	report := root.LookupPath(cue.ParsePath("policyReport"))
	if !report.Exists() {
		return nil
	}
	packs, err := report.Fields()
	if err != nil {
		return nil
	}
	var out []Diagnostic
	for packs.Next() {
		items, err := packs.Value().List()
		if err != nil {
			continue
		}
		for items.Next() {
			var v struct {
				Rule    string `json:"rule"`
				Node    string `json:"node"`
				Edge    string `json:"edge"`
				Message string `json:"message"`
			}
			if err := items.Value().Decode(&v); err != nil {
				continue
			}
			out = append(out, Diagnostic{
				Kind:    kindPolicy,
				Rule:    v.Rule,
				NodeID:  v.Node,
				EdgeID:  v.Edge,
				Message: v.Message,
			})
		}
	}
	return out
}

// Save implements Evaluator: validate, then persist a new immutable version.
// Versions remain single-file (the primary data.cue); multi-file versioning is a
// separate concern from live multi-file editing.
func (e *cueEvaluator) Save(ctx context.Context, projectID, data string) (string, []Diagnostic, error) {
	dir, err := e.resolveProjectDir(projectID)
	if err != nil {
		return "", nil, err
	}
	files := []File{{Name: "data.cue", Content: data}}
	if _, _, diags, err := e.evaluate(ctx, files, ""); err != nil || len(diags) > 0 {
		return "", diags, err
	}
	version, err := e.writeVersion(dir, []byte(data))
	if err != nil {
		return "", nil, err
	}
	return version, nil, nil
}

// Format implements Evaluator.
func (e *cueEvaluator) Format(source string) (string, error) {
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

// EvalExpr implements Evaluator. It compiles source as a standalone CUE value in a
// fresh context (so a runaway snippet's memory is reclaimed once it finishes),
// validates concreteness, and marshals the result. It runs under the same deadline
// and panic recovery as Eval, since REPL input is equally untrusted; the snippet
// sees no schema and no package, and nothing is persisted.
func (e *cueEvaluator) EvalExpr(ctx context.Context, source string) (json.RawMessage, []Diagnostic, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan exprResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("recovered panic in CUE repl evaluation: %v\n%s", r, debug.Stack())
				done <- exprResult{err: errEvalPanic}
			}
		}()
		done <- evalExprValue(cuecontext.New().CompileString(source), e.cueDir)
	}()

	select {
	case <-ctx.Done():
		return nil, nil, errTimeout
	case r := <-done:
		switch {
		case r.err != nil:
			return nil, nil, r.err
		case len(r.diags) > 0:
			return nil, r.diags, nil
		case len(r.json) > e.maxOutputBytes:
			return nil, nil, errOutputTooLarge
		default:
			return r.json, nil, nil
		}
	}
}

type exprResult struct {
	json  json.RawMessage
	diags []Diagnostic
	err   error
}

// evalExprValue validates a compiled REPL value and marshals it to JSON, mapping a
// compile error to a parse diagnostic and a non-concrete result to incomplete.
func evalExprValue(value cue.Value, cueDir string) exprResult {
	if err := value.Err(); err != nil {
		return exprResult{diags: diagnosticsFrom(err, cueDir, kindParse)}
	}
	if err := value.Validate(cue.Concrete(true)); err != nil {
		return exprResult{diags: diagnosticsFrom(err, cueDir, kindIncomplete)}
	}
	out, err := value.MarshalJSON()
	if err != nil {
		return exprResult{diags: diagnosticsFrom(err, cueDir, kindIncomplete)}
	}
	return exprResult{json: out}
}

// evaluate runs build + concreteness validation under a deadline. Because a
// runaway CUE unification cannot be force-killed, the worker goroutine is left
// to finish (or leak) on timeout rather than blocking the request; the router's
// concurrency cap bounds how many such goroutines can exist at once. A fresh
// cue.Context per call means a leaked evaluation's memory is reclaimed once it
// finally completes, instead of interning forever on a shared context.
func (e *cueEvaluator) evaluate(ctx context.Context, files []File, facts string) (cue.Value, cue.Value, []Diagnostic, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan buildResult, 1)
	go func() {
		done <- recoverToResult(func() buildResult {
			root, diagram, diags, err := e.build(files, facts)
			if err == nil && len(diags) == 0 {
				if verr := diagram.Validate(cue.Concrete(true)); verr != nil {
					diags = diagnosticsFrom(verr, e.cueDir, kindIncomplete)
				}
			}
			return buildResult{root, diagram, diags, err}
		})
	}()

	select {
	case <-ctx.Done():
		return cue.Value{}, cue.Value{}, nil, errTimeout
	case r := <-done:
		return r.root, r.diagram, r.diags, r.err
	}
}

type buildResult struct {
	root    cue.Value
	diagram cue.Value
	diags   []Diagnostic
	err     error
}

// recoverToResult runs fn on the worker goroutine under a panic recovery. The
// cuelang build/validate path processes untrusted input and can panic; since a
// panic is per-goroutine and gin.Recovery only guards the request goroutine,
// an unrecovered panic here would kill the whole process. Recovering converts it
// into errEvalPanic delivered through the normal channel, keeping the server up.
// The recovered value and stack are logged server-side only; the client sees a
// fixed, position-free error.
func recoverToResult(fn func() buildResult) (result buildResult) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered panic in CUE evaluation: %v\n%s", r, debug.Stack())
			result = buildResult{err: errEvalPanic}
		}
	}()
	return fn()
}

// build overlays the client's editable files on the disk schema and returns the
// `diagram` value. schema.cue is always read fresh from disk and is never
// overlaid: every client filename passes validEditableName (a bare .cue name,
// never schema.cue), and the overlay key is server-built via filepath.Join, so
// the hand-owned schema can never be supplied, replaced, or escaped by a client.
func (e *cueEvaluator) build(files []File, facts string) (cue.Value, cue.Value, []Diagnostic, error) {
	overlay := map[string]load.Source{}
	for _, f := range files {
		if !validEditableName(f.Name) {
			return cue.Value{}, cue.Value{}, []Diagnostic{{
				Message: fmt.Sprintf("invalid file name %q", f.Name),
				Kind:    kindParse,
			}}, nil
		}
		overlay[filepath.Join(e.cueDir, f.Name)] = load.FromString(f.Content)
	}
	// Drift: overlay a backend-authored harness that unifies the imported facts
	// with infra.#Actual and computes driftReport. Only present when vetting with
	// facts, so /eval and /save are unaffected. Wrapping facts in ( ) forces it to
	// be a single expression, so a client cannot inject extra package fields.
	if facts != "" {
		overlay[e.factsPath()] = load.FromString(fmt.Sprintf(driftHarnessCUE, facts))
	}
	cfg := &load.Config{Dir: e.cueDir, Overlay: overlay}

	instances := load.Instances([]string{"."}, cfg)
	if len(instances) == 0 {
		return cue.Value{}, cue.Value{}, nil, errors.New("no CUE instance loaded")
	}
	if err := instances[0].Err; err != nil {
		return cue.Value{}, cue.Value{}, diagnosticsFrom(err, e.cueDir, kindParse), nil
	}

	value := cuecontext.New().BuildInstance(instances[0])
	if err := value.Err(); err != nil {
		return cue.Value{}, cue.Value{}, diagnosticsFrom(err, e.cueDir, kindSchema), nil
	}

	diagram := value.LookupPath(cue.ParsePath("diagram"))
	if !diagram.Exists() {
		return cue.Value{}, cue.Value{}, []Diagnostic{{
			Message: "no `diagram` field in data.cue",
			Kind:    kindIncomplete,
		}}, nil
	}
	return value, diagram, nil, nil
}

// factsPath is the overlay path of the backend-authored drift harness. The name
// has no leading underscore/dot, so CUE's loader does not skip it.
func (e *cueEvaluator) factsPath() string {
	return filepath.Join(e.cueDir, "facts_overlay.cue")
}

// driftHarnessCUE is overlaid (never written to disk) during a drift vet. %s is
// the imported facts value. driftReport matches diagram edges (by node label) to
// live links (by service name): missing = in the diagram but not the infra;
// extra = in the infra but not the diagram.
const driftHarnessCUE = `package diagram

import (
	"list"
	"github.com/stratorys/cueto/infra"
)

actual: infra.#Actual & (%s)

driftReport: {
	_expected: [for e in diagram.edges {"\(diagram.nodes[e.source].label)->\(diagram.nodes[e.target].label)"}]
	_actual: [for l in actual.links {"\(l.source)->\(l.target)"}]
	missing: [for x in _expected if !list.Contains(_actual, x) {x}]
	extra: [for a in _actual if !list.Contains(_expected, a) {a}]
}
`

// writeVersion stores data as an immutable, content-addressed version and
// returns its id (the sha256 hex of the content). Writes go only into the
// configured versions dir - never the CUE package dir - so schema.cue and the
// seed data.cue are untouched and version files never join `package diagram`.
// Identical content is idempotent: an existing version is reused, not rewritten.
func (e *cueEvaluator) writeVersion(dir string, data []byte) (string, error) {
	if e.versionsDir == "" {
		return "", errNoVersionsDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:])
	path := filepath.Join(dir, id+".cue")

	// O_EXCL makes creation atomic: concurrent saves of the same content race on
	// the same name and all but one see ErrExist, which is success (idempotent).
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return id, nil
	}
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	// Record save order + timestamp in the append-only index. Only the fresh-create
	// branch reaches here (idempotent re-saves returned above), so a version is
	// indexed exactly once. The index is derived metadata: a failure to append is
	// not fatal to the save, since the version file itself is the source of truth.
	_ = e.appendIndex(dir, id)
	return id, nil
}

// indexPath is a project's append-only log of save events (one JSON object per line).
func (e *cueEvaluator) indexPath(dir string) string {
	return filepath.Join(dir, "index.jsonl")
}

// appendIndex records one save event. Content hashes carry no order or time, so
// this log is what lets ListVersions present true save order and timestamps.
func (e *cueEvaluator) appendIndex(dir, id string) error {
	line, err := json.Marshal(VersionMeta{Version: id, SavedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(e.indexPath(dir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// readIndex reads a project's save-order log into a map of id -> first-save time. A
// missing index is not an error (older versions predate it); such versions fall
// back to their file mtime in ListVersions.
func (e *cueEvaluator) readIndex(dir string) map[string]time.Time {
	times := map[string]time.Time{}
	f, err := os.Open(e.indexPath(dir))
	if err != nil {
		return times
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var meta VersionMeta
		if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
			continue
		}
		// Keep the first (earliest) timestamp for an id; ignore any later dup line.
		if _, seen := times[meta.Version]; !seen {
			times[meta.Version] = meta.SavedAt
		}
	}
	return times
}

// ListVersions implements Evaluator. It enumerates the version files and stamps
// each with its indexed save time (or mtime when it predates the index), newest
// first.
func (e *cueEvaluator) ListVersions(_ context.Context, projectID string) ([]VersionMeta, error) {
	dir, err := e.resolveProjectDir(projectID)
	if err != nil {
		return nil, err
	}
	times := e.readIndex(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []VersionMeta{}, nil
		}
		return nil, err
	}
	out := make([]VersionMeta, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".cue") {
			continue
		}
		id := strings.TrimSuffix(name, ".cue")
		saved, ok := times[id]
		if !ok {
			if info, err := entry.Info(); err == nil {
				saved = info.ModTime()
			}
		}
		out = append(out, VersionMeta{Version: id, SavedAt: saved})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavedAt.After(out[j].SavedAt) })
	return out, nil
}

// ReadVersion implements Evaluator. The id is regex-validated before any path is
// built, so it can never traverse out of the versions dir.
func (e *cueEvaluator) ReadVersion(_ context.Context, projectID, id string) (string, error) {
	dir, err := e.resolveProjectDir(projectID)
	if err != nil {
		return "", err
	}
	if !versionIDPattern.MatchString(id) {
		return "", errInvalidVersionID
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".cue"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errVersionNotFound
		}
		return "", err
	}
	return string(data), nil
}

// ReadSeed implements Evaluator. The path is server-built from CueDir, so it can
// never traverse outside the package dir.
func (e *cueEvaluator) ReadSeed(_ context.Context) (string, error) {
	data, err := os.ReadFile(filepath.Join(e.cueDir, "data.cue"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errSeedNotFound
		}
		return "", err
	}
	return string(data), nil
}
