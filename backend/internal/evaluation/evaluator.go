// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package evaluation is the pure CUE engine: it unifies the editable file set
// against the imported diagram schema, evaluates REPL expressions, and derives
// inlay hints and the static CUE reference. It knows nothing about projects,
// disks, or source formats; its input is a prepared file set and its output is
// JSON, hints, and diagnostics. It never imports the workspace or content
// concerns.
package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"

	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/domain"
)

// Operational errors, distinct from user-input diagnostics. Handlers map these
// to HTTP status codes; they never carry CUE positions or host paths.
var (
	ErrTimeout        = errors.New("evaluation timed out")
	ErrOutputTooLarge = errors.New("evaluation output too large")
	errEvalPanic      = errors.New("evaluation failed")
)

// Engine is the CUE evaluator. It owns the CUE-facing configuration and holds no
// persistence: version and project storage is the workspace concern.
//
// Round-trip fidelity: evaluation concretizes references and if-guards and drops
// comments by design. Diagram logic lives in the imported diagram schema, so
// data.cue is expected to be flat concrete data; the graph->CUE regeneration
// flattens it the same way.
type Engine struct {
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

// New builds an Engine from plain parameters: the diagram schema dir, the
// per-evaluation deadline, and the evaluated-output byte cap. The engine reads no
// config struct, so the same code backs the HTTP server, the CLI, and MCP.
func New(cueDir string, timeout time.Duration, maxOutputBytes int) *Engine {
	return &Engine{
		cueDir:         cueDir,
		timeout:        timeout,
		maxOutputBytes: maxOutputBytes,
	}
}

// schemaRoot builds the diagram schema package once and returns its root value,
// from which hint generation reads the #Node/#Column/#Edge definitions. The
// definitions moved out of the project root into the imported package, so they are
// no longer top-level fields of an evaluated project. A build failure yields a
// zero value, which makes hint generation a no-op rather than an error.
func (e *Engine) schemaRoot() cue.Value {
	e.schemaOnce.Do(func() {
		instances := load.Instances([]string{"./diagram"}, &load.Config{Dir: e.cueDir})
		if len(instances) == 0 || instances[0].Err != nil {
			return
		}
		e.schemaRootVal = cuecontext.New().BuildInstance(instances[0])
	})
	return e.schemaRootVal
}

// Eval unifies the editable file set into one diagram and returns its concrete
// JSON and inlay hints, or input diagnostics when the data is invalid/incomplete.
// Hints are non-empty only on success. The error is non-nil only for operational
// failures (timeout, output too large) not tied to a source position. Provenance
// is derived separately by the authoring concern from the same file set.
func (e *Engine) Eval(ctx context.Context, src Source) (json.RawMessage, []Hint, []diag.Diagnostic, error) {
	_, diagram, diags, err := e.evaluate(ctx, src, "")
	if err != nil || len(diags) > 0 {
		return nil, nil, diags, err
	}
	out, merr := diagram.MarshalJSON()
	if merr != nil {
		return nil, nil, diag.From(merr, src.Dir, diag.KindIncomplete), nil
	}
	if len(out) > e.maxOutputBytes {
		return nil, nil, nil, ErrOutputTooLarge
	}
	return out, hintsFrom(e.schemaRoot(), diagram), nil, nil
}

// Vet unifies the file set and validates it against the schema, returning the
// same diagnostics an eval would surface. Diagnostics are empty when clean.
func (e *Engine) Vet(ctx context.Context, src Source) ([]diag.Diagnostic, error) {
	_, _, diags, err := e.evaluate(ctx, src, "")
	return diags, err
}

// EvalExpr evaluates a standalone CUE snippet in a fresh context - no schema, no
// package overlay - and returns its concrete value as JSON. It backs the REPL
// scratchpad: the input is ephemeral and never joins the file set, the schema,
// saved versions, or the diagram. It runs under the same deadline and panic
// recovery as Eval, since REPL input is equally untrusted; nothing is persisted.
func (e *Engine) EvalExpr(ctx context.Context, source string) (json.RawMessage, []diag.Diagnostic, error) {
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

// EvalQuery evaluates expr as a single CUE expression against the editable file
// set overlaid on the schema, so the expression can reference the live `diagram`.
// Like EvalExpr the input is ephemeral: it is overlaid in a throwaway build, never
// joins the file set, the schema, or a saved version, and does not alter /eval
// output. It runs under the same deadline, panic recovery, and output bound as
// Eval via evaluate.
func (e *Engine) EvalQuery(ctx context.Context, src Source, expr string) (json.RawMessage, []diag.Diagnostic, error) {
	_, result, diags, err := e.evaluate(ctx, src, expr)
	if err != nil || len(diags) > 0 {
		return nil, diags, err
	}
	out, merr := result.MarshalJSON()
	if merr != nil {
		return nil, diag.From(merr, src.Dir, diag.KindIncomplete), nil
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
func (e *Engine) evaluate(ctx context.Context, src Source, query string) (cue.Value, cue.Value, []diag.Diagnostic, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan buildResult, 1)
	go func() {
		done <- recoverToResult(func() buildResult {
			// primary is the value whose concreteness gates success: the diagram for
			// a plain build, or the query result when a REPL expression is overlaid.
			root, primary, diags, err := e.build(src, query)
			if err == nil && len(diags) == 0 {
				if verr := primary.Validate(cue.Concrete(true)); verr != nil {
					diags = diag.From(verr, src.Dir, diag.KindIncomplete)
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
// client filename passes domain.ValidEditableName (a safe relative path that
// reserves the diagram/ and cue.mod dirs), and the overlay key is server-built
// via filepath.Join, so the schema can never be supplied, replaced, or escaped
// by a client.
func (e *Engine) build(src Source, query string) (cue.Value, cue.Value, []diag.Diagnostic, error) {
	overlay := map[string]load.Source{}
	for _, f := range src.Overlay {
		if !domain.ValidEditableName(f.Name) {
			return cue.Value{}, cue.Value{}, []diag.Diagnostic{{
				Message: fmt.Sprintf("invalid file name %q", f.Name),
				Kind:    diag.KindParse,
			}}, nil
		}
		overlay[filepath.Join(src.Dir, f.Name)] = load.FromString(f.Content)
	}
	// REPL query: overlay a backend-authored field binding the expression into the
	// diagram package so it can read the live `diagram`. Only present for /repl
	// queries, so /eval, /vet, and /save are unaffected. replQuerySource wraps the
	// expr in ( ) and prepends imports for any standard-library packages it uses.
	if query != "" {
		overlay[replPath(src.Dir)] = load.FromString(replQuerySource(query))
	}
	cfg := &load.Config{Dir: src.Dir, Overlay: overlay}

	instances := load.Instances([]string{"."}, cfg)
	if len(instances) == 0 {
		return cue.Value{}, cue.Value{}, nil, errors.New("no CUE instance loaded")
	}
	if err := instances[0].Err; err != nil {
		return cue.Value{}, cue.Value{}, diag.From(err, src.Dir, diag.KindParse), nil
	}

	value := cuecontext.New().BuildInstance(instances[0])
	if err := value.Err(); err != nil {
		return cue.Value{}, cue.Value{}, diag.From(err, src.Dir, diag.KindSchema), nil
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

// replPath is the overlay path of the backend-authored REPL query binding, rooted
// at the source's module dir. The name has no leading underscore/dot, so the
// loader includes it.
func replPath(dir string) string {
	return filepath.Join(dir, "repl_query.cue")
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
