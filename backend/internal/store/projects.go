// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package store

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
)

// ProjectMeta identifies one project and its display name. The id is a stable
// filesystem-safe slug (also the version-store subdirectory name); the name is
// the mutable label shown in the UI.
type ProjectMeta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DefaultProjectID is the auto-created project that adopts any legacy flat
// version store on first run.
const DefaultProjectID = "default"

// projectIDPattern is the exact shape of a project id: a bare lowercase slug that
// cannot contain a path separator or dot, so it can never traverse out of the
// versions dir when joined into a path. Mirrors versionIDPattern's discipline.
var projectIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// projectDir is the per-project version store: versionsDir/<id>/.
func (s *Store) projectDir(id string) string {
	return filepath.Join(s.versionsDir, id)
}

// registryPath is the project registry (projects.json), stored at the versions
// root alongside the per-project subdirectories.
func (s *Store) registryPath() string {
	return filepath.Join(s.versionsDir, "projects.json")
}

// ResolveProjectDir validates the id, bootstraps the registry if needed, and
// confirms the project exists, returning its version-store directory. It takes the
// registry lock; version ops call it before touching the filesystem.
func (s *Store) ResolveProjectDir(id string) (string, error) {
	if s.versionsDir == "" {
		return "", ErrNoVersionsDir
	}
	if !projectIDPattern.MatchString(id) {
		return "", ErrInvalidProjectID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureBootstrapLocked(); err != nil {
		return "", err
	}
	list, err := s.readRegistryLocked()
	if err != nil {
		return "", err
	}
	for _, p := range list {
		if p.ID == id {
			return s.projectDir(id), nil
		}
	}
	return "", ErrProjectNotFound
}

// readRegistryLocked reads projects.json. A missing file yields an empty list (the
// caller bootstraps). Must be called with s.mu held.
func (s *Store) readRegistryLocked() ([]ProjectMeta, error) {
	data, err := os.ReadFile(s.registryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []ProjectMeta{}, nil
		}
		return nil, err
	}
	var list []ProjectMeta
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// writeRegistryLocked persists the registry atomically (write-temp-then-rename), so
// a crash mid-write never leaves a truncated projects.json. Must hold s.mu.
func (s *Store) writeRegistryLocked(list []ProjectMeta) error {
	if err := os.MkdirAll(s.versionsDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.registryPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.registryPath())
}

// ensureBootstrapLocked creates the registry on first use. When a legacy flat
// version store exists (loose <hash>.cue files and an index.jsonl directly under
// versionsDir, from before projects), it migrates them into the "default" project
// so no saved history is lost. A no-op once projects.json exists. Must hold s.mu.
func (s *Store) ensureBootstrapLocked() error {
	if _, err := os.Stat(s.registryPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	now := time.Now().UTC()
	if err := s.migrateLegacyStoreLocked(); err != nil {
		return err
	}
	return s.writeRegistryLocked([]ProjectMeta{{
		ID:        DefaultProjectID,
		Name:      "Default",
		CreatedAt: now,
		UpdatedAt: now,
	}})
}

// migrateLegacyStoreLocked moves any loose version files and index.jsonl at the
// versions root into versionsDir/default/. Safe when there is nothing to move. A
// fresh store with nothing to migrate leaves the default project empty (a blank
// canvas); users get the committed sample via "New from sample".
func (s *Store) migrateLegacyStoreLocked() error {
	entries, err := os.ReadDir(s.versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dest := s.projectDir(DefaultProjectID)
	moved := false
	for _, entry := range entries {
		name := entry.Name()
		isVersion := !entry.IsDir() && strings.HasSuffix(name, ".cue")
		isIndex := !entry.IsDir() && name == "index.jsonl"
		if !isVersion && !isIndex {
			continue
		}
		if !moved {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			moved = true
		}
		if err := os.Rename(filepath.Join(s.versionsDir, name), filepath.Join(dest, name)); err != nil {
			return err
		}
	}
	return nil
}

// ListProjects returns the registered projects, newest-updated first. The first
// call bootstraps the registry, migrating any legacy flat version store into a
// "default" project.
func (s *Store) ListProjects(_ context.Context) ([]ProjectMeta, error) {
	if s.versionsDir == "" {
		return nil, ErrNoVersionsDir
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureBootstrapLocked(); err != nil {
		return nil, err
	}
	list, err := s.readRegistryLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	return list, nil
}

// Create registers a new project. The id is a uniquified slug of the name; when
// seedData is non-empty it is written as the project's first version (the caller
// supplies the seed text), otherwise the project starts empty.
func (s *Store) Create(name string, seedData []byte) (ProjectMeta, error) {
	if s.versionsDir == "" {
		return ProjectMeta{}, ErrNoVersionsDir
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureBootstrapLocked(); err != nil {
		return ProjectMeta{}, err
	}
	list, err := s.readRegistryLocked()
	if err != nil {
		return ProjectMeta{}, err
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

	if err := os.MkdirAll(s.projectDir(id), 0o755); err != nil {
		return ProjectMeta{}, err
	}
	if len(seedData) > 0 {
		if _, werr := s.WriteVersion(s.projectDir(id), seedData); werr != nil {
			return ProjectMeta{}, werr
		}
	}

	now := time.Now().UTC()
	meta := ProjectMeta{ID: id, Name: display, CreatedAt: now, UpdatedAt: now}
	if err := s.writeRegistryLocked(append(list, meta)); err != nil {
		return ProjectMeta{}, err
	}
	return meta, nil
}

// RenameProject changes a project's display name.
func (s *Store) RenameProject(_ context.Context, id, name string) (ProjectMeta, error) {
	if s.versionsDir == "" {
		return ProjectMeta{}, ErrNoVersionsDir
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureBootstrapLocked(); err != nil {
		return ProjectMeta{}, err
	}
	list, err := s.readRegistryLocked()
	if err != nil {
		return ProjectMeta{}, err
	}
	display := strings.TrimSpace(name)
	if display == "" {
		display = "Untitled"
	}
	for i := range list {
		if list[i].ID == id {
			list[i].Name = display
			list[i].UpdatedAt = time.Now().UTC()
			if err := s.writeRegistryLocked(list); err != nil {
				return ProjectMeta{}, err
			}
			return list[i], nil
		}
	}
	return ProjectMeta{}, ErrProjectNotFound
}

// DeleteProject removes a project and its version store. The last remaining
// project cannot be deleted, so the app always has somewhere to land.
func (s *Store) DeleteProject(_ context.Context, id string) error {
	if s.versionsDir == "" {
		return ErrNoVersionsDir
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureBootstrapLocked(); err != nil {
		return err
	}
	list, err := s.readRegistryLocked()
	if err != nil {
		return err
	}
	if len(list) <= 1 {
		return ErrLastProject
	}
	next := make([]ProjectMeta, 0, len(list))
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
	if err := s.writeRegistryLocked(next); err != nil {
		return err
	}
	return os.RemoveAll(s.projectDir(id))
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
