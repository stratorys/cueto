package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"

	"github.com/stratorys/cueto/backend/internal/evaluation"
)

// ProjectRef is the transport-neutral identity of knowledge being addressed.
// An HTTP adapter may derive ModuleDir from a project id; CLI and MCP can pass a
// module directly. Overlays preserve the editor's unsaved-buffer semantics.
type ProjectRef struct {
	ModuleDir string
	Package   string
	Overlay   map[string][]byte
}

// Query is an ad-hoc CUE expression evaluated against a compiled module.
type Query struct {
	Expression string
}

type QueryResult struct {
	Result      json.RawMessage
	Diagnostics []Diagnostic
}

// DomainDescription gives agents a stable description without exposing the
// entire compiled module.
type DomainDescription struct {
	Domain
	Members []string
}

// EvaluationRequest selects one optional named evaluation. Inputs are declared
// by the CUE contract in phase two; parameter binding is deliberately deferred
// until the contract defines input substitution semantics.
type EvaluationRequest struct {
	Name string
}

type EvaluationResult struct {
	Name        string
	Description string
	Input       json.RawMessage
	Output      json.RawMessage
}

type ProvenanceResult struct {
	Name       string
	Provenance Provenance
}

// Runtime is the transport-independent knowledge service shared by CLI, HTTP,
// MCP, and visual adapters. Its methods intentionally express knowledge tasks,
// not CUE loader or HTTP concerns.
type Runtime interface {
	Catalog(context.Context, ProjectRef) (Catalog, error)
	Describe(context.Context, ProjectRef, string) (DomainDescription, error)
	Get(context.Context, ProjectRef, string, string) (json.RawMessage, error)
	Query(context.Context, ProjectRef, Query) (QueryResult, error)
	Evaluate(context.Context, ProjectRef, EvaluationRequest) (EvaluationResult, error)
	Provenance(context.Context, ProjectRef, string) (ProvenanceResult, error)
	Health(context.Context, ProjectRef) (Health, error)
}

// CueRuntime is the first Runtime implementation. It delegates all CUE work to
// CueCompiler, preserving its evaluator limits, overlay guard, and diagnostics.
type CueRuntime struct {
	compiler *CueCompiler
}

func NewRuntime(compiler *CueCompiler) *CueRuntime { return &CueRuntime{compiler: compiler} }

var _ Runtime = (*CueRuntime)(nil)

func (r *CueRuntime) Catalog(ctx context.Context, project ProjectRef) (Catalog, error) {
	compiled, err := r.compile(ctx, project)
	if err != nil {
		return Catalog{}, err
	}
	return compiled.Catalog, nil
}

func (r *CueRuntime) Describe(ctx context.Context, project ProjectRef, name string) (DomainDescription, error) {
	compiled, err := r.compile(ctx, project)
	if err != nil {
		return DomainDescription{}, err
	}
	for _, domain := range compiled.Catalog.Domains {
		if domain.Name != name {
			continue
		}
		result := DomainDescription{Domain: domain}
		for _, entry := range compiled.Catalog.Entries {
			if entry.Name == name {
				// Membership is intentionally supplied only for structural registries;
				// explicit collections can be arbitrary CUE values.
				for _, registry := range implicitRegistries(compiled.Value) {
					if registry.Name == name {
						result.Members = registry.Members
					}
				}
				break
			}
		}
		return result, nil
	}
	return DomainDescription{}, fmt.Errorf("unknown domain %q", name)
}

func (r *CueRuntime) Get(ctx context.Context, project ProjectRef, domain, key string) (json.RawMessage, error) {
	compiled, err := r.compile(ctx, project)
	if err != nil {
		return nil, err
	}
	collection := compiled.Value.LookupPath(cue.MakePath(cue.Str(domain)))
	for _, candidate := range compiled.Catalog.Domains {
		if candidate.Name == domain && candidate.Explicit && candidate.Collection.Exists() {
			collection = candidate.Collection
			break
		}
	}
	if !collection.Exists() {
		return nil, fmt.Errorf("unknown domain %q", domain)
	}
	value := collection.LookupPath(cue.MakePath(cue.Str(key)))
	if !value.Exists() {
		return nil, fmt.Errorf("unknown %s entry %q", domain, key)
	}
	result, diagnostics, err := r.compiler.encode(value, compileRequest(project))
	if err != nil {
		return nil, err
	}
	if len(diagnostics) > 0 {
		return nil, &DiagnosticError{Diagnostics: diagnostics}
	}
	return result, nil
}

func (r *CueRuntime) Query(ctx context.Context, project ProjectRef, query Query) (QueryResult, error) {
	result, diagnostics, err := r.compiler.Query(ctx, compileRequest(project), query.Expression)
	return QueryResult{Result: result, Diagnostics: diagnostics}, err
}

func (r *CueRuntime) Evaluate(ctx context.Context, project ProjectRef, request EvaluationRequest) (EvaluationResult, error) {
	compiled, err := r.compile(ctx, project)
	if err != nil {
		return EvaluationResult{}, err
	}
	for _, evaluation := range compiled.Catalog.Evaluations {
		if evaluation.Name != request.Name {
			continue
		}
		input, diagnostics, err := r.compiler.encode(evaluation.Input, compileRequest(project))
		if err != nil {
			return EvaluationResult{}, err
		}
		if len(diagnostics) > 0 {
			return EvaluationResult{}, &DiagnosticError{Diagnostics: diagnostics}
		}
		output, diagnostics, err := r.compiler.encode(evaluation.Output, compileRequest(project))
		if err != nil {
			return EvaluationResult{}, err
		}
		if len(diagnostics) > 0 {
			return EvaluationResult{}, &DiagnosticError{Diagnostics: diagnostics}
		}
		return EvaluationResult{Name: evaluation.Name, Description: evaluation.Description, Input: input, Output: output}, nil
	}
	return EvaluationResult{}, fmt.Errorf("unknown evaluation %q", request.Name)
}

func (r *CueRuntime) Provenance(ctx context.Context, project ProjectRef, name string) (ProvenanceResult, error) {
	compiled, err := r.compile(ctx, project)
	if err != nil {
		return ProvenanceResult{}, err
	}
	provenance, err := sourceProvenance(project)
	if err != nil {
		return ProvenanceResult{}, err
	}
	result := provenance
	if len(result.Entries) == 0 {
		// Retain the value-based projection as a fallback for future runtimes
		// that compile from a non-filesystem source.
		projection, err := (ProvenanceProjection{}).Discover(compiled.Value)
		if err != nil {
			return ProvenanceResult{}, err
		}
		result = projection.(Provenance)
	}
	if name == "" {
		return ProvenanceResult{Provenance: result}, nil
	}
	filtered := Provenance{Entries: []ProvenanceEntry{}}
	for _, entry := range result.Entries {
		if entry.Name == name {
			filtered.Entries = append(filtered.Entries, entry)
		}
	}
	if len(filtered.Entries) == 0 {
		return ProvenanceResult{}, fmt.Errorf("no provenance for %q", name)
	}
	return ProvenanceResult{Name: name, Provenance: filtered}, nil
}

// sourceProvenance is declaration-level provenance from the prepared module
// source. Unified CUE values do not reliably retain one source conjunct, so the
// runtime intentionally parses source here, just as diagram authoring already
// does. Editor overlays replace on-disk files before parsing.
func sourceProvenance(project ProjectRef) (Provenance, error) {
	files := map[string][]byte{}
	root, err := filepath.Abs(project.ModuleDir)
	if err != nil {
		return Provenance{}, err
	}
	packageDir := root
	if project.Package != "" && project.Package != "." {
		packageDir = filepath.Join(root, project.Package)
	}
	err = filepath.WalkDir(packageDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "cue.mod" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".cue") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = contents
		return nil
	})
	if err != nil {
		return Provenance{}, err
	}
	for name, content := range project.Overlay {
		files[name] = content
	}

	result := Provenance{Entries: []ProvenanceEntry{}}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		file, err := parser.ParseFile(name, files[name])
		if err != nil {
			continue // compilation owns syntax diagnostics
		}
		for _, decl := range file.Decls {
			field, ok := decl.(*ast.Field)
			if !ok {
				continue
			}
			label, _, err := ast.LabelName(field.Label)
			if err != nil {
				continue
			}
			result.Entries = append(result.Entries, ProvenanceEntry{Name: label, File: name, Line: field.Pos().Line()})
		}
	}
	return result, nil
}

func (r *CueRuntime) Health(ctx context.Context, project ProjectRef) (Health, error) {
	compiled, err := r.compiler.Compile(ctx, compileRequest(project))
	if err != nil {
		return Health{}, err
	}
	return compiled.Health, nil
}

func (r *CueRuntime) compile(ctx context.Context, project ProjectRef) (*CompiledKnowledge, error) {
	compiled, err := r.compiler.Compile(ctx, compileRequest(project))
	if err != nil {
		return nil, err
	}
	if len(compiled.Diagnostics) > 0 {
		return nil, &DiagnosticError{Diagnostics: compiled.Diagnostics}
	}
	return compiled, nil
}

func compileRequest(project ProjectRef) CompileRequest {
	return CompileRequest{ModuleDir: project.ModuleDir, Package: project.Package, Overlay: project.Overlay}
}

func implicitRegistries(value cue.Value) []evaluation.RegistryInfo {
	return evaluation.DiscoverRegistries(value)
}

// DiagnosticError preserves source diagnostics through Runtime's error-returning
// methods. HTTP and MCP adapters can inspect it with errors.As.
type DiagnosticError struct {
	Diagnostics []Diagnostic
}

func (e *DiagnosticError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "knowledge diagnostics"
	}
	return e.Diagnostics[0].Message
}
