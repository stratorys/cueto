package knowledge

import (
	"path/filepath"
	"sort"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
)

// Provenance maps top-level declarations to their CUE source. It is generic;
// diagram element provenance remains an adapter concern until it is generalized.
type Provenance struct {
	Entries []ProvenanceEntry
}

type ProvenanceEntry struct {
	Name string
	File string
	Line int
}

type ProvenanceProjection struct{}

func (ProvenanceProjection) Name() string { return "provenance" }

func (ProvenanceProjection) Discover(value cue.Value) (any, error) {
	result := Provenance{Entries: []ProvenanceEntry{}}
	it, err := value.Fields(cue.Optional(true), cue.Definitions(true))
	if err != nil {
		return result, nil
	}
	for it.Next() {
		node := it.Value().Source()
		if node == nil || !node.Pos().IsValid() {
			continue
		}
		pos := node.Pos()
		result.Entries = append(result.Entries, ProvenanceEntry{Name: it.Selector().String(), File: filepath.Base(pos.Filename()), Line: pos.Line()})
	}
	// A unified value may not retain a single conjunct as Value.Source (for
	// example a registry combines its pattern and concrete members). Its syntax
	// still retains the root field positions, which gives the generic runtime a
	// useful declaration-level fallback without diagram-specific AST parsing.
	if len(result.Entries) == 0 {
		if root, ok := value.Syntax(cue.Raw()).(*ast.StructLit); ok {
			for _, decl := range root.Elts {
				field, ok := decl.(*ast.Field)
				if !ok || !field.Pos().IsValid() {
					continue
				}
				name, _, err := ast.LabelName(field.Label)
				if err != nil {
					continue
				}
				pos := field.Pos()
				result.Entries = append(result.Entries, ProvenanceEntry{Name: name, File: filepath.Base(pos.Filename()), Line: pos.Line()})
			}
		}
	}
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Name < result.Entries[j].Name })
	return result, nil
}
