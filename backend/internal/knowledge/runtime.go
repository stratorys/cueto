// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// Query is the safe, data-oriented agent query model. It never accepts CUE
// source: records are exported and filtered in Go against the catalog.
type Query struct {
	Domain string      `json:"domain"`
	Select []string    `json:"select"`
	Where  []Predicate `json:"where,omitempty"`
	Limit  int         `json:"limit,omitempty"`
	Expand []Expansion `json:"expand,omitempty"`
}

type Predicate struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

// Expansion is reserved for bounded relation expansion. The initial safe query
// implementation rejects it rather than silently issuing unbounded graph walks.
type Expansion struct {
	Field  string   `json:"field"`
	Select []string `json:"select,omitempty"`
	Limit  int      `json:"limit,omitempty"`
}

type QueryResult struct {
	Result json.RawMessage `json:"result"`
	Count  int             `json:"count"`
}

// DomainDescription gives agents a stable description without exposing the
// entire compiled module.
type DomainDescription struct {
	Domain
	Members []string
}

// EvalRequest selects one optional named evaluation. Inputs are declared by the
// CUE contract in phase two; parameter binding is deliberately deferred until
// the contract defines input substitution semantics.
type EvalRequest struct {
	Evaluation string          `json:"evaluation"`
	Input      json.RawMessage `json:"input"`
}

type EvalResult struct {
	Status     string          `json:"status"`
	Result     json.RawMessage `json:"result"`
	Revision   string          `json:"revision"`
	Evaluation string          `json:"evaluation"`
}

type ProvenanceResult struct {
	Name         string
	Provenance   Provenance
	Observations []Observation `json:"observations"`
}

// Runtime is the transport-independent knowledge service shared by CLI, HTTP,
// MCP, and visual adapters. Its methods intentionally express knowledge tasks,
// not CUE loader or HTTP concerns.
type Runtime interface {
	Catalog(context.Context, ProjectRef) (Catalog, error)
	Describe(context.Context, ProjectRef, string) (DomainDescription, error)
	Get(context.Context, ProjectRef, string, string) (json.RawMessage, error)
	Query(context.Context, ProjectRef, Query) (QueryResult, error)
	Eval(context.Context, ProjectRef, EvalRequest) (EvalResult, error)
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
	collection, ok := domainCollection(compiled, domain)
	if !ok {
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
	if query.Domain == "" {
		return QueryResult{}, fmt.Errorf("query domain is required")
	}
	if len(query.Expand) > 0 {
		return QueryResult{}, fmt.Errorf("relation expansion is not available in the initial safe query API")
	}
	compiled, err := r.compile(ctx, project)
	if err != nil {
		return QueryResult{}, err
	}
	var domain Domain
	found := false
	for _, candidate := range compiled.Catalog.Domains {
		if candidate.Name == query.Domain {
			domain, found = candidate, true
			break
		}
	}
	if !found {
		return QueryResult{}, fmt.Errorf("unknown domain %q", query.Domain)
	}
	if err := validateQuery(domain, query); err != nil {
		return QueryResult{}, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 0 || limit > 1000 {
		return QueryResult{}, fmt.Errorf("query limit must be between 1 and 1000")
	}
	collection, ok := domainCollection(compiled, query.Domain)
	if !ok {
		return QueryResult{}, fmt.Errorf("unknown domain %q", query.Domain)
	}
	raw, diagnostics, err := r.compiler.encode(collection, compileRequest(project))
	if err != nil {
		return QueryResult{}, err
	}
	if len(diagnostics) > 0 {
		return QueryResult{}, &DiagnosticError{Diagnostics: diagnostics}
	}
	var records map[string]map[string]any
	if err := json.Unmarshal(raw, &records); err != nil {
		return QueryResult{}, fmt.Errorf("domain %q is not a record collection", query.Domain)
	}
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]map[string]any, 0, min(limit, len(keys)))
	for _, key := range keys {
		record := records[key]
		if !matches(record, query.Where) {
			continue
		}
		selected := map[string]any{"id": key}
		if len(query.Select) == 0 {
			for field, value := range record {
				selected[field] = value
			}
		} else {
			for _, field := range query.Select {
				if field != "id" {
					selected[field] = record[field]
				}
			}
		}
		result = append(result, selected)
		if len(result) == limit {
			break
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return QueryResult{}, err
	}
	return QueryResult{Result: encoded, Count: len(result)}, nil
}

// Repl retains trusted arbitrary CUE evaluation for developer tooling. It is not
// part of Runtime, so MCP and other agent transports default to Query instead.
func (r *CueRuntime) Repl(ctx context.Context, project ProjectRef, expression string) (json.RawMessage, []Diagnostic, error) {
	return r.compiler.Query(ctx, compileRequest(project), expression)
}

func domainCollection(compiled *CompiledKnowledge, name string) (cue.Value, bool) {
	collection := compiled.Value.LookupPath(cue.MakePath(cue.Str(name)))
	for _, candidate := range compiled.Catalog.Domains {
		if candidate.Name == name && candidate.Explicit && candidate.Collection.Exists() {
			collection = candidate.Collection
			break
		}
	}
	return collection, collection.Exists()
}

func validateQuery(domain Domain, query Query) error {
	known := func(field string) bool { _, ok := domain.Fields[field]; return ok }
	for _, field := range query.Select {
		if field != "id" && !known(field) {
			return fmt.Errorf("unknown field %q for domain %q", field, domain.Name)
		}
	}
	for _, predicate := range query.Where {
		if !known(predicate.Field) {
			return fmt.Errorf("unknown field %q for domain %q", predicate.Field, domain.Name)
		}
		switch predicate.Operator {
		case "eq", "neq", "in", "exists", "gt", "gte", "lt", "lte":
		default:
			return fmt.Errorf("unsupported query operator %q", predicate.Operator)
		}
		if predicate.Operator == "in" {
			if _, ok := predicate.Value.([]any); !ok {
				return fmt.Errorf("operator in requires an array value")
			}
		}
	}
	return nil
}

func matches(record map[string]any, predicates []Predicate) bool {
	for _, predicate := range predicates {
		value, exists := record[predicate.Field]
		switch predicate.Operator {
		case "exists":
			if !exists || value == nil {
				return false
			}
		case "eq":
			if !exists || !reflect.DeepEqual(value, predicate.Value) {
				return false
			}
		case "neq":
			if exists && reflect.DeepEqual(value, predicate.Value) {
				return false
			}
		case "in":
			values, _ := predicate.Value.([]any)
			found := false
			for _, candidate := range values {
				if reflect.DeepEqual(value, candidate) {
					found = true
					break
				}
			}
			if !exists || !found {
				return false
			}
		case "gt", "gte", "lt", "lte":
			cmp, ok := compare(value, predicate.Value)
			if !ok {
				return false
			}
			if (predicate.Operator == "gt" && cmp <= 0) || (predicate.Operator == "gte" && cmp < 0) || (predicate.Operator == "lt" && cmp >= 0) || (predicate.Operator == "lte" && cmp > 0) {
				return false
			}
		}
	}
	return true
}

func compare(left, right any) (int, bool) {
	if l, ok := left.(float64); ok {
		r, ok := right.(float64)
		if !ok {
			return 0, false
		}
		if l < r {
			return -1, true
		}
		if l > r {
			return 1, true
		}
		return 0, true
	}
	if l, ok := left.(string); ok {
		r, ok := right.(string)
		if !ok {
			return 0, false
		}
		if l < r {
			return -1, true
		}
		if l > r {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func (r *CueRuntime) Eval(ctx context.Context, project ProjectRef, request EvalRequest) (EvalResult, error) {
	compiled, err := r.compile(ctx, project)
	if err != nil {
		return EvalResult{}, err
	}
	for _, evaluation := range compiled.Catalog.Evaluations {
		if evaluation.Name != request.Evaluation {
			continue
		}
		if len(request.Input) == 0 || !json.Valid(request.Input) {
			return EvalResult{}, fmt.Errorf("evaluation input must be valid JSON")
		}
		withInput := projectWithEvaluationInput(project, evaluation.Path, evaluation.Name, request.Input)
		executed, err := r.compiler.Compile(ctx, compileRequest(withInput))
		if err != nil {
			return EvalResult{}, err
		}
		if len(executed.Diagnostics) > 0 {
			return EvalResult{}, &DiagnosticError{Diagnostics: executed.Diagnostics}
		}
		value := evaluationValue(executed.Value, evaluation.Path, evaluation.Name, "result")
		if !value.Exists() {
			// Phase-two compatibility: output remains readable until callers migrate.
			value = evaluationValue(executed.Value, evaluation.Path, evaluation.Name, "output")
		}
		result, diagnostics, err := r.compiler.encode(value, compileRequest(withInput))
		if err != nil {
			return EvalResult{}, err
		}
		if len(diagnostics) > 0 {
			return EvalResult{}, &DiagnosticError{Diagnostics: diagnostics}
		}
		return EvalResult{Status: "success", Result: result, Revision: executed.Revision, Evaluation: evaluation.Name}, nil
	}
	return EvalResult{}, fmt.Errorf("unknown evaluation %q", request.Evaluation)
}

func projectWithEvaluationInput(project ProjectRef, path, name string, input json.RawMessage) ProjectRef {
	overlay := make(map[string][]byte, len(project.Overlay)+1)
	for name, content := range project.Overlay {
		overlay[name] = content
	}
	label, _ := json.Marshal(name)
	prefix := "evaluations"
	if path == "knowledge.evaluations" {
		prefix = "knowledge: evaluations"
	}
	overlay["cueto_evaluation_input.cue"] = []byte(fmt.Sprintf("package main\n\n%s: {%s: {input: %s}}\n", prefix, label, input))
	project.Overlay = overlay
	return project
}

func evaluationValue(root cue.Value, path, name, field string) cue.Value {
	selectors := []cue.Selector{cue.Str("evaluations"), cue.Str(name), cue.Str(field)}
	if path == "knowledge.evaluations" {
		selectors = append([]cue.Selector{cue.Str("knowledge")}, selectors...)
	}
	return root.LookupPath(cue.MakePath(selectors...))
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
		return ProvenanceResult{Provenance: result, Observations: compiled.Catalog.Observations}, nil
	}
	filtered := Provenance{Entries: []ProvenanceEntry{}}
	for _, entry := range result.Entries {
		if entry.Name == name {
			filtered.Entries = append(filtered.Entries, entry)
		}
	}
	semantic := make([]Observation, 0)
	for _, observation := range compiled.Catalog.Observations {
		if observation.Name == name || observation.Entity == name {
			semantic = append(semantic, observation)
		}
	}
	if len(filtered.Entries) == 0 && len(semantic) == 0 {
		return ProvenanceResult{}, fmt.Errorf("no provenance for %q", name)
	}
	return ProvenanceResult{Name: name, Provenance: filtered, Observations: semantic}, nil
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
		if filepath.IsAbs(project.Package) {
			return Provenance{}, fmt.Errorf("package %q escapes module root", project.Package)
		}
		packageDir = filepath.Join(root, project.Package)
		rel, err := filepath.Rel(root, packageDir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return Provenance{}, fmt.Errorf("package %q escapes module root", project.Package)
		}
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
