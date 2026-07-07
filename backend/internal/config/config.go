// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds the server tunables. Every bound is env-overridable so the
// hardening limits can be tightened in production without a rebuild, mirroring
// how CUE_DIR and PORT already work.
type Config struct {
	CueDir         string
	WorkspaceDir   string // the user's module root; Sources root here
	Port           string
	MaxBodyBytes   int64         // request body cap, bytes
	MaxOutputBytes int           // evaluated JSON cap, bytes
	EvalTimeout    time.Duration // per-request evaluation deadline
	MaxConcurrent  int           // concurrent evaluations before 429
}

// Load reads configuration from the environment, applying safe defaults.
// CueDir is resolved to an absolute path so overlay and diagnostics paths are
// stable regardless of the working directory. WORKSPACE_DIR is required and must
// be an existing directory (the user's module root); a missing or non-directory
// path fails fast rather than surfacing as a per-request evaluation error.
func Load() (Config, error) {
	cueDir, err := filepath.Abs(envString("CUE_DIR", "../cue"))
	if err != nil {
		return Config{}, err
	}

	rawWorkspace := envString("WORKSPACE_DIR", "")
	if rawWorkspace == "" {
		return Config{}, errors.New("WORKSPACE_DIR is required")
	}
	workspaceDir, err := filepath.Abs(rawWorkspace)
	if err != nil {
		return Config{}, err
	}
	info, statErr := os.Stat(workspaceDir)
	if statErr != nil || !info.IsDir() {
		return Config{}, fmt.Errorf("WORKSPACE_DIR (%s) is not a directory", workspaceDir)
	}

	return Config{
		CueDir:         cueDir,
		WorkspaceDir:   workspaceDir,
		Port:           envString("PORT", "8091"),
		MaxBodyBytes:   envInt64("MAX_BODY_BYTES", 1<<20),        // 1 MiB
		MaxOutputBytes: int(envInt64("MAX_OUTPUT_BYTES", 4<<20)), // 4 MiB
		EvalTimeout:    time.Duration(envInt64("EVAL_TIMEOUT_MS", 2000)) * time.Millisecond,
		MaxConcurrent:  int(envInt64("MAX_CONCURRENT", 4)),
	}, nil
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}
