package knowledge

import "cuelang.org/go/cue"

// DiagramProjection is the compatibility projection for explicitly authored
// diagrams. It is intentionally separate from compilation: the compiler never
// requires a module to import Cueto's diagram schema. The legacy inferred-view
// projection can be moved here in the next phase once its discovery rules are
// expressed against the generic catalog.
type DiagramProjection struct{}

func (DiagramProjection) Name() string { return "diagram" }

func (DiagramProjection) Discover(value cue.Value) (any, error) {
	// An authored diagram remains a normal CUE declaration. Returning the value
	// preserves its types and provenance for a later JSON/UI adapter.
	return value.LookupPath(cue.ParsePath("diagram")), nil
}
