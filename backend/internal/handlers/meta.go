// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CueMeta returns the static CUE reference (builtin functions and importable
// packages with their members) that backs the REPL's autocomplete and reference
// browser. The payload is version-static; the evaluator computes it once.
func (h *handlers) CueMeta(c *gin.Context) {
	c.JSON(http.StatusOK, h.eval.Introspect())
}
