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
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/stratorys/cueto/backend/internal/domain"
)

// maxHistoryCommits caps how many commits History returns, bounding the response
// the same way the evaluation output cap bounds eval. A file with a longer history
// is truncated to its most recent commits.
const maxHistoryCommits = 100

// History returns the commits that touched the file at scope, newest first, read
// from git and never mutating it. A workspace that is not a git repository yields
// an empty history rather than an error, so the panel degrades gracefully. The
// scope is validated and confined to the workspace root, then resolved to a path
// relative to the git worktree root (the workspace may be a subdirectory of the
// repo), which is what git filters on.
func (r *Repo) History(_ context.Context, scope string) ([]domain.HistoryEntry, error) {
	gitRepo, relPath, err := r.openAndRel(scope)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return []domain.HistoryEntry{}, nil
		}
		return nil, err
	}

	// A freshly initialized repo has no commits yet (an unborn HEAD), so there is no
	// history to walk. Treat it like a non-repo and return an empty list rather than
	// surfacing "reference not found".
	if _, err := gitRepo.Head(); err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return []domain.HistoryEntry{}, nil
		}
		return nil, err
	}

	iter, err := gitRepo.Log(&git.LogOptions{FileName: &relPath, Order: git.LogOrderCommitterTime})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	out := make([]domain.HistoryEntry, 0, maxHistoryCommits)
	err = iter.ForEach(func(commit *object.Commit) error {
		out = append(out, domain.HistoryEntry{
			Version: commit.Hash.String(),
			Label:   subject(commit.Message),
			At:      commit.Author.When.UTC(),
		})
		if len(out) >= maxHistoryCommits {
			return errStopIter
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopIter) {
		return nil, err
	}
	return out, nil
}

// FileAt returns the file at scope at a given version. An empty version reads the
// current working-tree file (how the client loads a buffer and obtains its base
// token); a non-empty version is a full git commit hash and reads the blob at that
// commit. Content is bounded by the output cap.
func (r *Repo) FileAt(_ context.Context, scope, version string) (string, error) {
	target, ok := r.resolve(scope)
	if !ok {
		return "", ErrInvalidPath
	}

	if version == "" {
		body, err := os.ReadFile(target)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", ErrFileNotFound
			}
			return "", err
		}
		if r.maxOutputBytes > 0 && len(body) > r.maxOutputBytes {
			return "", ErrOutputTooLarge
		}
		return string(body), nil
	}

	if !commitPattern.MatchString(version) {
		return "", ErrInvalidCommit
	}
	gitRepo, relPath, err := r.openAndRel(scope)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return "", ErrCommitNotFound
		}
		return "", err
	}
	commit, err := gitRepo.CommitObject(plumbing.NewHash(version))
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return "", ErrCommitNotFound
		}
		return "", err
	}
	file, err := commit.File(relPath)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return "", ErrFileNotFound
		}
		return "", err
	}
	if r.maxOutputBytes > 0 && int(file.Size) > r.maxOutputBytes {
		return "", ErrOutputTooLarge
	}
	contents, err := file.Contents()
	if err != nil {
		return "", err
	}
	return contents, nil
}

// openAndRel validates scope, opens the git repository containing the workspace
// (walking up to the .git), and returns the file path relative to the worktree
// root, which is what git log and commit lookups filter on. The workspace may be a
// subdirectory of the repo, so the path carries that prefix. Symlinks are resolved
// on both roots before the relative computation, so a symlinked temp root (as on
// macOS, where /var is /private/var) does not corrupt the path.
func (r *Repo) openAndRel(scope string) (*git.Repository, string, error) {
	if !domain.ValidEditableName(scope) {
		return nil, "", ErrInvalidPath
	}
	gitRepo, err := git.PlainOpenWithOptions(r.dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, "", err
	}
	wt, err := gitRepo.Worktree()
	if err != nil {
		return nil, "", err
	}
	root, err := filepath.EvalSymlinks(wt.Filesystem.Root())
	if err != nil {
		return nil, "", err
	}
	dir, err := filepath.EvalSymlinks(r.dir)
	if err != nil {
		return nil, "", err
	}
	prefix, err := filepath.Rel(root, dir)
	if err != nil {
		return nil, "", err
	}
	return gitRepo, filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(scope))), nil
}

// subject is the first line of a commit message, the human label for a version.
func subject(message string) string {
	if i := strings.IndexByte(message, '\n'); i >= 0 {
		return strings.TrimSpace(message[:i])
	}
	return strings.TrimSpace(message)
}

// errStopIter halts ForEach once the commit cap is reached without treating the
// early exit as a real error.
var errStopIter = errors.New("stop")
