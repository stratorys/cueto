// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Command backend serves CUE evaluation for the diagram app.
//
// The hand-owned schema.cue is loaded fresh from disk (CUE_DIR, default ../cue)
// and is never machine-written. The editable data.cue is supplied per request
// and overlaid on top, so the canvas only ever round-trips data.cue while
// schema.cue stays authoritative. All evaluation runs in-process under
// body-size, output-size, deadline, and concurrency bounds; see
// internal/config and internal/handlers.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/stratorys/cueto/backend/internal/authoring"
	"github.com/stratorys/cueto/backend/internal/config"
	"github.com/stratorys/cueto/backend/internal/evaluation"
	"github.com/stratorys/cueto/backend/internal/handlers"
	"github.com/stratorys/cueto/backend/internal/workspace"
)

func main() {
	// Load .env if present; real environment variables always take precedence
	// and a missing file is not an error (defaults in config.go apply).
	_ = godotenv.Load()

	// Default to release mode (quiet, no debug route dump). GIN_MODE overrides it,
	// e.g. GIN_MODE=debug for the route listing.
	if mode := os.Getenv("GIN_MODE"); mode != "" {
		gin.SetMode(mode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Load config: %v", err)
	}

	// Explicit server timeouts bound the connection layer that the body cap and
	// eval deadline do not: slow-client (slowloris) reads and stuck writes.
	// WriteTimeout must exceed the eval deadline or long evaluations get cut off
	// mid-response.
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handlers.NewRouter(evaluation.New(cfg), workspace.New(cfg), authoring.New(), cfg),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      cfg.EvalTimeout + 10*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Serve until a termination signal, then drain in-flight requests so running
	// evaluations finish (or hit their own deadline) instead of being cut off.
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Serve: %v", err)
		}
	}()
	log.Printf("Listening on :%s, schema dir %s", cfg.Port, cfg.CueDir)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	stop()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Shutdown: %v", err)
	}
}
