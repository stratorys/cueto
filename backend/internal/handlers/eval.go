// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/diag"
)

// Eval returns the default view's diagram JSON plus the names of every discovered
// view, or 400 with structured diagnostics. A knowledge-only module is a success
// with an empty view list and an empty diagram, distinct from an error. Provenance
// is derived by the authoring concern from the same file set, so the response still
// carries the node/edge->file attribution the canvas needs.
func (h *handlers) Eval(c *gin.Context) {
	var req dataRequest
	if !bindJSON(c, &req) {
		return
	}
	files := req.files()
	out, views, hints, diags, err := h.eval.Eval(c.Request.Context(), h.source(files))
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	diagram := json.RawMessage(out)
	if out == nil {
		diagram = json.RawMessage("{}")
	}
	prov := h.authoring.ProvenanceFor(files)
	c.JSON(http.StatusOK, gin.H{"diagram": diagram, "views": views, "hints": hints, "provenance": prov})
}

// EvalExpr backs /repl. With editor files it evaluates Source as an expression
// against the live diagram (EvalQuery); without them it evaluates a standalone
// snippet (EvalExpr). It answers 200 {result:<json>} on success, or 400 with
// diagnostics on a compile/concreteness error. Nothing is persisted; the input
// never joins the file set, the schema, or a saved version.
func (h *handlers) EvalExpr(c *gin.Context) {
	var req sourceRequest
	if !bindJSON(c, &req) {
		return
	}
	var (
		out   json.RawMessage
		diags []diag.Diagnostic
		err   error
	)
	if len(req.Files) > 0 {
		out, diags, err = h.eval.EvalQuery(c.Request.Context(), h.source(req.Files), req.Source)
	} else {
		out, diags, err = h.eval.EvalExpr(c.Request.Context(), req.Source)
	}
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": json.RawMessage(out)})
}

// ReplKeys backs /repl/keys: the dotted identifier field paths of every top-level
// data field in the overlaid editor file set, for the REPL's autocomplete over the
// whole data (not just the diagram). 200 {keys:[...]} on success, 400 with
// diagnostics when the diagram is invalid/incomplete. Nothing is persisted.
func (h *handlers) ReplKeys(c *gin.Context) {
	var req sourceRequest
	if !bindJSON(c, &req) {
		return
	}
	keys, diags, err := h.eval.Keys(c.Request.Context(), h.source(req.Files))
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	c.JSON(http.StatusOK, gin.H{"keys": keys})
}

// Vet reports validation diagnostics. Keeping the existing contract it answers
// 200 with {ok:false, diagnostics:[...]} for invalid input and {ok:true} on pass.
func (h *handlers) Vet(c *gin.Context) {
	var req dataRequest
	if !bindJSON(c, &req) {
		return
	}
	diags, err := h.eval.Vet(c.Request.Context(), h.source(req.files()))
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

// Format runs cue fmt over the provided source.
func (h *handlers) Format(c *gin.Context) {
	var req sourceRequest
	if !bindJSON(c, &req) {
		return
	}
	formatted, err := h.authoring.Format(req.Source)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diag.From(err, h.cueDir, diag.KindParse)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"formatted": formatted})
}
