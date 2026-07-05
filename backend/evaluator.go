package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
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
	// Eval returns the concrete diagram JSON, or input diagnostics when the data
	// is invalid/incomplete. The error is non-nil only for operational failures
	// (timeout, output too large) that are not tied to a source position.
	Eval(ctx context.Context, data string) (json.RawMessage, []Diagnostic, error)
	// Vet validates data against the schema, returning diagnostics (empty when ok).
	Vet(ctx context.Context, data string) ([]Diagnostic, error)
	// Format runs `cue fmt` over arbitrary source text.
	Format(source string) (string, error)
}

// Operational errors, distinct from user-input diagnostics. Handlers map these
// to HTTP status codes; they never carry CUE positions or host paths.
var (
	errTimeout        = errors.New("evaluation timed out")
	errOutputTooLarge = errors.New("evaluation output too large")
)

// cueEvaluator evaluates schema.cue + a per-request data.cue in-process.
//
// Round-trip fidelity: evaluation concretizes references and if-guards and drops
// comments by design. Diagram logic lives in schema.cue, so data.cue is expected
// to be flat concrete data; the graph->CUE regeneration flattens it the same way.
type cueEvaluator struct {
	cueDir         string
	timeout        time.Duration
	maxOutputBytes int
}

func newCueEvaluator(cfg Config) *cueEvaluator {
	return &cueEvaluator{
		cueDir:         cfg.CueDir,
		timeout:        cfg.EvalTimeout,
		maxOutputBytes: cfg.MaxOutputBytes,
	}
}

// Eval implements Evaluator.
func (e *cueEvaluator) Eval(ctx context.Context, data string) (json.RawMessage, []Diagnostic, error) {
	diagram, diags, err := e.evaluate(ctx, data)
	if err != nil || len(diags) > 0 {
		return nil, diags, err
	}
	out, merr := diagram.MarshalJSON()
	if merr != nil {
		return nil, diagnosticsFrom(merr, e.cueDir, kindIncomplete), nil
	}
	if len(out) > e.maxOutputBytes {
		return nil, nil, errOutputTooLarge
	}
	return out, nil, nil
}

// Vet implements Evaluator.
func (e *cueEvaluator) Vet(ctx context.Context, data string) ([]Diagnostic, error) {
	_, diags, err := e.evaluate(ctx, data)
	return diags, err
}

// Format implements Evaluator.
func (e *cueEvaluator) Format(source string) (string, error) {
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

// evaluate runs build + concreteness validation under a deadline. Because a
// runaway CUE unification cannot be force-killed, the worker goroutine is left
// to finish (or leak) on timeout rather than blocking the request; the router's
// concurrency cap bounds how many such goroutines can exist at once. A fresh
// cue.Context per call means a leaked evaluation's memory is reclaimed once it
// finally completes, instead of interning forever on a shared context.
func (e *cueEvaluator) evaluate(ctx context.Context, data string) (cue.Value, []Diagnostic, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan buildResult, 1)
	go func() {
		diagram, diags, err := e.build(data)
		if err == nil && len(diags) == 0 {
			if verr := diagram.Validate(cue.Concrete(true)); verr != nil {
				diags = diagnosticsFrom(verr, e.cueDir, kindIncomplete)
			}
		}
		done <- buildResult{diagram, diags, err}
	}()

	select {
	case <-ctx.Done():
		return cue.Value{}, nil, errTimeout
	case r := <-done:
		return r.diagram, r.diags, r.err
	}
}

type buildResult struct {
	diagram cue.Value
	diags   []Diagnostic
	err     error
}

// build overlays the client's data.cue on the disk schema and returns the
// `diagram` value. schema.cue is always read fresh from disk and is the ONLY
// non-overlaid input; the overlay carries exactly one entry (data.cue), so the
// hand-owned schema can never be supplied or mutated by the client.
func (e *cueEvaluator) build(data string) (cue.Value, []Diagnostic, error) {
	overlay := map[string]load.Source{
		e.dataPath(): load.FromString(data),
	}
	cfg := &load.Config{Dir: e.cueDir, Overlay: overlay}

	instances := load.Instances([]string{"."}, cfg)
	if len(instances) == 0 {
		return cue.Value{}, nil, errors.New("no CUE instance loaded")
	}
	if err := instances[0].Err; err != nil {
		return cue.Value{}, diagnosticsFrom(err, e.cueDir, kindParse), nil
	}

	value := cuecontext.New().BuildInstance(instances[0])
	if err := value.Err(); err != nil {
		return cue.Value{}, diagnosticsFrom(err, e.cueDir, kindSchema), nil
	}

	diagram := value.LookupPath(cue.ParsePath("diagram"))
	if !diagram.Exists() {
		return cue.Value{}, []Diagnostic{{
			Message: "no `diagram` field in data.cue",
			Kind:    kindIncomplete,
		}}, nil
	}
	return diagram, nil, nil
}

// dataPath is the absolute overlay path of the client-supplied data.cue.
func (e *cueEvaluator) dataPath() string {
	return filepath.Join(e.cueDir, "data.cue")
}

// saveDataPath returns the only path the backend is ever permitted to write:
// data.cue inside the schema dir. schema.cue is hand-owned and must never be
// machine-written, so any other target is rejected. No write endpoint exists
// yet; this encodes the invariant so a future /save cannot violate it.
func (e *cueEvaluator) saveDataPath(name string) (string, error) {
	if filepath.Base(name) != "data.cue" {
		return "", fmt.Errorf("refusing to write %q: only data.cue is machine-writable", filepath.Base(name))
	}
	return e.dataPath(), nil
}
