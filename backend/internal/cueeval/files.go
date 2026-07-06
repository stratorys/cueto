// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package cueeval

import (
	"path/filepath"
	"regexp"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

// File is one client-supplied editable CUE file: a bare filename (guarded by
// validEditableName) and its full source text. Multiple files unify into one
// `package diagram`, so nodes may be authored across several files.
type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Provenance attributes each diagram element to the editable file that authored
// it, so a canvas edit can be written back into the right file. Nodes maps a
// node id to its filename; Edges names the single file that owns the edge list
// (edges are a CUE list and cannot be split across files by unification).
type Provenance struct {
	Nodes map[string]string `json:"nodes"`
	Edges string            `json:"edges"`
}

// editableNamePattern is the strict shape of a client filename: bare word plus
// a .cue suffix, no other dots, no separators.
var editableNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+\.cue$`)

// validEditableName reports whether name is a safe client-supplied CUE filename.
// It must be a bare base name (no path separators or traversal), match the strict
// pattern, and not be the reserved hand-owned schema.cue. The schema check is
// case-insensitive because macOS/APFS is case-insensitive by default, so
// Schema.cue would collide with the on-disk file. This guard is what lets the
// N-file overlay accept client filenames without a client supplying, replacing,
// or escaping past the hand-owned schema.
func validEditableName(name string) bool {
	if name != filepath.Base(name) {
		return false
	}
	if !editableNamePattern.MatchString(name) {
		return false
	}
	if strings.EqualFold(name, "schema.cue") {
		return false
	}
	return true
}

// provenanceFrom derives element->file attribution by parsing each file's source
// AST. It deliberately does NOT use the unified cue.Value: in cue v0.17
// Value.Pos() returns an arbitrary conjunct (often the schema.cue pattern
// constraint), so value-based attribution is unreliable. A node id is attributed
// to the first file that declares it under diagram.nodes; the edge list to the
// first file that declares diagram.edges. A file that fails to parse contributes
// nothing here (eval surfaces its syntax error separately).
func provenanceFrom(files []File) Provenance {
	prov := Provenance{Nodes: map[string]string{}}
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
