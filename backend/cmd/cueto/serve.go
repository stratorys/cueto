// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/assets"
	"github.com/stratorys/cueto/backend/internal/authoring"
	"github.com/stratorys/cueto/backend/internal/config"
	"github.com/stratorys/cueto/backend/internal/evaluation"
	"github.com/stratorys/cueto/backend/internal/handlers"
	"github.com/stratorys/cueto/backend/internal/home"
	"github.com/stratorys/cueto/backend/internal/projects"
	"github.com/stratorys/cueto/backend/internal/server"
)

// Serve defaults, overridable by config.cue in the home root and then by flags.
// They mirror the dev server's env defaults so both surfaces behave alike.
const (
	serveDefaultPort          = 8091
	serveDefaultBodyBytes     = 1 << 20
	serveDefaultOutputBytes   = 4 << 20
	serveDefaultEvalTimeoutMs = 2000
	serveDefaultMaxConcurrent = 4
	serveSchemaDirName        = "schema"
)

// runServe is the standalone, self-contained server: everything lives under the
// cueto home (config.cue, state.json, projects/, materialized schema/), the web
// UI and demo project ship inside the binary, and no environment variable names
// a directory or a project.
func runServe(args []string) error {
	fset := flag.NewFlagSet("serve", flag.ExitOnError)
	homeFlag := fset.String("home", "", "cueto home (default $XDG_DATA_HOME/cueto or ~/.cueto)")
	projectsFlag := fset.String("projects", "", "projects root (default <home>/projects)")
	portFlag := fset.Int("port", 0, "listen port (default config.cue port or 8091)")
	cueFlag := fset.String("cue", "", "diagram schema dir (default: embedded schema under <home>/schema)")
	if err := fset.Parse(args); err != nil {
		return err
	}

	router, cfg, err := prepareServe(*homeFlag, *projectsFlag, *portFlag, *cueFlag)
	if err != nil {
		return err
	}
	log.Printf("cueto serving on http://localhost:%s (projects %s)", cfg.Port, cfg.ProjectsDir)
	return server.Run(router, cfg.Port, cfg.EvalTimeout)
}

// prepareServe resolves the home, applies config.cue under the flag overrides,
// materializes the embedded schema, seeds the demo project into an empty projects
// root, and assembles the API router with the embedded web UI mounted as the
// fallback route. Split from runServe so tests can exercise everything up to the
// listening socket.
func prepareServe(homeDir, projectsDir string, port int, cueDir string) (*gin.Engine, config.Config, error) {
	if homeDir == "" {
		root, err := home.DefaultRoot()
		if err != nil {
			return nil, config.Config{}, err
		}
		homeDir = root
	}
	h := home.New(homeDir)
	if err := h.Ensure(); err != nil {
		return nil, config.Config{}, err
	}
	fileCfg, err := h.LoadConfig()
	if err != nil {
		return nil, config.Config{}, err
	}

	if port == 0 {
		port = fileCfg.Port
	}
	if port == 0 {
		port = serveDefaultPort
	}
	if projectsDir == "" {
		projectsDir = h.ProjectsDir()
	}
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		return nil, config.Config{}, err
	}
	if cueDir == "" {
		cueDir = filepath.Join(h.Root(), serveSchemaDirName)
		if err := assets.MaterializeSchema(cueDir); err != nil {
			return nil, config.Config{}, fmt.Errorf("materialize schema: %w", err)
		}
	}

	// First run: an empty projects root gets the demo project, so the first page
	// a new user sees is a rendered graph rather than an empty editor.
	manager := projects.New(projectsDir)
	existing, err := manager.List()
	if err != nil {
		return nil, config.Config{}, err
	}
	if len(existing) == 0 {
		if _, err := manager.Seed(assets.DemoProjectID, assets.Demo()); err != nil {
			return nil, config.Config{}, fmt.Errorf("seed demo project: %w", err)
		}
		log.Printf("seeded demo project %q into %s", assets.DemoProjectID, projectsDir)
	}

	cfg := config.Config{
		CueDir:         cueDir,
		ProjectsDir:    projectsDir,
		Port:           strconv.Itoa(port),
		MaxBodyBytes:   pickInt64(fileCfg.MaxBodyBytes, serveDefaultBodyBytes),
		MaxOutputBytes: pickInt(fileCfg.MaxOutputBytes, serveDefaultOutputBytes),
		EvalTimeout:    time.Duration(pickInt(fileCfg.EvalTimeoutMs, serveDefaultEvalTimeoutMs)) * time.Millisecond,
		MaxConcurrent:  pickInt(fileCfg.MaxConcurrent, serveDefaultMaxConcurrent),
	}

	gin.SetMode(gin.ReleaseMode)
	router := handlers.NewRouter(evaluation.New(cfg.CueDir, cfg.EvalTimeout, cfg.MaxOutputBytes), authoring.New(), cfg, h)
	router.NoRoute(uiHandler(assets.WebUI()))
	return router, cfg, nil
}

func pickInt(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}

func pickInt64(value, fallback int64) int64 {
	if value != 0 {
		return value
	}
	return fallback
}

// uiHandler serves the embedded web UI for any GET the API does not claim, with
// the single-page fallback: a path that names no built asset serves index.html
// so client-side routes and reloads land in the app.
func uiHandler(ui fs.FS) gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(ui))
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(ui, path); err != nil {
			c.Request.URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}
