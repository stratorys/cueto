// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stratorys/cueto/backend/internal/home"
)

// testHome routes home.DefaultRoot to a temp dir via XDG_DATA_HOME so resolution
// tests never touch the real ~/.cueto. Returns the home and its projects root.
func testHome(t *testing.T) *home.Home {
	t.Helper()
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	h := home.New(filepath.Join(data, "cueto"))
	if err := h.Ensure(); err != nil {
		t.Fatal(err)
	}
	return h
}

func addProject(t *testing.T, h *home.Home, id string) string {
	t.Helper()
	dir := filepath.Join(h.ProjectsDir(), id)
	if err := os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	module := "module: \"example.com/" + id + "\"\nlanguage: version: \"v0.17.0\"\n"
	if err := os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResolveModuleDirExplicitCWins(t *testing.T) {
	testHome(t)
	dir, err := resolveModuleDir("/some/dir", "ignored")
	if err != nil || dir != "/some/dir" {
		t.Fatalf("resolve = %q, %v; want /some/dir", dir, err)
	}
}

func TestResolveModuleDirCwdModule(t *testing.T) {
	testHome(t)
	module := t.TempDir()
	if err := os.MkdirAll(filepath.Join(module, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(module)
	dir, err := resolveModuleDir("", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved, _ := filepath.EvalSymlinks(dir); resolved != mustEval(t, module) {
		t.Fatalf("resolve = %q, want cwd module %q", dir, module)
	}
}

func TestResolveModuleDirProjectFlag(t *testing.T) {
	h := testHome(t)
	t.Chdir(t.TempDir())
	want := addProject(t, h, "acme")
	addProject(t, h, "beta")
	dir, err := resolveModuleDir("", "acme")
	if err != nil || dir != want {
		t.Fatalf("resolve = %q, %v; want %q", dir, err, want)
	}
	if _, err := resolveModuleDir("", "ghost"); err == nil || !strings.Contains(err.Error(), "unknown project") {
		t.Fatalf("unknown -p err = %v", err)
	}
}

func TestResolveModuleDirSelection(t *testing.T) {
	h := testHome(t)
	t.Chdir(t.TempDir())
	addProject(t, h, "acme")
	want := addProject(t, h, "beta")
	if err := h.SetSelection(h.ProjectsDir(), "beta"); err != nil {
		t.Fatal(err)
	}
	dir, err := resolveModuleDir("", "")
	if err != nil || dir != want {
		t.Fatalf("resolve = %q, %v; want selected %q", dir, err, want)
	}
}

func TestResolveModuleDirOnlyProject(t *testing.T) {
	h := testHome(t)
	t.Chdir(t.TempDir())
	want := addProject(t, h, "acme")
	dir, err := resolveModuleDir("", "")
	if err != nil || dir != want {
		t.Fatalf("resolve = %q, %v; want only project %q", dir, err, want)
	}
}

func TestResolveModuleDirNoProjects(t *testing.T) {
	testHome(t)
	t.Chdir(t.TempDir())
	if _, err := resolveModuleDir("", ""); err == nil || !strings.Contains(err.Error(), "no projects") {
		t.Fatalf("err = %v, want no-projects guidance", err)
	}
}

func TestResolveModuleDirMultipleUnselected(t *testing.T) {
	h := testHome(t)
	t.Chdir(t.TempDir())
	addProject(t, h, "acme")
	addProject(t, h, "beta")
	_, err := resolveModuleDir("", "")
	if err == nil || !strings.Contains(err.Error(), "acme") || !strings.Contains(err.Error(), "beta") {
		t.Fatalf("err = %v, want project ids listed", err)
	}
}

func TestResolveSchemaDirMaterializesEmbedded(t *testing.T) {
	testHome(t)
	dir, err := resolveSchemaDir("")
	if err != nil {
		t.Fatalf("resolveSchemaDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "diagram", "diagram.cue")); err != nil {
		t.Fatalf("embedded schema not materialized: %v", err)
	}
	if explicit, err := resolveSchemaDir("/my/cue"); err != nil || explicit != "/my/cue" {
		t.Fatalf("explicit -cue = %q, %v", explicit, err)
	}
}

func TestRunUsePersistsSelection(t *testing.T) {
	h := testHome(t)
	addProject(t, h, "acme")
	addProject(t, h, "beta")
	if err := runUse([]string{"beta"}); err != nil {
		t.Fatalf("use: %v", err)
	}
	if got := h.Selection(h.ProjectsDir()); got != "beta" {
		t.Fatalf("selection = %q, want beta", got)
	}
	if err := runUse([]string{"ghost"}); err == nil {
		t.Fatal("use accepted unknown project")
	}
}

func mustEval(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
