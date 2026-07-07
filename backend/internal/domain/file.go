// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package domain holds the shared vocabulary the concern services and the HTTP
// transport exchange: the editable File, element Provenance, a Project, a saved
// Version, and a version Manifest. It carries data types and the pure filename
// guard only; all behavior lives in the concern packages that import it.
package domain

// File is one client-supplied editable CUE file: a filename (guarded by
// ValidEditableName) and its full source text. Multiple files unify into one
// `package main`, so nodes may be authored across several files.
type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Provenance attributes each diagram element to the editable file that authored
// it, so a canvas edit can be written back into the right file. Nodes maps a
// node id to its filename; Edges names the single file that owns the edge list
// (edges are a CUE list and cannot be split across files by unification).
type Provenance struct {
	Nodes map[string]string `json:"nodes"`
	Edges string            `json:"edges"`
}
