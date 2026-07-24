package diag

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/stratorys/cueto/cueto-core/membrane"
)

var update = flag.Bool("update", false, "regenerate golden files")

// TestClassifyGolden drives one representative error per class through Classify and
// compares the machine (JSON) and human renderings against committed goldens.
func TestClassifyGolden(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	cases := []struct {
		name string
		err  error
	}{
		{"nonexistent_reference", loadErr(t, "broken-ref")},
		{"incompatible_type", craft(t, "x: string\nx: 3")},
		{"violated_bound", craft(t, "x: >5\nx: 3")},
		{"forbidden_field", craft(t, "#C: {a: int}\nx: #C\nx: {a: 1, b: 2}")},
		{"empty_disjunction", craft(t, "x: >10 | <0\nx: 5")},
		{"unclassified", craft(t, "x: undefinedRef")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.err)
			if len(got) == 0 {
				t.Fatalf("expected at least one diagnostic")
			}
			normalize(got, root)

			machine, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			checkGolden(t, tc.name+".json", append(machine, '\n'))
			checkGolden(t, tc.name+".human.txt", []byte(renderHuman(got)))
		})
	}
}

// TestNonexistentReferenceShape asserts the headline behavior directly, independent of
// the golden bytes: the carol reference collapses to one diagnostic naming both legal
// owners as alternatives.
func TestNonexistentReferenceShape(t *testing.T) {
	got := Classify(loadErr(t, "broken-ref"))
	if len(got) != 1 {
		t.Fatalf("expected a single collapsed diagnostic, got %d", len(got))
	}
	d := got[0]
	if d.Class != NonexistentReference {
		t.Fatalf("expected NonexistentReference, got %s", d.Class)
	}
	if strings.Join(d.Alternatives, ",") != "alice,bob" {
		t.Fatalf("expected alternatives [alice bob], got %v", d.Alternatives)
	}
	if d.Raw == "" {
		t.Fatalf("Raw must always be retained")
	}
}

// TestClassifyDeterministic hashes 100 classifications of the same error and requires
// them identical.
func TestClassifyDeterministic(t *testing.T) {
	base := loadErr(t, "broken-ref")
	var want []byte
	for i := 0; i < 100; i++ {
		b, err := json.Marshal(Classify(base))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if i == 0 {
			want = b
			continue
		}
		if !bytes.Equal(b, want) {
			t.Fatalf("run %d differed from run 0", i)
		}
	}
}

// TestClassifyNonCUEError degrades a plain error to a single Unclassified diagnostic.
func TestClassifyNonCUEError(t *testing.T) {
	got := Classify(fmt.Errorf("something not from CUE"))
	if len(got) != 1 || got[0].Class != Unclassified {
		t.Fatalf("expected one Unclassified diagnostic, got %+v", got)
	}
	if got[0].Raw == "" {
		t.Fatalf("Raw must be retained")
	}
}

// loadErr loads a fixture expected to fail and returns its native CUE error.
func loadErr(t *testing.T, fixture string) error {
	t.Helper()
	_, err := membrane.Load(filepath.Join("..", "testdata", "membrane", fixture))
	if err == nil {
		t.Fatalf("fixture %s should fail to load", fixture)
	}
	return err
}

// craft compiles a snippet expected to be invalid and returns its native CUE error.
func craft(t *testing.T, src string) error {
	t.Helper()
	v := cuecontext.New().CompileString(src)
	if err := v.Err(); err != nil {
		return err
	}
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return err
	}
	t.Fatalf("snippet did not produce an error: %q", src)
	return nil
}

// normalize strips the absolute module-root prefix from origin files so goldens are
// portable across machines.
func normalize(ds []Diagnostic, root string) {
	prefix := root + string(filepath.Separator)
	for i := range ds {
		for j := range ds[i].Origins {
			ds[i].Origins[j].File = strings.TrimPrefix(ds[i].Origins[j].File, prefix)
		}
	}
}

// renderHuman produces the human-facing rendering: class, message, origins, and any
// alternatives, one diagnostic per block.
func renderHuman(ds []Diagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&b, "%s: %s\n", d.Class, d.Message)
		for _, o := range d.Origins {
			fmt.Fprintf(&b, "  %s:%d\n", o.File, o.Line)
		}
		if len(d.Alternatives) > 0 {
			fmt.Fprintf(&b, "  alternatives: %s\n", strings.Join(d.Alternatives, ", "))
		}
	}
	return b.String()
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run: go test ./diag/... -update)", name, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("golden %s mismatch:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
