// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/authoring"
	"github.com/stratorys/cueto/backend/internal/evaluation"
	"github.com/stratorys/cueto/backend/internal/home"
)

// scaffoldModule makes dir/id a minimal CUE module so the projects manager lists
// it, without needing git (session resolution never reads git).
func scaffoldModule(t *testing.T, root, id string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, id, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	module := "module: \"example.com/" + id + "\"\nlanguage: version: \"v0.17.0\"\n"
	if err := os.WriteFile(filepath.Join(root, id, "cue.mod", "module.cue"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sessionRouter builds a router over a temp projects root with a real home-backed
// selection store, returning both so tests can inspect persisted state.
func sessionRouter(t *testing.T, projectsRoot string) (*gin.Engine, *home.Home) {
	t.Helper()
	cfg := testConfig(t)
	cfg.ProjectsDir = projectsRoot
	h := home.New(filepath.Join(t.TempDir(), "cueto-home"))
	return NewRouter(evaluation.New(cfg.CueDir, cfg.EvalTimeout, cfg.MaxOutputBytes), authoring.New(), cfg, h), h
}

type sessionResponse struct {
	CurrentProject string `json:"currentProject"`
	Projects       []struct {
		ID string `json:"id"`
	} `json:"projects"`
}

func getSession(t *testing.T, router *gin.Engine) sessionResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/session", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /session = %d, body %s", rec.Code, rec.Body.String())
	}
	var resp sessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return resp
}

func TestSessionNoProjects(t *testing.T) {
	router, _ := sessionRouter(t, t.TempDir())
	resp := getSession(t, router)
	if resp.CurrentProject != "" || len(resp.Projects) != 0 {
		t.Fatalf("session = %+v, want empty", resp)
	}
}

func TestSessionOnlyProjectIsDefaultAndPersisted(t *testing.T) {
	root := t.TempDir()
	scaffoldModule(t, root, "acme")
	router, h := sessionRouter(t, root)
	resp := getSession(t, router)
	if resp.CurrentProject != "acme" {
		t.Fatalf("currentProject = %q, want acme", resp.CurrentProject)
	}
	if got := h.Selection(root); got != "acme" {
		t.Fatalf("persisted selection = %q, want acme", got)
	}
}

func TestSessionMultipleProjectsNoneSelected(t *testing.T) {
	root := t.TempDir()
	scaffoldModule(t, root, "acme")
	scaffoldModule(t, root, "beta")
	router, _ := sessionRouter(t, root)
	resp := getSession(t, router)
	if resp.CurrentProject != "" {
		t.Fatalf("currentProject = %q, want empty (onboarding)", resp.CurrentProject)
	}
	if len(resp.Projects) != 2 {
		t.Fatalf("projects = %+v, want 2", resp.Projects)
	}
}

func TestSessionUsesPersistedSelection(t *testing.T) {
	root := t.TempDir()
	scaffoldModule(t, root, "acme")
	scaffoldModule(t, root, "beta")
	router, h := sessionRouter(t, root)
	if err := h.SetSelection(root, "beta"); err != nil {
		t.Fatal(err)
	}
	if resp := getSession(t, router); resp.CurrentProject != "beta" {
		t.Fatalf("currentProject = %q, want beta", resp.CurrentProject)
	}
}

func TestSessionStaleSelectionFallsBack(t *testing.T) {
	root := t.TempDir()
	scaffoldModule(t, root, "acme")
	scaffoldModule(t, root, "beta")
	router, h := sessionRouter(t, root)
	if err := h.SetSelection(root, "gone"); err != nil {
		t.Fatal(err)
	}
	if resp := getSession(t, router); resp.CurrentProject != "" {
		t.Fatalf("currentProject = %q, want empty for stale selection", resp.CurrentProject)
	}
}

func TestSetSessionProject(t *testing.T) {
	root := t.TempDir()
	scaffoldModule(t, root, "acme")
	scaffoldModule(t, root, "beta")
	router, h := sessionRouter(t, root)

	body := []byte(`{"id":"beta"}`)
	req := httptest.NewRequest(http.MethodPost, "/session/project", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /session/project = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := h.Selection(root); got != "beta" {
		t.Fatalf("persisted selection = %q, want beta", got)
	}
	if resp := getSession(t, router); resp.CurrentProject != "beta" {
		t.Fatalf("currentProject after switch = %q, want beta", resp.CurrentProject)
	}
}

func TestSetSessionProjectUnknownIs404(t *testing.T) {
	root := t.TempDir()
	scaffoldModule(t, root, "acme")
	router, h := sessionRouter(t, root)
	for _, id := range []string{"ghost", "../escape", ""} {
		body, _ := json.Marshal(map[string]string{"id": id})
		req := httptest.NewRequest(http.MethodPost, "/session/project", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("POST id %q = %d, want 404", id, rec.Code)
		}
	}
	if got := h.Selection(root); got != "" {
		t.Fatalf("selection = %q, want empty after rejected writes", got)
	}
}
