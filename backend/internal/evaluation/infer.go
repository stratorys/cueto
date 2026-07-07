// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package evaluation

import (
	"fmt"
	"sort"

	"cuelang.org/go/cue"

	"github.com/stratorys/cueto/backend/internal/diag"
)

// Inference derives an entity-relation diagram from a module that writes only schemas
// and data, with no diagram authoring. It reads the membrane idiom the user already
// wrote for integrity's sake: a registry (a struct with open labels) is a set of
// nodes, and a field constrained to a registry's key set is a relation. Detection is
// by shape only - cueto learns no domain vocabulary. See
// .claude/docs/inference-fixtures.md for the detection and projection contract.

// inferNodeMax and inferEdgeMax bound a projection so an unexpectedly large module
// cannot produce an unbounded diagram; the evaluation output byte cap is the second
// net. Exceeding either returns a clear diagnostic rather than a truncated diagram.
const (
	inferNodeMax = 2000
	inferEdgeMax = 5000
)

// foreignProbe is a string almost certainly outside any registry's key set. A
// reference field rejects it (a disjunction of the keys), a plain string field
// accepts it; that difference is how detector two tells a key-set reference from an
// ordinary string without extracting literals.
const foreignProbe = "\x00__cueto_not_a_registry_key__\x00"

// nameFields are the member fields, in priority order, a node label is read from
// before falling back to the member key. This is shape, not vocabulary: any of the
// three, whatever the user's domain calls its entities.
var nameFields = []string{"name", "title", "label"}

// TraceEntry records which detection rule produced one inferred element, so a "why is
// this here" inspector can explain an inferred node or edge. It is carried in the eval
// response alongside the diagram.
type TraceEntry struct {
	Element string `json:"element"`
	Kind    string `json:"kind"`   // "node" | "edge"
	Rule    string `json:"rule"`   // "registry" | "key-set-ref" | "attr-ref"
	Detail  string `json:"detail"` // registry field, or "source.field -> target"
}

// registry is a detected open-label struct: its field name, its member schema (the
// pattern constraint, reached via the AnyString selector), and its concrete members
// keyed by label. keys is the sorted member key set.
type registry struct {
	field   string
	schema  cue.Value
	members map[string]cue.Value
	keys    []string
}

// inferDiagram detects registries and references in the built project value and
// projects them into a #Diagram-shaped value, validated against the bundled schema
// before returning. The returned value is concrete and ready to marshal; the trace
// has one entry per emitted element. A nil value with nil diagnostics means the module
// has nothing to infer (no registries), a valid empty outcome the caller renders as no
// view. Diagnostics are returned for a bounds breach or a projection that fails schema
// validation (a projection bug, never silently drawn).
func (e *Engine) inferDiagram(ctx *cue.Context, project cue.Value) (cue.Value, []TraceEntry, []diag.Diagnostic) {
	registries := detectRegistries(project)
	if len(registries) == 0 {
		return cue.Value{}, nil, nil
	}

	nodes, nodeTrace := projectNodes(registries)
	if len(nodes) > inferNodeMax {
		return cue.Value{}, nil, boundDiag("nodes", len(nodes), inferNodeMax)
	}
	edges, edgeTrace := projectEdges(ctx, registries)
	if len(edges) > inferEdgeMax {
		return cue.Value{}, nil, boundDiag("edges", len(edges), inferEdgeMax)
	}

	diagram, diags := e.buildDiagram(ctx, nodes, edges)
	if diags != nil {
		return cue.Value{}, nil, diags
	}
	return diagram, append(nodeTrace, edgeTrace...), nil
}

// detectRegistries returns the top-level regular fields of project whose labels are
// open (a `[ID=string]: #Thing` pattern), each with its member schema and concrete
// members. Registries are returned sorted by field name so projection is deterministic.
// A struct with concrete labels only (a closed record) is not a registry.
func detectRegistries(project cue.Value) []registry {
	iter, err := project.Fields()
	if err != nil {
		return nil
	}
	var out []registry
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsString() {
			continue
		}
		fv := iter.Value()
		if fv.IncompleteKind() != cue.StructKind || !fv.Allows(cue.AnyString) {
			continue
		}
		// A registry has a pattern constraint (`[ID=string]: #Thing`), so its member
		// schema is a struct. A plain open record has no pattern - its any-label lookup
		// is top, not a struct - so it is not a registry even though structs are open by
		// default in CUE. Comparing to StructKind exactly (not a bitmask test) excludes
		// top, whose kind mask includes the struct bit.
		schema := fv.LookupPath(cue.MakePath(cue.AnyString))
		if !schema.Exists() || schema.IncompleteKind() != cue.StructKind {
			continue
		}
		reg := registry{
			field:   sel.Unquoted(),
			schema:  schema,
			members: map[string]cue.Value{},
		}
		collectMembers(fv, &reg)
		out = append(out, reg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].field < out[j].field })
	return out
}

// collectMembers fills reg.members and reg.keys with the registry's concrete member
// labels. Non-string labels are skipped (node ids and references are string keys).
func collectMembers(fv cue.Value, reg *registry) {
	iter, err := fv.Fields()
	if err != nil {
		return
	}
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsString() {
			continue
		}
		key := sel.Unquoted()
		reg.members[key] = iter.Value()
		reg.keys = append(reg.keys, key)
	}
	sort.Strings(reg.keys)
}

// nodeID and edgeID are the deterministic id conventions (see the fixture spec). The
// registry-field prefix keeps node ids unique when two registries share a member key.
func nodeID(regField, key string) string { return regField + "/" + key }
func edgeID(source, field, target string) string {
	return fmt.Sprintf("%s--%s-->%s", source, field, target)
}

// projectedNode is one node before encoding: its id, label, kind (registry field), and
// its remaining concrete scalar fields for the data card.
type projectedNode struct {
	id    string
	label string
	data  map[string]interface{}
}

// projectNodes turns every registry member into a node and returns one trace entry per
// node. The label is the first present name-like field, else the member key; remaining
// concrete scalar fields (not the label source, not references) form the data card.
func projectNodes(registries []registry) ([]projectedNode, []TraceEntry) {
	var nodes []projectedNode
	var trace []TraceEntry
	for _, reg := range registries {
		for _, key := range reg.keys {
			member := reg.members[key]
			id := nodeID(reg.field, key)
			label, labelField := memberLabel(member, key)
			nodes = append(nodes, projectedNode{
				id:    id,
				label: label,
				data:  scalarData(member, labelField),
			})
			trace = append(trace, TraceEntry{
				Element: id, Kind: "node", Rule: "registry", Detail: reg.field,
			})
		}
	}
	return nodes, trace
}

// memberLabel returns the node label and the field it came from (empty when it fell
// back to the key). It reads the first name-like field that holds a concrete string.
func memberLabel(member cue.Value, key string) (string, string) {
	for _, f := range nameFields {
		fv := member.LookupPath(cue.ParsePath(f))
		if s, err := fv.String(); err == nil && s != "" {
			return s, f
		}
	}
	return key, ""
}

// scalarData collects a member's remaining concrete scalar fields (string, int, float,
// bool) into the data card, skipping the label source, structs, and lists. A field that
// is a reference is a scalar string too and is kept in the data card as well; the edge
// it produces is separate.
func scalarData(member cue.Value, labelField string) map[string]interface{} {
	iter, err := member.Fields()
	if err != nil {
		return nil
	}
	data := map[string]interface{}{}
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsString() {
			continue
		}
		name := sel.Unquoted()
		if name == labelField {
			continue
		}
		fv := iter.Value()
		if !isScalar(fv.IncompleteKind()) || fv.Validate(cue.Concrete(true)) != nil {
			continue
		}
		var decoded interface{}
		if fv.Decode(&decoded) != nil {
			continue
		}
		data[name] = decoded
	}
	if len(data) == 0 {
		return nil
	}
	return data
}

// isScalar reports whether k is a single scalar kind (not a struct, list, or the
// wildcard). References and plain data both live here.
func isScalar(k cue.Kind) bool {
	switch k {
	case cue.StringKind, cue.IntKind, cue.FloatKind, cue.NumberKind, cue.BoolKind:
		return true
	default:
		return false
	}
}

// projectEdges detects reference fields on each registry's member schema and emits an
// edge for every concrete reference a member declares to an existing target member.
// Edges are returned sorted (source, field, target) for determinism, with one trace
// entry each.
func projectEdges(ctx *cue.Context, registries []registry) ([]projectedEdge, []TraceEntry) {
	byField := make(map[string]*registry, len(registries))
	for i := range registries {
		byField[registries[i].field] = &registries[i]
	}
	var edges []projectedEdge
	for _, reg := range registries {
		refs := detectReferences(ctx, reg, registries)
		for _, key := range reg.keys {
			member := reg.members[key]
			source := nodeID(reg.field, key)
			for _, ref := range refs {
				target := byField[ref.targetField]
				for _, tkey := range referencedKeys(member, ref) {
					if _, ok := target.members[tkey]; !ok {
						continue
					}
					tid := nodeID(ref.targetField, tkey)
					edges = append(edges, projectedEdge{
						id: edgeID(source, ref.field, tid), source: source, target: tid, label: ref.field,
					})
				}
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].source != edges[j].source {
			return edges[i].source < edges[j].source
		}
		if edges[i].label != edges[j].label {
			return edges[i].label < edges[j].label
		}
		return edges[i].target < edges[j].target
	})
	trace := make([]TraceEntry, len(edges))
	for i, e := range edges {
		trace[i] = TraceEntry{
			Element: e.id, Kind: "edge", Rule: e.rule,
			Detail: fmt.Sprintf("%s.%s -> %s", e.source, e.label, e.target),
		}
	}
	return edges, trace
}

// projectedEdge is one edge before encoding. rule records which detector produced it
// (key-set idiom or explicit attribute), for the trace.
type projectedEdge struct {
	id     string
	source string
	target string
	label  string
	rule   string
}

// reference is a member-schema field detected as a relation to a registry: the field
// name, the target registry field, whether the field is a list of references, and the
// rule that detected it.
type reference struct {
	field       string
	targetField string
	list        bool
	rule        string
}

// detectReferences walks reg's member schema and returns each field that is a reference
// to some registry: by the key-set idiom (a string or list-of-strings field whose
// constraint accepts exactly a registry's key set), or by an explicit @ref(field)
// attribute. The key-set idiom wins when both are present on one field. Fields are
// returned sorted by name.
func detectReferences(ctx *cue.Context, reg registry, registries []registry) []reference {
	if !reg.schema.Exists() {
		return nil
	}
	iter, err := reg.schema.Fields(cue.Optional(true))
	if err != nil {
		return nil
	}
	var refs []reference
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsString() {
			continue
		}
		field := sel.Unquoted()
		fv := iter.Value()
		if ref, ok := keySetReference(ctx, field, fv, registries); ok {
			refs = append(refs, ref)
			continue
		}
		if ref, ok := attrReference(field, fv, registries); ok {
			refs = append(refs, ref)
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].field < refs[j].field })
	return refs
}

// keySetReference reports whether field fv is a key-set reference. A string field is
// matched directly; a list field is matched on its element constraint (one edge per
// element). The target is the first registry, by field name, whose key set the
// constraint accepts.
func keySetReference(ctx *cue.Context, field string, fv cue.Value, registries []registry) (reference, bool) {
	kind := fv.IncompleteKind()
	switch {
	case kind&cue.StringKind != 0:
		if target, ok := matchRegistry(ctx, fv, registries); ok {
			return reference{field: field, targetField: target, rule: "key-set-ref"}, true
		}
	case kind&cue.ListKind != 0:
		elem := fv.LookupPath(cue.MakePath(cue.AnyIndex))
		if elem.Exists() && elem.IncompleteKind()&cue.StringKind != 0 {
			if target, ok := matchRegistry(ctx, elem, registries); ok {
				return reference{field: field, targetField: target, list: true, rule: "key-set-ref"}, true
			}
		}
	}
	return reference{}, false
}

// matchRegistry returns the field name of the first registry (by name) whose key set
// the string constraint fv accepts exactly - unifying with every key yet rejecting a
// foreign probe. A plain `string` accepts the probe and matches nothing.
func matchRegistry(ctx *cue.Context, fv cue.Value, registries []registry) (string, bool) {
	for _, reg := range registries {
		if refersTo(ctx, fv, reg.keys) {
			return reg.field, true
		}
	}
	return "", false
}

// refersTo reports whether the string constraint fv accepts every key in keys and
// rejects the foreign probe: the unification test for a key-set reference. An empty key
// set never matches (nothing to reference).
func refersTo(ctx *cue.Context, fv cue.Value, keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	for _, k := range keys {
		if fv.Unify(ctx.Encode(k)).Validate() != nil {
			return false
		}
	}
	return fv.Unify(ctx.Encode(foreignProbe)).Validate() != nil
}

// attrReference reads an explicit @ref(registryField) attribute on a schema field, the
// escape hatch for when the key-set idiom is not used. The named registry must exist.
func attrReference(field string, fv cue.Value, registries []registry) (reference, bool) {
	attr := fv.Attribute("ref")
	if attr.Err() != nil {
		return reference{}, false
	}
	target, err := attr.String(0)
	if err != nil {
		return reference{}, false
	}
	for _, reg := range registries {
		if reg.field == target {
			list := fv.IncompleteKind()&cue.ListKind != 0
			return reference{field: field, targetField: target, list: list, rule: "attr-ref"}, true
		}
	}
	return reference{}, false
}

// referencedKeys returns the concrete target keys a member declares through reference
// ref: the single concrete string for a scalar reference, or every concrete element for
// a list reference. An empty string (an unset optional default) yields no key.
func referencedKeys(member cue.Value, ref reference) []string {
	fv := member.LookupPath(cue.ParsePath(ref.field))
	if !ref.list {
		if s, err := fv.String(); err == nil && s != "" {
			return []string{s}
		}
		return nil
	}
	iter, err := fv.List()
	if err != nil {
		return nil
	}
	var keys []string
	for iter.Next() {
		if s, err := iter.Value().String(); err == nil && s != "" {
			keys = append(keys, s)
		}
	}
	return keys
}

// diagramJSON, nodeJSON, and edgeJSON are the encode shape unified with #Diagram. A
// node carries no id (the pattern injects it from the map key) and no x/y (auto-layout);
// an edge carries its own id and a fixed "relation" kind.
type diagramJSON struct {
	Nodes map[string]nodeJSON `json:"nodes"`
	Edges []edgeJSON          `json:"edges"`
}

type nodeJSON struct {
	Type  string                 `json:"type"`
	Label string                 `json:"label"`
	Data  map[string]interface{} `json:"data,omitempty"`
}

type edgeJSON struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
}

// buildDiagram encodes the projected nodes and edges, unifies them with the bundled
// #Diagram, and validates the result is concrete before returning it. A validation
// failure means a projection bug, not user input, so it is an internal diagnostic: the
// canvas is never handed something it cannot draw. The returned value is ready to
// marshal by the caller exactly like a discovered view.
func (e *Engine) buildDiagram(ctx *cue.Context, nodes []projectedNode, edges []projectedEdge) (cue.Value, []diag.Diagnostic) {
	schema := e.schemaDiagram(ctx)
	if !schema.Exists() {
		return cue.Value{}, []diag.Diagnostic{{Message: "diagram schema unavailable", Kind: diag.KindInternal}}
	}
	out := diagramJSON{Nodes: make(map[string]nodeJSON, len(nodes)), Edges: make([]edgeJSON, 0, len(edges))}
	for _, n := range nodes {
		out.Nodes[n.id] = nodeJSON{Type: "entity", Label: n.label, Data: n.data}
	}
	for _, edge := range edges {
		out.Edges = append(out.Edges, edgeJSON{
			ID: edge.id, Source: edge.source, Target: edge.target, Kind: "relation", Label: edge.label,
		})
	}
	encoded := ctx.Encode(out)
	if err := encoded.Err(); err != nil {
		return cue.Value{}, diag.From(err, e.cueDir, diag.KindInternal)
	}
	diagram := schema.Unify(encoded)
	if err := diagram.Validate(cue.Concrete(true)); err != nil {
		return cue.Value{}, diag.From(err, e.cueDir, diag.KindInternal)
	}
	return diagram, nil
}

// boundDiag is the diagnostic returned when a projection exceeds a count bound. It
// carries no source position: the breach is about the size of the derived diagram, not
// a place in the user's text.
func boundDiag(what string, got, max int) []diag.Diagnostic {
	return []diag.Diagnostic{{
		Message: fmt.Sprintf("inferred diagram has %d %s, over the limit of %d", got, what, max),
		Kind:    diag.KindInternal,
	}}
}
