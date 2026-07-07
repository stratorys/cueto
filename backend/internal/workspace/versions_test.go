// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stratorys/cueto/backend/internal/config"
)

// countFiles returns the number of non-directory entries under dir, treating a
// missing dir as zero.
func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read %s: %v", dir, err)
	}
	n := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			n++
		}
	}
	return n
}

// TestSaveVersionWritesBlobAndManifest pins the on-disk shape of a save: the file
// body lands content-addressed under blobs/ at its sha256, the version is the
// manifest bound to that blob, and reading it back is byte-identical.
func TestSaveVersionWritesBlobAndManifest(t *testing.T) {
	const data = "package main\n\ndiagram: {}\n"
	ctx := context.Background()
	dataDir := t.TempDir()
	w := New(config.Config{DataDir: dataDir, CueDir: t.TempDir()})

	proj, err := w.CreateProject(ctx, "demo", "blank")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	version, err := w.SaveVersion(ctx, proj.ID, data)
	if err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}

	vdir := filepath.Join(dataDir, proj.ID, "versions")
	if got := countFiles(t, filepath.Join(vdir, "blobs")); got != 1 {
		t.Fatalf("blobs = %d, want 1", got)
	}
	// The blob is keyed by the content hash; the manifest is keyed by the version id.
	sum := sha256.Sum256([]byte(data))
	blobHash := hex.EncodeToString(sum[:])
	if _, err := os.Stat(filepath.Join(vdir, "blobs", blobHash)); err != nil {
		t.Fatalf("blob not stored at its content hash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vdir, "manifests", version)); err != nil {
		t.Fatalf("manifest not stored at its version id: %v", err)
	}

	got, err := w.ReadVersion(ctx, proj.ID, version)
	if err != nil {
		t.Fatalf("ReadVersion: %v", err)
	}
	if got != data {
		t.Fatalf("ReadVersion = %q, want %q", got, data)
	}
}

// TestSaveVersionDedups pins idempotency: saving identical content twice yields
// one id, one blob, one manifest, and a single index line.
func TestSaveVersionDedups(t *testing.T) {
	const data = "package main\n\ndiagram: nodes: {}\n"
	ctx := context.Background()
	dataDir := t.TempDir()
	w := New(config.Config{DataDir: dataDir, CueDir: t.TempDir()})

	proj, err := w.CreateProject(ctx, "demo", "blank")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	first, err := w.SaveVersion(ctx, proj.ID, data)
	if err != nil {
		t.Fatalf("first SaveVersion: %v", err)
	}
	second, err := w.SaveVersion(ctx, proj.ID, data)
	if err != nil {
		t.Fatalf("second SaveVersion: %v", err)
	}
	if first != second {
		t.Fatalf("identical content produced different ids: %q vs %q", first, second)
	}

	vdir := filepath.Join(dataDir, proj.ID, "versions")
	if got := countFiles(t, filepath.Join(vdir, "blobs")); got != 1 {
		t.Fatalf("blobs = %d, want 1", got)
	}
	if got := countFiles(t, filepath.Join(vdir, "manifests")); got != 1 {
		t.Fatalf("manifests = %d, want 1", got)
	}

	index, err := os.ReadFile(filepath.Join(vdir, "index.jsonl"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	lines := 0
	for _, line := range strings.Split(strings.TrimSpace(string(index)), "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	if lines != 1 {
		t.Fatalf("index has %d lines, want 1", lines)
	}
}

// TestReadVersionReturnsRemovedFieldTextAsIs pins the Phase 0 decision: saved
// versions are readable as text as-is; the store never schema-validates on read
// (nor on write - validation is the transport's concern). The content below uses
// fields removed from the trimmed schema (role, region, policies). Re-evaluating
// it through the CUE engine would fail unification, but reading the stored blob
// must return the original bytes verbatim. If a future change makes reads validate,
// this breaks loudly.
func TestReadVersionReturnsRemovedFieldTextAsIs(t *testing.T) {
	const data = `package main

diagram: {
	policies: ["security"]
	nodes: gw: {
		id:     "gw"
		type:   "process"
		label:  "Gateway"
		role:   "gateway"
		region: "eu-west-1"
	}
}
`
	ctx := context.Background()
	w := New(config.Config{DataDir: t.TempDir(), CueDir: t.TempDir()})

	proj, err := w.CreateProject(ctx, "demo", "blank")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	version, err := w.SaveVersion(ctx, proj.ID, data)
	if err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}

	versions, err := w.ListVersions(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != version {
		t.Fatalf("versions = %+v, want the saved id %q", versions, version)
	}

	got, err := w.ReadVersion(ctx, proj.ID, version)
	if err != nil {
		t.Fatalf("ReadVersion: %v", err)
	}
	if got != data {
		t.Fatalf("ReadVersion returned altered text:\n--- got ---\n%s\n--- want ---\n%s", got, data)
	}
}
