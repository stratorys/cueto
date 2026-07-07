// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package evaluation

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/stratorys/cueto/backend/internal/diag"
)

// Layer-2 graph checks. `cue vet` (Vet) decides Layer 1: typed referential
// integrity between nodes, which is pure CUE. Check decides Layer 2: claims a
// schema makes about the world outside CUE that the compiler cannot - a field
// marked @file must name a file that exists on disk, a field marked @uri must
// name a URI that resolves. The policy is the user's own schema attributes, read
// by shape, so cueto learns no domain vocabulary (the same posture as @ref
// inference). Attributes are inert to CUE, so a user module stays `cue vet`-clean
// without importing anything from cueto.

// checkVisitMax and checkDepthMax bound the walk so a pathological or cyclic-looking
// composed value cannot blow the stack or run unbounded, mirroring the caps the key
// walk uses. The walk stops descending once either is hit.
const (
	checkVisitMax = 20000
	checkDepthMax = 32
)

// Check walks the composed module and returns a diagnostic for every @file/@uri
// attribute whose concrete value does not resolve. It never gates concreteness (an
// abstract field carrying the attribute is a schema line, not a claim, and is
// skipped) and never reaches the network: an http(s) URI is checked for syntactic
// validity only. Diagnostics are empty when clean. It runs under the same deadline
// and panic recovery as Vet, since the composed value comes from untrusted CUE.
func (e *Engine) Check(ctx context.Context, src Source) ([]diag.Diagnostic, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan vetResult, 1)
	go func() {
		done <- recoverVet(func() vetResult { return vetResult{diags: e.checkModule(src)} })
	}()

	select {
	case <-ctx.Done():
		return nil, ErrTimeout
	case r := <-done:
		return r.diags, r.err
	}
}

// checkModule builds every package in the module and walks each for @file/@uri
// existence claims. Each package is built standalone (its imports resolved), so an
// attribute on a member is read off the concrete value exactly as declared, without
// depending on a root package re-exposing it. A localized build/validity error (a
// dangling typed reference, say) is left to Vet, which owns Layer 1; Check walks the
// value regardless, since a bottom field is simply not a concrete string and is
// skipped, while its healthy siblings still carry checkable references. A parse
// failure that yields no value at all is the one case Check cannot walk, so those
// instances are skipped. Diagnostics are deduplicated by position and message so a
// value re-exposed through an import is not reported twice.
func (e *Engine) checkModule(src Source) []diag.Diagnostic {
	instances, diags := e.loadModule(src, "")
	if diags != nil {
		return diags
	}
	ctx := cuecontext.New()
	seen := map[string]struct{}{}
	var out []diag.Diagnostic
	for _, inst := range instances {
		if inst.Err != nil {
			continue
		}
		walkChecks(ctx.BuildInstance(inst), src.Dir, seen, &out)
	}
	return out
}

// walkChecks visits every value in root and appends a diagnostic for each concrete
// @file/@uri value that fails to resolve. The walk is bounded by a visit count and a
// depth cap; seen dedups by position+message across packages and re-exposed values.
func walkChecks(root cue.Value, moduleDir string, seen map[string]struct{}, out *[]diag.Diagnostic) {
	visits := 0
	root.Walk(func(x cue.Value) bool {
		visits++
		if visits > checkVisitMax || len(x.Path().Selectors()) > checkDepthMax {
			return false
		}
		if d, ok := checkFile(x, moduleDir); ok {
			appendUnique(out, seen, d)
		}
		if d, ok := checkURI(x, root, moduleDir); ok {
			appendUnique(out, seen, d)
		}
		return true
	}, nil)
}

// checkFile reports a diagnostic when x carries @file and its concrete string names
// a path that does not exist inside the module. A non-concrete or non-string value
// carrying the attribute is a schema line, not a claim, so it is skipped.
func checkFile(x cue.Value, moduleDir string) (diag.Diagnostic, bool) {
	attr := x.Attribute("file")
	if attr.Err() != nil {
		return diag.Diagnostic{}, false
	}
	rel, ok := concreteString(x)
	if !ok {
		return diag.Diagnostic{}, false
	}
	return fileExists(x, moduleDir, rel)
}

// checkURI reports a diagnostic when x carries @uri and its concrete string does not
// resolve. A relative or file: URI resolves like @file. A cue: URI resolves as a
// dotted path against the package root value. An http(s) URI is checked for
// syntactic validity only - cueto never reaches the network.
func checkURI(x cue.Value, root cue.Value, moduleDir string) (diag.Diagnostic, bool) {
	attr := x.Attribute("uri")
	if attr.Err() != nil {
		return diag.Diagnostic{}, false
	}
	raw, ok := concreteString(x)
	if !ok {
		return diag.Diagnostic{}, false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return referenceDiag(x, fmt.Sprintf("invalid URI %q", raw)), true
	}
	switch u.Scheme {
	case "", "file":
		// A relative URI parses its path into u.Path with an empty host; file://a/b
		// parses the first segment as the host. Joining both covers both shapes.
		return fileExists(x, moduleDir, filepath.FromSlash(u.Host+u.Path))
	case "cue":
		if resolveCuePath(root, u).Exists() {
			return diag.Diagnostic{}, false
		}
		return referenceDiag(x, fmt.Sprintf("cue reference %q does not resolve", raw)), true
	case "http", "https":
		if u.Host == "" {
			return referenceDiag(x, fmt.Sprintf("invalid URI %q", raw)), true
		}
		return diag.Diagnostic{}, false
	default:
		return diag.Diagnostic{}, false
	}
}

// fileExists resolves rel inside moduleDir and reports a diagnostic when it does not
// exist or would escape the module. Path containment is checked here (not via the
// .cue-only editable-name guard, which does not fit arbitrary file references): an
// absolute path or one that climbs out of the module is refused.
func fileExists(x cue.Value, moduleDir, rel string) (diag.Diagnostic, bool) {
	target, ok := resolveWithin(moduleDir, rel)
	if !ok {
		return referenceDiag(x, fmt.Sprintf("referenced path %q escapes the module", rel)), true
	}
	if _, err := os.Stat(target); err != nil {
		return referenceDiag(x, fmt.Sprintf("referenced file %q does not exist", rel)), true
	}
	return diag.Diagnostic{}, false
}

// resolveWithin joins rel under dir and confirms it stays inside, refusing an absolute
// path or one that climbs out via traversal. filepath.Join cleans the result, so a
// `../` segment that escapes is caught by the prefix check.
func resolveWithin(dir, rel string) (string, bool) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", false
	}
	target := filepath.Join(dir, rel)
	prefix := dir + string(filepath.Separator)
	if target != dir && !strings.HasPrefix(target, prefix) {
		return "", false
	}
	return target, true
}

// resolveCuePath resolves a cue: URI as a dotted path against the package root value.
// The authority and path segments join into a CUE path (cue://people/marty ->
// people.marty), which is looked up on root.
func resolveCuePath(root cue.Value, u *url.URL) cue.Value {
	segments := []string{}
	if u.Host != "" {
		segments = append(segments, u.Host)
	}
	for _, seg := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if seg != "" {
			segments = append(segments, seg)
		}
	}
	if len(segments) == 0 {
		return cue.Value{}
	}
	return root.LookupPath(cue.ParsePath(strings.Join(segments, ".")))
}

// concreteString returns x's value when it is a concrete string, reporting false for
// a non-string or non-concrete value (a schema-level abstract field).
func concreteString(x cue.Value) (string, bool) {
	if x.IncompleteKind() != cue.StringKind {
		return "", false
	}
	s, err := x.String()
	if err != nil {
		return "", false
	}
	return s, true
}

// referenceDiag builds a Layer-2 diagnostic anchored at x's source position. Only the
// line and column are taken from the position, never the filename, so no host path
// leaks; the message carries the user-supplied reference text, which is safe.
func referenceDiag(x cue.Value, message string) diag.Diagnostic {
	d := diag.Diagnostic{Message: message, Kind: diag.KindReference}
	if pos := x.Pos(); pos.IsValid() {
		d.Line = pos.Line()
		d.Column = pos.Column()
	}
	return d
}

// appendUnique appends d unless a diagnostic with the same position and message was
// already recorded, so a value re-exposed through an import is reported once.
func appendUnique(out *[]diag.Diagnostic, seen map[string]struct{}, d diag.Diagnostic) {
	key := fmt.Sprintf("%d:%d:%s", d.Line, d.Column, d.Message)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*out = append(*out, d)
}
