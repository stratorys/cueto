// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package knowledge

import (
	"sort"

	"cuelang.org/go/cue"

	"github.com/stratorys/cueto/backend/internal/evaluation"
)

// Catalog is the first generic projection: top-level declarations exposed by a
// compiled package. It deliberately contains no diagram concepts.
type Catalog struct {
	Entries     []CatalogEntry    `json:"entries"`
	Metadata    *Metadata         `json:"metadata,omitempty"`
	Domains     []Domain          `json:"domains"`
	Evaluations []NamedEvaluation `json:"evaluations"`
	Checks      []Check           `json:"checks"`
}

type CatalogEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Metadata is optional module-level context supplied through knowledge.metadata.
type Metadata struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Revision    string `json:"revision,omitempty"`
}

// Domain is either explicitly declared under knowledge.domains or inferred from
// Cueto's existing open-label registry shape. Collection retains the typed CUE
// value for downstream projections without forcing a lossy JSON representation.
type Domain struct {
	Name        string           `json:"name"`
	Path        string           `json:"path"`
	Kind        string           `json:"kind"`
	Description string           `json:"description,omitempty"`
	Key         string           `json:"key"`
	KeyType     string           `json:"keyType"`
	Explicit    bool             `json:"explicit"`
	Fields      map[string]Field `json:"fields"`
	Collection  cue.Value        `json:"-"`
}

type Field struct {
	Type     string    `json:"type"`
	Required bool      `json:"required"`
	Relation *Relation `json:"relation,omitempty"`
}

type Relation struct {
	Domain      string `json:"domain"`
	Cardinality string `json:"cardinality"`
	Rule        string `json:"rule,omitempty"`
}

// NamedEvaluation describes an optional, declared agent-facing operation.
// Input and Output stay typed CUE values for an MCP/HTTP adapter to marshal.
type NamedEvaluation struct {
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	InputSchema  Schema    `json:"inputSchema"`
	OutputSchema Schema    `json:"outputSchema"`
	Input        cue.Value `json:"-"`
	Output       cue.Value `json:"-"`
}

type Schema struct {
	Type   string           `json:"type"`
	Fields map[string]Field `json:"fields,omitempty"`
}

type Check struct {
	Name  string `json:"name"`
	Value bool   `json:"value"`
}

// BuildCatalog is the schema-catalog compiler pass. It overlays optional
// explicit metadata on the same registry/relation discovery used by diagrams.
func BuildCatalog(root cue.Value) (Catalog, error) {
	projected, err := (KnowledgeCatalogProjection{}).Discover(root)
	if err != nil {
		return Catalog{}, err
	}
	catalog := projected.(Catalog)
	DiscoverExplicitKnowledge(root, &catalog)
	explicit := map[string]int{}
	for i, domain := range catalog.Domains {
		if domain.Explicit {
			explicit[domain.Name] = i
		}
	}
	for _, registry := range evaluation.DescribeRegistries(root) {
		domain := domainFromRegistry(registry)
		if i, ok := explicit[registry.Name]; ok {
			domain.Description = catalog.Domains[i].Description
			domain.Key = catalog.Domains[i].Key
			domain.Explicit = true
			domain.Collection = catalog.Domains[i].Collection
			catalog.Domains[i] = domain
			continue
		}
		catalog.Domains = append(catalog.Domains, domain)
	}
	sort.Slice(catalog.Domains, func(i, j int) bool { return catalog.Domains[i].Name < catalog.Domains[j].Name })
	return catalog, nil
}

func domainFromRegistry(registry evaluation.RegistrySchemaInfo) Domain {
	fields := make(map[string]Field, len(registry.Fields))
	for _, field := range registry.Fields {
		var relation *Relation
		if field.Relation != nil {
			relation = &Relation{Domain: field.Relation.Domain, Cardinality: field.Relation.Cardinality, Rule: field.Relation.Rule}
		}
		fields[field.Name] = Field{Type: field.Type, Required: field.Required, Relation: relation}
	}
	return Domain{Name: registry.Name, Path: registry.Name, Kind: "registry", Key: "id", KeyType: "string", Fields: fields}
}

// KnowledgeCatalogProjection discovers a stable top-level catalog from any CUE
// value. Rich domain and relation discovery will extend this projection rather
// than changing the compiler contract.
type KnowledgeCatalogProjection struct{}

func (KnowledgeCatalogProjection) Name() string { return "knowledge-catalog" }

func (KnowledgeCatalogProjection) Discover(value cue.Value) (any, error) {
	catalog := Catalog{Entries: []CatalogEntry{}, Domains: []Domain{}, Evaluations: []NamedEvaluation{}, Checks: []Check{}}
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

// DiscoverExplicitKnowledge reads the optional dedicated metadata field. No
// import is required to discover it: modules that unify it with cueto/knowledge
// receive schema validation, while plain CUE modules remain discoverable.
func DiscoverExplicitKnowledge(value cue.Value, catalog *Catalog) {
	knowledge := value.LookupPath(cue.ParsePath("knowledge"))
	if !knowledge.Exists() {
		return
	}
	if metadata := knowledge.LookupPath(cue.ParsePath("metadata")); metadata.Exists() {
		catalog.Metadata = &Metadata{
			Title:       concreteString(metadata.LookupPath(cue.ParsePath("title"))),
			Description: concreteString(metadata.LookupPath(cue.ParsePath("description"))),
			Revision:    concreteString(metadata.LookupPath(cue.ParsePath("revision"))),
		}
	}
	if domains := knowledge.LookupPath(cue.ParsePath("domains")); domains.Exists() {
		it, err := domains.Fields()
		if err == nil {
			for it.Next() {
				entry := it.Value()
				key := concreteString(entry.LookupPath(cue.ParsePath("key")))
				if key == "" {
					key = "id"
				}
				catalog.Domains = append(catalog.Domains, Domain{
					Name:        it.Selector().Unquoted(),
					Path:        it.Selector().Unquoted(),
					Kind:        "declared",
					Description: concreteString(entry.LookupPath(cue.ParsePath("description"))),
					Key:         key,
					KeyType:     "string",
					Fields:      map[string]Field{},
					Explicit:    true,
					Collection:  entry.LookupPath(cue.ParsePath("collection")),
				})
			}
		}
	}
	if evaluations := knowledge.LookupPath(cue.ParsePath("evaluations")); evaluations.Exists() {
		it, err := evaluations.Fields()
		if err == nil {
			for it.Next() {
				entry := it.Value()
				catalog.Evaluations = append(catalog.Evaluations, NamedEvaluation{
					Name:         it.Selector().Unquoted(),
					Description:  concreteString(entry.LookupPath(cue.ParsePath("description"))),
					InputSchema:  schemaFor(entry.LookupPath(cue.ParsePath("input"))),
					OutputSchema: schemaFor(entry.LookupPath(cue.ParsePath("output"))),
					Input:        entry.LookupPath(cue.ParsePath("input")),
					Output:       entry.LookupPath(cue.ParsePath("output")),
				})
			}
		}
	}
	if checks := knowledge.LookupPath(cue.ParsePath("checks")); checks.Exists() {
		it, err := checks.Fields()
		if err == nil {
			for it.Next() {
				v, err := it.Value().Bool()
				if err == nil {
					catalog.Checks = append(catalog.Checks, Check{Name: it.Selector().Unquoted(), Value: v})
				}
			}
		}
	}
	sort.Slice(catalog.Domains, func(i, j int) bool { return catalog.Domains[i].Name < catalog.Domains[j].Name })
	sort.Slice(catalog.Evaluations, func(i, j int) bool { return catalog.Evaluations[i].Name < catalog.Evaluations[j].Name })
	sort.Slice(catalog.Checks, func(i, j int) bool { return catalog.Checks[i].Name < catalog.Checks[j].Name })
}

func concreteString(value cue.Value) string {
	result, err := value.String()
	if err != nil {
		return ""
	}
	return result
}

func schemaFor(value cue.Value) Schema {
	schema := Schema{Type: typeFor(value)}
	if value.IncompleteKind()&cue.StructKind == 0 {
		return schema
	}
	schema.Fields = map[string]Field{}
	it, err := value.Fields(cue.Optional(true))
	if err != nil {
		return schema
	}
	for it.Next() {
		if it.Selector().IsString() {
			schema.Fields[it.Selector().Unquoted()] = Field{Type: typeFor(it.Value()), Required: !it.IsOptional()}
		}
	}
	return schema
}

func typeFor(value cue.Value) string {
	kind := value.IncompleteKind()
	if kind&cue.ListKind != 0 {
		element := value.LookupPath(cue.MakePath(cue.AnyIndex))
		if element.Exists() {
			return "list<" + typeFor(element) + ">"
		}
		return "list<value>"
	}
	switch {
	case kind&cue.StringKind != 0:
		return "string"
	case kind&cue.IntKind != 0:
		return "int"
	case kind&(cue.FloatKind|cue.NumberKind) != 0:
		return "number"
	case kind&cue.BoolKind != 0:
		return "bool"
	case kind&cue.StructKind != 0:
		return "struct"
	default:
		return "value"
	}
}
