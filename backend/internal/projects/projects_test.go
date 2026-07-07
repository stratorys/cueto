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

	git "github.com/go-git/go-git/v5"
)

func TestCreateInitializesGitModuleWithOneCommit(t *testing.T) {
	root := t.TempDir()
	m := New(root)

	p, err := m.Create("Acme Catalog")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID != "acme-catalog" {
		t.Fatalf("id = %q, want acme-catalog", p.ID)
	}

	dir := filepath.Join(root, "acme-catalog")
	for _, rel := range []string{"cue.mod/module.cue", "main.cue"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("scaffold missing %s: %v", rel, err)
		}
	}

	repository, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("open git: %v", err)
	}
	head, err := repository.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	commit, err := repository.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("commit object: %v", err)
	}
	if commit.Message != "Initialize project" {
		t.Fatalf("message = %q, want Initialize project", commit.Message)
	}
	if commit.NumParents() != 0 {
		t.Fatalf("parents = %d, want 0 (single initial commit)", commit.NumParents())
	}
}

func TestCreateRefusesExistingNonEmpty(t *testing.T) {
	m := New(t.TempDir())
	if _, err := m.Create("demo"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := m.Create("demo"); !errors.Is(err, ErrExists) {
		t.Fatalf("second create err = %v, want ErrExists", err)
	}
}

func TestCreateRejectsUnusableName(t *testing.T) {
	m := New(t.TempDir())
	for _, name := range []string{"   ", "!!!", ""} {
		if _, err := m.Create(name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("create(%q) err = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestListReturnsOnlyModulesSorted(t *testing.T) {
	root := t.TempDir()
	m := New(root)
	if _, err := m.Create("beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("alpha"); err != nil {
		t.Fatal(err)
	}
	// A stray non-module directory must be skipped, not listed.
	if err := os.MkdirAll(filepath.Join(root, "not-a-project"), 0o755); err != nil {
		t.Fatal(err)
	}

	ps, err := m.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ps) != 2 || ps[0].ID != "alpha" || ps[1].ID != "beta" {
		t.Fatalf("projects = %+v, want [alpha beta] sorted", ps)
	}
}

func TestResolveRejectsUnknownAndTraversal(t *testing.T) {
	root := t.TempDir()
	m := New(root)
	if _, err := m.Create("alpha"); err != nil {
		t.Fatal(err)
	}

	if _, ok := m.Resolve("alpha"); !ok {
		t.Fatal("resolve(alpha) not ok, want ok")
	}
	for _, bad := range []string{"missing", "..", "../x", "a/b", ".", "", "cue.mod"} {
		if _, ok := m.Resolve(bad); ok {
			t.Fatalf("resolve(%q) ok, want rejected", bad)
		}
	}
}
