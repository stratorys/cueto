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
)

// evalService is the CUE evaluation the transport depends on. Keeping it a small
// interface isolates the cuelang library behind one seam so the transport stays
// library-agnostic and tests can substitute a fake.
type evalService interface {
	Eval(ctx context.Context, files []domain.File) (json.RawMessage, []evaluation.Hint, []diag.Diagnostic, error)
	EvalExpr(ctx context.Context, source string) (json.RawMessage, []diag.Diagnostic, error)
	EvalQuery(ctx context.Context, files []domain.File, expr string) (json.RawMessage, []diag.Diagnostic, error)
	Keys(ctx context.Context, files []domain.File) ([]string, []diag.Diagnostic, error)
	Introspect() evaluation.CueMeta
	Vet(ctx context.Context, files []domain.File) ([]diag.Diagnostic, error)
}

// workspaceService is the project + version persistence the transport depends on.
type workspaceService interface {
	SaveVersion(ctx context.Context, projectID, data string) (string, error)
	ListVersions(ctx context.Context, projectID string) ([]domain.Version, error)
	ReadVersion(ctx context.Context, projectID, id string) (string, error)
	ReadSeed() (string, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	CreateProject(ctx context.Context, name, seed string) (domain.Project, error)
	RenameProject(ctx context.Context, id, name string) (domain.Project, error)
	DeleteProject(ctx context.Context, id string) error
}

// authoringService is the canvas write-back the transport depends on.
type authoringService interface {
	Format(source string) (string, error)
	Rewrite(op authoring.RewriteOp) (string, []diag.Diagnostic, error)
	ProvenanceFor(files []domain.File) domain.Provenance
}

// handlers hold the concern services and the schema dir needed to scrub host
// paths from any diagnostics built at this layer.
type handlers struct {
	eval      evalService
	ws        workspaceService
	authoring authoringService
	cueDir    string
}

type dataRequest struct {
	Data string `json:"data"`
	// Editable file set for multi-file packages. When empty, Data is treated as a
	// single legacy data.cue, so older single-file clients keep working.
	Files []domain.File `json:"files"`
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

// projectRequest is the body for project create/rename. Seed ("blank" | "sample")
// is only read on create.
type projectRequest struct {
	Name string `json:"name"`
	Seed string `json:"seed"`
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
