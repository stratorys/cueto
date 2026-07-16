package knowledge

import (
	"sort"

	"cuelang.org/go/cue"
)

// Catalog is the first generic projection: top-level declarations exposed by a
// compiled package. It deliberately contains no diagram concepts.
type Catalog struct {
	Entries []CatalogEntry
}

type CatalogEntry struct {
	Name string
	Kind string
}

// KnowledgeCatalogProjection discovers a stable top-level catalog from any CUE
// value. Rich domain and relation discovery will extend this projection rather
// than changing the compiler contract.
type KnowledgeCatalogProjection struct{}

func (KnowledgeCatalogProjection) Name() string { return "knowledge-catalog" }

func (KnowledgeCatalogProjection) Discover(value cue.Value) (any, error) {
	catalog := Catalog{Entries: []CatalogEntry{}}
	it, err := value.Fields(cue.Optional(true), cue.Definitions(true))
	if err != nil {
		return catalog, nil
	}
	for it.Next() {
		catalog.Entries = append(catalog.Entries, CatalogEntry{
			Name: it.Selector().String(),
			Kind: it.Value().IncompleteKind().String(),
		})
	}
	sort.Slice(catalog.Entries, func(i, j int) bool { return catalog.Entries[i].Name < catalog.Entries[j].Name })
	return catalog, nil
}
