package knowledge

import (
	"sort"

	"cuelang.org/go/cue"
)

// Catalog is the first generic projection: top-level declarations exposed by a
// compiled package. It deliberately contains no diagram concepts.
type Catalog struct {
	Entries     []CatalogEntry
	Metadata    *Metadata
	Domains     []Domain
	Evaluations []NamedEvaluation
	Checks      []Check
}

type CatalogEntry struct {
	Name string
	Kind string
}

// Metadata is optional module-level context supplied through knowledge.metadata.
type Metadata struct {
	Title       string
	Description string
	Revision    string
}

// Domain is either explicitly declared under knowledge.domains or inferred from
// Cueto's existing open-label registry shape. Collection retains the typed CUE
// value for downstream projections without forcing a lossy JSON representation.
type Domain struct {
	Name        string
	Description string
	Key         string
	Explicit    bool
	Collection  cue.Value
}

// NamedEvaluation describes an optional, declared agent-facing operation.
// Input and Output stay typed CUE values for an MCP/HTTP adapter to marshal.
type NamedEvaluation struct {
	Name        string
	Description string
	Input       cue.Value
	Output      cue.Value
}

type Check struct {
	Name  string
	Value bool
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
					Description: concreteString(entry.LookupPath(cue.ParsePath("description"))),
					Key:         key,
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
					Name:        it.Selector().Unquoted(),
					Description: concreteString(entry.LookupPath(cue.ParsePath("description"))),
					Input:       entry.LookupPath(cue.ParsePath("input")),
					Output:      entry.LookupPath(cue.ParsePath("output")),
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
