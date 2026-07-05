package main

import (
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
	Port           string
	MaxBodyBytes   int64         // request body cap, bytes
	MaxOutputBytes int           // evaluated JSON cap, bytes
	EvalTimeout    time.Duration // per-request evaluation deadline
	MaxConcurrent  int           // concurrent evaluations before 429
}

// loadConfig reads configuration from the environment, applying safe defaults.
// CueDir is resolved to an absolute path so overlay and diagnostics paths are
// stable regardless of the working directory.
func loadConfig() (Config, error) {
	abs, err := filepath.Abs(envString("CUE_DIR", "../cue"))
	if err != nil {
		return Config{}, err
	}
	return Config{
		CueDir:         abs,
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
