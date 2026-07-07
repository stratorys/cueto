// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package repo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stratorys/cueto/backend/internal/domain"
)

func TestSaveCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, 1<<20)

	res, err := r.Save(context.Background(), domain.SaveRequest{Scope: "model/teams.cue", Data: "package m\n"})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if res.Conflict {
		t.Fatalf("new file must not conflict")
	}
	if res.Version != ContentHash("package m\n") {
		t.Fatalf("version = %q, want content hash", res.Version)
	}
	// The file lands at the nested path, parent dirs created.
	body, err := os.ReadFile(filepath.Join(dir, "model", "teams.cue"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(body) != "package m\n" {
		t.Fatalf("written content = %q", body)
	}
}

func TestSaveOverwriteWithMatchingToken(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, 1<<20)
	first, _ := r.Save(context.Background(), domain.SaveRequest{Scope: "a.cue", Data: "one\n"})

	res, err := r.Save(context.Background(), domain.SaveRequest{Scope: "a.cue", Data: "two\n", BaseVersion: first.Version})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if res.Conflict {
		t.Fatalf("matching token must not conflict")
	}
	body, _ := os.ReadFile(filepath.Join(dir, "a.cue"))
	if string(body) != "two\n" {
		t.Fatalf("content = %q, want overwritten", body)
	}
}

func TestSaveConflictOnStaleToken(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, 1<<20)
	r.Save(context.Background(), domain.SaveRequest{Scope: "a.cue", Data: "one\n"})
	// The file was changed out-of-band since the client loaded "one".
	if err := os.WriteFile(filepath.Join(dir, "a.cue"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("out-of-band write: %v", err)
	}

	res, err := r.Save(context.Background(), domain.SaveRequest{Scope: "a.cue", Data: "two\n", BaseVersion: ContentHash("one\n")})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !res.Conflict {
		t.Fatalf("stale token must conflict")
	}
	body, _ := os.ReadFile(filepath.Join(dir, "a.cue"))
	if string(body) != "changed\n" {
		t.Fatalf("conflicting save must not overwrite, got %q", body)
	}
}

func TestSaveConflictClobberingUntrackedFile(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, 1<<20)
	// A file exists but the client sent no token (thinks it is creating one).
	if err := os.WriteFile(filepath.Join(dir, "a.cue"), []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	res, err := r.Save(context.Background(), domain.SaveRequest{Scope: "a.cue", Data: "new\n"})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !res.Conflict {
		t.Fatalf("empty token against an existing file must conflict")
	}
}

func TestSaveConflictWhenLoadedFileVanished(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, 1<<20)
	// The client carries a token for a file that no longer exists.
	res, err := r.Save(context.Background(), domain.SaveRequest{Scope: "a.cue", Data: "x\n", BaseVersion: ContentHash("old\n")})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !res.Conflict {
		t.Fatalf("token for a vanished file must conflict")
	}
}

func TestSaveRejectsUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	r := New(dir, 1<<20)
	for _, scope := range []string{"../escape.cue", "/etc/passwd", "cue.mod/module.cue", "diagram/x.cue", "sub//x.cue", "no-extension"} {
		res, err := r.Save(context.Background(), domain.SaveRequest{Scope: scope, Data: "x\n"})
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("scope %q: err = %v (res %+v), want ErrInvalidPath", scope, err, res)
		}
	}
	// Nothing escaped the workspace root.
	if _, err := os.Stat("/etc/passwd.cueto"); err == nil {
		t.Fatal("unexpected file outside workspace")
	}
}
