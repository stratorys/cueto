// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package evaluation

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stratorys/cueto/backend/internal/diag"
)

// membraneEngine builds an engine pointed at the real cueto diagram schema, the same
// one the server and CLI use, so the acceptance fixture exercises the full path.
func membraneEngine(t *testing.T) *Engine {
	t.Helper()
	cueDir, err := filepath.Abs("../../../cue")
	if err != nil {
		t.Fatal(err)
	}
	return New(cueDir, 30*time.Second, 64<<20)
}

func membraneSource(t *testing.T, variant string) Source {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "membrane", variant))
	if err != nil {
		t.Fatal(err)
	}
	return Source{Dir: dir}
}

// TestMembraneValid is the end-to-end acceptance case: a user module of hand-authored
// schema and data, no diagram field, passes both gates and infers a diagram.
func TestMembraneValid(t *testing.T) {
	e := membraneEngine(t)
	src := membraneSource(t, "valid")

	vetDiags, err := e.Vet(context.Background(), src)
	if err != nil {
		t.Fatalf("vet: %v", err)
	}
	if len(vetDiags) > 0 {
		t.Fatalf("vet should be clean, got %+v", vetDiags)
	}

	checkDiags, err := e.Check(context.Background(), src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(checkDiags) > 0 {
		t.Fatalf("check should be clean, got %+v", checkDiags)
	}

	out, views, _, trace, _, evalDiags, err := e.Eval(context.Background(), src)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if len(evalDiags) > 0 {
		t.Fatalf("eval should be clean, got %+v", evalDiags)
	}
	if len(out) == 0 || len(views) == 0 || len(trace) == 0 {
		t.Fatalf("eval should infer a diagram: out=%d views=%v trace=%d", len(out), views, len(trace))
	}
}

// TestMembraneBroken proves Layer 1: removing an owner from the roster breaks the
// typed reference, so vet (cue vet) rejects the module. Every file exists here, so
// this fixture isolates the compiler-decided failure.
func TestMembraneBroken(t *testing.T) {
	e := membraneEngine(t)
	src := membraneSource(t, "broken")

	vetDiags, err := e.Vet(context.Background(), src)
	if err != nil {
		t.Fatalf("vet: %v", err)
	}
	if len(vetDiags) == 0 {
		t.Fatal("vet should be red on the dangling owner reference")
	}

	// Check must not report a false "all clear" on a module that does not evaluate: it
	// cannot walk a bottom package, so it says so rather than passing.
	checkDiags, err := e.Check(context.Background(), src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(checkDiags) == 0 {
		t.Fatal("check should signal it could not verify an un-evaluable package")
	}
}

// TestMembraneMissingFile proves Layer 2: the CUE is fully valid (vet clean) but a
// readme names a file that is not on disk, which only the graph check can catch. vet
// runs before check in CI, so check inspecting an already-evaluable module is the
// intended composition.
func TestMembraneMissingFile(t *testing.T) {
	e := membraneEngine(t)
	src := membraneSource(t, "missing-file")

	vetDiags, err := e.Vet(context.Background(), src)
	if err != nil {
		t.Fatalf("vet: %v", err)
	}
	if len(vetDiags) > 0 {
		t.Fatalf("vet should be clean (the break is world-facing, not CUE), got %+v", vetDiags)
	}

	checkDiags, err := e.Check(context.Background(), src)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(checkDiags) == 0 {
		t.Fatal("check should be red on the missing readme file")
	}
	for _, d := range checkDiags {
		if d.Kind != diag.KindReference {
			t.Errorf("diagnostic kind = %q, want %q", d.Kind, diag.KindReference)
		}
	}
}
