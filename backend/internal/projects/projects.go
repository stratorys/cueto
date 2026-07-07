// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package projects manages the projects root: the parent directory under which
// each child is a git repository plus a CUE module. It lists projects, resolves a
// project id to its module directory (validated and confined to the root), and
// creates a new project by initializing a git repo, scaffolding a minimal module,
// and making one initial commit. It never mutates a project's git state after
// creation; from then on git is the only history.
package projects

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// projectIDPattern is the exact shape of a project id: a single path segment of
// ASCII word bytes. It doubles as the directory name under the root, so it can
// never carry a separator or a traversal.
var projectIDPattern = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9_-]*$")

// moduleMarker is the directory every CUE module carries at its root; a child of
// the projects root counts as a project only when it contains one.
const moduleMarker = "cue.mod"

// Errors distinct from user-input diagnostics. Handlers map these to status codes.
var (
	ErrInvalidName = errors.New("invalid project name")
	ErrExists      = errors.New("project already exists")
)

// Project is one project under the root: its id (also the directory name) and its
// display name. For now the name equals the id; a mutable label can be layered on
// later without changing the on-disk model.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Manager owns the projects root. root is an absolute directory that already
// exists (validated by config at startup).
type Manager struct {
	root string
}

// New returns a Manager rooted at an absolute projects dir.
func New(root string) *Manager {
	return &Manager{root: root}
}

// List returns every child of the root that is a CUE module (contains cue.mod),
// sorted by id. A non-module child (a stray directory, a file) is skipped rather
// than erroring, so the root can hold unrelated content without breaking listing.
func (m *Manager) List() ([]Project, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !projectIDPattern.MatchString(entry.Name()) {
			continue
		}
		if !isModule(filepath.Join(m.root, entry.Name())) {
			continue
		}
		out = append(out, Project{ID: entry.Name(), Name: entry.Name()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Resolve validates a project id and returns its absolute module directory. It
// returns false for a malformed id or a directory that is not a CUE module, so a
// client can never reach a path outside the root or one that will not evaluate.
func (m *Manager) Resolve(id string) (string, bool) {
	if !projectIDPattern.MatchString(id) {
		return "", false
	}
	dir := filepath.Join(m.root, id)
	if !isModule(dir) {
		return "", false
	}
	return dir, true
}

// Create initializes a new project under the root: it slugifies the name into an
// id, refuses an id that already names a non-empty directory (so it never writes
// over existing content or re-inits a repo), scaffolds a minimal module, and makes
// one initial commit. The commit is the single, deliberate write to git state; no
// project git is ever touched again.
func (m *Manager) Create(name string) (Project, error) {
	id := slugify(name)
	if !projectIDPattern.MatchString(id) {
		return Project{}, ErrInvalidName
	}
	dir := filepath.Join(m.root, id)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return Project{}, ErrExists
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Project{}, err
	}
	if err := scaffold(dir, id); err != nil {
		return Project{}, err
	}
	if err := initCommit(dir); err != nil {
		return Project{}, err
	}
	return Project{ID: id, Name: id}, nil
}

// isModule reports whether dir is a CUE module root (carries a cue.mod directory).
func isModule(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, moduleMarker))
	return err == nil && info.IsDir()
}

// slugify turns a display name into a filesystem-safe, single-segment id: lower
// case, non-word runs collapsed to a single hyphen, leading/trailing hyphens
// trimmed. An empty or all-invalid name yields "", which Create rejects.
func slugify(name string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
