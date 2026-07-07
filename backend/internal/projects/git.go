// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package projects

import (
	"os"
	"path/filepath"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// languageVersion is the CUE language version the scaffolded module declares. It
// matches the pinned cuelang.org/go version the engine evaluates against.
const languageVersion = "v0.17.0"

// scaffold writes a minimal, vocabulary-free module into dir: a cue.mod declaring
// a module path derived from the id, and an empty package main to open in the
// editor. It adds no schema and no example names, so a fresh project imposes
// nothing on the user's world.
func scaffold(dir, id string) error {
	files := map[string]string{
		filepath.Join(moduleMarker, "module.cue"): "module: \"example.com/" + id + "\"\nlanguage: version: \"" + languageVersion + "\"\n",
		"main.cue": "package main\n",
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// initCommit initializes a git repo in dir, stages the scaffold, and makes one
// commit. This is the only time cueto writes a project's git state: it gives the
// new project a baseline so its history is non-empty, and nothing touches git
// again.
func initCommit(dir string) error {
	repository, err := git.PlainInit(dir, false)
	if err != nil {
		return err
	}
	worktree, err := repository.Worktree()
	if err != nil {
		return err
	}
	if err := worktree.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}
	signature := gitIdentity()
	if _, err := worktree.Commit("Initialize project", &git.CommitOptions{Author: &signature, Committer: &signature}); err != nil {
		return err
	}
	return nil
}

// gitIdentity resolves the commit author from the user's own git config (global
// then system scope), falling back to a cueto identity when none is set. Using the
// user's identity keeps the scaffold commit consistent with commits they make
// themselves afterward.
func gitIdentity() object.Signature {
	name, email := "cueto", "cueto@localhost"
	for _, scope := range []gitconfig.Scope{gitconfig.GlobalScope, gitconfig.SystemScope} {
		cfg, err := gitconfig.LoadConfig(scope)
		if err != nil {
			continue
		}
		if cfg.User.Name != "" {
			name, email = cfg.User.Name, cfg.User.Email
			break
		}
		if cfg.Author.Name != "" {
			name, email = cfg.Author.Name, cfg.Author.Email
			break
		}
	}
	return object.Signature{Name: name, Email: email, When: time.Now()}
}
