// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package evaluation

import "github.com/stratorys/cueto/backend/internal/domain"

// Source is the unit every file-touching engine operation works on: a directory
// containing a cue.mod, plus an overlay of unsaved editor buffers layered on top
// of what is on disk. Making the module root a per-call input, rather than a fixed
// engine field, is the seam that lets the HTTP server, the CLI, and MCP drive the
// same evaluation code against different directories.
//
// Overlay entries are client-supplied and each Name is guarded by
// domain.ValidEditableName before it becomes an overlay key, so a client can never
// supply, replace, or escape a path outside Dir.
//
// View names which discovered view Eval renders. Empty selects the default view;
// a name that no longer matches also falls back to the default, so a stale client
// selection never fails the eval.
type Source struct {
	Dir string // module root (contains cue.mod)
	// Package optionally selects a package below Dir for generic compilation. An
	// empty value selects the module-root package, preserving the diagram
	// evaluator's existing behaviour.
	Package string
	Overlay []domain.File // unsaved client buffers layered over Dir
	View    string        // discovered view to render; empty = default
}
