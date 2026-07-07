// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/authoring"
	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/domain"
	"github.com/stratorys/cueto/backend/internal/evaluation"
	"github.com/stratorys/cueto/backend/internal/projects"
	"github.com/stratorys/cueto/backend/internal/repo"
)

// evalService is the CUE evaluation the transport depends on. Keeping it a small
// interface isolates the cuelang library behind one seam so the transport stays
// library-agnostic and tests can substitute a fake.
type evalService interface {
	Eval(ctx context.Context, src evaluation.Source) (json.RawMessage, []string, []evaluation.Hint, []evaluation.TraceEntry, []diag.Diagnostic, error)
	EvalExpr(ctx context.Context, source string) (json.RawMessage, []diag.Diagnostic, error)
	EvalQuery(ctx context.Context, src evaluation.Source, expr string) (json.RawMessage, []diag.Diagnostic, error)
	Keys(ctx context.Context, src evaluation.Source) ([]string, []diag.Diagnostic, error)
	Introspect() evaluation.CueMeta
	Vet(ctx context.Context, src evaluation.Source) ([]diag.Diagnostic, error)
}

// authoringService is the canvas write-back the transport depends on.
type authoringService interface {
	Format(source string) (string, error)
	Rewrite(op authoring.RewriteOp) (string, []diag.Diagnostic, error)
	ProvenanceFor(files []domain.File) domain.Provenance
}

// handlers hold the concern services, the projects manager that resolves a project
// id to its module dir, the schema dir needed to scrub host paths from diagnostics
// built at this layer, and the output cap threaded into per-project repos.
type handlers struct {
	eval           evalService
	authoring      authoringService
	projects       *projects.Manager
	cueDir         string
	maxOutputBytes int
}

// projectDir resolves the :id path param to an absolute module dir, writing a 404
// and returning false when the id is malformed or names no project. Every
// project-scoped handler calls it first, so an unknown project can never reach the
// evaluation or persistence layers.
func (h *handlers) projectDir(c *gin.Context) (string, bool) {
	id := c.Param("id")
	dir, ok := h.projects.Resolve(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "unknown project: " + id, Kind: diag.KindInternal}},
		})
		return "", false
	}
	return dir, true
}

// source wraps a client file set into an evaluation.Source rooted at a project's
// module dir. It is the single place the transport builds a Source, so the module
// root always comes from a resolved project id.
func (h *handlers) source(dir string, files []domain.File) evaluation.Source {
	return evaluation.Source{Dir: dir, Overlay: files}
}

// repoFor returns a Repo rooted at a project's module dir. Construction is cheap
// (it holds only the dir and the output cap), so a fresh one per request avoids
// any shared mutable state across projects.
func (h *handlers) repoFor(dir string) *repo.Repo {
	return repo.New(dir, h.maxOutputBytes)
}

type dataRequest struct {
	Data string `json:"data"`
	// Editable file set for multi-file packages. When empty, Data is treated as a
	// single data.cue, so single-file clients keep working.
	Files []domain.File `json:"files"`
	// View selects which discovered view /eval renders. Empty (the default and the
	// single-file client's behavior) renders the default view.
	View string `json:"view"`
}

// files returns the editable set: the explicit Files, or a single data.cue built
// from Data when Files is empty (the single-file compatibility path).
func (r dataRequest) files() []domain.File {
	if len(r.Files) > 0 {
		return r.Files
	}
	return []domain.File{{Name: "data.cue", Content: r.Data}}
}

type sourceRequest struct {
	Source string `json:"source"`
	// When present, /repl evaluates Source as a single CUE expression against these
	// editor files overlaid on the schema, so it can reference the live `diagram`.
	// When empty, Source is a standalone snippet with no schema or diagram in scope.
	Files []domain.File `json:"files"`
}

// bindJSON decodes the body, translating an over-limit body into 413 and any
// other decode failure into 400. It returns false when it has already written
// the response.
func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"diagnostics": []diag.Diagnostic{{Message: "request body too large", Kind: diag.KindInternal}},
			})
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "invalid request body", Kind: diag.KindParse}},
		})
		return false
	}
	return true
}

// writeOpError maps operational evaluation errors to status codes. These are not
// tied to a source position and their messages are fixed, leaking nothing.
func writeOpError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, evaluation.ErrTimeout):
		c.JSON(http.StatusGatewayTimeout, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "evaluation timed out", Kind: diag.KindInternal}},
		})
	case errors.Is(err, evaluation.ErrOutputTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "evaluation output too large", Kind: diag.KindInternal}},
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "internal error", Kind: diag.KindInternal}},
		})
	}
}
