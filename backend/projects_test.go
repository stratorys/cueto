// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// reqJSON issues an arbitrary-method JSON request (POST helpers cover the common
// case; project rename/delete need PATCH/DELETE).
func reqJSON(router *gin.Engine, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func createProject(t *testing.T, router *gin.Engine, name, seed string) ProjectMeta {
	t.Helper()
	body, _ := json.Marshal(projectRequest{Name: name, Seed: seed})
	rec := reqJSON(router, http.MethodPost, "/projects", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("create %q status = %d, body %q", name, rec.Code, rec.Body.String())
	}
	var out struct {
		Project ProjectMeta `json:"project"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	return out.Project
}

func listProjects(t *testing.T, router *gin.Engine) []ProjectMeta {
	t.Helper()
	rec := getJSON(router, "/projects")
	if rec.Code != http.StatusOK {
		t.Fatalf("list projects status = %d", rec.Code)
	}
	var out struct {
		Projects []ProjectMeta `json:"projects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return out.Projects
}

func TestProjectCreateListRenameDelete(t *testing.T) {
	router := realRouter(t, testConfig(t))

	alpha := createProject(t, router, "Alpha System", "blank")
	if alpha.ID != "alpha-system" {
		t.Fatalf("id = %q, want alpha-system", alpha.ID)
	}
	beta := createProject(t, router, "Alpha System", "blank") // same name -> uniquified id
	if beta.ID != "alpha-system-2" {
		t.Fatalf("dup id = %q, want alpha-system-2", beta.ID)
	}

	// Default (bootstrapped) plus the two created.
	if got := listProjects(t, router); len(got) != 3 {
		t.Fatalf("projects = %d, want 3 (%+v)", len(got), got)
	}

	// Rename.
	body, _ := json.Marshal(projectRequest{Name: "Renamed"})
	rec := reqJSON(router, http.MethodPatch, "/projects/"+alpha.ID, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename status = %d, body %q", rec.Code, rec.Body.String())
	}
	for _, p := range listProjects(t, router) {
		if p.ID == alpha.ID && p.Name != "Renamed" {
			t.Fatalf("rename did not persist: %+v", p)
		}
	}

	// Delete one.
	rec = reqJSON(router, http.MethodDelete, "/projects/"+beta.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if got := listProjects(t, router); len(got) != 2 {
		t.Fatalf("after delete projects = %d, want 2", len(got))
	}

	// Deleting a missing project is 404.
	rec = reqJSON(router, http.MethodDelete, "/projects/does-not-exist", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d, want 404", rec.Code)
	}
}

func TestProjectDeleteLastRefused(t *testing.T) {
	router := realRouter(t, testConfig(t))
	// Only the bootstrapped default exists; deleting it must be refused (409).
	if got := listProjects(t, router); len(got) != 1 {
		t.Fatalf("projects = %d, want 1", len(got))
	}
	rec := reqJSON(router, http.MethodDelete, "/projects/"+defaultProjectID, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete last status = %d, want 409", rec.Code)
	}
}

func TestProjectVersionsIsolated(t *testing.T) {
	router := realRouter(t, testConfig(t))
	other := createProject(t, router, "Other", "blank")

	if rec := postJSON(router, "/projects/default/save", evalBody(t, validData)); rec.Code != http.StatusOK {
		t.Fatalf("save default status = %d, body %q", rec.Code, rec.Body.String())
	}

	countVersions := func(pid string) int {
		rec := getJSON(router, "/projects/"+pid+"/versions")
		if rec.Code != http.StatusOK {
			t.Fatalf("list %s status = %d", pid, rec.Code)
		}
		var out struct {
			Versions []VersionMeta `json:"versions"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return len(out.Versions)
	}

	if n := countVersions("default"); n != 1 {
		t.Fatalf("default versions = %d, want 1", n)
	}
	if n := countVersions(other.ID); n != 0 {
		t.Fatalf("other versions = %d, want 0 (save must not leak across projects)", n)
	}
}

func TestProjectSampleSeedWritesVersion(t *testing.T) {
	router := realRouter(t, testConfig(t))
	sampled := createProject(t, router, "Sampled", "sample")
	rec := getJSON(router, "/projects/"+sampled.ID+"/versions")
	var out struct {
		Versions []VersionMeta `json:"versions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	// The repo's seed cue/data.cue exists, so a "sample" project opens with one
	// version already written.
	if len(out.Versions) != 1 {
		t.Fatalf("sample-seeded versions = %d, want 1", len(out.Versions))
	}
}

func TestLegacyStoreMigratesToDefault(t *testing.T) {
	cfg := testConfig(t)
	// Pre-seed a legacy flat store: a loose version file + an index line, as written
	// before projects existed.
	hash := strings.Repeat("a", 64)
	if err := os.WriteFile(filepath.Join(cfg.VersionsDir, hash+".cue"), []byte("package diagram\n"), 0o644); err != nil {
		t.Fatalf("seed legacy version: %v", err)
	}
	indexLine := `{"version":"` + hash + `","savedAt":"2026-01-01T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(cfg.VersionsDir, "index.jsonl"), []byte(indexLine), 0o644); err != nil {
		t.Fatalf("seed legacy index: %v", err)
	}

	router := realRouter(t, cfg)
	// First project op bootstraps + migrates.
	if got := listProjects(t, router); len(got) != 1 || got[0].ID != defaultProjectID {
		t.Fatalf("projects = %+v, want a single default", got)
	}
	// The legacy version now lives under default and is listed with its indexed time.
	rec := getJSON(router, "/projects/default/versions")
	var out struct {
		Versions []VersionMeta `json:"versions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Versions) != 1 || out.Versions[0].Version != hash {
		t.Fatalf("migrated versions = %+v, want the legacy hash", out.Versions)
	}
	// The loose files are gone from the root.
	if _, err := os.Stat(filepath.Join(cfg.VersionsDir, hash+".cue")); !os.IsNotExist(err) {
		t.Fatalf("legacy version file should have moved out of the root")
	}
}
