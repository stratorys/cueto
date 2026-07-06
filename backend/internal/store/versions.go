// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package store

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// VersionMeta identifies one saved version and when it was first saved. SavedAt
// comes from the append-only index when present, else the file mtime.
type VersionMeta struct {
	Version string    `json:"version"`
	SavedAt time.Time `json:"savedAt"`
}

// versionIDPattern is the exact shape of a content-hash id (sha256 hex). Reads
// are rejected unless the id matches, so a version id from the URL can never
// escape the versions dir via path traversal.
var versionIDPattern = regexp.MustCompile("^[a-f0-9]{64}$")

// WriteVersion stores data as an immutable, content-addressed version under dir
// and returns its id (the sha256 hex of the content). Writes go only into the
// versions dir - never the CUE package dir - so the seed data.cue is untouched
// and version files never join the default `package main`. Identical content
// is idempotent: an existing version is reused, not rewritten.
func (s *Store) WriteVersion(dir string, data []byte) (string, error) {
	if s.versionsDir == "" {
		return "", ErrNoVersionsDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:])
	path := filepath.Join(dir, id+".cue")

	// O_EXCL makes creation atomic: concurrent saves of the same content race on
	// the same name and all but one see ErrExist, which is success (idempotent).
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return id, nil
	}
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	// Record save order + timestamp in the append-only index. Only the fresh-create
	// branch reaches here (idempotent re-saves returned above), so a version is
	// indexed exactly once. The index is derived metadata: a failure to append is
	// not fatal to the save, since the version file itself is the source of truth.
	_ = s.appendIndex(dir, id)
	return id, nil
}

// indexPath is a project's append-only log of save events (one JSON object per line).
func (s *Store) indexPath(dir string) string {
	return filepath.Join(dir, "index.jsonl")
}

// appendIndex records one save event. Content hashes carry no order or time, so
// this log is what lets ListVersions present true save order and timestamps.
func (s *Store) appendIndex(dir, id string) error {
	line, err := json.Marshal(VersionMeta{Version: id, SavedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.indexPath(dir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// readIndex reads a project's save-order log into a map of id -> first-save time. A
// missing index is not an error (older versions predate it); such versions fall
// back to their file mtime in ListVersions.
func (s *Store) readIndex(dir string) map[string]time.Time {
	times := map[string]time.Time{}
	f, err := os.Open(s.indexPath(dir))
	if err != nil {
		return times
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var meta VersionMeta
		if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
			continue
		}
		// Keep the first (earliest) timestamp for an id; ignore any later dup line.
		if _, seen := times[meta.Version]; !seen {
			times[meta.Version] = meta.SavedAt
		}
	}
	return times
}

// ListVersions enumerates a project's version files and stamps each with its
// indexed save time (or mtime when it predates the index), newest first.
func (s *Store) ListVersions(_ context.Context, projectID string) ([]VersionMeta, error) {
	dir, err := s.ResolveProjectDir(projectID)
	if err != nil {
		return nil, err
	}
	times := s.readIndex(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []VersionMeta{}, nil
		}
		return nil, err
	}
	out := make([]VersionMeta, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".cue") {
			continue
		}
		id := strings.TrimSuffix(name, ".cue")
		saved, ok := times[id]
		if !ok {
			if info, err := entry.Info(); err == nil {
				saved = info.ModTime()
			}
		}
		out = append(out, VersionMeta{Version: id, SavedAt: saved})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavedAt.After(out[j].SavedAt) })
	return out, nil
}

// ReadVersion returns the stored data.cue text of one of a project's versions by
// its content hash. The id is regex-validated before any path is built, so it can
// never traverse out of the versions dir.
func (s *Store) ReadVersion(_ context.Context, projectID, id string) (string, error) {
	dir, err := s.ResolveProjectDir(projectID)
	if err != nil {
		return "", err
	}
	if !versionIDPattern.MatchString(id) {
		return "", ErrInvalidVersionID
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".cue"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrVersionNotFound
		}
		return "", err
	}
	return string(data), nil
}
