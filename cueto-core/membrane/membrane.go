// Package membrane loads a CUE module and exposes lookup, attribute reading, and
// source-origin resolution over the evaluated value. It is the only core package that
// touches CUE evaluation; consumers other than graph are allowed to see cue.Value and
// cue.Attribute directly, since wrapping them would be a partial reimplementation of
// the unifier's surface.
//
// Errors from loading a broken membrane are returned as native cuelang.org/go errors,
// never stringified, so diag can classify them.
package membrane

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

// Handle is a loaded, evaluated membrane. It owns the cue.Context that produced its
// root value; both live for the lifetime of the handle.
type Handle struct {
	ctx  *cue.Context
	root cue.Value
	dir  string
}

// Origin is a source location within the membrane. It is the one neutral type here,
// so that prov and diag can carry file and line without importing CUE.
type Origin struct {
	File string
	Line int
}

// Load evaluates the CUE module rooted at dir and returns a handle to it. A membrane
// that fails to load or evaluate yields the native CUE error and no handle.
func Load(dir string) (*Handle, error) {
	instances := load.Instances([]string{"."}, &load.Config{Dir: dir})
	if len(instances) == 0 {
		return nil, &LoadError{Dir: dir, Reason: "no CUE instance found"}
	}
	inst := instances[0]
	if inst.Err != nil {
		return nil, inst.Err
	}

	ctx := cuecontext.New()
	root := ctx.BuildInstance(inst)
	if err := root.Err(); err != nil {
		return nil, err
	}
	return &Handle{ctx: ctx, root: root, dir: dir}, nil
}

// Lookup resolves a dotted CUE path against the membrane root.
func (h *Handle) Lookup(path string) (cue.Value, error) {
	p := cue.ParsePath(path)
	if err := p.Err(); err != nil {
		return cue.Value{}, &PathError{Path: path, Err: err}
	}
	v := h.root.LookupPath(p)
	if !v.Exists() {
		return cue.Value{}, &NotFoundError{Path: path}
	}
	return v, nil
}

// Attributes returns every field attribute declared on the value at path.
func (h *Handle) Attributes(path string) ([]cue.Attribute, error) {
	v, err := h.Lookup(path)
	if err != nil {
		return nil, err
	}
	return v.Attributes(cue.ValueAttr), nil
}

// DefinedAt returns the source origin of the value at path. A struct value's position
// can resolve to the schema definition it unified with rather than the user's data
// file; DefinedAt returns whatever position CUE reports.
func (h *Handle) DefinedAt(path string) (Origin, error) {
	v, err := h.Lookup(path)
	if err != nil {
		return Origin{}, err
	}
	pos := v.Pos()
	if !pos.IsValid() {
		return Origin{}, &NoOriginError{Path: path}
	}
	return Origin{File: pos.Filename(), Line: pos.Line()}, nil
}
