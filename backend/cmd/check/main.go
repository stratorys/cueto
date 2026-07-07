// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Command check is the CI entrypoint for diagram validation. It evaluates a
// candidate diagram (data.cue) against the schema and exits nonzero if it fails
// to build or its rendered view is not concrete. `cue vet ./...` covers the
// pure-CUE gate; this tool overlays a candidate data.cue on the module so CI can
// validate the file without writing it to disk. It drives the same engine as the
// server, so it inherits whole-module load, view discovery, and the view
// concreteness gate rather than hardcoding a single top-level field.
//
//	check -cue ../cue -data data.cue
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/domain"
	"github.com/stratorys/cueto/backend/internal/evaluation"
)

// CLI evaluation bounds. Generous next to the server's per-request caps: a CI run
// is trusted, single-shot, and not a shared surface, so the deadline and output
// cap exist only to bound a pathological input, not to ration a live service.
const (
	checkTimeout        = 30 * time.Second
	checkMaxOutputBytes = 64 << 20
)

func main() {
	cueDir := flag.String("cue", "../cue", "CUE module directory")
	dataPath := flag.String("data", "", "path to the diagram's data.cue")
	flag.Parse()

	if *dataPath == "" {
		fmt.Fprintln(os.Stderr, "check: -data is required")
		os.Exit(2)
	}
	if err := run(*cueDir, *dataPath); err != nil {
		fmt.Fprintf(os.Stderr, "check: %v\n", err)
		os.Exit(1)
	}
}

func run(cueDir, dataPath string) error {
	cueDir, err := filepath.Abs(cueDir)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(dataPath)
	if err != nil {
		return err
	}

	engine := evaluation.New(cueDir, checkTimeout, checkMaxOutputBytes)
	src := evaluation.Source{
		Dir:     cueDir,
		Overlay: []domain.File{{Name: "data.cue", Content: string(data)}},
	}
	_, views, _, _, diags, err := engine.Eval(context.Background(), src)
	if err != nil {
		return err
	}
	if len(diags) > 0 {
		return errors.New(formatDiags(diags))
	}

	if len(views) == 0 {
		fmt.Println("OK: module is valid (no diagram view).")
	} else {
		fmt.Println("OK: diagram passes the schema.")
	}
	return nil
}

// formatDiags renders evaluation diagnostics as an indented, multi-line message
// so the CI log points at the offending line, matching what the editor surfaces.
func formatDiags(diags []diag.Diagnostic) string {
	var b strings.Builder
	b.WriteString("diagram does not pass the schema:")
	for _, d := range diags {
		if d.Line > 0 {
			fmt.Fprintf(&b, "\n  %d:%d: %s", d.Line, d.Column, d.Message)
		} else {
			fmt.Fprintf(&b, "\n  %s", d.Message)
		}
	}
	return b.String()
}
