// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handlers hold the Evaluator seam and the schema dir needed to scrub host paths
// from any diagnostics built at this layer.
type handlers struct {
	eval   Evaluator
	cueDir string
}

type dataRequest struct {
	Data string `json:"data"`
	// Editable file set for multi-file packages. When empty, Data is treated as a
	// single legacy data.cue, so older single-file clients keep working.
	Files []File `json:"files"`
	// Optional imported infra facts (from /import/*). When present, /vet also
	// reports drift between the diagram and this live topology.
	Facts string `json:"facts"`
}

// files returns the editable set: the explicit Files, or a single data.cue built
// from Data when Files is empty (the single-file compatibility path).
func (r dataRequest) files() []File {
	if len(r.Files) > 0 {
		return r.Files
	}
	return []File{{Name: "data.cue", Content: r.Data}}
}

type sourceRequest struct {
	Source string `json:"source"`
}

// Eval returns the concrete diagram JSON, or 400 with structured diagnostics.
func (h *handlers) Eval(c *gin.Context) {
	var req dataRequest
	if !bindJSON(c, &req) {
		return
	}
	out, hints, prov, diags, err := h.eval.Eval(c.Request.Context(), req.files())
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	c.JSON(http.StatusOK, gin.H{"diagram": json.RawMessage(out), "hints": hints, "provenance": prov})
}

// Vet reports validation diagnostics. Keeping the existing contract it answers
// 200 with {ok:false, diagnostics:[...]} for invalid input and {ok:true} on pass.
func (h *handlers) Vet(c *gin.Context) {
	var req dataRequest
	if !bindJSON(c, &req) {
		return
	}
	diags, err := h.eval.Vet(c.Request.Context(), req.files(), req.Facts)
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusOK, gin.H{"ok": false, "diagnostics": diags})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ImportCompose parses docker-compose YAML into #Actual facts (JSON) for a drift
// check. It answers 200 {facts:"..."} or 400 with kindImport diagnostics.
func (h *handlers) ImportCompose(c *gin.Context) {
	var req sourceRequest
	if !bindJSON(c, &req) {
		return
	}
	facts, diags, err := h.eval.ImportCompose(req.Source)
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	c.JSON(http.StatusOK, gin.H{"facts": facts})
}

// Save validates the data and, when valid, stores it as a new immutable version.
// It answers 200 with {ok:true, version:"<hash>"} or 400 with diagnostics.
func (h *handlers) Save(c *gin.Context) {
	var req dataRequest
	if !bindJSON(c, &req) {
		return
	}
	version, diags, err := h.eval.Save(c.Request.Context(), req.Data)
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "version": version})
}

// ListVersions returns the saved versions newest-first as {versions:[...]}.
func (h *handlers) ListVersions(c *gin.Context) {
	versions, err := h.eval.ListVersions(c.Request.Context())
	if err != nil {
		writeOpError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// ReadVersion returns one version's stored data.cue as {version, data}. A
// malformed id is 400; an unknown (but well-formed) id is 404.
func (h *handlers) ReadVersion(c *gin.Context) {
	id := c.Param("id")
	data, err := h.eval.ReadVersion(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidVersionID):
			c.JSON(http.StatusBadRequest, gin.H{
				"diagnostics": []Diagnostic{{Message: "invalid version id", Kind: kindInternal}},
			})
		case errors.Is(err, errVersionNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"diagnostics": []Diagnostic{{Message: "version not found", Kind: kindInternal}},
			})
		default:
			writeOpError(c, err)
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
		if errors.Is(err, errSeedNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"diagnostics": []Diagnostic{{Message: "seed data.cue not found", Kind: kindInternal}},
			})
			return
		}
		writeOpError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// Rewrite splices canvas edits into one editable file and returns the new text
// as {content}. A syntax error (in the file or a supplied body) is 400.
func (h *handlers) Rewrite(c *gin.Context) {
	var op RewriteOp
	if !bindJSON(c, &op) {
		return
	}
	content, diags, err := h.eval.Rewrite(op)
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content})
}

// Format runs cue fmt over the provided source.
func (h *handlers) Format(c *gin.Context) {
	var req sourceRequest
	if !bindJSON(c, &req) {
		return
	}
	formatted, err := h.eval.Format(req.Source)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diagnosticsFrom(err, h.cueDir, kindParse)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"formatted": formatted})
}

// bindJSON decodes the body, translating an over-limit body into 413 and any
// other decode failure into 400. It returns false when it has already written
// the response.
func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"diagnostics": []Diagnostic{{Message: "request body too large", Kind: kindInternal}},
			})
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"diagnostics": []Diagnostic{{Message: "invalid request body", Kind: kindParse}},
		})
		return false
	}
	return true
}

// writeOpError maps operational evaluator errors to status codes. These are not
// tied to a source position and their messages are fixed, leaking nothing.
func writeOpError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errTimeout):
		c.JSON(http.StatusGatewayTimeout, gin.H{
			"diagnostics": []Diagnostic{{Message: "evaluation timed out", Kind: kindInternal}},
		})
	case errors.Is(err, errOutputTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"diagnostics": []Diagnostic{{Message: "evaluation output too large", Kind: kindInternal}},
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"diagnostics": []Diagnostic{{Message: "internal error", Kind: kindInternal}},
		})
	}
}
