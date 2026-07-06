// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Command check is the CI entrypoint for diagram validation. It evaluates a
// diagram (data.cue) against the schema and exits nonzero if it fails to build
// or is not concrete. `cue vet ./...` covers the pure-CUE gate; this tool loads
// the same package with an overlaid data.cue so CI can validate a candidate file
// without writing it to disk.
//
//	check -cue ../cue -data data.cue
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
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
	fmt.Println("OK: diagram passes the schema.")
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
	overlay := map[string]load.Source{
		filepath.Join(cueDir, "data.cue"): load.FromString(string(data)),
	}

	instances := load.Instances([]string{"."}, &load.Config{Dir: cueDir, Overlay: overlay})
	if len(instances) == 0 {
		return fmt.Errorf("no CUE instance loaded")
	}
	if err := instances[0].Err; err != nil {
		return err
	}
	value := cuecontext.New().BuildInstance(instances[0])
	if err := value.Err(); err != nil {
		return err
	}
	return value.LookupPath(cue.ParsePath("diagram")).Validate(cue.Concrete(true))
}
