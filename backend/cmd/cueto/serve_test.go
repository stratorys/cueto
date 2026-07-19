// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stratorys/cueto/backend/internal/assets"
)

// TestPrepareServeFirstRun exercises the whole standalone bootstrap on a fresh
// home: directories created, schema materialized, demo seeded, session resolving
// to the demo, knowledge served from it, and the embedded UI as fallback.
func TestPrepareServeFirstRun(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "cueto-home")
	router, cfg, err := prepareServe(homeDir, "", 0, "")
	if err != nil {
		t.Fatalf("prepareServe: %v", err)
	}
	if cfg.Port != "8091" {
		t.Fatalf("port = %s, want default 8091", cfg.Port)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "schema", "diagram", "diagram.cue")); err != nil {
		t.Fatalf("schema not materialized: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "projects", assets.DemoProjectID, "catalog.cue")); err != nil {
		t.Fatalf("demo not seeded: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session", nil))
	var session struct {
		CurrentProject string `json:"currentProject"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if session.CurrentProject != assets.DemoProjectID {
		t.Fatalf("currentProject = %q, want %s", session.CurrentProject, assets.DemoProjectID)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/"+assets.DemoProjectID+"/knowledge/catalog", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "blastRadius") {
		t.Fatalf("knowledge catalog = %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "cueto") {
		t.Fatalf("UI root = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/spa/route", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA fallback = %d, want 200 index.html", rec.Code)
	}
}

// TestPrepareServeSecondRunKeepsProjects reruns the bootstrap over an existing
// home and checks it neither reseeds nor duplicates anything.
func TestPrepareServeSecondRunKeepsProjects(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "cueto-home")
	if _, _, err := prepareServe(homeDir, "", 0, ""); err != nil {
		t.Fatalf("first run: %v", err)
	}
	marker := filepath.Join(homeDir, "projects", assets.DemoProjectID, "marker.cue")
	if err := os.WriteFile(marker, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	router, _, err := prepareServe(homeDir, "", 0, "")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("user file lost on second run: %v", err)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects", nil))
	var list struct {
		Projects []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode projects: %v", err)
	}
	if len(list.Projects) != 1 || list.Projects[0].ID != assets.DemoProjectID {
		t.Fatalf("projects after second run = %+v, want only the demo", list.Projects)
	}
}

// TestPrepareServeReadsConfigCue checks config.cue drives the port and that a
// flag beats it.
func TestPrepareServeReadsConfigCue(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "cueto-home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.cue"), []byte("port: 9000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, cfg, err := prepareServe(homeDir, "", 0, "")
	if err != nil {
		t.Fatalf("prepareServe: %v", err)
	}
	if cfg.Port != "9000" {
		t.Fatalf("port = %s, want 9000 from config.cue", cfg.Port)
	}
	_, cfg, err = prepareServe(homeDir, "", 9500, "")
	if err != nil {
		t.Fatalf("prepareServe with flag: %v", err)
	}
	if cfg.Port != "9500" {
		t.Fatalf("port = %s, want flag 9500 over config.cue", cfg.Port)
	}
}

// TestPrepareServeRejectsBadConfig ensures a config.cue typo fails startup with
// a diagnostic instead of being silently ignored.
func TestPrepareServeRejectsBadConfig(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "cueto-home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.cue"), []byte("prot: 9000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareServe(homeDir, "", 0, ""); err == nil {
		t.Fatal("prepareServe accepted invalid config.cue")
	}
}
