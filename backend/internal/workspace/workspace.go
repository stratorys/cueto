// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package workspace owns everything on disk under the data dir: the project
// registry and each project's immutable, content-addressed versions. It stores
// and reads opaque data.cue text and holds no CUE knowledge; validation is the
// caller's concern. It also reads the platform seed data.cue (under the CUE dir)
// used to seed a "from sample" project.
package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/stratorys/cueto/backend/internal/config"
)

// Workspace is the filesystem-backed project + version store rooted at dataDir.
// seedDir is the platform CUE dir that holds the read-only seed data.cue.
type Workspace struct {
	dataDir string
	seedDir string
	// Guards the project registry (projects.json) read-modify-write. Per-version
	// files are content-addressed and written atomically, so only registry
	// mutations need serializing.
	mu sync.Mutex
}

// New returns a Workspace rooted at cfg.DataDir, reading its seed from
// cfg.CueDir. An empty data dir is valid to construct but every operation then
// fails with ErrNoDataDir.
func New(cfg config.Config) *Workspace {
	return &Workspace{dataDir: cfg.DataDir, seedDir: cfg.CueDir}
}

// Storage errors, distinct from user-input diagnostics. Callers map these to
// HTTP status codes; they never carry CUE positions or host paths.
var (
	ErrNoDataDir        = errors.New("data directory is not configured")
	ErrInvalidVersionID = errors.New("invalid version id")
	ErrVersionNotFound  = errors.New("version not found")
	ErrInvalidProjectID = errors.New("invalid project id")
	ErrProjectNotFound  = errors.New("project not found")
	ErrLastProject      = errors.New("cannot delete the last project")
	ErrSeedNotFound     = errors.New("seed data.cue not found")
)

// ReadSeed returns the on-disk seed data.cue text (the fallback used to seed a
// "from sample" project). The path is server-built from seedDir, so it can never
// traverse outside the platform CUE dir. It is a static fixture, never a saved
// version.
func (w *Workspace) ReadSeed() (string, error) {
	data, err := os.ReadFile(filepath.Join(w.seedDir, "data.cue"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrSeedNotFound
		}
		return "", err
	}
	return string(data), nil
}
