// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/diag"
)

// SelectionStore persists which project is current for a projects root. The home
// package implements it; a nil store disables persistence but keeps the
// only-project resolution, so the endpoints degrade rather than disappear.
type SelectionStore interface {
	Selection(projectsDir string) string
	SetSelection(projectsDir, id string) error
}

// setSessionRequest is the body for switching the current project.
type setSessionRequest struct {
	ID string `json:"id"`
}

// Session resolves the current project without any environment variable naming
// one: the persisted selection if it still resolves, else the only project (which
// is then persisted), else none - the frontend shows onboarding on none. The
// projects list rides along so the client bootstraps from a single request.
func (h *handlers) Session(c *gin.Context) {
	ps, err := h.projects.List()
	if err != nil {
		writeOpError(c, err)
		return
	}
	current := ""
	if h.selection != nil {
		if id := h.selection.Selection(h.projectsDir); id != "" {
			if _, ok := h.projects.Resolve(id); ok {
				current = id
			}
		}
	}
	if current == "" && len(ps) == 1 {
		current = ps[0].ID
		if h.selection != nil {
			// Best-effort: a failed persist only means the same resolution runs again
			// next time, so it must not fail the read.
			_ = h.selection.SetSelection(h.projectsDir, current)
		}
	}
	c.JSON(http.StatusOK, gin.H{"currentProject": current, "projects": ps})
}

// SetSessionProject persists the current project. An id that does not resolve to
// a project under the root is 404, so the state can never name a project that is
// not really there.
func (h *handlers) SetSessionProject(c *gin.Context) {
	var req setSessionRequest
	if !bindJSON(c, &req) {
		return
	}
	if _, ok := h.projects.Resolve(req.ID); !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "unknown project", Kind: diag.KindInternal}},
		})
		return
	}
	if h.selection != nil {
		if err := h.selection.SetSelection(h.projectsDir, req.ID); err != nil {
			writeOpError(c, err)
			return
		}
	}
	c.Status(http.StatusNoContent)
}
