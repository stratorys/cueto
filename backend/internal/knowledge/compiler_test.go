package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cuelang.org/go/cue"

	"github.com/stratorys/cueto/backend/internal/evaluation"
)

func testModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"example.com/knowledge\"\nlanguage: version: \"v0.17.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCompileBuildsGenericValueAndCatalog(t *testing.T) {
	dir := testModule(t, map[string]string{
		"data.cue": "package main\n\n#Person: {name: string}\npeople: {marty: #Person & {name: \"Marty\"}}\n",
	})
	compiler := New(evaluation.New("", time.Second, 1<<20))

	got, err := compiler.Compile(context.Background(), CompileRequest{ModuleDir: dir})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !got.Health.Valid || len(got.Diagnostics) != 0 {
		t.Fatalf("health=%+v diagnostics=%+v, want clean compilation", got.Health, got.Diagnostics)
	}
	if !got.Value.LookupPath(cue.ParsePath("people.marty.name")).Exists() {
		t.Fatal("compiled value does not contain people.marty.name")
	}
	if len(got.Catalog.Entries) != 2 || got.Catalog.Entries[0].Name != "#Person" || got.Catalog.Entries[1].Name != "people" {
		t.Fatalf("catalog=%+v, want #Person and people", got.Catalog.Entries)
	}
}

func TestCompileOverlaysAndSelectsPackage(t *testing.T) {
	dir := testModule(t, map[string]string{
		"data.cue":    "package main\n\nroot: true\n",
		"sub/sub.cue": "package sub\n\nvalue: \"disk\"\n",
	})
	compiler := New(evaluation.New("", time.Second, 1<<20))
	request := CompileRequest{
		ModuleDir: dir,
		Package:   "sub",
		Overlay: map[string][]byte{
			"sub/overlay.cue": []byte("package sub\n\nextra: 42\n"),
		},
	}

	got, err := compiler.Compile(context.Background(), request)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !got.Value.LookupPath(cue.ParsePath("value")).Exists() || !got.Value.LookupPath(cue.ParsePath("extra")).Exists() {
		t.Fatalf("selected package value misses disk or overlay fields: %v", got.Value)
	}
	if got.Revision == revision(CompileRequest{ModuleDir: dir, Package: "sub"}) {
		t.Fatal("overlay must contribute to revision")
	}
}

func TestCompileDiscoversExplicitAndImplicitKnowledge(t *testing.T) {
	dir := testModule(t, map[string]string{
		"data.cue": `package main

#Customer: {name: string}
customers: [ID=string]: #Customer
customers: {acme: {name: "Acme"}}
let customersCollection = customers

products: [ID=string]: {sku: string}
products: {starter: {sku: "starter"}}

pricingInput: {customer: string}
pricingResult: {discount: 0.2}

knowledge: {
	metadata: {title: "Company knowledge", description: "Canonical catalog"}
	domains: {
		customers: {
			description: "Canonical customers"
			collection: customersCollection
		}
	}
	evaluations: {
		"pricing.enterpriseDiscount": {
			description: "Evaluate an enterprise discount"
			input: pricingInput
			output: pricingResult
		}
	}
	checks: {catalogComplete: true}
}
`,
	})
	compiler := New(evaluation.New("", time.Second, 1<<20))

	got, err := compiler.Compile(context.Background(), CompileRequest{ModuleDir: dir})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(got.Diagnostics) != 0 {
		t.Fatalf("compile diagnostics=%+v", got.Diagnostics)
	}
	if got.Catalog.Metadata == nil || got.Catalog.Metadata.Title != "Company knowledge" {
		t.Fatalf("metadata=%+v, want explicit metadata", got.Catalog.Metadata)
	}
	if len(got.Catalog.Domains) != 2 {
		t.Fatalf("domains=%+v, want explicit customers and inferred products", got.Catalog.Domains)
	}
	if got.Catalog.Domains[0].Name != "customers" || !got.Catalog.Domains[0].Explicit || got.Catalog.Domains[1].Name != "products" || got.Catalog.Domains[1].Explicit {
		t.Fatalf("domains=%+v, want explicit customers and implicit products", got.Catalog.Domains)
	}
	if len(got.Catalog.Evaluations) != 1 || got.Catalog.Evaluations[0].Name != "pricing.enterpriseDiscount" {
		t.Fatalf("evaluations=%+v, want named pricing evaluation", got.Catalog.Evaluations)
	}
	if len(got.Catalog.Checks) != 1 || !got.Catalog.Checks[0].Value {
		t.Fatalf("checks=%+v, want catalogComplete=true", got.Catalog.Checks)
	}
}
