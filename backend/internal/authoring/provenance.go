// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package authoring

import (
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"

	"github.com/stratorys/cueto/backend/internal/domain"
)

// provenanceFrom derives element->file attribution by parsing each file's source
// AST. It deliberately does NOT use the unified cue.Value: in cue v0.17
// Value.Pos() returns an arbitrary conjunct (often the schema.cue pattern
// constraint), so value-based attribution is unreliable. A node id is attributed
// to the first file that declares it under diagram.nodes; the edge list to the
// first file that declares diagram.edges. A file that fails to parse contributes
// nothing here (eval surfaces its syntax error separately).
func provenanceFrom(files []domain.File) domain.Provenance {
	prov := domain.Provenance{Nodes: map[string]string{}}
	for _, f := range files {
		parsed, err := parser.ParseFile(f.Name, f.Content)
		if err != nil {
			continue
		}
		diagramDecls := structDecls(parsed.Decls, "diagram")
		nodesDecls := structDecls(diagramDecls, "nodes")
		for _, id := range fieldNames(nodesDecls) {
			if _, seen := prov.Nodes[id]; !seen {
				prov.Nodes[id] = f.Name
			}
		}
		if prov.Edges == "" && len(fieldsNamed(diagramDecls, "edges")) > 0 {
			prov.Edges = f.Name
		}
	}
	return prov
}

// contributingStructs returns every struct literal an expression contributes,
// unwrapping `A & B` conjunctions and ignoring non-struct operands (e.g. the
// #Diagram reference in `#Diagram & {…}`).
func contributingStructs(expr ast.Expr) []*ast.StructLit {
	switch e := expr.(type) {
	case *ast.StructLit:
		return []*ast.StructLit{e}
	case *ast.BinaryExpr:
		if e.Op == token.AND {
			return append(contributingStructs(e.X), contributingStructs(e.Y)...)
		}
	}
	return nil
}

// fieldsNamed returns the fields labelled name across the given decls.
func fieldsNamed(decls []ast.Decl, name string) []*ast.Field {
	var out []*ast.Field
	for _, d := range decls {
		field, ok := d.(*ast.Field)
		if !ok {
			continue
		}
		if label, _, err := ast.LabelName(field.Label); err == nil && label == name {
			out = append(out, field)
		}
	}
	return out
}

// structDecls returns the child decls under the field name in decls, descending
// into `#X & {…}` conjunctions and the nested structs of path-form fields
// (`diagram: nodes: id: {…}` parses as nested StructLits). Contributions from
// several declarations of the same field are concatenated.
func structDecls(decls []ast.Decl, name string) []ast.Decl {
	var out []ast.Decl
	for _, field := range fieldsNamed(decls, name) {
		for _, lit := range contributingStructs(field.Value) {
			out = append(out, lit.Elts...)
		}
	}
	return out
}

// fieldNames returns the label of every field declaration in decls, in order.
func fieldNames(decls []ast.Decl) []string {
	var out []string
	for _, d := range decls {
		if field, ok := d.(*ast.Field); ok {
			if label, _, err := ast.LabelName(field.Label); err == nil {
				out = append(out, label)
			}
		}
	}
	return out
}
