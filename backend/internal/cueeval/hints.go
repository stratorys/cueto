// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package cueeval

import (
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"
)

// Hint is one inlay annotation surfaced to the editor. It renders as ghost text
// (never real source) at a 1-based position in the client's data.cue. A "type"
// hint reports the schema constraint of a field the user wrote; an "optional"
// hint lists a struct's declared-but-unset optional fields.
type Hint struct {
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
}

// Hint kinds, surfaced as the Kind field of a Hint in the API. A "type" hint
// reports a field's schema constraint; an "optional" hint lists a struct's
// declared-but-unset optional fields.
const (
	HintType     = "type"
	HintOptional = "optional"
)

// hintsFrom walks the concrete diagram and joins each written field back to its
// schema definition (#Node, #Column, #Edge) to produce inlay hints. It is called
// only on a fully valid, concrete evaluation, so the walk never sees errors.
// schema is the diagram schema package root that holds the three definitions
// (they live in the imported package, not on the evaluated project value).
//
// The walk is deliberately bounded to the three known definitions rather than
// generic: the schema shape is fixed, and a bounded walk stays predictable.
func hintsFrom(schema, diagram cue.Value) []Hint {
	nodeFields := defFields(schema.LookupPath(cue.ParsePath("#Node")))
	colFields := defFields(schema.LookupPath(cue.ParsePath("#Column")))
	edgeFields := defFields(schema.LookupPath(cue.ParsePath("#Edge")))

	var hints []Hint

	nodes, err := diagram.LookupPath(cue.ParsePath("nodes")).Fields()
	if err == nil {
		for nodes.Next() {
			node := nodes.Value()
			hints = append(hints, structHints(node, nodeFields)...)
			if cols, err := node.LookupPath(cue.ParsePath("columns")).List(); err == nil {
				for cols.Next() {
					hints = append(hints, structHints(cols.Value(), colFields)...)
				}
			}
		}
	}

	if edges, err := diagram.LookupPath(cue.ParsePath("edges")).List(); err == nil {
		for edges.Next() {
			hints = append(hints, structHints(edges.Value(), edgeFields)...)
		}
	}

	return hints
}

// defInfo captures a schema definition's field type labels and which of them are
// optional, indexed by field name.
type defInfo struct {
	types    map[string]string
	optional []string
}

// defFields reads a definition's fields (including optional ones) into a defInfo.
// Returns a zero-value defInfo (no types) when the definition is absent, which
// makes structHints a no-op for that definition.
func defFields(def cue.Value) defInfo {
	info := defInfo{types: map[string]string{}}
	if !def.Exists() {
		return info
	}
	iter, err := def.Fields(cue.Optional(true), cue.Definitions(false))
	if err != nil {
		return info
	}
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsString() {
			continue
		}
		name := sel.Unquoted()
		info.types[name] = typeLabel(iter.Value())
		if iter.IsOptional() {
			info.optional = append(info.optional, name)
		}
	}
	return info
}

// structHints emits a type hint for each field of instance the user wrote in
// data.cue, plus a single "optional" hint (anchored to the struct's own line)
// listing the definition's optional fields the user has not set. Fields injected
// by the schema (e.g. a node's id) carry a non-data.cue position and are skipped.
func structHints(instance cue.Value, def defInfo) []Hint {
	if len(def.types) == 0 {
		return nil
	}

	present := map[string]bool{}
	var hints []Hint
	// The struct value's own Pos() resolves to the schema definition (it unified
	// with it), not data.cue, so it cannot anchor the optional hint. The first written field's
	// data.cue line, minus one, is the struct's opening line in generated CUE
	// (one field per line, brace on the key line).
	firstLine := 0

	iter, err := instance.Fields()
	if err != nil {
		return nil
	}
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsString() {
			continue
		}
		name := sel.Unquoted()
		present[name] = true
		label, ok := def.types[name]
		if !ok {
			continue
		}
		pos, ok := dataPos(iter.Value().Pos())
		if !ok {
			continue
		}
		if firstLine == 0 || pos.Line() < firstLine {
			firstLine = pos.Line()
		}
		hints = append(hints, Hint{Line: pos.Line(), Column: pos.Column(), Label: label, Kind: HintType})
	}

	var missing []string
	for _, name := range def.optional {
		if !present[name] {
			missing = append(missing, name+"?")
		}
	}
	if len(missing) > 0 && firstLine > 1 {
		hints = append(hints, Hint{
			Line:  firstLine - 1,
			Label: strings.Join(missing, ", "),
			Kind:  HintOptional,
		})
	}

	return hints
}

// typeLabel renders a schema field's constraint as compact one-line text, e.g.
// `"entity" | "table"` or `number`. Falls back to the value's kind when the
// constraint cannot be formatted onto a single line.
func typeLabel(v cue.Value) string {
	if node := v.Syntax(cue.Raw()); node != nil {
		if b, err := format.Node(node, format.Simplify()); err == nil {
			s := strings.TrimSpace(string(b))
			if s != "" && !strings.Contains(s, "\n") {
				return s
			}
		}
	}
	return v.IncompleteKind().String()
}

// dataPos returns a position only when it lands in the client's data.cue, so
// hints never point at schema.cue or leak host paths.
func dataPos(pos token.Pos) (token.Pos, bool) {
	if pos.IsValid() && filepath.Base(pos.Filename()) == "data.cue" {
		return pos, true
	}
	return token.Pos{}, false
}
