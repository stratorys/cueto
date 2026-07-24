// Package diag classifies native CUE errors into stable, machine-readable diagnostics.
// It is the only package that interprets cuelang.org/go errors; every other core
// package returns native CUE errors and lets diag translate them.
//
// The central algorithm is the disjunction collapse: when a key-set reference names a
// value that matches none of its legal disjuncts, CUE emits one "empty disjunction"
// summary error plus one "conflicting values" sibling per legal disjunct, all sharing
// the same failing path. diag groups those siblings and collapses them into a single
// NonexistentReference diagnostic whose Alternatives is the sorted disjunct list.
package diag

import (
	"encoding/json"
	"sort"
	"strings"

	cueerrors "cuelang.org/go/cue/errors"

	"github.com/stratorys/cueto/cueto-core/membrane"
)

// Class is the category a diagnostic falls into. Unclassified is the safe fallback so
// an unrecognized CUE error shape degrades to a raw passthrough rather than a wrong
// class.
type Class int

const (
	Unclassified Class = iota
	NonexistentReference
	ViolatedBound
	ForbiddenField
	IncompatibleType
	EmptyDisjunction
)

// className is the stable serialized name of each class, used in machine rendering.
var className = map[Class]string{
	Unclassified:         "Unclassified",
	NonexistentReference: "NonexistentReference",
	ViolatedBound:        "ViolatedBound",
	ForbiddenField:       "ForbiddenField",
	IncompatibleType:     "IncompatibleType",
	EmptyDisjunction:     "EmptyDisjunction",
}

func (c Class) String() string {
	if name, ok := className[c]; ok {
		return name
	}
	return "Unclassified"
}

// MarshalJSON renders the class as its stable name so golden files stay readable and
// independent of the iota ordering.
func (c Class) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// Diagnostic is one classified problem. Raw always retains the native CUE rendering so
// no information is lost, even when Class is Unclassified.
type Diagnostic struct {
	Class        Class             `json:"class"`
	Message      string            `json:"message"`
	Origins      []membrane.Origin `json:"origins,omitempty"`
	Alternatives []string          `json:"alternatives,omitempty"`
	Raw          string            `json:"raw"`
}

// Classify walks the CUE error list, groups siblings by failing path, and translates
// each group into one or more diagnostics. A non-CUE error passes through as a single
// Unclassified diagnostic.
func Classify(err error) []Diagnostic {
	if err == nil {
		return nil
	}
	list := cueerrors.Errors(err)
	if len(list) == 0 {
		msg := err.Error()
		return []Diagnostic{{Class: Unclassified, Message: msg, Raw: msg}}
	}

	out := make([]Diagnostic, 0, len(list))
	for _, group := range groupByPath(list) {
		out = append(out, classifyGroup(group)...)
	}
	return out
}

// classifyGroup translates one path-group of sibling errors. A group carrying an empty
// disjunction collapses to a single diagnostic; otherwise each sibling is classified on
// its own.
func classifyGroup(group []cueerrors.Error) []Diagnostic {
	if hasEmptyDisjunction(group) {
		return []Diagnostic{collapseDisjunction(group)}
	}

	out := make([]Diagnostic, 0, len(group))
	for _, e := range group {
		msg := e.Error()
		out = append(out, Diagnostic{
			Class:   classifyMessage(msg),
			Message: msg,
			Origins: originsOf([]cueerrors.Error{e}),
			Raw:     msg,
		})
	}
	return out
}

// classifyMessage maps a single error message to a class by its distinctive phrasing.
func classifyMessage(msg string) Class {
	switch {
	case strings.Contains(msg, "out of bound"):
		return ViolatedBound
	case strings.Contains(msg, "not allowed"):
		return ForbiddenField
	case strings.Contains(msg, "empty disjunction"):
		return EmptyDisjunction
	case strings.Contains(msg, "conflicting values"):
		return IncompatibleType
	default:
		return Unclassified
	}
}

// originsOf collects the distinct source positions of a set of errors, preferring
// InputPositions (conflict errors expose location only there) and falling back to the
// primary Position. Results are deduplicated and sorted for determinism.
func originsOf(errs []cueerrors.Error) []membrane.Origin {
	seen := make(map[membrane.Origin]bool)
	var origins []membrane.Origin
	add := func(file string, line int) {
		o := membrane.Origin{File: file, Line: line}
		if o.File == "" && o.Line == 0 {
			return
		}
		if seen[o] {
			return
		}
		seen[o] = true
		origins = append(origins, o)
	}
	for _, e := range errs {
		for _, p := range e.InputPositions() {
			if p.IsValid() {
				add(p.Filename(), p.Line())
			}
		}
		if p := e.Position(); p.IsValid() {
			add(p.Filename(), p.Line())
		}
	}
	sort.Slice(origins, func(i, j int) bool {
		if origins[i].File != origins[j].File {
			return origins[i].File < origins[j].File
		}
		return origins[i].Line < origins[j].Line
	})
	return origins
}
