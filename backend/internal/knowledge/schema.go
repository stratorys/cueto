package knowledge

import "cuelang.org/go/cue"

// Projection is a consumer of a compiled CUE value. Projections may provide
// diagrams, catalogs, provenance, or named evaluations without changing the
// generic compiler.
type Projection interface {
	Name() string
	Discover(cue.Value) (any, error)
}

// SchemaProjection is reserved for richer schema introspection. In phase one it
// exposes the same declaration inventory as the catalog while keeping schema
// concerns behind their own named extension point.
type SchemaProjection struct{}

func (SchemaProjection) Name() string { return "schema" }

func (SchemaProjection) Discover(value cue.Value) (any, error) {
	return KnowledgeCatalogProjection{}.Discover(value)
}
