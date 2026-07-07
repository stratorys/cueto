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
	"github.com/stratorys/cueto/backend/internal/repo"
)

// NewRouter wires middleware and routes. Handlers depend only on the small
// per-concern service interfaces; every untrusted-input bound lives either in
// middleware here or inside a service deadline. Sources root at the user's
// workspace module (cfg.WorkspaceDir), and git is the only history: saves write
// the real file and the history panel reads commits read-only.
func NewRouter(eval evalService, auth authoringService, cfg config.Config) *gin.Engine {
	r := gin.New()
	// Trust no proxies: this backend is reached directly, so client-supplied
	// X-Forwarded-For headers must not be believed.
	_ = r.SetTrustedProxies(nil)
	r.Use(gin.Recovery(), cors(), limitBody(cfg.MaxBodyBytes), limitConcurrency(cfg.MaxConcurrent))

	ws := repo.New(cfg.WorkspaceDir, cfg.MaxOutputBytes)
	h := &handlers{
		eval:      eval,
		save:      ws,
		history:   ws,
		authoring: auth,
		moduleDir: cfg.WorkspaceDir,
		cueDir:    cfg.CueDir,
	}
	r.POST("/eval", h.Eval)
	r.POST("/repl", h.EvalExpr)
	r.POST("/repl/keys", h.ReplKeys)
	r.GET("/cue/meta", h.CueMeta)
	r.POST("/vet", h.Vet)
	r.POST("/format", h.Format)
	r.POST("/rewrite", h.Rewrite)
	r.GET("/config", h.Config)

	// Git is the only history: saves write the real file, and the panel reads
	// commits read-only. No project registry, no version store.
	r.POST("/workspace/save", h.WorkspaceSave)
	r.GET("/workspace/history", h.WorkspaceHistory)
	r.GET("/workspace/file", h.WorkspaceFile)
	return r
}

// cors mirrors the previous permissive policy: any origin, GET + POST + OPTIONS.
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
