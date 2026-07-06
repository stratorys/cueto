// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package cueeval evaluates the default project (package main) against the
// imported diagram schema in-process. It isolates the cuelang library behind the
// Evaluator seam and delegates version and project persistence to an embedded
// store.Store.
package cueeval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"

	"github.com/stratorys/cueto/backend/internal/config"
	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/store"
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
	Eval(ctx context.Context, files []File) (json.RawMessage, []Hint, Provenance, []diag.Diagnostic, error)
	// EvalExpr evaluates a standalone CUE snippet in a fresh context - no schema, no
	// package overlay - and returns its concrete value as JSON. It backs the REPL
	// scratchpad: the input is ephemeral and never joins the file set, the schema,
	// saved versions, or the diagram. Diagnostics carry compile/concreteness errors.
	EvalExpr(ctx context.Context, source string) (json.RawMessage, []diag.Diagnostic, error)
	// EvalQuery evaluates expr as a single CUE expression against the editable file
	// set overlaid on the schema, so the expression can reference the live `diagram`
	// (e.g. `diagram.nodes.x.owner`). Like EvalExpr the input is ephemeral: it is
	// overlaid in a throwaway build, never joins the file set, the schema, or a saved
	// version, and does not alter /eval output. Diagnostics carry compile or
	// concreteness errors from the expression or the underlying diagram.
	EvalQuery(ctx context.Context, files []File, expr string) (json.RawMessage, []diag.Diagnostic, error)
	// Keys returns the dotted identifier field paths of every top-level data field
	// in the editable file set overlaid on the schema (people, people.george,
	// diagram, diagram.nodes, ...), for the REPL's autocomplete over the whole data.
	// It reads the value's structure, not a concrete result; diagnostics mirror an
	// invalid/incomplete diagram and the overlay is thrown away.
	Keys(ctx context.Context, files []File) ([]string, []diag.Diagnostic, error)
	// Introspect returns the CUE builtin functions and importable standard-library
	// packages (each with its members) that a REPL query can reference. The result
	// is static per CUE version, so it is computed once and cached; it feeds the
	// REPL's autocomplete and reference browser.
	Introspect() CueMeta
	// Vet unifies the file set and validates it against the schema. Diagnostics are
	// empty when clean.
	Vet(ctx context.Context, files []File) ([]diag.Diagnostic, error)
	// Save validates data and, only when valid, stores it as an immutable version
	// keyed by its content hash, returning the version id. Diagnostics are returned
	// (and nothing is written) when the data is invalid. The seed data.cue and the
	// diagram schema package are never touched.
	Save(ctx context.Context, projectID, data string) (string, []diag.Diagnostic, error)
	// ListVersions returns a project's saved versions newest-first, for the
	// history/diff view.
	ListVersions(ctx context.Context, projectID string) ([]store.VersionMeta, error)
	// ReadVersion returns the stored data.cue text of one of a project's versions by
	// its content hash. Both ids are validated before any filesystem access.
	ReadVersion(ctx context.Context, projectID, id string) (string, error)
	// ReadSeed returns the on-disk seed data.cue text (the fallback used to seed a
	// "from sample" project). It is a static fixture, never a saved version.
	ReadSeed(ctx context.Context) (string, error)
	// ListProjects returns the registered projects (newest-updated first). The first
	// call bootstraps the registry, migrating any legacy flat version store into a
	// "default" project.
	ListProjects(ctx context.Context) ([]store.ProjectMeta, error)
	// CreateProject registers a new project seeded either "blank" or "sample" (a copy
	// of the seed data.cue as its first version), returning its metadata.
	CreateProject(ctx context.Context, name, seed string) (store.ProjectMeta, error)
	// RenameProject changes a project's display name.
	RenameProject(ctx context.Context, id, name string) (store.ProjectMeta, error)
	// DeleteProject removes a project and its version store. The last project cannot
	// be deleted.
	DeleteProject(ctx context.Context, id string) error
	// Format runs `cue fmt` over arbitrary source text.
	Format(source string) (string, error)
	// Rewrite splices canvas edits (node upserts/deletes and an optional edge list)
	// into one editable file's source, preserving its hand-written CUE and comments.
	// Diagnostics are returned on a syntax error; nothing is otherwise validated.
	Rewrite(op RewriteOp) (string, []diag.Diagnostic, error)
}

// Operational errors, distinct from user-input diagnostics. Handlers map these
// to HTTP status codes; they never carry CUE positions or host paths.
var (
	ErrTimeout        = errors.New("evaluation timed out")
	ErrOutputTooLarge = errors.New("evaluation output too large")
	ErrSeedNotFound   = errors.New("seed data.cue not found")
	errEvalPanic      = errors.New("evaluation failed")
)

// cueEvaluator is the concrete Evaluator. It owns the CUE-facing configuration
// and embeds the version/project store, so persistence operations (ListVersions,
// projects) are served directly by store.Store while CUE evaluation stays here.
//
// Round-trip fidelity: evaluation concretizes references and if-guards and drops
// comments by design. Diagram logic lives in the imported diagram schema, so
// data.cue is expected to be flat concrete data; the graph->CUE regeneration
// flattens it the same way.
type cueEvaluator struct {
	*store.Store
	cueDir         string
	timeout        time.Duration
	maxOutputBytes int
	// Memoizes the static CUE builtin/package reference (see Introspect).
	metaOnce sync.Once
	meta     CueMeta
	// Memoizes the diagram schema package root, whose #Node/#Column/#Edge
	// definitions drive inlay-hint generation. The definitions live in the imported
	// package now, so they are loaded once here rather than read off a project value.
	schemaOnce    sync.Once
	schemaRootVal cue.Value
}

// schemaRoot builds the diagram schema package once and returns its root value,
// from which hint generation reads the #Node/#Column/#Edge definitions. The
// definitions moved out of the project root into the imported package, so they are
// no longer top-level fields of an evaluated project. A build failure yields a
// zero value, which makes hint generation a no-op rather than an error.
func (e *cueEvaluator) schemaRoot() cue.Value {
	e.schemaOnce.Do(func() {
		instances := load.Instances([]string{"./diagram"}, &load.Config{Dir: e.cueDir})
		if len(instances) == 0 || instances[0].Err != nil {
			return
		}
		e.schemaRootVal = cuecontext.New().BuildInstance(instances[0])
	})
	return e.schemaRootVal
}

// New builds an Evaluator from cfg, wiring a store rooted at cfg.VersionsDir.
func New(cfg config.Config) Evaluator {
	return &cueEvaluator{
		Store:          store.New(cfg.VersionsDir),
		cueDir:         cfg.CueDir,
		timeout:        cfg.EvalTimeout,
		maxOutputBytes: cfg.MaxOutputBytes,
	}
}

// Eval implements Evaluator.
func (e *cueEvaluator) Eval(ctx context.Context, files []File) (json.RawMessage, []Hint, Provenance, []diag.Diagnostic, error) {
	_, diagram, diags, err := e.evaluate(ctx, files, "")
	if err != nil || len(diags) > 0 {
		return nil, nil, Provenance{}, diags, err
	}
	out, merr := diagram.MarshalJSON()
	if merr != nil {
		return nil, nil, Provenance{}, diag.From(merr, e.cueDir, diag.KindIncomplete), nil
	}
	if len(out) > e.maxOutputBytes {
		return nil, nil, Provenance{}, nil, ErrOutputTooLarge
	}
	return out, hintsFrom(e.schemaRoot(), diagram), provenanceFrom(files), nil, nil
}

// Vet implements Evaluator: it unifies the file set and validates it against the
// schema, returning the same diagnostics an eval would surface.
func (e *cueEvaluator) Vet(ctx context.Context, files []File) ([]diag.Diagnostic, error) {
	_, _, diags, err := e.evaluate(ctx, files, "")
	return diags, err
}

// Save implements Evaluator: validate, then persist a new immutable version.
// Versions remain single-file (the primary data.cue); multi-file versioning is a
// separate concern from live multi-file editing.
func (e *cueEvaluator) Save(ctx context.Context, projectID, data string) (string, []diag.Diagnostic, error) {
	dir, err := e.ResolveProjectDir(projectID)
	if err != nil {
		return "", nil, err
	}
	files := []File{{Name: "data.cue", Content: data}}
	if _, _, diags, err := e.evaluate(ctx, files, ""); err != nil || len(diags) > 0 {
		return "", diags, err
	}
	version, err := e.WriteVersion(dir, []byte(data))
	if err != nil {
		return "", nil, err
	}
	return version, nil, nil
}

// CreateProject implements Evaluator. A "sample" seed reads the on-disk seed
// data.cue and hands it to the store as the project's first version; "blank"
// leaves the project empty. Reading the seed is the only CUE-side concern, so the
// rest of the creation is delegated to the store.
func (e *cueEvaluator) CreateProject(ctx context.Context, name, seed string) (store.ProjectMeta, error) {
	var seedData []byte
	if seed == "sample" {
		if data, err := e.ReadSeed(ctx); err == nil {
			seedData = []byte(data)
		}
	}
	return e.Store.Create(name, seedData)
}

// ReadSeed implements Evaluator. The path is server-built from cueDir, so it can
// never traverse outside the package dir.
func (e *cueEvaluator) ReadSeed(_ context.Context) (string, error) {
	data, err := os.ReadFile(filepath.Join(e.cueDir, "data.cue"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrSeedNotFound
		}
		return "", err
	}
	return string(data), nil
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
func (e *cueEvaluator) EvalExpr(ctx context.Context, source string) (json.RawMessage, []diag.Diagnostic, error) {
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
		return nil, nil, ErrTimeout
	case r := <-done:
		switch {
		case r.err != nil:
			return nil, nil, r.err
		case len(r.diags) > 0:
			return nil, r.diags, nil
		case len(r.json) > e.maxOutputBytes:
			return nil, nil, ErrOutputTooLarge
		default:
			return r.json, nil, nil
		}
	}
}

// EvalQuery implements Evaluator. It overlays the editable files on the schema
// and binds expr into the diagram package (see replQuerySource), so the expression
// can read the live `diagram`, then marshals the concrete result. It runs under
// the same deadline, panic recovery, and output bound as Eval via evaluate; the
// overlay is thrown away, so nothing is persisted and /eval output is unaffected.
func (e *cueEvaluator) EvalQuery(ctx context.Context, files []File, expr string) (json.RawMessage, []diag.Diagnostic, error) {
	_, result, diags, err := e.evaluate(ctx, files, expr)
	if err != nil || len(diags) > 0 {
		return nil, diags, err
	}
	out, merr := result.MarshalJSON()
	if merr != nil {
		return nil, diag.From(merr, e.cueDir, diag.KindIncomplete), nil
	}
	if len(out) > e.maxOutputBytes {
		return nil, nil, ErrOutputTooLarge
	}
	return out, nil, nil
}

type exprResult struct {
	json  json.RawMessage
	diags []diag.Diagnostic
	err   error
}

// evalExprValue validates a compiled REPL value and marshals it to JSON, mapping a
// compile error to a parse diagnostic and a non-concrete result to incomplete.
func evalExprValue(value cue.Value, cueDir string) exprResult {
	if err := value.Err(); err != nil {
		return exprResult{diags: diag.From(err, cueDir, diag.KindParse)}
	}
	if err := value.Validate(cue.Concrete(true)); err != nil {
		return exprResult{diags: diag.From(err, cueDir, diag.KindIncomplete)}
	}
	out, err := value.MarshalJSON()
	if err != nil {
		return exprResult{diags: diag.From(err, cueDir, diag.KindIncomplete)}
	}
	return exprResult{json: out}
}

// evaluate runs build + concreteness validation under a deadline. Because a
// runaway CUE unification cannot be force-killed, the worker goroutine is left
// to finish (or leak) on timeout rather than blocking the request; the router's
// concurrency cap bounds how many such goroutines can exist at once. A fresh
// cue.Context per call means a leaked evaluation's memory is reclaimed once it
// finally completes, instead of interning forever on a shared context.
func (e *cueEvaluator) evaluate(ctx context.Context, files []File, query string) (cue.Value, cue.Value, []diag.Diagnostic, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan buildResult, 1)
	go func() {
		done <- recoverToResult(func() buildResult {
			// primary is the value whose concreteness gates success: the diagram for
			// a plain build, or the query result when a REPL expression is overlaid.
			root, primary, diags, err := e.build(files, query)
			if err == nil && len(diags) == 0 {
				if verr := primary.Validate(cue.Concrete(true)); verr != nil {
					diags = diag.From(verr, e.cueDir, diag.KindIncomplete)
				}
			}
			return buildResult{root, primary, diags, err}
		})
	}()

	select {
	case <-ctx.Done():
		return cue.Value{}, cue.Value{}, nil, ErrTimeout
	case r := <-done:
		return r.root, r.diagram, r.diags, r.err
	}
}

type buildResult struct {
	root    cue.Value
	diagram cue.Value
	diags   []diag.Diagnostic
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

// build overlays the client's editable files on the default project (package
// main) and returns the `diagram` value. The schema lives in the diagram/
// subpackage, imported by the project, and is never an overlay target: every
// client filename passes validEditableName (a bare .cue name at the module root),
// and the overlay key is server-built via filepath.Join, so the schema can never
// be supplied, replaced, or escaped by a client.
func (e *cueEvaluator) build(files []File, query string) (cue.Value, cue.Value, []diag.Diagnostic, error) {
	overlay := map[string]load.Source{}
	for _, f := range files {
		if !validEditableName(f.Name) {
			return cue.Value{}, cue.Value{}, []diag.Diagnostic{{
				Message: fmt.Sprintf("invalid file name %q", f.Name),
				Kind:    diag.KindParse,
			}}, nil
		}
		overlay[filepath.Join(e.cueDir, f.Name)] = load.FromString(f.Content)
	}
	// REPL query: overlay a backend-authored field binding the expression into the
	// diagram package so it can read the live `diagram`. Only present for /repl
	// queries, so /eval, /vet, and /save are unaffected. replQuerySource wraps the
	// expr in ( ) and prepends imports for any standard-library packages it uses.
	if query != "" {
		overlay[e.replPath()] = load.FromString(replQuerySource(query))
	}
	cfg := &load.Config{Dir: e.cueDir, Overlay: overlay}

	instances := load.Instances([]string{"."}, cfg)
	if len(instances) == 0 {
		return cue.Value{}, cue.Value{}, nil, errors.New("no CUE instance loaded")
	}
	if err := instances[0].Err; err != nil {
		return cue.Value{}, cue.Value{}, diag.From(err, e.cueDir, diag.KindParse), nil
	}

	value := cuecontext.New().BuildInstance(instances[0])
	if err := value.Err(); err != nil {
		return cue.Value{}, cue.Value{}, diag.From(err, e.cueDir, diag.KindSchema), nil
	}

	diagram := value.LookupPath(cue.ParsePath("diagram"))
	if !diagram.Exists() {
		return cue.Value{}, cue.Value{}, []diag.Diagnostic{{
			Message: "no `diagram` field in data.cue",
			Kind:    diag.KindIncomplete,
		}}, nil
	}
	// A REPL query returns its bound expression as the primary value; the diagram
	// still had to build and resolve for the expression to read it.
	if query != "" {
		return value, value.LookupPath(cue.ParsePath("replResult")), nil, nil
	}
	return value, diagram, nil, nil
}

// replPath is the overlay path of the backend-authored REPL query binding. The
// name has no leading underscore/dot, so the loader includes it.
func (e *cueEvaluator) replPath() string {
	return filepath.Join(e.cueDir, "repl_query.cue")
}

// replQuerySource builds the file overlaid (never written to disk) for a /repl
// query. The surrounding ( ) force expr to a single expression so a client cannot
// inject extra package fields; replImports prepends imports for exactly the
// standard-library packages expr references (an unused import is a CUE error, so
// this must be exact). Being in the default project `package main`, the expression
// resolves the live `diagram`. replResult is looked up and marshaled; it never
// affects /eval, which builds without this overlay.
func replQuerySource(expr string) string {
	return fmt.Sprintf("package main\n\n%sreplResult: (%s)\n", replImports(expr), expr)
}
