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
	"github.com/stratorys/cueto/backend/internal/domain"
	"github.com/stratorys/cueto/backend/internal/workspace"
)

// Save validates the data and, when valid, stores it as a new immutable version.
// It answers 200 with {ok:true, version:"<hash>"} or 400 with diagnostics. The
// two concerns are composed here: the evaluation service validates, and only on a
// clean result does the workspace service persist, so an invalid diagram is never
// written.
func (h *handlers) Save(c *gin.Context) {
	var req dataRequest
	if !bindJSON(c, &req) {
		return
	}
	files := []domain.File{{Name: "data.cue", Content: req.Data}}
	diags, err := h.eval.Vet(c.Request.Context(), h.source(files))
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	version, err := h.store.SaveVersion(c.Request.Context(), c.Param("pid"), req.Data)
	if err != nil {
		writeProjectError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "version": version})
}

// ListVersions returns a project's saved versions newest-first as {versions:[...]}.
func (h *handlers) ListVersions(c *gin.Context) {
	versions, err := h.store.ListVersions(c.Request.Context(), c.Param("pid"))
	if err != nil {
		writeProjectError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// ReadVersion returns one of a project's versions as {version, data}. A malformed
// id is 400; an unknown (but well-formed) id is 404.
func (h *handlers) ReadVersion(c *gin.Context) {
	id := c.Param("id")
	data, err := h.store.ReadVersion(c.Request.Context(), c.Param("pid"), id)
	if err != nil {
		switch {
		case errors.Is(err, workspace.ErrInvalidVersionID):
			c.JSON(http.StatusBadRequest, gin.H{
				"diagnostics": []diag.Diagnostic{{Message: "invalid version id", Kind: diag.KindInternal}},
			})
		case errors.Is(err, workspace.ErrVersionNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"diagnostics": []diag.Diagnostic{{Message: "version not found", Kind: diag.KindInternal}},
			})
		default:
			writeProjectError(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"version": id, "data": data})
}

// Seed returns the on-disk seed data.cue as {data}, the mount-time fallback when
// no saved version exists. A missing seed file is 404.
func (h *handlers) Seed(c *gin.Context) {
	data, err := h.store.ReadSeed()
	if err != nil {
		if errors.Is(err, workspace.ErrSeedNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"diagnostics": []diag.Diagnostic{{Message: "seed data.cue not found", Kind: diag.KindInternal}},
			})
			return
		}
		writeOpError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}
