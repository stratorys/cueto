// Package knowledge defines Cueto's diagram-independent compiled knowledge
// model. It is intentionally small in phase one: callers receive the compiled
// CUE value plus projections that can evolve without making diagrams the core
// product contract.
package knowledge

import (
	"context"

	"cuelang.org/go/cue"
)

// CompileRequest selects one package in a CUE module and overlays unsaved CUE
// files. Overlay keys are module-relative filenames; validation is delegated to
// the established evaluation loader so all adapters enforce the same guard.
type CompileRequest struct {
	ModuleDir string
	Package   string
	Overlay   map[string][]byte
}

// CompiledKnowledge is a generic CUE compilation result. Value remains a CUE
// value deliberately: projections consume the typed, unified graph rather than
// lossy JSON. Catalog, health, and diagnostics are transport-safe summaries.
type CompiledKnowledge struct {
	Revision    string
	Value       cue.Value
	Catalog     Catalog
	Diagnostics []Diagnostic
	Health      Health
}

// Compiler is the stable entry point shared by CLI, HTTP, MCP, and diagram
// adapters. Operational failures use error; source failures are diagnostics.
type Compiler interface {
	Compile(ctx context.Context, request CompileRequest) (*CompiledKnowledge, error)
}
