// Command check is the CI entrypoint for architecture validation. It evaluates a
// diagram (data.cue) against the schema, its opted-in policy packs, and - when
// infra facts are supplied - drift against the live topology, then exits nonzero
// if anything fails. `cue vet ./...` covers the pure-CUE policy gate; this tool
// additionally does drift (which needs facts overlaid) and emits machine-readable
// JSON for CI annotations.
//
//	check -cue ../cue -data data.cue [-facts facts.json] [-json]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

// driftHarnessCUE mirrors the overlay in ../../evaluator.go: it unifies the
// imported facts with infra.#Actual and computes driftReport. Kept in sync by
// hand since the two binaries do not share a package.
const driftHarnessCUE = `package diagram

import (
	"list"
	"github.com/stratorys/cue-diagram/infra"
)

actual: infra.#Actual & (%s)

driftReport: {
	_expected: [for e in diagram.edges {"\(diagram.nodes[e.source].label)->\(diagram.nodes[e.target].label)"}]
	_actual: [for l in actual.links {"\(l.source)->\(l.target)"}]
	missing: [for x in _expected if !list.Contains(_actual, x) {x}]
	extra: [for a in _actual if !list.Contains(_expected, a) {a}]
}
`

type finding struct {
	Kind    string `json:"kind"` // "policy" | "drift"
	Rule    string `json:"rule,omitempty"`
	Message string `json:"message"`
}

func main() {
	cueDir := flag.String("cue", "../cue", "CUE module directory")
	dataPath := flag.String("data", "", "path to the diagram's data.cue")
	factsPath := flag.String("facts", "", "path to imported infra facts JSON (enables drift)")
	asJSON := flag.Bool("json", false, "emit findings as JSON")
	flag.Parse()

	if *dataPath == "" {
		fmt.Fprintln(os.Stderr, "check: -data is required")
		os.Exit(2)
	}
	findings, err := run(*cueDir, *dataPath, *factsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check: %v\n", err)
		os.Exit(2)
	}
	report(findings, *asJSON)
	if len(findings) > 0 {
		os.Exit(1)
	}
}

func run(cueDir, dataPath, factsPath string) ([]finding, error) {
	cueDir, err := filepath.Abs(cueDir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(dataPath)
	if err != nil {
		return nil, err
	}
	overlay := map[string]load.Source{
		absJoin(cueDir, "data.cue"): load.FromString(string(data)),
	}
	if factsPath != "" {
		facts, err := os.ReadFile(factsPath)
		if err != nil {
			return nil, err
		}
		overlay[absJoin(cueDir, "facts_overlay.cue")] = load.FromString(fmt.Sprintf(driftHarnessCUE, string(facts)))
	}

	instances := load.Instances([]string{"."}, &load.Config{Dir: cueDir, Overlay: overlay})
	if len(instances) == 0 {
		return nil, fmt.Errorf("no CUE instance loaded")
	}
	if err := instances[0].Err; err != nil {
		return nil, err
	}
	value := cuecontext.New().BuildInstance(instances[0])
	if err := value.Err(); err != nil {
		return nil, err
	}
	if err := value.LookupPath(cue.ParsePath("diagram")).Validate(cue.Concrete(true)); err != nil {
		return nil, err
	}

	var findings []finding
	findings = append(findings, policyFindings(value)...)
	findings = append(findings, driftFindings(value)...)
	return findings, nil
}

func policyFindings(root cue.Value) []finding {
	report := root.LookupPath(cue.ParsePath("policyReport"))
	if !report.Exists() {
		return nil
	}
	packs, err := report.Fields()
	if err != nil {
		return nil
	}
	var out []finding
	for packs.Next() {
		items, err := packs.Value().List()
		if err != nil {
			continue
		}
		for items.Next() {
			var v struct {
				Rule    string `json:"rule"`
				Message string `json:"message"`
			}
			if err := items.Value().Decode(&v); err != nil {
				continue
			}
			out = append(out, finding{Kind: "policy", Rule: v.Rule, Message: v.Message})
		}
	}
	return out
}

func driftFindings(root cue.Value) []finding {
	report := root.LookupPath(cue.ParsePath("driftReport"))
	if !report.Exists() {
		return nil
	}
	var out []finding
	for _, section := range []struct{ field, message string }{
		{"missing", "diagram edge %s is not present in the live infra"},
		{"extra", "live infra has %s, missing from the diagram"},
	} {
		items, err := report.LookupPath(cue.ParsePath(section.field)).List()
		if err != nil {
			continue
		}
		for items.Next() {
			edge, err := items.Value().String()
			if err != nil {
				continue
			}
			out = append(out, finding{Kind: "drift", Message: fmt.Sprintf(section.message, edge)})
		}
	}
	return out
}

func report(findings []finding, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(findings)
		return
	}
	if len(findings) == 0 {
		fmt.Println("OK: diagram passes schema, policies, and drift.")
		return
	}
	for _, f := range findings {
		if f.Rule != "" {
			fmt.Printf("%s [%s] %s\n", f.Kind, f.Rule, f.Message)
		} else {
			fmt.Printf("%s %s\n", f.Kind, f.Message)
		}
	}
}

func absJoin(dir, name string) string {
	return filepath.Join(dir, name)
}
