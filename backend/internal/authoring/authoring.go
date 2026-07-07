// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package authoring is canvas write-back: it formats source, splices canvas edits
// into a file's CUE (preserving comments), and derives element->file provenance by
// parsing source ASTs. It is stateless and pure, and does not evaluate CUE.
package authoring

import (
	"fmt"

	"cuelang.org/go/cue/format"

	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/domain"
)

// Service provides the authoring operations. It holds no state; New returns a
// ready value and the methods are pure.
type Service struct{}

// New returns an authoring Service.
func New() *Service { return &Service{} }

// Format runs `cue fmt` over arbitrary source text.
func (Service) Format(source string) (string, error) {
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

// Rewrite splices canvas edits (node upserts/deletes and an optional edge list)
// into one editable file's source, preserving its hand-written CUE and comments.
// The filename is guarded the same way as an overlay file, so a rewrite can never
// target schema.cue or escape the CUE dir. Diagnostics are returned on a syntax
// error; nothing is otherwise validated.
func (Service) Rewrite(op RewriteOp) (string, []diag.Diagnostic, error) {
	if !domain.ValidEditableName(op.Name) {
		return "", []diag.Diagnostic{{Message: fmt.Sprintf("invalid file name %q", op.Name), Kind: diag.KindParse}}, nil
	}
	return rewriteFile(op)
}

// ProvenanceFor attributes each diagram element to the editable file that authored
// it, so a canvas edit can be written back into the right file.
func (Service) ProvenanceFor(files []domain.File) domain.Provenance {
	return provenanceFrom(files)
}
