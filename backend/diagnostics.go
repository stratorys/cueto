// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"path/filepath"
	"strings"

	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/token"
)

// Diagnostic is one structured, position-remapped error surfaced to the editor.
// Line and Column are 1-based positions in the client's data.cue text, or 0 when
// the error carries no position. Rule/NodeID/EdgeID anchor policy and drift
// findings to a graph element when CUE gives no text position.
type Diagnostic struct {
	Message string `json:"message"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Kind    string `json:"kind"`
	Rule    string `json:"rule,omitempty"`
	NodeID  string `json:"nodeId,omitempty"`
	EdgeID  string `json:"edgeId,omitempty"`
}

// Error kinds. They mean different things to the editor: a parse error is a
// syntax typo, a schema error is a contract violation, incomplete means a value
// is missing or non-concrete, internal is an operational failure. policy and
// drift are governance findings (not source errors); import is an infra-file
// parse failure.
const (
	kindParse      = "parse"
	kindSchema     = "schema"
	kindIncomplete = "incomplete"
	kindInternal   = "internal"
	kindPolicy     = "policy"
	kindDrift      = "drift"
	kindImport     = "import"
)

// diagnosticsFrom converts a CUE error tree into structured diagnostics. It
// reports every error (not just the first), remaps positions in the overlaid
// data.cue to relative line/column, and strips absolute server paths so nothing
// about the host filesystem leaks to the client.
//
// kind classifies the whole batch by where the error arose (the call site knows
// whether it is parsing, unifying, or checking concreteness); this is more
// reliable than guessing from the error's position.
func diagnosticsFrom(err error, cueDir, kind string) []Diagnostic {
	if err == nil {
		return nil
	}
	list := cueerrors.Errors(err)
	if len(list) == 0 {
		return []Diagnostic{{Message: scrub(err.Error(), cueDir), Kind: kind}}
	}

	out := make([]Diagnostic, 0, len(list))
	for _, e := range list {
		d := Diagnostic{Kind: kind, Message: scrub(e.Error(), cueDir)}
		if pos, ok := bestPosition(e); ok {
			d.Line = pos.Line()
			d.Column = pos.Column()
		}
		out = append(out, d)
	}
	return out
}

// bestPosition picks the most editor-useful position for an error. Conflict
// errors often expose their location only via InputPositions, so both are
// considered; a position inside the client's data.cue always wins so the editor
// underlines the text the user can actually change.
func bestPosition(e cueerrors.Error) (token.Pos, bool) {
	candidates := append([]token.Pos{e.Position()}, e.InputPositions()...)
	var fallback token.Pos
	haveFallback := false
	for _, p := range candidates {
		if !p.IsValid() {
			continue
		}
		if filepath.Base(p.Filename()) == "data.cue" {
			return p, true
		}
		if !haveFallback {
			fallback, haveFallback = p, true
		}
	}
	return fallback, haveFallback
}

// scrub removes the absolute schema directory prefix from a message so error
// text never reveals host paths.
func scrub(msg, cueDir string) string {
	return strings.ReplaceAll(msg, cueDir+string(filepath.Separator), "")
}
