// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package store persists projects and their immutable, content-addressed
// versions on disk under a single versions root. It owns no CUE knowledge: it
// stores and reads opaque data.cue text, leaving validation to the caller.
package store

import (
	"errors"
	"sync"
)

// Store is the filesystem-backed project + version store rooted at versionsDir.
type Store struct {
	versionsDir string
	// Guards the project registry (projects.json) read-modify-write. Per-version
	// files are content-addressed and written atomically, so only registry
	// mutations need serializing.
	mu sync.Mutex
}

// New returns a Store rooted at versionsDir. An empty versionsDir is valid to
// construct but every operation then fails with ErrNoVersionsDir.
func New(versionsDir string) *Store {
	return &Store{versionsDir: versionsDir}
}

// Storage errors, distinct from user-input diagnostics. Callers map these to
// HTTP status codes; they never carry CUE positions or host paths.
var (
	ErrNoVersionsDir    = errors.New("versions directory is not configured")
	ErrInvalidVersionID = errors.New("invalid version id")
	ErrVersionNotFound  = errors.New("version not found")
	ErrInvalidProjectID = errors.New("invalid project id")
	ErrProjectNotFound  = errors.New("project not found")
	ErrLastProject      = errors.New("cannot delete the last project")
)
