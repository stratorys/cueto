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
	"github.com/stratorys/cueto/backend/internal/projects"
)

// createProjectRequest is the body for creating a project: a display name that is
// slugified into the project id (also the directory name under the root).
type createProjectRequest struct {
	Name string `json:"name"`
}

// ListProjects returns the projects under the root as {projects:[{id,name}]}, each
// a git repo plus a CUE module. This is the picker's data source.
func (h *handlers) ListProjects(c *gin.Context) {
	ps, err := h.projects.List()
	if err != nil {
		writeOpError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"projects": ps})
}

// CreateProject creates a project by git-initializing a fresh directory under the
// root, scaffolding a minimal module, and making one initial commit. An empty or
// unusable name is 400; a name that collides with an existing non-empty project is
// 409, so a project is never written over.
func (h *handlers) CreateProject(c *gin.Context) {
	var req createProjectRequest
	if !bindJSON(c, &req) {
		return
	}
	p, err := h.projects.Create(req.Name)
	if err != nil {
		switch {
		case errors.Is(err, projects.ErrInvalidName):
			c.JSON(http.StatusBadRequest, gin.H{
				"diagnostics": []diag.Diagnostic{{Message: "invalid project name", Kind: diag.KindParse}},
			})
		case errors.Is(err, projects.ErrExists):
			c.JSON(http.StatusConflict, gin.H{
				"diagnostics": []diag.Diagnostic{{Message: "a project with that name already exists", Kind: diag.KindInternal}},
			})
		default:
			writeOpError(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"project": p})
}
