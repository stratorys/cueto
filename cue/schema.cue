// Hand-written schema. The canvas never rewrites this file.
// data.cue holds the concrete diagram instance and is the only file the
// canvas round-trips.
package diagram

#Diagram: {
	nodes: [ID=string]: #Node & {id: ID}
	edges: [...#Edge]
}

#Node: {
	id:    string
	type:  "entity" | "table" | "process" | "decision"
	x:     number
	y:     number
	label: string
	// Typed payload for a DB table.
	columns?: [...#Column]
}

#Column: {
	name:   string
	dbType: string
	pk?:    bool
	fk?:    bool
}

#Edge: {
	id:            string
	source:        string
	sourceHandle?: string
	target:        string
	targetHandle?: string
	kind:          "relation" | "arrow" | "inherit"
	card?:         "1-1" | "1-n" | "n-n"
}
