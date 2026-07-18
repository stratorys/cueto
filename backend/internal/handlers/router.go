// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package handlers is the HTTP transport: it wires routes and middleware onto
// the per-concern services (evaluation, workspace, authoring) and keeps every
// untrusted-input bound either in middleware here or inside a service deadline.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/config"
	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/evaluation"
	"github.com/stratorys/cueto/backend/internal/knowledge"
	"github.com/stratorys/cueto/backend/internal/projects"
)

// NewRouter wires middleware and routes. Every untrusted-input bound lives either
// in middleware here or inside a service deadline. Module-independent operations
// (config, introspection, format, rewrite, project list/create) are top-level;
// every operation that touches a module is scoped to /projects/:id, so the module
// root always comes from a resolved project. Git is the only history: saves write
// the real file and the history panel reads commits read-only.
func NewRouter(eval evalService, auth authoringService, cfg config.Config) *gin.Engine {
	r := gin.New()
	// Trust no proxies: this backend is reached directly, so client-supplied
	// X-Forwarded-For headers must not be believed.
	_ = r.SetTrustedProxies(nil)
	r.Use(gin.Recovery(), cors(), limitBody(cfg.MaxBodyBytes), limitConcurrency(cfg.MaxConcurrent))

	h := &handlers{
		eval:           eval,
		authoring:      auth,
		projects:       projects.New(cfg.ProjectsDir),
		cueDir:         cfg.CueDir,
		maxOutputBytes: cfg.MaxOutputBytes,
	}
	if engine, ok := eval.(*evaluation.Engine); ok {
		h.runtime = knowledge.NewRuntime(knowledge.New(engine))
	}

	// Module-independent operations.
	r.GET("/config", h.Config)
	r.GET("/cue/meta", h.CueMeta)
	r.POST("/format", h.Format)
	r.POST("/rewrite", h.Rewrite)
	r.GET("/projects", h.ListProjects)
	r.POST("/projects", h.CreateProject)

	// Project-scoped operations: evaluation and git-backed persistence, all rooted
	// at the resolved project module.
	r.POST("/projects/:id/eval", h.Eval)
	r.POST("/projects/:id/repl", h.EvalExpr)
	r.POST("/projects/:id/repl/keys", h.ReplKeys)
	r.POST("/projects/:id/vet", h.Vet)
	r.GET("/projects/:id/knowledge/catalog", h.KnowledgeCatalog)
	r.GET("/projects/:id/knowledge/domains/:domain", h.KnowledgeDescribe)
	r.GET("/projects/:id/knowledge/domains/:domain/:key", h.KnowledgeGet)
	r.POST("/projects/:id/knowledge/query", h.KnowledgeQuery)
	r.POST("/projects/:id/knowledge/eval/:name", h.KnowledgeEval)
	r.GET("/projects/:id/knowledge/provenance", h.KnowledgeProvenance)
	r.GET("/projects/:id/knowledge/health", h.KnowledgeHealth)
	r.GET("/projects/:id/tree", h.Tree)
	r.POST("/projects/:id/save", h.WorkspaceSave)
	r.GET("/projects/:id/file", h.WorkspaceFile)
	r.DELETE("/projects/:id/file", h.WorkspaceDeleteFile)
	r.GET("/projects/:id/history", h.WorkspaceHistory)
	return r
}

// cors is a permissive policy: any origin, and every method the API uses. PATCH and
// DELETE are listed so the browser's preflight allows project rename and file/project
// delete; without them those requests are blocked before they reach a handler.
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// limitBody caps the request body so a giant payload cannot exhaust memory while
// decoding. An over-limit read surfaces as *http.MaxBytesError during binding.
func limitBody(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// limitConcurrency bounds how many requests run at once. This is what makes the
// unkillable evaluation goroutines survivable: at most maxConcurrent can exist,
// so a flood of evaluation bombs rejects with 429 instead of spawning unbounded
// leaking goroutines.
func limitConcurrency(maxConcurrent int) gin.HandlerFunc {
	sem := make(chan struct{}, maxConcurrent)
	return func(c *gin.Context) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			c.Next()
		default:
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"diagnostics": []diag.Diagnostic{{Message: "server busy, retry shortly", Kind: diag.KindInternal}},
			})
		}
	}
}
