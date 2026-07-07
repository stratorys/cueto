// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package evaluation

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// CueMember is one entry a REPL query can reference: a builtin, or a member of an
// imported package. IsFunc distinguishes callables (strings.ToUpper) from value
// constants (math.Pi) so the UI can render and complete them differently.
type CueMember struct {
	Name   string `json:"name"`
	IsFunc bool   `json:"isFunc"`
}

// CuePackage is one importable standard-library package. Path is the import path
// (encoding/json); Name is the identifier it binds to by default (json).
type CuePackage struct {
	Path    string      `json:"path"`
	Name    string      `json:"name"`
	Members []CueMember `json:"members"`
}

// CueMeta is the static reference a REPL query can draw on: the no-import builtin
// functions and every importable standard-library package with its members.
type CueMeta struct {
	Builtins []CueMember  `json:"builtins"`
	Packages []CuePackage `json:"packages"`
}

// replPackages are the hermetic standard-library packages a REPL expression may
// import. The tool/* packages are excluded: they define command tasks for
// `cue cmd`, not functions usable in an expression (and the bare "tool" package
// is undefined outside that context).
var replPackages = []string{
	"crypto/ed25519", "crypto/hmac", "crypto/md5", "crypto/sha1", "crypto/sha256", "crypto/sha512",
	"encoding/base64", "encoding/csv", "encoding/hex", "encoding/json", "encoding/openapi",
	"encoding/toml", "encoding/yaml",
	"html", "list", "math", "math/bits", "net", "path", "regexp", "strconv", "strings", "struct",
	"text/tabwriter", "text/template", "time", "uuid",
}

// cueBuiltins are the functions callable without an import. CUE exposes no runtime
// enumeration for them, so the list is fixed here; it is stable across releases.
var cueBuiltins = []CueMember{
	{Name: "and", IsFunc: true},
	{Name: "close", IsFunc: true},
	{Name: "div", IsFunc: true},
	{Name: "len", IsFunc: true},
	{Name: "mod", IsFunc: true},
	{Name: "or", IsFunc: true},
	{Name: "quo", IsFunc: true},
	{Name: "rem", IsFunc: true},
}

// replPackageByName maps each importable package's default identifier (json) to
// its import path (encoding/json). The last path segment is the bind name, and no
// two replPackages share one, so the mapping is unambiguous.
var replPackageByName = func() map[string]string {
	m := make(map[string]string, len(replPackages))
	for _, p := range replPackages {
		m[p[strings.LastIndex(p, "/")+1:]] = p
	}
	return m
}()

// pkgRefPattern matches a bare `name.` reference: an identifier that is not itself
// a field access (not preceded by `.`) and is followed by a member selector. This
// distinguishes a package use (strings.ToUpper) from a diagram field of the same
// name (diagram.strings), so import injection never adds an unused import.
var pkgRefPattern = regexp.MustCompile(`(?:^|[^\w.])([A-Za-z_]\w*)\.`)

// replImports returns the CUE import lines for the standard-library packages expr
// references, or "" when it uses none. It must be exact: CUE rejects an unused
// import, so a blanket import would break every query that does not use it.
func replImports(expr string) string {
	seen := map[string]bool{}
	var paths []string
	for _, m := range pkgRefPattern.FindAllStringSubmatch(expr, -1) {
		path, ok := replPackageByName[m[1]]
		if !ok || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return ""
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&b, "import %q\n", p)
	}
	return b.String()
}

// Introspect returns the CUE builtin functions and importable standard-library
// packages that a REPL query can reference. The reference is static per CUE
// version, so it is built once (buildCueMeta walks each package) and memoized.
func (e *Engine) Introspect() CueMeta {
	e.metaOnce.Do(func() { e.meta = buildCueMeta() })
	return e.meta
}

// buildCueMeta enumerates each importable package's members by compiling a tiny
// program that imports it under a fixed alias and walking the resulting struct's
// fields. This is version-accurate - no hardcoded member lists - and pure, so its
// result is safe to cache. A package that fails to import is skipped rather than
// failing the whole reference.
func buildCueMeta() CueMeta {
	ctx := cuecontext.New()
	packages := make([]CuePackage, 0, len(replPackages))
	for _, path := range replPackages {
		members, ok := packageMembers(ctx, path)
		if !ok {
			continue
		}
		packages = append(packages, CuePackage{
			Path:    path,
			Name:    path[strings.LastIndex(path, "/")+1:],
			Members: members,
		})
	}
	return CueMeta{Builtins: cueBuiltins, Packages: packages}
}

// packageMembers imports path under the alias `x` and returns its exported members
// sorted by name, or ok=false if the package cannot be imported or walked.
func packageMembers(ctx *cue.Context, path string) ([]CueMember, bool) {
	v := ctx.CompileString(fmt.Sprintf("import x %q\nout: x", path))
	if v.Err() != nil {
		return nil, false
	}
	it, err := v.LookupPath(cue.ParsePath("out")).Fields(cue.All())
	if err != nil {
		return nil, false
	}
	members := []CueMember{}
	for it.Next() {
		members = append(members, CueMember{
			Name:   it.Selector().String(),
			IsFunc: it.Value().IncompleteKind().String() == "func",
		})
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	return members, true
}
