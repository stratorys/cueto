// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Command backend serves CUE evaluation for the diagram app in development: it
// is configured through the environment (.env), pairs with the Vite dev server,
// and serves no UI itself. The packaged, self-contained equivalent is `cueto
// serve`, which embeds the UI and schema and needs no checkout.
//
// The hand-owned schema.cue is loaded fresh from disk (CUE_DIR, default ../cue)
// and is never machine-written. The editable data.cue is supplied per request
// and overlaid on top, so the canvas only ever round-trips data.cue while
// schema.cue stays authoritative. All evaluation runs in-process under
// body-size, output-size, deadline, and concurrency bounds; see
// internal/config and internal/handlers.
package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/stratorys/cueto/backend/internal/authoring"
	"github.com/stratorys/cueto/backend/internal/config"
	"github.com/stratorys/cueto/backend/internal/evaluation"
	"github.com/stratorys/cueto/backend/internal/handlers"
	"github.com/stratorys/cueto/backend/internal/home"
	"github.com/stratorys/cueto/backend/internal/server"
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

	// Selection state lives in the standard cueto home even for the env-driven dev
	// server, keyed by projects root, so dev and packaged serve never clobber each
	// other. A home that cannot resolve only disables persistence.
	var sel handlers.SelectionStore
	if root, homeErr := home.DefaultRoot(); homeErr == nil {
		sel = home.New(root)
	}

	router := handlers.NewRouter(evaluation.New(cfg.CueDir, cfg.EvalTimeout, cfg.MaxOutputBytes), authoring.New(), cfg, sel)
	log.Printf("Listening on :%s, schema dir %s, projects dir %s", cfg.Port, cfg.CueDir, cfg.ProjectsDir)
	if err := server.Run(router, cfg.Port, cfg.EvalTimeout); err != nil {
		log.Fatalf("Serve: %v", err)
	}
}
