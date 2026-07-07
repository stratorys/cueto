// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/authoring"
)

// Rewrite splices canvas edits into one editable file and returns the new text
// as {content}. A syntax error (in the file or a supplied body) is 400.
func (h *handlers) Rewrite(c *gin.Context) {
	var op authoring.RewriteOp
	if !bindJSON(c, &op) {
		return
	}
	content, diags, err := h.authoring.Rewrite(op)
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
