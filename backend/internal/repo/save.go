// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package repo

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/stratorys/cueto/backend/internal/domain"
)

// Save writes req.Data to the real file at req.Scope inside the workspace and
// returns the new content token. Validating the data against the schema is the
// caller's concern: the transport runs the evaluation service before calling Save,
// so an invalid diagram is never written. Save never touches git state.
//
// Conflict detection is optimistic: req.BaseVersion is the token the client loaded.
// The write is refused (SaveResult.Conflict) when the on-disk body no longer hashes
// to it, when a supposedly new file already exists, or when a supposedly existing
// file has vanished. Detect and refuse is the safe default; the client re-loads and
// re-applies. On success the write is atomic (temp file plus rename).
func (r *Repo) Save(_ context.Context, req domain.SaveRequest) (domain.SaveResult, error) {
	target, ok := r.resolve(req.Scope)
	if !ok {
		return domain.SaveResult{}, ErrInvalidPath
	}

	current, err := os.ReadFile(target)
	switch {
	case err == nil:
		// The file exists: the client must have loaded it and the on-disk body must
		// still match the token it loaded, or another writer changed it meanwhile.
		if req.BaseVersion == "" || ContentHash(string(current)) != req.BaseVersion {
			return domain.SaveResult{Conflict: true}, nil
		}
	case errors.Is(err, os.ErrNotExist):
		// The file does not exist: the client must be creating it (no token). A token
		// means it loaded a file that has since been removed.
		if req.BaseVersion != "" {
			return domain.SaveResult{Conflict: true}, nil
		}
	default:
		return domain.SaveResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return domain.SaveResult{}, err
	}
	if err := atomicWrite(target, []byte(req.Data)); err != nil {
		return domain.SaveResult{}, err
	}
	return domain.SaveResult{Version: ContentHash(req.Data)}, nil
}

// atomicWrite writes body to path via a temporary file in the same directory then
// renames it into place, so a crash mid-write never leaves a truncated file and a
// reader never sees a partial buffer.
func atomicWrite(path string, body []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".cueto-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
