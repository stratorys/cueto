// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package repo is the workspace-mode persistence: it writes a validated buffer to
// the real file inside the user's checkout and reads history back from git,
// read-only. It never stages, commits, or otherwise mutates git state; git is the
// only version store in workspace mode. All paths are validated by the same
// domain guard the overlay uses and confined to the workspace root, so a client
// can never write or read outside it.
package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/stratorys/cueto/backend/internal/domain"
)

// commitPattern is the exact shape of a full git commit hash, so a client-supplied
// commit ref can never traverse or reach an unexpected object.
var commitPattern = regexp.MustCompile("^[a-f0-9]{40}$")

// Repo writes and reads files under a single workspace root. dir is the absolute
// module root (contains cue.mod, under a git worktree); maxOutputBytes bounds any
// content read back, mirroring the evaluation output cap.
type Repo struct {
	dir            string
	maxOutputBytes int
}

// New returns a Repo rooted at an absolute workspace dir.
func New(dir string, maxOutputBytes int) *Repo {
	return &Repo{dir: dir, maxOutputBytes: maxOutputBytes}
}

// Storage errors, distinct from user-input diagnostics. Handlers map these to
// status codes; they never carry CUE positions or host paths.
var (
	ErrInvalidPath    = errors.New("invalid file path")
	ErrInvalidCommit  = errors.New("invalid commit id")
	ErrFileNotFound   = errors.New("file not found")
	ErrCommitNotFound = errors.New("commit not found")
	ErrOutputTooLarge = errors.New("file content too large")
)

// ContentHash is the base-version token for optimistic concurrency: the sha256 hex
// of a file body. A save carries the token it loaded, and the write is refused
// when the on-disk body no longer hashes to it. Single-sourced here so the read
// side and the conflict check compute it the same way.
func ContentHash(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

// resolve validates a client path against the shared editable-name guard and joins
// it under the workspace root, refusing anything that would escape it. The guard
// already rejects traversal, absolute paths, and separator tricks; the prefix
// check is defense in depth against a symlinked segment.
func (r *Repo) resolve(rel string) (string, bool) {
	if !domain.ValidEditableName(rel) {
		return "", false
	}
	target := filepath.Join(r.dir, filepath.FromSlash(rel))
	prefix := r.dir + string(filepath.Separator)
	if target != r.dir && !strings.HasPrefix(target, prefix) {
		return "", false
	}
	return target, true
}
