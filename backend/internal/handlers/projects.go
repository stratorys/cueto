// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/workspace"
)

// ListProjects returns the registered projects as {projects:[...]}.
func (h *handlers) ListProjects(c *gin.Context) {
	projects, err := h.ws.ListProjects(c.Request.Context())
	if err != nil {
		writeOpError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// CreateProject registers a new project ({name, seed}) and returns its metadata.
func (h *handlers) CreateProject(c *gin.Context) {
	var req projectRequest
	if !bindJSON(c, &req) {
		return
	}
	meta, err := h.ws.CreateProject(c.Request.Context(), req.Name, req.Seed)
	if err != nil {
		writeProjectError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"project": meta})
}

// RenameProject updates a project's display name ({name}).
func (h *handlers) RenameProject(c *gin.Context) {
	var req projectRequest
	if !bindJSON(c, &req) {
		return
	}
	meta, err := h.ws.RenameProject(c.Request.Context(), c.Param("pid"), req.Name)
	if err != nil {
		writeProjectError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"project": meta})
}

// DeleteProject removes a project. Refusing to delete the last one is 409.
func (h *handlers) DeleteProject(c *gin.Context) {
	if err := h.ws.DeleteProject(c.Request.Context(), c.Param("pid")); err != nil {
		writeProjectError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// writeProjectError maps project-scoped errors to status codes: a malformed id is
// 400, an unknown project 404, deleting the last project 409; anything else falls
// through to the operational error path.
func writeProjectError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspace.ErrInvalidProjectID):
		c.JSON(http.StatusBadRequest, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "invalid project id", Kind: diag.KindInternal}},
		})
	case errors.Is(err, workspace.ErrProjectNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "project not found", Kind: diag.KindInternal}},
		})
	case errors.Is(err, workspace.ErrLastProject):
		c.JSON(http.StatusConflict, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "cannot delete the last project", Kind: diag.KindInternal}},
		})
	default:
		writeOpError(c, err)
	}
}
