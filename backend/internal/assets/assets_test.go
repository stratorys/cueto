// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package assets

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestSchemaMatchesRepo pins the embedded schema to the hand-owned cue/ module:
// every embedded file must byte-match its repo counterpart and every repo schema
// file must be embedded. data.cue is excluded deliberately - it is the repo's
// default project instance, not schema a standalone binary needs.
func TestSchemaMatchesRepo(t *testing.T) {
	assertTreesEqual(t, Schema(), "../../../cue", func(rel string) bool {
		return rel != "data.cue"
	})
}

// TestDemoMatchesRepo pins the embedded demo to examples/service-catalog, so the
// project seeded on first run is exactly the one the README documents.
func TestDemoMatchesRepo(t *testing.T) {
	assertTreesEqual(t, Demo(), "../../../examples/service-catalog", func(string) bool {
		return true
	})
}

func TestMaterializeSchemaWritesModule(t *testing.T) {
	dst := t.TempDir()
	if err := MaterializeSchema(dst); err != nil {
		t.Fatalf("MaterializeSchema: %v", err)
	}
	for _, rel := range []string{"cue.mod/module.cue", "diagram/diagram.cue", "knowledge/knowledge.cue"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Fatalf("materialized schema missing %s: %v", rel, err)
		}
	}
}

// assertTreesEqual checks the embedded tree and the repo directory hold the same
// files with the same bytes, ignoring repo files rejected by include.
func assertTreesEqual(t *testing.T, embedded fs.FS, repoDir string, include func(rel string) bool) {
	t.Helper()
	embeddedFiles := map[string]bool{}
	err := fs.WalkDir(embedded, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		embeddedFiles[path] = true
		want, rerr := os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(path)))
		if rerr != nil {
			t.Errorf("embedded %s has no repo counterpart: %v", path, rerr)
			return nil
		}
		got, gerr := fs.ReadFile(embedded, path)
		if gerr != nil {
			return gerr
		}
		if !bytes.Equal(got, want) {
			t.Errorf("embedded %s differs from repo copy; re-copy it from %s", path, repoDir)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded: %v", err)
	}
	err = filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(repoDir, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if include(rel) && !embeddedFiles[rel] {
			t.Errorf("repo file %s is not embedded; copy it into the assets package", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
}
