// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRootPrefersXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	root, err := DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	if root != filepath.Join("/tmp/xdg-data", "cueto") {
		t.Fatalf("root = %q, want $XDG_DATA_HOME/cueto", root)
	}
}

func TestDefaultRootFallsBackToDotCueto(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	root, err := DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	if filepath.Base(root) != ".cueto" {
		t.Fatalf("root = %q, want ~/.cueto", root)
	}
}

func TestEnsureCreatesProjectsDir(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "cueto"))
	if err := h.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	info, err := os.Stat(h.ProjectsDir())
	if err != nil || !info.IsDir() {
		t.Fatalf("projects dir missing after Ensure: %v", err)
	}
}

func TestLoadConfigMissingFileIsEmpty(t *testing.T) {
	h := New(t.TempDir())
	cfg, err := h.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg != (Config{}) {
		t.Fatalf("cfg = %+v, want zero", cfg)
	}
}

func TestLoadConfigReadsFields(t *testing.T) {
	h := New(t.TempDir())
	content := "port: 9000\nmaxConcurrent: 8\n"
	if err := os.WriteFile(filepath.Join(h.Root(), "config.cue"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := h.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Port != 9000 || cfg.MaxConcurrent != 8 {
		t.Fatalf("cfg = %+v, want port 9000, maxConcurrent 8", cfg)
	}
	if cfg.MaxBodyBytes != 0 {
		t.Fatalf("unset field = %d, want zero", cfg.MaxBodyBytes)
	}
}

func TestLoadConfigRejectsInvalid(t *testing.T) {
	h := New(t.TempDir())
	for name, content := range map[string]string{
		"bad type":     "port: \"nope\"\n",
		"out of range": "port: 0\n",
		"unknown key":  "prot: 9000\n",
	} {
		if err := os.WriteFile(filepath.Join(h.Root(), "config.cue"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := h.LoadConfig(); err == nil {
			t.Fatalf("%s: LoadConfig accepted %q", name, content)
		} else if !strings.Contains(err.Error(), "config.cue") {
			t.Fatalf("%s: error %q does not name config.cue", name, err)
		}
	}
}

func TestSelectionRoundTrip(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "cueto"))
	projects := t.TempDir()
	if got := h.Selection(projects); got != "" {
		t.Fatalf("Selection before write = %q, want empty", got)
	}
	if err := h.SetSelection(projects, "acme"); err != nil {
		t.Fatalf("SetSelection: %v", err)
	}
	if got := h.Selection(projects); got != "acme" {
		t.Fatalf("Selection = %q, want acme", got)
	}
	other := t.TempDir()
	if err := h.SetSelection(other, "demo"); err != nil {
		t.Fatalf("SetSelection other root: %v", err)
	}
	if got := h.Selection(projects); got != "acme" {
		t.Fatalf("Selection after other root write = %q, want acme", got)
	}
}

func TestReadStateCorruptFileErrors(t *testing.T) {
	h := New(t.TempDir())
	if err := os.WriteFile(filepath.Join(h.Root(), "state.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.ReadState(); err == nil {
		t.Fatal("ReadState accepted corrupt json")
	}
}
