// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stratorys/cueto/backend/internal/domain"
)

// DefaultProjectID is the project created on first run so the app always has
// somewhere to land.
const DefaultProjectID = "default"

// projectIDPattern is the exact shape of a project id: a bare lowercase slug that
// cannot contain a path separator or dot, so it can never traverse out of the
// data dir when joined into a path. Mirrors versionIDPattern's discipline.
var projectIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// projectDir is the per-project version store: dataDir/<id>/.
func (w *Workspace) projectDir(id string) string {
	return filepath.Join(w.dataDir, id)
}

// registryPath is the project registry (projects.json), stored at the data
// root alongside the per-project subdirectories.
func (w *Workspace) registryPath() string {
	return filepath.Join(w.dataDir, "projects.json")
}

// ResolveProjectDir validates the id, bootstraps the registry if needed, and
// confirms the project exists, returning its version-store directory. It takes the
// registry lock; version ops call it before touching the filesystem.
func (w *Workspace) ResolveProjectDir(id string) (string, error) {
	if w.dataDir == "" {
		return "", ErrNoDataDir
	}
	if !projectIDPattern.MatchString(id) {
		return "", ErrInvalidProjectID
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureBootstrapLocked(); err != nil {
		return "", err
	}
	list, err := w.readRegistryLocked()
	if err != nil {
		return "", err
	}
	for _, p := range list {
		if p.ID == id {
			return w.projectDir(id), nil
		}
	}
	return "", ErrProjectNotFound
}

// readRegistryLocked reads projects.json. A missing file yields an empty list (the
// caller bootstraps). Must be called with w.mu held.
func (w *Workspace) readRegistryLocked() ([]domain.Project, error) {
	data, err := os.ReadFile(w.registryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.Project{}, nil
		}
		return nil, err
	}
	var list []domain.Project
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// writeRegistryLocked persists the registry atomically (write-temp-then-rename), so
// a crash mid-write never leaves a truncated projects.json. Must hold w.mu.
func (w *Workspace) writeRegistryLocked(list []domain.Project) error {
	if err := os.MkdirAll(w.dataDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := w.registryPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, w.registryPath())
}

// ensureBootstrapLocked creates the registry on first use, seeding a single empty
// "default" project so the app always has somewhere to land (users get the
// committed sample via "New from sample"). A no-op once projects.json exists. Must
// hold w.mu.
func (w *Workspace) ensureBootstrapLocked() error {
	if _, err := os.Stat(w.registryPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	now := time.Now().UTC()
	return w.writeRegistryLocked([]domain.Project{{
		ID:        DefaultProjectID,
		Name:      "Default",
		CreatedAt: now,
		UpdatedAt: now,
	}})
}

// ListProjects returns the registered projects, newest-updated first. The first
// call bootstraps the registry with a default project.
func (w *Workspace) ListProjects(_ context.Context) ([]domain.Project, error) {
	if w.dataDir == "" {
		return nil, ErrNoDataDir
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureBootstrapLocked(); err != nil {
		return nil, err
	}
	list, err := w.readRegistryLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	return list, nil
}

// CreateProject registers a new project seeded either "blank" or "sample" (a copy
// of the seed data.cue as its first version), returning its metadata. Reading the
// seed is best-effort: a missing seed yields a blank project rather than failing.
func (w *Workspace) CreateProject(_ context.Context, name, seed string) (domain.Project, error) {
	var seedData []byte
	if seed == "sample" {
		if data, err := w.ReadSeed(); err == nil {
			seedData = []byte(data)
		}
	}
	return w.create(name, seedData)
}

// create registers a new project. The id is a uniquified slug of the name; when
// seedData is non-empty it is written as the project's first version, otherwise
// the project starts empty.
func (w *Workspace) create(name string, seedData []byte) (domain.Project, error) {
	if w.dataDir == "" {
		return domain.Project{}, ErrNoDataDir
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureBootstrapLocked(); err != nil {
		return domain.Project{}, err
	}
	list, err := w.readRegistryLocked()
	if err != nil {
		return domain.Project{}, err
	}

	display := strings.TrimSpace(name)
	if display == "" {
		display = "Untitled"
	}
	taken := make(map[string]bool, len(list))
	for _, p := range list {
		taken[p.ID] = true
	}
	id := uniqueProjectID(slugifyProject(display), taken)

	if err := os.MkdirAll(w.projectDir(id), 0o755); err != nil {
		return domain.Project{}, err
	}
	if len(seedData) > 0 {
		if _, werr := w.writeVersion(w.projectDir(id), seedData); werr != nil {
			return domain.Project{}, werr
		}
	}

	now := time.Now().UTC()
	meta := domain.Project{ID: id, Name: display, CreatedAt: now, UpdatedAt: now}
	if err := w.writeRegistryLocked(append(list, meta)); err != nil {
		return domain.Project{}, err
	}
	return meta, nil
}

// RenameProject changes a project's display name.
func (w *Workspace) RenameProject(_ context.Context, id, name string) (domain.Project, error) {
	if w.dataDir == "" {
		return domain.Project{}, ErrNoDataDir
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureBootstrapLocked(); err != nil {
		return domain.Project{}, err
	}
	list, err := w.readRegistryLocked()
	if err != nil {
		return domain.Project{}, err
	}
	display := strings.TrimSpace(name)
	if display == "" {
		display = "Untitled"
	}
	for i := range list {
		if list[i].ID == id {
			list[i].Name = display
			list[i].UpdatedAt = time.Now().UTC()
			if err := w.writeRegistryLocked(list); err != nil {
				return domain.Project{}, err
			}
			return list[i], nil
		}
	}
	return domain.Project{}, ErrProjectNotFound
}

// DeleteProject removes a project and its version store. The last remaining
// project cannot be deleted, so the app always has somewhere to land.
func (w *Workspace) DeleteProject(_ context.Context, id string) error {
	if w.dataDir == "" {
		return ErrNoDataDir
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureBootstrapLocked(); err != nil {
		return err
	}
	list, err := w.readRegistryLocked()
	if err != nil {
		return err
	}
	if len(list) <= 1 {
		return ErrLastProject
	}
	next := make([]domain.Project, 0, len(list))
	found := false
	for _, p := range list {
		if p.ID == id {
			found = true
			continue
		}
		next = append(next, p)
	}
	if !found {
		return ErrProjectNotFound
	}
	if err := w.writeRegistryLocked(next); err != nil {
		return err
	}
	return os.RemoveAll(w.projectDir(id))
}

// slugifyProject reduces a display name to a bare id charset (lowercase, digits,
// hyphens), collapsing other runs to a single hyphen. Falls back to "project" when
// nothing usable remains, so the result always matches projectIDPattern.
func slugifyProject(name string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	if slug == "" {
		return "project"
	}
	return slug
}

// uniqueProjectID disambiguates a base slug against ids already in use: base,
// base-2, base-3, ...
func uniqueProjectID(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		candidate := base + "-" + strconv.Itoa(n)
		if !taken[candidate] {
			return candidate
		}
	}
}
