// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"path/filepath"
	"testing"
)

func TestLoadWorkspaceDir(t *testing.T) {
	t.Run("unset errors", func(t *testing.T) {
		t.Setenv("WORKSPACE_DIR", "")
		if _, err := Load(); err == nil {
			t.Fatal("want error for missing WORKSPACE_DIR, got nil")
		}
	})

	t.Run("set resolves to absolute", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("WORKSPACE_DIR", dir)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		want, _ := filepath.Abs(dir)
		if cfg.WorkspaceDir != want {
			t.Fatalf("WorkspaceDir = %q, want %q", cfg.WorkspaceDir, want)
		}
	})

	t.Run("missing dir errors", func(t *testing.T) {
		t.Setenv("WORKSPACE_DIR", filepath.Join(t.TempDir(), "does-not-exist"))
		if _, err := Load(); err == nil {
			t.Fatal("want error for non-existent WORKSPACE_DIR, got nil")
		}
	})
}
