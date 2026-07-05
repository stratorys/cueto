// Command backend serves CUE evaluation for the diagram app.
//
// The hand-owned schema.cue is loaded fresh from disk (CUE_DIR, default ../cue)
// and is never machine-written. The editable data.cue is supplied per request
// and overlaid on top, so the canvas only ever round-trips data.cue while
// schema.cue stays authoritative. All evaluation runs in-process under
// body-size, output-size, deadline, and concurrency bounds; see config.go and
// router.go.
package main

import (
	"log"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env if present; real environment variables always take precedence
	// and a missing file is not an error (defaults in config.go apply).
	_ = godotenv.Load()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Load config: %v", err)
	}

	router := newRouter(newCueEvaluator(cfg), cfg)

	log.Printf("Listening on :%s, schema dir %s", cfg.Port, cfg.CueDir)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Serve: %v", err)
	}
}
