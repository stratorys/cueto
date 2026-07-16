// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package knowledge provides Cueto's optional explicit knowledge contract.
// Modules do not need to import it: Cueto continues to discover registries and
// relations structurally. Importing it makes the metadata surface type-checked
// and stable for CLI, HTTP, and MCP consumers.
//
// When a domain's label is also its collection field, bind the collection with a
// top-level let and refer to that alias from the domain. CUE resolves an unqualified
// `customers` inside domains.customers as the enclosing field, which is a cycle.
package knowledge

#Knowledge: {
	metadata: {
		title:        string
		description?: string
		revision?:    string
	}

	domains: [string]: #Domain
	evaluations?: [string]: #Evaluation
	observations?: [string]: #Observation
	checks?: [string]: bool
}

#Domain: {
	description?: string
	collection:   _
	key?:         string | *"id"
}

#Evaluation: {
	description: string
	input:       _
	result:      _
}

// #Evaluations is the optional root-level contract for phase-six named
// evaluations: `evaluations: knowledge.#Evaluations & { ... }`.
#Evaluations: [string]: #Evaluation

#SourceRef: {
	kind: "file" | "uri" | "database" | "manual"
	uri: string @uri()
	pointer?: string
	retrievedAt?: string
}

#Observation: {
	entity: string
	field: string
	value: _
	source: #SourceRef
	status: "active" | "stale" | "disputed"
	authority?: int & >=0 & <=100
}

#Observations: [string]: #Observation
