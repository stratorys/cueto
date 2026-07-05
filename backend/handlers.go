package main

import (
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
	out, diags, err := h.eval.Eval(c.Request.Context(), req.Data)
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	c.Data(http.StatusOK, "application/json", out)
}

// Vet reports validation diagnostics. Keeping the existing contract it answers
// 200 with {ok:false, diagnostics:[...]} for invalid input and {ok:true} on pass.
func (h *handlers) Vet(c *gin.Context) {
	var req dataRequest
	if !bindJSON(c, &req) {
		return
	}
	diags, err := h.eval.Vet(c.Request.Context(), req.Data)
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
