// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package cueeval

import (
	"context"
	"regexp"

	"cuelang.org/go/cue"

	"github.com/stratorys/cueto/backend/internal/diag"
)

// A CUE bare identifier. Non-identifier keys are addressed with quoted/index
// syntax the REPL does not insert, so paths through them are skipped.
var identRE = regexp.MustCompile(`^[A-Za-z_]\w*$`)

// keyDepthMax bounds recursion so a deep (or cyclic-looking) unified value cannot
// blow the stack; keyCountMax bounds the total so a large diagram cannot produce
// an unbounded completion list.
const (
	keyDepthMax = 8
	keyCountMax = 5000
)

// Keys implements Evaluator. It builds the editable file set overlaid on the
// schema and returns the dotted, identifier-only field paths of every top-level
// data field (people, people.george, diagram, diagram.nodes, ...), so the REPL
// autocompletes the whole data, not just the diagram. Definitions, hidden fields,
// and non-identifier keys are skipped (completion inserts bare dotted references);
// lists carry their own path but are not descended. It reads the value's
// structure, not a concrete result, so an incomplete field still contributes its
// path. Diagnostics from an invalid/incomplete diagram are surfaced the same way a
// query would; the overlay is thrown away, so nothing is persisted.
func (e *cueEvaluator) Keys(ctx context.Context, files []File) ([]string, []diag.Diagnostic, error) {
	root, _, diags, err := e.evaluate(ctx, files, "")
	if err != nil || len(diags) > 0 {
		return nil, diags, err
	}
	keys := make([]string, 0, 64)
	walkKeys(root, "", 0, &keys)
	return keys, nil, nil
}

// walkKeys appends the dotted identifier paths of v's regular fields to out under
// prefix (empty at the root), descending structs up to keyDepthMax. Definitions,
// hidden, optional, and non-identifier fields are skipped; lists are not descended,
// matching the `a.b.c` references the REPL can insert.
func walkKeys(v cue.Value, prefix string, depth int, out *[]string) {
	if depth >= keyDepthMax || len(*out) >= keyCountMax || v.Kind() != cue.StructKind {
		return
	}
	iter, err := v.Fields()
	if err != nil {
		return
	}
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsString() {
			continue
		}
		name := sel.Unquoted()
		if !identRE.MatchString(name) {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		*out = append(*out, path)
		if len(*out) >= keyCountMax {
			return
		}
		walkKeys(iter.Value(), path, depth+1, out)
	}
}
