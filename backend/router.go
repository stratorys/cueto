package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// newRouter wires middleware and routes. Handlers depend only on the Evaluator
// interface; every untrusted-input bound lives either in middleware here or
// inside the evaluator's deadline.
func newRouter(eval Evaluator, cfg Config) *gin.Engine {
	r := gin.New()
	// Trust no proxies: this backend is reached directly, so client-supplied
	// X-Forwarded-For headers must not be believed.
	_ = r.SetTrustedProxies(nil)
	r.Use(gin.Recovery(), cors(), limitBody(cfg.MaxBodyBytes), limitConcurrency(cfg.MaxConcurrent))

	h := &handlers{eval: eval, cueDir: cfg.CueDir}
	r.POST("/eval", h.Eval)
	r.POST("/vet", h.Vet)
	r.POST("/save", h.Save)
	r.POST("/format", h.Format)
	return r
}

// cors mirrors the previous permissive policy: any origin, POST + OPTIONS.
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
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
				"diagnostics": []Diagnostic{{Message: "server busy, retry shortly", Kind: kindInternal}},
			})
		}
	}
}
