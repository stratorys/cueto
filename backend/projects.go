// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

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

// The auto-created project that adopts any legacy flat version store on first run.
const defaultProjectID = "default"

// projectIDPattern is the exact shape of a project id: a bare lowercase slug that
// cannot contain a path separator or dot, so it can never traverse out of the
// versions dir when joined into a path. Mirrors versionIDPattern's discipline.
var projectIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// projectDir is the per-project version store: versionsDir/<id>/.
func (e *cueEvaluator) projectDir(id string) string {
	return filepath.Join(e.versionsDir, id)
}

// registryPath is the project registry (projects.json), stored at the versions
// root alongside the per-project subdirectories.
func (e *cueEvaluator) registryPath() string {
	return filepath.Join(e.versionsDir, "projects.json")
}

// resolveProjectDir validates the id, bootstraps the registry if needed, and
// confirms the project exists, returning its version-store directory. It takes the
// registry lock; version ops call it before touching the filesystem.
func (e *cueEvaluator) resolveProjectDir(id string) (string, error) {
	if e.versionsDir == "" {
		return "", errNoVersionsDir
	}
	if !projectIDPattern.MatchString(id) {
		return "", errInvalidProjectID
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensureBootstrapLocked(); err != nil {
		return "", err
	}
	list, err := e.readRegistryLocked()
	if err != nil {
		return "", err
	}
	for _, p := range list {
		if p.ID == id {
			return e.projectDir(id), nil
		}
	}
	return "", errProjectNotFound
}

// readRegistryLocked reads projects.json. A missing file yields an empty list (the
// caller bootstraps). Must be called with e.mu held.
func (e *cueEvaluator) readRegistryLocked() ([]ProjectMeta, error) {
	data, err := os.ReadFile(e.registryPath())
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
// a crash mid-write never leaves a truncated projects.json. Must hold e.mu.
func (e *cueEvaluator) writeRegistryLocked(list []ProjectMeta) error {
	if err := os.MkdirAll(e.versionsDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := e.registryPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, e.registryPath())
}

// ensureBootstrapLocked creates the registry on first use. When a legacy flat
// version store exists (loose <hash>.cue files and an index.jsonl directly under
// versionsDir, from before projects), it migrates them into the "default" project
// so no saved history is lost. A no-op once projects.json exists. Must hold e.mu.
func (e *cueEvaluator) ensureBootstrapLocked() error {
	if _, err := os.Stat(e.registryPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	now := time.Now().UTC()
	if err := e.migrateLegacyStoreLocked(); err != nil {
		return err
	}
	return e.writeRegistryLocked([]ProjectMeta{{
		ID:        defaultProjectID,
		Name:      "Default",
		CreatedAt: now,
		UpdatedAt: now,
	}})
}

// migrateLegacyStoreLocked moves any loose version files and index.jsonl at the
// versions root into versionsDir/default/. Safe when there is nothing to move. A
// fresh store with nothing to migrate leaves the default project empty (a blank
// canvas); users get the committed sample via "New from sample".
func (e *cueEvaluator) migrateLegacyStoreLocked() error {
	entries, err := os.ReadDir(e.versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dest := e.projectDir(defaultProjectID)
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
		if err := os.Rename(filepath.Join(e.versionsDir, name), filepath.Join(dest, name)); err != nil {
			return err
		}
	}
	return nil
}

// ListProjects implements Evaluator, newest-updated first.
func (e *cueEvaluator) ListProjects(_ context.Context) ([]ProjectMeta, error) {
	if e.versionsDir == "" {
		return nil, errNoVersionsDir
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensureBootstrapLocked(); err != nil {
		return nil, err
	}
	list, err := e.readRegistryLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	return list, nil
}

// CreateProject implements Evaluator. The id is a uniquified slug of the name; a
// "sample" seed writes a first version copied from the seed data.cue, while
// "blank" (the default) leaves the project empty.
func (e *cueEvaluator) CreateProject(ctx context.Context, name, seed string) (ProjectMeta, error) {
	if e.versionsDir == "" {
		return ProjectMeta{}, errNoVersionsDir
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensureBootstrapLocked(); err != nil {
		return ProjectMeta{}, err
	}
	list, err := e.readRegistryLocked()
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

	if err := os.MkdirAll(e.projectDir(id), 0o755); err != nil {
		return ProjectMeta{}, err
	}
	if seed == "sample" {
		if data, rerr := e.ReadSeed(ctx); rerr == nil {
			if _, werr := e.writeVersion(e.projectDir(id), []byte(data)); werr != nil {
				return ProjectMeta{}, werr
			}
		}
	}

	now := time.Now().UTC()
	meta := ProjectMeta{ID: id, Name: display, CreatedAt: now, UpdatedAt: now}
	if err := e.writeRegistryLocked(append(list, meta)); err != nil {
		return ProjectMeta{}, err
	}
	return meta, nil
}

// RenameProject implements Evaluator.
func (e *cueEvaluator) RenameProject(_ context.Context, id, name string) (ProjectMeta, error) {
	if e.versionsDir == "" {
		return ProjectMeta{}, errNoVersionsDir
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensureBootstrapLocked(); err != nil {
		return ProjectMeta{}, err
	}
	list, err := e.readRegistryLocked()
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
			if err := e.writeRegistryLocked(list); err != nil {
				return ProjectMeta{}, err
			}
			return list[i], nil
		}
	}
	return ProjectMeta{}, errProjectNotFound
}

// DeleteProject implements Evaluator. The last remaining project cannot be deleted,
// so the app always has somewhere to land.
func (e *cueEvaluator) DeleteProject(_ context.Context, id string) error {
	if e.versionsDir == "" {
		return errNoVersionsDir
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensureBootstrapLocked(); err != nil {
		return err
	}
	list, err := e.readRegistryLocked()
	if err != nil {
		return err
	}
	if len(list) <= 1 {
		return errLastProject
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
		return errProjectNotFound
	}
	if err := e.writeRegistryLocked(next); err != nil {
		return err
	}
	return os.RemoveAll(e.projectDir(id))
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
