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

	"github.com/stratorys/cueto/backend/internal/cueeval"
	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/store"
)

// Save validates the data and, when valid, stores it as a new immutable version.
// It answers 200 with {ok:true, version:"<hash>"} or 400 with diagnostics.
func (h *handlers) Save(c *gin.Context) {
	var req dataRequest
	if !bindJSON(c, &req) {
		return
	}
	version, diags, err := h.eval.Save(c.Request.Context(), c.Param("pid"), req.Data)
	if err != nil {
		writeProjectError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "version": version})
}

// ListVersions returns a project's saved versions newest-first as {versions:[...]}.
func (h *handlers) ListVersions(c *gin.Context) {
	versions, err := h.eval.ListVersions(c.Request.Context(), c.Param("pid"))
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
	data, err := h.eval.ReadVersion(c.Request.Context(), c.Param("pid"), id)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidVersionID):
			c.JSON(http.StatusBadRequest, gin.H{
				"diagnostics": []diag.Diagnostic{{Message: "invalid version id", Kind: diag.KindInternal}},
			})
		case errors.Is(err, store.ErrVersionNotFound):
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
	data, err := h.eval.ReadSeed(c.Request.Context())
	if err != nil {
		if errors.Is(err, cueeval.ErrSeedNotFound) {
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
