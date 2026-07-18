// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Command cueto is the CI and command-line face of the engine. It runs the same
// evaluation the server and (later) the MCP adapter run, so the editor, CI, and an
// agent can never disagree about whether a module is valid. Three subcommands, each
// a thin wrapper over one engine operation:
//
//	cueto vet   -C <dir>              # pure-CUE validity of the whole module (Layer 1)
//	cueto check -C <dir>             # @file/@uri graph checks the compiler cannot do (Layer 2)
//	cueto graph -C <dir> [-view v]   # the discovered or inferred diagram as JSON
//
// vet and check read the module as committed on disk (no editor overlay): that is
// the CI gate. Each exits nonzero on any diagnostic so it drops straight into a CI
// step. Only graph consults the cueto-owned diagram schema (-cue), for view
// discovery and inference.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/evaluation"
	"github.com/stratorys/cueto/backend/internal/knowledge"
)

// CLI evaluation bounds. Generous next to the server's per-request caps: a CI run is
// trusted, single-shot, and not a shared surface, so the deadline and output cap
// exist only to bound a pathological input, not to ration a live service.
const (
	cliTimeout        = 30 * time.Second
	cliMaxOutputBytes = 64 << 20
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	command := os.Args[1]
	args := os.Args[2:]

	var err error
	switch command {
	case "vet":
		err = runVet(args)
	case "check":
		err = runCheck(args)
	case "graph":
		err = runGraph(args)
	case "catalog":
		err = runCatalog(args)
	case "describe":
		err = runDescribe(args)
	case "get":
		err = runGet(args)
	case "query":
		err = runQuery(args)
	case "eval":
		err = runEval(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "cueto: unknown command %q\n", command)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cueto %s: %v\n", command, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `cueto - evaluate and validate a CUE module

usage:
  cueto vet   -C <dir>              validate the whole module (Layer 1, pure CUE)
  cueto check -C <dir>             run @file/@uri graph checks (Layer 2)
  cueto graph -C <dir> [-view v]   print the discovered/inferred diagram as JSON
  cueto catalog -C <dir>            print the knowledge catalog as JSON
  cueto describe <domain> -C <dir>  describe one catalog domain
  cueto get <domain> <id> -C <dir>  print one domain record
  cueto query <query.json> -C <dir> run a safe knowledge query
  cueto eval <name> --input <json> -C <dir> run a named evaluation

flags:
  -C    module root directory (contains cue.mod); default "."
  -cue  cueto diagram schema directory; default "../cue" (graph only)
  -view discovered view to render (graph only)
`)
}

func runtimeFor(moduleDir, cueDir string) (*knowledge.CueRuntime, knowledge.ProjectRef, error) {
	engine, src, err := setup(moduleDir, cueDir, "")
	if err != nil {
		return nil, knowledge.ProjectRef{}, err
	}
	return knowledge.NewRuntime(knowledge.New(engine)), knowledge.ProjectRef{ModuleDir: src.Dir}, nil
}

func commandFlags(name string, args []string) (*flag.FlagSet, *string, *string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	dir := fs.String("C", ".", "module root directory")
	cueDir := fs.String("cue", "../cue", "cueto schema directory")
	if err := fs.Parse(normalizeFlags(args)); err != nil {
		return nil, nil, nil, err
	}
	return fs, dir, cueDir, nil
}

// normalizeFlags permits the documented `command positional -C dir` form even
// though Go's flag package otherwise stops parsing at the first positional arg.
func normalizeFlags(args []string) []string {
	flags, positional := []string{}, []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "-C" || args[i] == "-cue" || args[i] == "--input" {
			flags = append(flags, args[i])
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, args[i])
	}
	return append(flags, positional...)
}

func printJSON(value any) error {
	out, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		fmt.Println(string(out))
	}
	return err
}

func runCatalog(args []string) error {
	_, dir, cueDir, err := commandFlags("catalog", args)
	if err != nil {
		return err
	}
	runtime, project, err := runtimeFor(*dir, *cueDir)
	if err != nil {
		return err
	}
	result, err := runtime.Catalog(context.Background(), project)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runDescribe(args []string) error {
	fs, dir, cueDir, err := commandFlags("describe", args)
	if err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cueto describe <domain> -C <dir>")
	}
	runtime, project, err := runtimeFor(*dir, *cueDir)
	if err != nil {
		return err
	}
	result, err := runtime.Describe(context.Background(), project, fs.Arg(0))
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runGet(args []string) error {
	fs, dir, cueDir, err := commandFlags("get", args)
	if err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: cueto get <domain> <id> -C <dir>")
	}
	runtime, project, err := runtimeFor(*dir, *cueDir)
	if err != nil {
		return err
	}
	result, err := runtime.Get(context.Background(), project, fs.Arg(0), fs.Arg(1))
	if err != nil {
		return err
	}
	fmt.Println(string(result))
	return nil
}

func readJSONArg(name string) ([]byte, error) {
	if name == "-" {
		return os.ReadFile("/dev/stdin")
	}
	return os.ReadFile(name)
}

func runQuery(args []string) error {
	fs, dir, cueDir, err := commandFlags("query", args)
	if err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: cueto query <query.json> -C <dir>")
	}
	raw, err := readJSONArg(fs.Arg(0))
	if err != nil {
		return err
	}
	var query knowledge.Query
	if err := json.Unmarshal(raw, &query); err != nil {
		return err
	}
	runtime, project, err := runtimeFor(*dir, *cueDir)
	if err != nil {
		return err
	}
	result, err := runtime.Query(context.Background(), project, query)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runEval(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	dir := fs.String("C", ".", "module root directory")
	cueDir := fs.String("cue", "../cue", "cueto schema directory")
	input := fs.String("input", "", "input JSON file or - for stdin")
	if err := fs.Parse(normalizeFlags(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *input == "" {
		return errors.New("usage: cueto eval <name> --input <json> -C <dir>")
	}
	raw, err := readJSONArg(*input)
	if err != nil {
		return err
	}
	runtime, project, err := runtimeFor(*dir, *cueDir)
	if err != nil {
		return err
	}
	result, err := runtime.Eval(context.Background(), project, knowledge.EvalRequest{Evaluation: fs.Arg(0), Input: raw})
	if err != nil {
		return err
	}
	return printJSON(result)
}

// runVet validates the whole module and exits nonzero on any diagnostic. It never
// gates concreteness: an incomplete-but-valid module vets clean.
func runVet(args []string) error {
	fs := flag.NewFlagSet("vet", flag.ExitOnError)
	dir := fs.String("C", ".", "module root directory")
	cueDir := fs.String("cue", "../cue", "cueto diagram schema directory (unused by vet)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	engine, src, err := setup(*dir, *cueDir, "")
	if err != nil {
		return err
	}
	runtime := knowledge.NewRuntime(knowledge.New(engine))
	health, err := runtime.Health(context.Background(), knowledge.ProjectRef{ModuleDir: src.Dir, Package: src.Package})
	if err != nil {
		return err
	}
	if !health.Valid {
		return errors.New(formatDiags("module is not valid:", health.Diagnostics))
	}
	fmt.Println("OK: module is valid.")
	return nil
}

// runCheck runs the Layer-2 graph checks (referenced files exist, URIs resolve) and
// exits nonzero on any failure.
func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	dir := fs.String("C", ".", "module root directory")
	cueDir := fs.String("cue", "../cue", "cueto diagram schema directory (unused by check)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	engine, src, err := setup(*dir, *cueDir, "")
	if err != nil {
		return err
	}
	diags, err := engine.Check(context.Background(), src)
	if err != nil {
		return err
	}
	if len(diags) > 0 {
		return errors.New(formatDiags("module has broken references:", diags))
	}
	fmt.Println("OK: all references resolve.")
	return nil
}

// runGraph prints the discovered or inferred diagram as JSON. It consults the
// cueto-owned diagram schema (-cue) for view discovery and inference, and exits
// nonzero when the rendered view is invalid or incomplete.
func runGraph(args []string) error {
	fs := flag.NewFlagSet("graph", flag.ExitOnError)
	dir := fs.String("C", ".", "module root directory")
	cueDir := fs.String("cue", "../cue", "cueto diagram schema directory")
	view := fs.String("view", "", "discovered view to render")
	if err := fs.Parse(args); err != nil {
		return err
	}
	engine, src, err := setup(*dir, *cueDir, *view)
	if err != nil {
		return err
	}
	out, views, _, trace, legend, diags, err := engine.Eval(context.Background(), src)
	if err != nil {
		return err
	}
	if len(diags) > 0 {
		return errors.New(formatDiags("diagram does not pass the schema:", diags))
	}
	diagram := json.RawMessage("{}")
	if out != nil {
		diagram = json.RawMessage(out)
	}
	payload, err := json.MarshalIndent(map[string]interface{}{
		"views":   views,
		"diagram": diagram,
		"trace":   trace,
		"legend":  legend,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(payload))
	return nil
}

// setup resolves the module and schema directories to absolute paths and builds the
// engine and Source shared by every subcommand. The Source carries no overlay: the
// CLI evaluates the module as committed on disk, which is the CI gate.
func setup(moduleDir, cueDir, view string) (*evaluation.Engine, evaluation.Source, error) {
	absModule, err := filepath.Abs(moduleDir)
	if err != nil {
		return nil, evaluation.Source{}, err
	}
	absCue, err := filepath.Abs(cueDir)
	if err != nil {
		return nil, evaluation.Source{}, err
	}
	engine := evaluation.New(absCue, cliTimeout, cliMaxOutputBytes)
	return engine, evaluation.Source{Dir: absModule, View: view}, nil
}

// formatDiags renders diagnostics as an indented, multi-line message under a heading
// so the CI log points at the offending line, matching what the editor surfaces.
func formatDiags(heading string, diags []diag.Diagnostic) string {
	var b strings.Builder
	b.WriteString(heading)
	for _, d := range diags {
		if d.Line > 0 {
			fmt.Fprintf(&b, "\n  %d:%d: %s", d.Line, d.Column, d.Message)
		} else {
			fmt.Fprintf(&b, "\n  %s", d.Message)
		}
	}
	return b.String()
}
