// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/stratorys/cueto/backend/internal/domain"
	"github.com/stratorys/cueto/backend/internal/knowledge"
)

func (h *handlers) knowledgeProject(c *gin.Context, files []domain.File) (knowledge.ProjectRef, bool) {
	dir, ok := h.projectDir(c)
	if !ok {
		return knowledge.ProjectRef{}, false
	}
	if h.runtime == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "knowledge runtime unavailable"})
		return knowledge.ProjectRef{}, false
	}
	overlay := map[string][]byte{}
	for _, f := range files {
		overlay[f.Name] = []byte(f.Content)
	}
	return knowledge.ProjectRef{ModuleDir: dir, Overlay: overlay}, true
}

func knowledgeError(c *gin.Context, err error) {
	var d *knowledge.DiagnosticError
	if errors.As(err, &d) {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": d.Diagnostics})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}
func (h *handlers) KnowledgeCatalog(c *gin.Context) {
	p, ok := h.knowledgeProject(c, nil)
	if !ok {
		return
	}
	v, e := h.runtime.Catalog(c, p)
	if e != nil {
		knowledgeError(c, e)
		return
	}
	c.JSON(http.StatusOK, v)
}
func (h *handlers) KnowledgeDescribe(c *gin.Context) {
	p, ok := h.knowledgeProject(c, nil)
	if !ok {
		return
	}
	v, e := h.runtime.Describe(c, p, c.Param("domain"))
	if e != nil {
		knowledgeError(c, e)
		return
	}
	c.JSON(http.StatusOK, v)
}
func (h *handlers) KnowledgeGet(c *gin.Context) {
	p, ok := h.knowledgeProject(c, nil)
	if !ok {
		return
	}
	v, e := h.runtime.Get(c, p, c.Param("domain"), c.Param("key"))
	if e != nil {
		knowledgeError(c, e)
		return
	}
	c.Data(http.StatusOK, "application/json", v)
}
func (h *handlers) KnowledgeQuery(c *gin.Context) {
	var q knowledge.Query
	if !bindJSON(c, &q) {
		return
	}
	p, ok := h.knowledgeProject(c, nil)
	if !ok {
		return
	}
	v, e := h.runtime.Query(c, p, q)
	if e != nil {
		knowledgeError(c, e)
		return
	}
	c.JSON(http.StatusOK, v)
}
func (h *handlers) KnowledgeEval(c *gin.Context) {
	var body struct {
		Input json.RawMessage `json:"input"`
	}
	if !bindJSON(c, &body) {
		return
	}
	p, ok := h.knowledgeProject(c, nil)
	if !ok {
		return
	}
	v, e := h.runtime.Eval(c, p, knowledge.EvalRequest{Evaluation: c.Param("name"), Input: body.Input})
	if e != nil {
		knowledgeError(c, e)
		return
	}
	c.JSON(http.StatusOK, v)
}
func (h *handlers) KnowledgeProvenance(c *gin.Context) {
	p, ok := h.knowledgeProject(c, nil)
	if !ok {
		return
	}
	v, e := h.runtime.Provenance(c, p, c.Query("name"))
	if e != nil {
		knowledgeError(c, e)
		return
	}
	c.JSON(http.StatusOK, v)
}
func (h *handlers) KnowledgeHealth(c *gin.Context) {
	p, ok := h.knowledgeProject(c, nil)
	if !ok {
		return
	}
	v, e := h.runtime.Health(c, p)
	if e != nil {
		knowledgeError(c, e)
		return
	}
	c.JSON(http.StatusOK, v)
}
