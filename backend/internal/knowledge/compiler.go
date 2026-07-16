package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"

	"github.com/stratorys/cueto/backend/internal/domain"
	"github.com/stratorys/cueto/backend/internal/evaluation"
)

// CueCompiler adapts the existing bounded evaluator into the generic compiler
// contract. It never reimplements CUE loading, overlay validation, diagnostics,
// source scrubbing, deadlines, or panic recovery.
type CueCompiler struct {
	engine *evaluation.Engine
}

func New(engine *evaluation.Engine) *CueCompiler { return &CueCompiler{engine: engine} }

var _ Compiler = (*CueCompiler)(nil)

func (c *CueCompiler) Compile(ctx context.Context, request CompileRequest) (*CompiledKnowledge, error) {
	src := sourceFrom(request)
	value, diagnostics, err := c.engine.CompileValue(ctx, src)
	if err != nil {
		return nil, err
	}

	result := &CompiledKnowledge{
		Revision:    revisionFor(value, request),
		Value:       value,
		Diagnostics: diagnostics,
		Health:      Health{Valid: len(diagnostics) == 0, Diagnostics: diagnostics},
	}
	if len(diagnostics) > 0 {
		return result, nil
	}

	// Whole-module health deliberately remains the evaluator's Vet operation:
	// this preserves its sibling-package coverage and diagnostic de-duplication.
	healthDiagnostics, err := c.engine.Vet(ctx, src)
	if err != nil {
		return nil, err
	}
	result.Health = Health{Valid: len(healthDiagnostics) == 0, Diagnostics: healthDiagnostics}

	catalog, err := (KnowledgeCatalogProjection{}).Discover(value)
	if err != nil {
		return nil, err
	}
	result.Catalog = catalog.(Catalog)
	DiscoverExplicitKnowledge(value, &result.Catalog)
	addImplicitDomains(&result.Catalog, evaluation.DiscoverRegistries(value))
	return result, nil
}

// addImplicitDomains preserves Cueto's vocabulary-free discovery mode. An
// explicit domain wins when it has the same name, letting metadata enrich or
// intentionally replace an inferred registry without duplicate API entries.
func addImplicitDomains(catalog *Catalog, registries []evaluation.RegistryInfo) {
	explicit := make(map[string]bool, len(catalog.Domains))
	for _, domain := range catalog.Domains {
		if domain.Explicit {
			explicit[domain.Name] = true
		}
	}
	for _, registry := range registries {
		if explicit[registry.Name] {
			continue
		}
		catalog.Domains = append(catalog.Domains, Domain{Name: registry.Name, Key: "id"})
	}
	sort.Slice(catalog.Domains, func(i, j int) bool { return catalog.Domains[i].Name < catalog.Domains[j].Name })
}

func sourceFrom(request CompileRequest) evaluation.Source {
	names := make([]string, 0, len(request.Overlay))
	for name := range request.Overlay {
		names = append(names, name)
	}
	sort.Strings(names)
	overlay := make([]domain.File, 0, len(names))
	for _, name := range names {
		overlay = append(overlay, domain.File{Name: name, Content: string(request.Overlay[name])})
	}
	return evaluation.Source{Dir: request.ModuleDir, Package: request.Package, Overlay: overlay}
}

// revisionFor prefers the normalized compiled syntax, so a change in an on-disk
// module changes the revision even when no overlay is present. The request hash
// remains a safe fallback for malformed inputs that have no buildable value.
func revisionFor(value cue.Value, request CompileRequest) string {
	formatted, err := format.Node(value.Syntax(cue.Final()))
	if err != nil {
		return revision(request)
	}
	h := sha256.New()
	h.Write([]byte(request.ModuleDir))
	h.Write([]byte{0})
	h.Write([]byte(request.Package))
	h.Write([]byte{0})
	h.Write(formatted)
	return hex.EncodeToString(h.Sum(nil))
}

// revision is a deterministic fallback identity for a compile request that did
// not produce a CUE value. A workspace or git adapter may later replace this
// with a commit hash.
func revision(request CompileRequest) string {
	h := sha256.New()
	h.Write([]byte(request.ModuleDir))
	h.Write([]byte{0})
	h.Write([]byte(request.Package))
	names := make([]string, 0, len(request.Overlay))
	for name := range request.Overlay {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h.Write([]byte{0})
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(request.Overlay[name])
	}
	return hex.EncodeToString(h.Sum(nil))
}
