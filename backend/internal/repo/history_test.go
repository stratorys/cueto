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
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// commitFile writes rel under the repo worktree, stages it, and commits, returning
// the commit hash. The signature time steps forward per commit so committer-time
// ordering is deterministic.
func commitFile(t *testing.T, wt *git.Worktree, root, rel, content, msg string, when time.Time) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	if _, err := wt.Add(rel); err != nil {
		t.Fatalf("add %s: %v", rel, err)
	}
	sig := &object.Signature{Name: "Test", Email: "t@example.com", When: when}
	hash, err := wt.Commit(msg, &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	return hash.String()
}

// newGitWorkspace builds a git repo in a temp dir and returns the dir plus its
// worktree, for tests that need real commits without a git binary.
func newGitWorkspace(t *testing.T) (string, *git.Worktree) {
	t.Helper()
	dir := t.TempDir()
	gitRepo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, err := gitRepo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	return dir, wt
}

func TestHistoryListsCommitsNewestFirst(t *testing.T) {
	dir, wt := newGitWorkspace(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	commitFile(t, wt, dir, "a.cue", "v1\n", "first", base)
	commitFile(t, wt, dir, "other.cue", "x\n", "unrelated", base.Add(time.Minute))
	third := commitFile(t, wt, dir, "a.cue", "v2\n", "second edit", base.Add(2*time.Minute))

	r := New(dir, 1<<20)
	entries, err := r.History(context.Background(), "a.cue")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (only commits touching a.cue)", len(entries))
	}
	if entries[0].Version != third {
		t.Fatalf("newest entry = %q, want the second edit %q", entries[0].Version, third)
	}
	if entries[0].Label != "second edit" {
		t.Fatalf("label = %q, want commit subject", entries[0].Label)
	}
	if entries[0].At.IsZero() {
		t.Fatalf("entry missing a timestamp")
	}
}

func TestFileAtWorkingTreeAndCommit(t *testing.T) {
	dir, wt := newGitWorkspace(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	first := commitFile(t, wt, dir, "a.cue", "v1\n", "first", base)
	commitFile(t, wt, dir, "a.cue", "v2\n", "second", base.Add(time.Minute))
	// An uncommitted working-tree edit on top of the last commit.
	if err := os.WriteFile(filepath.Join(dir, "a.cue"), []byte("v3-wip\n"), 0o644); err != nil {
		t.Fatalf("wip write: %v", err)
	}

	r := New(dir, 1<<20)

	working, err := r.FileAt(context.Background(), "a.cue", "")
	if err != nil {
		t.Fatalf("working-tree read: %v", err)
	}
	if working != "v3-wip\n" {
		t.Fatalf("working tree = %q, want the uncommitted edit", working)
	}

	old, err := r.FileAt(context.Background(), "a.cue", first)
	if err != nil {
		t.Fatalf("read at commit: %v", err)
	}
	if old != "v1\n" {
		t.Fatalf("content at first commit = %q, want v1", old)
	}
}

func TestHistoryEmptyWhenRepoHasNoCommits(t *testing.T) {
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.cue"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := New(dir, 1<<20)
	entries, err := r.History(context.Background(), "a.cue")
	if err != nil {
		t.Fatalf("history on a repo with no commits must not error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %d, want 0 (unborn HEAD)", len(entries))
	}
}

func TestHistoryEmptyWhenNotARepo(t *testing.T) {
	dir := t.TempDir() // no git init
	if err := os.WriteFile(filepath.Join(dir, "a.cue"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := New(dir, 1<<20)
	entries, err := r.History(context.Background(), "a.cue")
	if err != nil {
		t.Fatalf("history on non-repo must not error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %d, want 0 for a non-repo workspace", len(entries))
	}
}

func TestFileAtRejects(t *testing.T) {
	dir, wt := newGitWorkspace(t)
	commitFile(t, wt, dir, "a.cue", "v1\n", "first", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	r := New(dir, 1<<20)

	if _, err := r.FileAt(context.Background(), "../escape.cue", ""); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("traversal path err = %v, want ErrInvalidPath", err)
	}
	if _, err := r.FileAt(context.Background(), "a.cue", "not-a-hash"); !errors.Is(err, ErrInvalidCommit) {
		t.Fatalf("bad commit err = %v, want ErrInvalidCommit", err)
	}
	if _, err := r.FileAt(context.Background(), "missing.cue", ""); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("missing working file err = %v, want ErrFileNotFound", err)
	}
	// A well-formed but absent commit hash.
	absent := "0123456789abcdef0123456789abcdef01234567"
	if _, err := r.FileAt(context.Background(), "a.cue", absent); !errors.Is(err, ErrCommitNotFound) {
		t.Fatalf("absent commit err = %v, want ErrCommitNotFound", err)
	}
}

func TestFileAtOutputBound(t *testing.T) {
	dir, wt := newGitWorkspace(t)
	commitFile(t, wt, dir, "a.cue", "0123456789\n", "first", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	r := New(dir, 4) // tiny cap
	if _, err := r.FileAt(context.Background(), "a.cue", ""); !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("oversized working read err = %v, want ErrOutputTooLarge", err)
	}
}
