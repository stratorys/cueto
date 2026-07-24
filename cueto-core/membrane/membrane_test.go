package membrane

import (
	"path/filepath"
	"strings"
	"testing"

	cueerrors "cuelang.org/go/cue/errors"
)

const fixtures = "../testdata/membrane"

func TestLoadValid(t *testing.T) {
	if _, err := Load(filepath.Join(fixtures, "valid")); err != nil {
		t.Fatalf("valid membrane should load, got %v", err)
	}
}

// TestLoadBrokenReturnsNativeError: a broken membrane (owner names a person removed
// from crew) fails evaluation and returns a native CUE error, not a membrane wrapper.
func TestLoadBrokenReturnsNativeError(t *testing.T) {
	h, err := Load(filepath.Join(fixtures, "broken"))
	if err == nil {
		t.Fatalf("broken membrane should fail to load")
	}
	if h != nil {
		t.Fatalf("broken membrane should yield no handle")
	}
	if len(cueerrors.Errors(err)) == 0 {
		t.Fatalf("error should be a native CUE error list, got %T: %v", err, err)
	}
}

// TestLoadMissingFileIsCleanCUE: the missing-file fixture is valid CUE (the absent file
// is only a graph-check concern), so it loads without error.
func TestLoadMissingFileIsCleanCUE(t *testing.T) {
	if _, err := Load(filepath.Join(fixtures, "missing-file")); err != nil {
		t.Fatalf("missing-file membrane is valid CUE and should load, got %v", err)
	}
}

func TestLookupResolvesScalar(t *testing.T) {
	h := mustLoadValid(t)
	v, err := h.Lookup("people.alice.name")
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	got, err := v.String()
	if err != nil {
		t.Fatalf("value is not a string: %v", err)
	}
	if got != "Alice" {
		t.Fatalf("expected Alice, got %q", got)
	}
}

func TestLookupMissingPath(t *testing.T) {
	h := mustLoadValid(t)
	if _, err := h.Lookup("people.nobody"); err == nil {
		t.Fatalf("lookup of a missing path should error")
	}
}

// TestAttributesReadsFileAttribute: the readme field carries @file(); Attributes must
// enumerate it.
func TestAttributesReadsFileAttribute(t *testing.T) {
	h := mustLoadValid(t)
	attrs, err := h.Attributes("components.backend.readme")
	if err != nil {
		t.Fatalf("attributes lookup failed: %v", err)
	}
	found := false
	for _, a := range attrs {
		if a.Name() == "file" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a @file attribute, got %d attributes", len(attrs))
	}
}

func TestDefinedAtReturnsOrigin(t *testing.T) {
	h := mustLoadValid(t)
	origin, err := h.DefinedAt("people.alice.name")
	if err != nil {
		t.Fatalf("DefinedAt failed: %v", err)
	}
	if origin.Line <= 0 {
		t.Fatalf("expected a positive line, got %d", origin.Line)
	}
	if !strings.HasSuffix(origin.File, ".cue") {
		t.Fatalf("expected a .cue source file, got %q", origin.File)
	}
}

func mustLoadValid(t *testing.T) *Handle {
	t.Helper()
	h, err := Load(filepath.Join(fixtures, "valid"))
	if err != nil {
		t.Fatalf("valid membrane should load, got %v", err)
	}
	return h
}
