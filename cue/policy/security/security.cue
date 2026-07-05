// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Starter policy pack: security zones and ownership. A pack is an importable
// module that checks a diagram structurally and produces a list of violations.
//
// Deliberately does NOT import the diagram package: the harness (package diagram)
// imports this pack, so importing back would be a cycle. It describes the shape
// it reads with an open struct and reads governed fields with the `*ref | default`
// idiom, so an unset optional field (including the schema's literal-union fields
// like role/call) reads as its fallback instead of an unresolved disjunction that
// would break the comprehensions.
package security

import "list"

#Violation: {
	rule:    string
	node?:   string
	edge?:   string
	message: string
}

// The shape this pack reads. Open so real nodes/edges keep their other fields;
// only the structural keys are required.
#DiagramLike: {
	nodes: [string]: {...}
	edges: [...{
		id:     string
		source: string
		target: string
		...
	}]
	...
}

// No synchronous call may span two regions.
#NoCrossRegionSync: {
	d: #DiagramLike
	violations: [
		for e in d.edges
		let sync = *e.sync | false
		let call = *e.call | ""
		if sync && call == "calls"
		let sregion = *d.nodes[e.source].region | ""
		let tregion = *d.nodes[e.target].region | ""
		if sregion != "" && tregion != "" && sregion != tregion {
			{rule: "no-cross-region-sync", edge: e.id, message: "sync call \(e.id): \(sregion) -> \(tregion)"}
		},
	]
}

// No edge may cross into or out of the PCI zone (isolate cardholder data).
#NoPciBoundaryCrossing: {
	d: #DiagramLike
	violations: [
		for e in d.edges
		let szone = *d.nodes[e.source].zone | ""
		let tzone = *d.nodes[e.target].zone | ""
		if (szone == "pci") != (tzone == "pci") {
			{rule: "no-pci-crossing", edge: e.id, message: "edge \(e.id) crosses the PCI boundary"}
		},
	]
}

// Every database must declare a backup owner.
#DbNeedsOwner: {
	d: #DiagramLike
	violations: [
		for id, n in d.nodes
		let role = *n.role | ""
		let owner = *n.owner | ""
		if role == "database" && owner == "" {
			{rule: "db-needs-owner", node: id, message: "database \(id) has no owner"}
		},
	]
}

// The aggregate pack: unify with {d: <diagram>} to get all violations. The
// individual checks already emit #Violation values, so no list type annotation is
// needed here (annotating `violations: [...#Violation]` alongside the concrete
// list makes CUE collapse it to []).
#Pack: {
	d: #DiagramLike
	// Bind via a let so the `{d: ...}` literals below reference this d rather than
	// self-referencing the field they define (which would be an empty cycle).
	let dd = d
	violations: list.Concat([
		(#NoCrossRegionSync & {d: dd}).violations,
		(#NoPciBoundaryCrossing & {d: dd}).violations,
		(#DbNeedsOwner & {d: dd}).violations,
	])
}
