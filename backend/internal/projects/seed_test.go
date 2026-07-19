// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package projects

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	git "github.com/go-git/go-git/v5"
)

func demoTree() fstest.MapFS {
	return fstest.MapFS{
		"cue.mod/module.cue": &fstest.MapFile{Data: []byte("module: \"example.com/demo\"\nlanguage: version: \"v0.17.0\"\n")},
		"catalog.cue":        &fstest.MapFile{Data: []byte("package main\n")},
	}
}

func TestSeedWritesTreeAndCommits(t *testing.T) {
	root := t.TempDir()
	m := New(root)

	p, err := m.Seed("demo", demoTree())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if p.ID != "demo" {
		t.Fatalf("id = %q, want demo", p.ID)
	}
	dir := filepath.Join(root, "demo")
	for _, rel := range []string{"cue.mod/module.cue", "catalog.cue"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("seeded file missing %s: %v", rel, err)
		}
	}
	repository, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("open git: %v", err)
	}
	if _, err := repository.Head(); err != nil {
		t.Fatalf("head after seed: %v", err)
	}
	if ps, err := m.List(); err != nil || len(ps) != 1 || ps[0].ID != "demo" {
		t.Fatalf("list after seed = %+v, %v", ps, err)
	}
}

func TestSeedRefusesExistingProject(t *testing.T) {
	root := t.TempDir()
	m := New(root)
	if _, err := m.Seed("demo", demoTree()); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if _, err := m.Seed("demo", demoTree()); !errors.Is(err, ErrExists) {
		t.Fatalf("second seed err = %v, want ErrExists", err)
	}
}

func TestSeedRejectsInvalidID(t *testing.T) {
	m := New(t.TempDir())
	if _, err := m.Seed("../escape", demoTree()); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
}
