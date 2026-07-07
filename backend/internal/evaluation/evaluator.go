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
	"sort"
	"sync"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
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
	// Memoizes the loaded diagram schema instance and its built root value. The
	// #Node/#Column/#Edge definitions drive inlay-hint generation (off the root
	// value); the instance is rebuilt into each per-call context so #Diagram can be
	// unified with a project value for view discovery.
	schemaOnce    sync.Once
	schemaInst    *build.Instance
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

// loadSchema loads the diagram schema package once. A load failure leaves the
// instance nil, which makes hint generation and view discovery no-ops rather than
// errors.
func (e *Engine) loadSchema() {
	e.schemaOnce.Do(func() {
		instances := load.Instances([]string{"./diagram"}, &load.Config{Dir: e.cueDir})
		if len(instances) == 0 || instances[0].Err != nil {
			return
		}
		e.schemaInst = instances[0]
		e.schemaRootVal = cuecontext.New().BuildInstance(instances[0])
	})
}

// schemaRoot returns the schema package root value, from which hint generation
// reads the #Node/#Column/#Edge definitions. A zero value (load failure) makes
// hint generation a no-op.
func (e *Engine) schemaRoot() cue.Value {
	e.loadSchema()
	return e.schemaRootVal
}

// Eval unifies the editable file set into one diagram and returns its concrete
// JSON and inlay hints, or input diagnostics when the data is invalid/incomplete.
// Hints are non-empty only on success. The error is non-nil only for operational
// failures (timeout, output too large) not tied to a source position. Provenance
// is derived separately by the authoring concern from the same file set.
func (e *Engine) Eval(ctx context.Context, src Source) (json.RawMessage, []string, []Hint, []TraceEntry, []diag.Diagnostic, error) {
	_, diagram, views, trace, diags, err := e.evaluate(ctx, src, "")
	if err != nil || len(diags) > 0 {
		return nil, nil, nil, nil, diags, err
	}
	// No view is a valid outcome (a knowledge-only module with nothing to infer): the
	// view list is empty and there is nothing to render, distinct from an error.
	if !diagram.Exists() {
		return nil, views, nil, nil, nil, nil
	}
	out, merr := diagram.MarshalJSON()
	if merr != nil {
		return nil, nil, nil, nil, diag.From(merr, src.Dir, diag.KindIncomplete), nil
	}
	if len(out) > e.maxOutputBytes {
		return nil, nil, nil, nil, nil, ErrOutputTooLarge
	}
	// Inlay hints join written diagram fields back to their schema positions; a derived
	// diagram has no such source, so hints are skipped when the view was inferred (the
	// trace is present exactly then).
	var hints []Hint
	if trace == nil {
		hints = hintsFrom(e.schemaRoot(), diagram)
	}
	return out, views, hints, trace, nil, nil
}

// Vet validates every package in the module for validity - parse, unification,
// schema, and closedness errors - across all packages, including siblings the root
// never imports. It never requires concreteness: an incomplete but valid module
// vets clean. Rendering a concrete view is eval's gate, not vet's. Diagnostics are
// empty when clean. It runs under the same deadline and panic recovery as eval.
func (e *Engine) Vet(ctx context.Context, src Source) ([]diag.Diagnostic, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan vetResult, 1)
	go func() {
		done <- recoverVet(func() vetResult { return vetResult{diags: e.vetModule(src)} })
	}()

	select {
	case <-ctx.Done():
		return nil, ErrTimeout
	case r := <-done:
		return r.diags, r.err
	}
}

// vetModule builds every package in the module and returns validity diagnostics
// from all of them. It selects no view and never gates concreteness: a package is
// vet-clean when it parses, unifies, and validates, even if incomplete. Validate
// without cue.Concrete reports structural errors (dangling references, schema and
// closedness violations) but not incompleteness.
func (e *Engine) vetModule(src Source) []diag.Diagnostic {
	instances, diags := e.loadModule(src, "")
	if diags != nil {
		return diags
	}
	ctx := cuecontext.New()
	// A composed error surfaces from every package that observes it (the package that
	// declares the conflict and any root package re-exposing it), so the raw list can
	// carry the same position and message more than once. Dedup by position+message,
	// the same key the graph check uses, so the report reads once per distinct error.
	seen := map[string]struct{}{}
	var out []diag.Diagnostic
	for _, inst := range instances {
		if inst.Err != nil {
			for _, d := range diag.From(inst.Err, src.Dir, diag.KindParse) {
				appendUnique(&out, seen, d)
			}
			continue
		}
		if err := ctx.BuildInstance(inst).Validate(); err != nil {
			for _, d := range diag.From(err, src.Dir, diag.KindSchema) {
				appendUnique(&out, seen, d)
			}
		}
	}
	return out
}

type vetResult struct {
	diags []diag.Diagnostic
	err   error
}

// recoverVet is the vet twin of recoverToResult: it runs fn on the worker goroutine
// under a panic recovery, converting an untrusted-input panic into errEvalPanic
// rather than killing the process.
func recoverVet(fn func() vetResult) (result vetResult) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered panic in CUE evaluation: %v\n%s", r, debug.Stack())
			result = vetResult{err: errEvalPanic}
		}
	}()
	return fn()
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
	_, result, _, _, diags, err := e.evaluate(ctx, src, expr)
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
func (e *Engine) evaluate(ctx context.Context, src Source, query string) (cue.Value, cue.Value, []string, []TraceEntry, []diag.Diagnostic, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan buildResult, 1)
	go func() {
		done <- recoverToResult(func() buildResult {
			// primary is the value whose concreteness gates success: the rendered view
			// (declared or inferred) for a plain build, or the query result when a REPL
			// expression is overlaid. A knowledge-only module has no view, so primary is
			// a zero value and there is nothing to gate.
			root, primary, views, trace, diags, err := e.build(src, query)
			if err == nil && len(diags) == 0 && primary.Exists() {
				if verr := primary.Validate(cue.Concrete(true)); verr != nil {
					diags = diag.From(verr, src.Dir, diag.KindIncomplete)
				}
			}
			return buildResult{root, primary, views, trace, diags, err}
		})
	}()

	select {
	case <-ctx.Done():
		return cue.Value{}, cue.Value{}, nil, nil, nil, ErrTimeout
	case r := <-done:
		return r.root, r.diagram, r.views, r.trace, r.diags, r.err
	}
}

type buildResult struct {
	root    cue.Value
	diagram cue.Value
	views   []string
	trace   []TraceEntry
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

// loadModule overlays the client's editable files on the module and loads every
// package in it (the recursive pattern), so packages the root imports and sibling
// packages in subdirectories both resolve. The schema lives in the diagram/
// subpackage and is never an overlay target: every client filename passes
// domain.ValidEditableName (a safe relative path that reserves the diagram/ and
// cue.mod dirs), and the overlay key is server-built via filepath.Join, so the
// schema can never be supplied, replaced, or escaped by a client. The diagnostics
// return is non-nil only for a rejected client filename.
func (e *Engine) loadModule(src Source, query string) ([]*build.Instance, []diag.Diagnostic) {
	overlay := map[string]load.Source{}
	for _, f := range src.Overlay {
		if !domain.ValidEditableName(f.Name) {
			return nil, []diag.Diagnostic{{
				Message: fmt.Sprintf("invalid file name %q", f.Name),
				Kind:    diag.KindParse,
			}}
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
	return load.Instances([]string{"./..."}, cfg), nil
}

// build overlays the client's editable files on the module and returns the root
// project value, the view to render, the names of every discovered view, and the
// inference trace (non-nil only when the view was derived, not declared). eval selects
// the root project instance rather than trusting slice order, and gates only that
// rendered view's concreteness; sibling packages are ignored here (whole-module
// validity is vet's job). When discovery finds no view, inference derives one from the
// module's registries and references; an explicit diagram-shaped field always wins.
func (e *Engine) build(src Source, query string) (cue.Value, cue.Value, []string, []TraceEntry, []diag.Diagnostic, error) {
	instances, diags := e.loadModule(src, query)
	if diags != nil {
		return cue.Value{}, cue.Value{}, nil, nil, diags, nil
	}
	root := rootInstance(instances, src.Dir)
	if root == nil {
		return cue.Value{}, cue.Value{}, nil, nil, nil, errors.New("no CUE instance loaded")
	}
	if err := root.Err; err != nil {
		return cue.Value{}, cue.Value{}, nil, nil, diag.From(err, src.Dir, diag.KindParse), nil
	}

	ctx := cuecontext.New()
	value := ctx.BuildInstance(root)
	if err := value.Err(); err != nil {
		return cue.Value{}, cue.Value{}, nil, nil, diag.From(err, src.Dir, diag.KindSchema), nil
	}

	// A REPL query returns its bound expression as the primary value; the diagram
	// still had to build and resolve for the expression to read it. View discovery
	// and inference are irrelevant to a query.
	if query != "" {
		return value, value.LookupPath(cue.ParsePath("replResult")), nil, nil, nil, nil
	}

	views := e.discoverViews(ctx, value)
	if len(views) > 0 {
		return value, views[selectView(views, src.View)].value, viewNames(views), nil, nil, nil
	}

	// No declared view: derive views from the module's schemas and data. A module with
	// nothing to infer (no registries) stays a valid knowledge-only "no view" outcome.
	inferred, inferDiags := e.inferViews(ctx, value)
	if inferDiags != nil {
		return value, cue.Value{}, nil, nil, inferDiags, nil
	}
	if len(inferred) == 0 {
		return value, cue.Value{}, nil, nil, nil, nil
	}
	sel := selectInferred(inferred, src.View)
	return value, inferred[sel].diagram, inferredNames(inferred), inferred[sel].trace, nil, nil
}

// inferredNames lists the derived view names for the switcher, in the order inferViews
// returns them (model, instances).
func inferredNames(views []inferredView) []string {
	names := make([]string, len(views))
	for i, v := range views {
		names[i] = v.name
	}
	return names
}

// selectInferred picks which derived view to render: the one named want when it exists,
// else the model view (the data model is the default lens), else the first. A stale
// client selection falls back rather than failing the eval.
func selectInferred(views []inferredView, want string) int {
	if want != "" {
		for i, v := range views {
			if v.name == want {
				return i
			}
		}
	}
	for i, v := range views {
		if v.name == viewModel {
			return i
		}
	}
	return 0
}

// view is a discovered diagram-shaped field of the project value.
type view struct {
	name  string
	value cue.Value
}

// discoverViews returns the top-level regular fields of value that render as
// diagrams, sorted by name. A field is a view when it unifies with the bundled
// #Diagram without error (closedness rejects knowledge fields like `people`) and
// carries a `nodes` field (excludes empty structs that vacuously unify). #Diagram
// is rebuilt into value's own context so the unification never crosses contexts.
// Only the rendered (default) view is later concreteness-gated; other views are
// listed, and knowledge fields are neither gated nor rendered.
func (e *Engine) discoverViews(ctx *cue.Context, value cue.Value) []view {
	schemaDiagram := e.schemaDiagram(ctx)
	if !schemaDiagram.Exists() {
		return nil
	}
	iter, err := value.Fields()
	if err != nil {
		return nil
	}
	var views []view
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsString() {
			continue
		}
		fv := iter.Value()
		if schemaDiagram.Unify(fv).Validate() != nil {
			continue
		}
		if !fv.LookupPath(cue.ParsePath("nodes")).Exists() {
			continue
		}
		views = append(views, view{name: sel.Unquoted(), value: fv})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].name < views[j].name })
	return views
}

// schemaDiagram builds the bundled #Diagram definition into ctx, the value view
// discovery unifies candidate fields against and inference validates its projection
// against. A zero value (schema load failure) makes both a no-op. It is rebuilt into
// the caller's context so no unification crosses contexts.
func (e *Engine) schemaDiagram(ctx *cue.Context) cue.Value {
	e.loadSchema()
	if e.schemaInst == nil {
		return cue.Value{}
	}
	return ctx.BuildInstance(e.schemaInst).LookupPath(cue.ParsePath("#Diagram"))
}

// viewNames projects the sorted view names, always non-nil so the eval response
// carries [] rather than null when there is no view.
func viewNames(views []view) []string {
	names := make([]string, len(views))
	for i, v := range views {
		names[i] = v.name
	}
	return names
}

// selectView picks which discovered view to render: the one named want when it
// exists, else the default. An empty or unknown want (e.g. a client selection
// left over from a prior edit that removed the view) falls back to the default
// rather than failing the eval.
func selectView(views []view, want string) int {
	if want != "" {
		for i, v := range views {
			if v.name == want {
				return i
			}
		}
	}
	return defaultView(views)
}

// defaultView picks the view the single-view frontend renders: the one named
// "diagram" when present (backward compatible with the seed), else the first by
// name (views are already name-sorted).
func defaultView(views []view) int {
	for i, v := range views {
		if v.name == "diagram" {
			return i
		}
	}
	return 0
}

// rootInstance returns the module-root package: the loaded instance whose
// directory is the source dir itself. Loading the whole module with "./..." also
// returns subpackages (the diagram schema, user sub-packages), so eval must select
// the project explicitly rather than trust slice order. A nil result means the
// module has no package at its root.
func rootInstance(instances []*build.Instance, dir string) *build.Instance {
	want, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	for _, inst := range instances {
		if filepath.Clean(inst.Dir) == want {
			return inst
		}
	}
	return nil
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
