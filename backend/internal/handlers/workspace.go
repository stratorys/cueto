// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package handlers

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stratorys/cueto/backend/internal/diag"
	"github.com/stratorys/cueto/backend/internal/domain"
	"github.com/stratorys/cueto/backend/internal/repo"
)

// Config reports the persistence mode so the frontend can key its behavior. There
// is one mode now: git-backed workspace. The field is kept for a stable frontend
// boot shape.
func (h *handlers) Config(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"mode": "workspace"})
}

// workspaceSaveRequest saves one editor buffer to a real file in the workspace.
// Path is the workspace-relative file path; BaseVersion is the content token the
// client loaded, used to detect a concurrent on-disk change.
type workspaceSaveRequest struct {
	Path        string `json:"path"`
	Data        string `json:"data"`
	BaseVersion string `json:"baseVersion"`
}

// WorkspaceSave validates the buffer against the whole module and, when valid,
// writes it to the real file. It mirrors the playground Save contract: the
// evaluation service validates first, and only a clean result is written, so an
// invalid diagram never reaches disk. A concurrent on-disk change is a 409, never a
// silent overwrite. No git state is touched.
func (h *handlers) WorkspaceSave(c *gin.Context) {
	dir, ok := h.projectDir(c)
	if !ok {
		return
	}
	var req workspaceSaveRequest
	if !bindJSON(c, &req) {
		return
	}
	if !domain.ValidEditableName(req.Path) {
		c.JSON(http.StatusBadRequest, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "invalid file name: " + req.Path, Kind: diag.KindParse}},
		})
		return
	}
	// Overlay the buffer at its path over the on-disk module and validate the whole
	// module as it would be after the write, so a save can never leave the module in
	// a state that does not evaluate.
	files := []domain.File{{Name: req.Path, Content: req.Data}}
	diags, err := h.eval.Vet(c.Request.Context(), h.source(dir, files))
	if err != nil {
		writeOpError(c, err)
		return
	}
	if len(diags) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"diagnostics": diags})
		return
	}
	res, err := h.repoFor(dir).Save(c.Request.Context(), domain.SaveRequest{
		Scope:       req.Path,
		Data:        req.Data,
		BaseVersion: req.BaseVersion,
	})
	if err != nil {
		writeRepoError(c, err)
		return
	}
	if res.Conflict {
		c.JSON(http.StatusConflict, gin.H{
			"conflict":    true,
			"diagnostics": []diag.Diagnostic{{Message: "file changed on disk since it was loaded", Kind: diag.KindInternal}},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": req.Path, "version": res.Version})
}

// WorkspaceHistory returns the git commits that touched ?path, newest first, as
// {entries:[{version,label,at}]}. A workspace that is not a git repo returns an
// empty list.
func (h *handlers) WorkspaceHistory(c *gin.Context) {
	dir, ok := h.projectDir(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if !domain.ValidEditableName(path) {
		c.JSON(http.StatusBadRequest, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "invalid file name: " + path, Kind: diag.KindParse}},
		})
		return
	}
	entries, err := h.repoFor(dir).History(c.Request.Context(), path)
	if err != nil {
		writeRepoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

// WorkspaceFile returns the content of ?path at ?commit as {data, version}. With no
// commit it reads the current working-tree file, and version is the content token
// the client carries back into a save for conflict detection. With a commit (a full
// git hash) it reads the blob at that commit.
func (h *handlers) WorkspaceFile(c *gin.Context) {
	dir, ok := h.projectDir(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if !domain.ValidEditableName(path) {
		c.JSON(http.StatusBadRequest, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "invalid file name: " + path, Kind: diag.KindParse}},
		})
		return
	}
	data, err := h.repoFor(dir).FileAt(c.Request.Context(), path, c.Query("commit"))
	if err != nil {
		writeRepoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "version": repo.ContentHash(data)})
}

// WorkspaceDeleteFile removes ?path from the project working tree. The removal
// shows in git status for the user to commit; cueto never stages or commits it.
func (h *handlers) WorkspaceDeleteFile(c *gin.Context) {
	dir, ok := h.projectDir(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if !domain.ValidEditableName(path) {
		c.JSON(http.StatusBadRequest, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "invalid file name: " + path, Kind: diag.KindParse}},
		})
		return
	}
	if err := h.repoFor(dir).Delete(c.Request.Context(), path); err != nil {
		writeRepoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "path": path})
}

// Tree lists the project's .cue files as workspace-relative slash paths, skipping
// the cue.mod and .git directories. It backs the frontend file tree.
func (h *handlers) Tree(c *gin.Context) {
	dir, ok := h.projectDir(c)
	if !ok {
		return
	}
	files, err := listTree(dir)
	if err != nil {
		writeOpError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

// listTree walks a module root and returns its .cue files as sorted slash paths
// relative to the root, pruning the cue.mod schema dir and the .git dir so only
// editable source shows in the tree.
func listTree(root string) ([]string, error) {
	out := make([]string, 0)
	err := fs.WalkDir(os.DirFS(root), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && (d.Name() == "cue.mod" || d.Name() == ".git") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".cue") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// writeRepoError maps workspace-store errors to status codes: a malformed path or
// commit is 400, an unknown file or commit 404, oversized content 413; anything
// else falls through to the operational error path.
func writeRepoError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repo.ErrInvalidPath):
		c.JSON(http.StatusBadRequest, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "invalid file path", Kind: diag.KindParse}},
		})
	case errors.Is(err, repo.ErrInvalidCommit):
		c.JSON(http.StatusBadRequest, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "invalid commit id", Kind: diag.KindInternal}},
		})
	case errors.Is(err, repo.ErrFileNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "file not found", Kind: diag.KindInternal}},
		})
	case errors.Is(err, repo.ErrCommitNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "commit not found", Kind: diag.KindInternal}},
		})
	case errors.Is(err, repo.ErrOutputTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"diagnostics": []diag.Diagnostic{{Message: "file content too large", Kind: diag.KindInternal}},
		})
	default:
		writeOpError(c, err)
	}
}
