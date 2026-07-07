// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// gitWorkspace builds a git-backed workspace module (its own cue.mod plus the given
// files, all committed) and returns the dir and a workspace-mode router pointed at
// it. Committing means the history endpoints have something to read.
func gitWorkspace(t *testing.T, files map[string]string) (string, *gin.Engine) {
	t.Helper()
	dir := t.TempDir()
	gitRepo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}
	wt, err := gitRepo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	all := map[string]string{"cue.mod/module.cue": "module: \"example.com/m\"\nlanguage: version: \"v0.17.0\"\n"}
	for rel, content := range files {
		all[rel] = content
	}
	for rel, content := range all {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
		if _, err := wt.Add(rel); err != nil {
			t.Fatalf("add %s: %v", rel, err)
		}
	}
	sig := &object.Signature{Name: "Test", Email: "t@example.com", When: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	if _, err := wt.Commit("seed", &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	cfg := testConfig(t)
	cfg.WorkspaceDir = dir
	return dir, realRouter(t, cfg)
}

// knowledgeOnly vets cleanly (no diagram-shaped view) so a save reaches disk.
const knowledgeOnly = "package main\n\npeople: {a: {name: \"A\"}}\n"

func TestConfigReportsMode(t *testing.T) {
	_, wsRouter := gitWorkspace(t, map[string]string{"data.cue": knowledgeOnly})
	rec := getJSON(wsRouter, "/config")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Mode string `json:"mode"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Mode != "workspace" {
		t.Fatalf("mode = %q, want workspace", body.Mode)
	}
}

// wsSave posts a workspace save and returns the recorder.
func wsSave(router *gin.Engine, path, data, base string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(workspaceSaveRequest{Path: path, Data: data, BaseVersion: base})
	return postJSON(router, "/workspace/save", body)
}

func TestWorkspaceSaveWritesRealFile(t *testing.T) {
	dir, router := gitWorkspace(t, map[string]string{"data.cue": knowledgeOnly})

	// Load the working-tree file to obtain its base token, then overwrite it.
	fileRec := getJSON(router, "/workspace/file?path=data.cue")
	var loaded struct {
		Data    string `json:"data"`
		Version string `json:"version"`
	}
	_ = json.Unmarshal(fileRec.Body.Bytes(), &loaded)

	next := "package main\n\npeople: {a: {name: \"A\"}, b: {name: \"B\"}}\n"
	rec := wsSave(router, "data.cue", next, loaded.Version)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.OK || body.Version == "" {
		t.Fatalf("want ok:true with a version, got %q", rec.Body.String())
	}
	// The real file on disk carries the new content.
	got, err := os.ReadFile(filepath.Join(dir, "data.cue"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != next {
		t.Fatalf("on-disk content = %q, want the saved buffer", got)
	}
}

func TestWorkspaceSaveInvalidNotWritten(t *testing.T) {
	dir, router := gitWorkspace(t, map[string]string{"data.cue": knowledgeOnly})
	// A field conflict fails vet, so nothing is written.
	rec := wsSave(router, "data.cue", "package main\n\nx: 1\nx: 2\n", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
	if len(decodeDiags(t, rec)) == 0 {
		t.Fatal("want diagnostics for the invalid save")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "data.cue"))
	if string(got) != knowledgeOnly {
		t.Fatalf("invalid save must not modify the file, got %q", got)
	}
}

func TestWorkspaceSaveConflict(t *testing.T) {
	_, router := gitWorkspace(t, map[string]string{"data.cue": knowledgeOnly})
	// An empty base token against an existing file is a refuse-to-clobber conflict.
	rec := wsSave(router, "data.cue", knowledgeOnly, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body %q)", rec.Code, rec.Body.String())
	}
	var body struct {
		Conflict bool `json:"conflict"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Conflict {
		t.Fatalf("want conflict:true, got %q", rec.Body.String())
	}
}

func TestWorkspaceHistoryAndFileAtCommit(t *testing.T) {
	_, router := gitWorkspace(t, map[string]string{"data.cue": knowledgeOnly})

	histRec := getJSON(router, "/workspace/history?path=data.cue")
	if histRec.Code != http.StatusOK {
		t.Fatalf("history status = %d", histRec.Code)
	}
	var hist struct {
		Entries []struct {
			Version string `json:"version"`
			Label   string `json:"label"`
		} `json:"entries"`
	}
	_ = json.Unmarshal(histRec.Body.Bytes(), &hist)
	if len(hist.Entries) != 1 || hist.Entries[0].Label != "seed" {
		t.Fatalf("entries = %+v, want the one seed commit", hist.Entries)
	}

	fileRec := getJSON(router, "/workspace/file?path=data.cue&commit="+hist.Entries[0].Version)
	if fileRec.Code != http.StatusOK {
		t.Fatalf("file status = %d, body %q", fileRec.Code, fileRec.Body.String())
	}
	var file struct {
		Data string `json:"data"`
	}
	_ = json.Unmarshal(fileRec.Body.Bytes(), &file)
	if file.Data != knowledgeOnly {
		t.Fatalf("file at commit = %q, want the seeded content", file.Data)
	}
}
