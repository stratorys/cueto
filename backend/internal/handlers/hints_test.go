// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stratorys/cueto/backend/internal/cueeval"
)

// evalHints runs /eval and returns the hints from a successful response.
func evalHints(t *testing.T, data string) []cueeval.Hint {
	t.Helper()
	router := realRouter(t, testConfig(t))
	rec := postJSON(router, "/eval", evalBody(t, data))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var out struct {
		Hints []cueeval.Hint `json:"hints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.Hints
}

func TestHintsTypeAndOptional(t *testing.T) {
	hints := evalHints(t, validData)
	if len(hints) == 0 {
		t.Fatal("want hints for valid data, got none")
	}

	var haveType, haveNodeOptional bool
	for _, h := range hints {
		if h.Line <= 0 {
			t.Fatalf("hint without a source line: %+v", h)
		}
		switch h.Kind {
		case cueeval.HintType:
			haveType = true
			// The type field's declared constraint must surface as the enum, not a
			// collapsed concrete value.
			if strings.Contains(h.Label, "process") && !strings.Contains(h.Label, "|") {
				t.Fatalf("type hint = %q, want the enum disjunction", h.Label)
			}
		case cueeval.HintOptional:
			// validData sets no optional node fields, so at least one struct must
			// offer width? (edge structs list their own optionals instead).
			if strings.Contains(h.Label, "width?") {
				haveNodeOptional = true
			}
		default:
			t.Fatalf("unexpected hint kind %q", h.Kind)
		}
	}
	if !haveType {
		t.Fatal("want at least one type hint")
	}
	if !haveNodeOptional {
		t.Fatal("want a node optional hint offering width?")
	}
}

// Hints must never surface a schema-injected field (e.g. a node's id, which
// carries a schema.cue position) or leak the host schema path.
func TestHintsSkipInjectedFields(t *testing.T) {
	cfg := testConfig(t)
	for _, h := range evalHints(t, validData) {
		if strings.Contains(h.Label, cfg.CueDir) {
			t.Fatalf("hint leaks host path: %q", h.Label)
		}
	}
}

// A field that violates the schema evaluates to diagnostics, not hints.
func TestHintsAbsentOnError(t *testing.T) {
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
	var out struct {
		Hints []cueeval.Hint `json:"hints"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Hints) != 0 {
		t.Fatalf("want no hints on error, got %+v", out.Hints)
	}
}
