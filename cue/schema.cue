// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Hand-written schema. The canvas never rewrites this file.
// data.cue holds the concrete diagram instance and is the only file the
// canvas round-trips.
package diagram

#Diagram: {
	nodes: [ID=string]: #Node & {id: ID}
	edges: [...#Edge]
}

#Node: {
	id:   string
	type: "entity" | "table" | "process" | "decision" | "shape" | "container"
	// Id of the containing node when nested; a child's x/y are relative to it.
	parent?: string
	// Optional coordinates. A canvas-drawn node carries them; a data-derived node
	// omits them and is auto-laid-out (its position stays view-only, never written
	// back), so a file that derives its diagram from data can stay coordinate-free.
	x?:      number
	y?:      number
	// Optional explicit size in graph units; the canvas falls back to a
	// content-derived size when these are absent.
	width?:  number
	height?: number
	label: string
	// Arbitrary structured payload, rendered as a key/value card. Lets a node
	// carry domain data (records, facts) with no bespoke schema field.
	data?: {...}
	// Typed payload for a DB table.
	columns?: [...#Column]
	// Annotation payload, set only when type is "shape".
	shape?: "rectangle" | "ellipse" | "diamond" | "line" | "text"
	// Optional per-shape colors (any CSS color string).
	fill?:   string
	stroke?: string
	// Line only: drag direction (true = "\", absent = "/").
	flip?:   bool
	icon?:   string
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
	kind:          "relation" | "arrow" | "inherit" | "line"
	// Optional free-form text drawn on the edge, edited inline on the canvas.
	label?:        string
	card?:         "1-1" | "1-n" | "n-n"
}
