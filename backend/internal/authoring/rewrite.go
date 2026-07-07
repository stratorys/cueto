// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package authoring

import (
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"

	"github.com/stratorys/cueto/backend/internal/diag"
)

// RewriteOp describes an in-place edit of one editable file's diagram content.
// Node and edge bodies are CUE source text (the frontend emits them), so keys
// stay unquoted and idiomatic; the backend only splices them into the right
// place, preserving all other hand-written content and comments in the file.
type RewriteOp struct {
	Name    string            `json:"name"`
	Content string            `json:"content"`
	Nodes   map[string]string `json:"nodes"`   // id -> CUE struct text, upserted under diagram.nodes
	Deletes []string          `json:"deletes"` // node ids to remove from diagram.nodes
	Edges   *string           `json:"edges"`   // when non-nil, CUE list text replacing diagram.edges
}

// rewriteFile applies op to its source and returns the reprinted file. It parses
// the source AST (never the evaluated value, which drops comments and if-guards),
// splices only the requested diagram.nodes fields and the edge list, and reprints
// with gofmt-style formatting. A syntax error in the source or in a supplied body
// comes back as diagnostics; nothing is written.
func rewriteFile(op RewriteOp) (string, []diag.Diagnostic, error) {
	file, err := parser.ParseFile(op.Name, op.Content, parser.ParseComments)
	if err != nil {
		return "", diag.From(err, "", diag.KindParse), nil
	}

	nodes := ensureField(&file.Decls, "diagram", "nodes")
	for id, body := range op.Nodes {
		value, perr := parser.ParseExpr(id, body)
		if perr != nil {
			return "", diag.From(perr, "", diag.KindParse), nil
		}
		upsertField(&nodes.Elts, id, value)
	}
	for _, id := range op.Deletes {
		deleteField(&nodes.Elts, id)
	}
	if op.Edges != nil {
		value, perr := parser.ParseExpr("edges", *op.Edges)
		if perr != nil {
			return "", diag.From(perr, "", diag.KindParse), nil
		}
		diagram := ensureField(&file.Decls, "diagram")
		upsertField(&diagram.Elts, "edges", value)
	}

	out, err := format.Node(file, format.Simplify())
	if err != nil {
		return "", nil, err
	}
	return string(out), nil, nil
}

// ensureField walks a field path from the file's top level, returning the struct
// literal at the end of the path and creating any missing links as `name: {}`.
// It descends `#X & {…}` conjunctions and path-form structs so a node lands in
// the same place regardless of how the file was authored.
func ensureField(decls *[]ast.Decl, path ...string) *ast.StructLit {
	current := declsHandle{decls: decls}
	var out *ast.StructLit
	for _, name := range path {
		out = current.ensureStruct(name)
		current = declsHandle{struct_: out}
	}
	return out
}

// declsHandle is a mutable reference to a declaration list: either the file's
// top-level Decls or a struct literal's Elts. Both need the same field ops but
// live behind different slices.
type declsHandle struct {
	decls   *[]ast.Decl
	struct_ *ast.StructLit
}

func (h declsHandle) elts() *[]ast.Decl {
	if h.struct_ != nil {
		return &h.struct_.Elts
	}
	return h.decls
}

// ensureStruct returns the struct literal that field `name` should edit into,
// creating the field (as `name: {}`) when it is absent and unwrapping a
// `ref & {…}` conjunction to the writable struct operand.
func (h declsHandle) ensureStruct(name string) *ast.StructLit {
	elts := h.elts()
	if field := findField(*elts, name); field != nil {
		return editableStruct(field)
	}
	lit := &ast.StructLit{}
	*elts = append(*elts, &ast.Field{Label: ast.NewIdent(name), Value: lit})
	return lit
}

// editableStruct returns a struct literal that edits into field's value. For a
// plain struct it is the value itself; for `#Diagram & {…}` it is the struct
// operand; for anything else (a lone reference) it wraps the value in
// `value & {…}` and returns the new struct.
func editableStruct(field *ast.Field) *ast.StructLit {
	if lit := firstStruct(field.Value); lit != nil {
		return lit
	}
	lit := &ast.StructLit{}
	field.Value = &ast.BinaryExpr{X: field.Value, Op: token.AND, Y: lit}
	return lit
}

// firstStruct returns the first struct literal an expression contributes,
// descending `A & B` conjunctions; nil when there is none.
func firstStruct(expr ast.Expr) *ast.StructLit {
	switch e := expr.(type) {
	case *ast.StructLit:
		return e
	case *ast.BinaryExpr:
		if e.Op == token.AND {
			if lit := firstStruct(e.X); lit != nil {
				return lit
			}
			return firstStruct(e.Y)
		}
	}
	return nil
}

// findField returns the first field labelled name in decls, or nil.
func findField(decls []ast.Decl, name string) *ast.Field {
	for _, d := range decls {
		if field, ok := d.(*ast.Field); ok {
			if label, _, err := ast.LabelName(field.Label); err == nil && label == name {
				return field
			}
		}
	}
	return nil
}

// upsertField replaces the value of an existing field named name (keeping the
// field's own comments) or appends `name: value` when it is absent.
func upsertField(elts *[]ast.Decl, name string, value ast.Expr) {
	if field := findField(*elts, name); field != nil {
		field.Value = value
		return
	}
	*elts = append(*elts, &ast.Field{Label: ast.NewIdent(name), Value: value})
}

// deleteField removes every field labelled name from elts.
func deleteField(elts *[]ast.Decl, name string) {
	kept := (*elts)[:0]
	for _, d := range *elts {
		if field, ok := d.(*ast.Field); ok {
			if label, _, err := ast.LabelName(field.Label); err == nil && label == name {
				continue
			}
		}
		kept = append(kept, d)
	}
	*elts = kept
}
