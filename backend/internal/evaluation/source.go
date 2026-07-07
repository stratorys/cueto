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
type Source struct {
	Dir     string        // module root (contains cue.mod)
	Overlay []domain.File // unsaved client buffers layered over Dir
}
