// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package workspace

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
	"time"

	"github.com/stratorys/cueto/backend/internal/domain"
)

// versionIDPattern is the exact shape of a sha256-hex id, matching both a version
// id (the hash of a manifest) and a blob hash (the hash of a file body). Reads are
// rejected unless the id matches, so an id from the URL - or a blob reference read
// out of a manifest - can never escape the data dir via path traversal.
var versionIDPattern = regexp.MustCompile("^[a-f0-9]{64}$")

// dataFileName is the single file a saved version carries today. A version is a
// manifest of one entry named data.cue; the multi-file set arrives later.
const dataFileName = "data.cue"

// SaveVersion persists data as a new immutable version of a project and returns
// its id. It resolves (and validates) the project first, so a bad project id or a
// missing project surfaces before anything is written. Validating the data
// against the schema is the caller's concern: the transport runs the evaluation
// service before calling SaveVersion, so an invalid diagram is never persisted.
func (w *Workspace) SaveVersion(_ context.Context, projectID, data string) (string, error) {
	dir, err := w.ResolveProjectDir(projectID)
	if err != nil {
		return "", err
	}
	return w.writeVersion(dir, []byte(data))
}

// writeVersion stores data as an immutable version of the project rooted at
// projectDir and returns its id. The file body is written content-addressed to
// blobs/<hash>, and the version is the manifest binding data.cue to that blob;
// the version id is the hash of the manifest bytes. Writes go only under the data
// dir - never the CUE package dir - so the seed data.cue is untouched and version
// files never join the default `package main`. Identical content is idempotent: an
// existing blob and manifest are reused, not rewritten.
func (w *Workspace) writeVersion(projectDir string, data []byte) (string, error) {
	if w.dataDir == "" {
		return "", ErrNoDataDir
	}
	vdir := w.versionsSubdir(projectDir)
	blobsDir := filepath.Join(vdir, "blobs")
	manifestsDir := filepath.Join(vdir, "manifests")
	if err := os.MkdirAll(blobsDir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(manifestsDir, 0o755); err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	blobHash := hex.EncodeToString(sum[:])
	if err := writeBlob(blobsDir, blobHash, data); err != nil {
		return "", err
	}

	id, fresh, err := writeManifest(manifestsDir, blobHash)
	if err != nil {
		return "", err
	}
	// Record save order + timestamp only when the manifest was newly created;
	// re-saving an identical set reuses it, so a version is indexed exactly once.
	// The index is derived metadata: a failed append is not fatal, since the
	// manifest and blobs remain the source of truth.
	if fresh {
		_ = w.appendIndex(vdir, id)
	}
	return id, nil
}

// versionsSubdir is a project's immutable history: index.jsonl, content-addressed
// blobs, and the manifests that bind a version to its blobs.
func (w *Workspace) versionsSubdir(projectDir string) string {
	return filepath.Join(projectDir, "versions")
}

// writeBlob stores body content-addressed under blobsDir at its hash. O_EXCL makes
// creation atomic: concurrent writes of the same body race on the same name and all
// but one see ErrExist, which is success (blobs dedup across versions).
func writeBlob(blobsDir, hash string, body []byte) error {
	f, err := os.OpenFile(filepath.Join(blobsDir, hash), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// writeManifest writes the one-entry manifest binding data.cue to blobHash and
// returns its id (the sha256 hex of the canonical manifest bytes) and whether it
// was newly created. O_EXCL keeps identical sets idempotent: the same file set
// yields the same manifest bytes, so the same id, written at most once.
func writeManifest(manifestsDir, blobHash string) (id string, fresh bool, err error) {
	raw, err := json.Marshal(domain.Manifest{Entries: []domain.ManifestEntry{{Name: dataFileName, Blob: blobHash}}})
	if err != nil {
		return "", false, err
	}
	sum := sha256.Sum256(raw)
	id = hex.EncodeToString(sum[:])

	f, err := os.OpenFile(filepath.Join(manifestsDir, id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return id, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return "", false, err
	}
	if err := f.Close(); err != nil {
		return "", false, err
	}
	return id, true, nil
}

// indexPath is a version store's append-only log of save events (one JSON object
// per line), living alongside the blobs and manifests.
func (w *Workspace) indexPath(vdir string) string {
	return filepath.Join(vdir, "index.jsonl")
}

// appendIndex records one save event. Version ids carry no order or time, so this
// log is what lets ListVersions present true save order and timestamps.
func (w *Workspace) appendIndex(vdir, id string) error {
	line, err := json.Marshal(domain.Version{Version: id, SavedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(w.indexPath(vdir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// readIndex reads a version store's save-order log into a map of id -> first-save
// time. A missing index is not an error; such versions fall back to their manifest
// file mtime in ListVersions.
func (w *Workspace) readIndex(vdir string) map[string]time.Time {
	times := map[string]time.Time{}
	f, err := os.Open(w.indexPath(vdir))
	if err != nil {
		return times
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var meta domain.Version
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

// ListVersions enumerates a project's manifests and stamps each with its indexed
// save time (or manifest mtime when absent from the index), newest first.
func (w *Workspace) ListVersions(_ context.Context, projectID string) ([]domain.Version, error) {
	dir, err := w.ResolveProjectDir(projectID)
	if err != nil {
		return nil, err
	}
	vdir := w.versionsSubdir(dir)
	times := w.readIndex(vdir)
	entries, err := os.ReadDir(filepath.Join(vdir, "manifests"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []domain.Version{}, nil
		}
		return nil, err
	}
	out := make([]domain.Version, 0, len(entries))
	for _, entry := range entries {
		id := entry.Name()
		if entry.IsDir() || !versionIDPattern.MatchString(id) {
			continue
		}
		saved, ok := times[id]
		if !ok {
			if info, err := entry.Info(); err == nil {
				saved = info.ModTime()
			}
		}
		out = append(out, domain.Version{Version: id, SavedAt: saved})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavedAt.After(out[j].SavedAt) })
	return out, nil
}

// ReadVersion returns the stored data.cue text of one of a project's versions by
// its id. The id resolves a manifest, whose data.cue entry names the blob to read.
// Both the id and the blob reference are regex-validated before any path is built,
// so neither can traverse out of the data dir.
func (w *Workspace) ReadVersion(_ context.Context, projectID, id string) (string, error) {
	dir, err := w.ResolveProjectDir(projectID)
	if err != nil {
		return "", err
	}
	if !versionIDPattern.MatchString(id) {
		return "", ErrInvalidVersionID
	}
	vdir := w.versionsSubdir(dir)
	raw, err := os.ReadFile(filepath.Join(vdir, "manifests", id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrVersionNotFound
		}
		return "", err
	}
	var manifest domain.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", err
	}
	blob := manifestDataBlob(manifest)
	if !versionIDPattern.MatchString(blob) {
		return "", ErrVersionNotFound
	}
	body, err := os.ReadFile(filepath.Join(vdir, "blobs", blob))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrVersionNotFound
		}
		return "", err
	}
	return string(body), nil
}

// manifestDataBlob returns the blob hash of a manifest's data.cue entry, falling
// back to the sole entry of a one-file version. An empty result (no data.cue, or
// an ambiguous multi-entry set) is caught by the caller's hash validation.
func manifestDataBlob(m domain.Manifest) string {
	for _, e := range m.Entries {
		if e.Name == dataFileName {
			return e.Blob
		}
	}
	if len(m.Entries) == 1 {
		return m.Entries[0].Blob
	}
	return ""
}
