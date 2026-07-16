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

func TestBuildCatalogDescribesSchema(t *testing.T) {
	dir := testModule(t, map[string]string{"data.cue": "package main\n\nproducts: [ID=string]: {sku: string}\nproducts: {starter: {sku: \"starter\"}}\n#ProductID: or([for id, _ in products {id}])\ncustomers: [ID=string]: {name: string, country?: string, productIds: [...#ProductID]}\ncustomers: {acme: {name: \"Acme\", productIds: [\"starter\"]}}\n"})
	compiled, err := New(evaluation.New("", time.Second, 1<<20)).Compile(context.Background(), CompileRequest{ModuleDir: dir})
	if err != nil || len(compiled.Diagnostics) != 0 {
		t.Fatalf("Compile = %+v, %v", compiled.Diagnostics, err)
	}
	var customers Domain
	for _, domain := range compiled.Catalog.Domains {
		if domain.Name == "customers" {
			customers = domain
		}
	}
	if customers.Kind != "registry" || !customers.Fields["name"].Required || customers.Fields["country"].Required {
		t.Fatalf("customers = %+v", customers)
	}
	if relation := customers.Fields["productIds"].Relation; relation == nil || relation.Domain != "products" || relation.Cardinality != "many" {
		t.Fatalf("relation = %+v", relation)
	}
}
