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

import (
	"path/filepath"
	"regexp"
	"strings"
)

// File is one client-supplied editable CUE file: a bare filename (guarded by
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

// editableNamePattern is the strict shape of a client filename: bare word plus
// a .cue suffix, no other dots, no separators.
var editableNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+\.cue$`)

// ValidEditableName reports whether name is a safe client-supplied CUE filename.
// It must be a bare base name (no path separators or traversal), match the strict
// pattern, and not be the reserved schema.cue. The schema check is
// case-insensitive because macOS/APFS is case-insensitive by default. The schema
// now lives in the diagram/ subpackage rather than a root schema.cue, so this
// reservation is vestigial; it is kept until the legacy layout is retired
// (Phase 3). This guard is what lets the N-file overlay accept client filenames
// without a client escaping the module root. It lives in domain because both the
// evaluation and authoring concerns enforce it, and evaluation must not depend on
// another concern to do so.
func ValidEditableName(name string) bool {
	if name != filepath.Base(name) {
		return false
	}
	if !editableNamePattern.MatchString(name) {
		return false
	}
	if strings.EqualFold(name, "schema.cue") {
		return false
	}
	return true
}
