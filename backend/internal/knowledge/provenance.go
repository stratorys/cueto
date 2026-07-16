package knowledge

import (
	"path/filepath"
	"sort"

	"cuelang.org/go/cue"
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
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Name < result.Entries[j].Name })
	return result, nil
}
