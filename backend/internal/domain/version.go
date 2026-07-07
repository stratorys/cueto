// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package domain

import "time"

// Project identifies one project and its display name. The id is a stable
// filesystem-safe slug (also the version-store subdirectory name); the name is
// the mutable label shown in the UI.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Version identifies one saved version and when it was first saved. SavedAt
// comes from the append-only index when present, else the file mtime.
type Version struct {
	Version string    `json:"version"`
	SavedAt time.Time `json:"savedAt"`
}

// Manifest is a saved version's whole file set: an ordered list of
// (filename -> blob hash) entries, so a version can snapshot more than a single
// data.cue. It is the on-disk version model: a version id is the hash of a
// manifest's bytes, and each entry's blob is stored content-addressed beside it.
type Manifest struct {
	Entries []ManifestEntry `json:"entries"`
}

// ManifestEntry binds one filename in a version to the content hash of its body.
type ManifestEntry struct {
	Name string `json:"name"`
	Blob string `json:"blob"`
}

// SaveRequest is the mode-agnostic input to a save. Scope is the project id in
// playground mode and the workspace-relative file path in workspace mode. Data is
// the buffer text. BaseVersion is the optimistic-concurrency token the client
// loaded (workspace mode only); empty means "creating a new file".
type SaveRequest struct {
	Scope       string
	Data        string
	BaseVersion string
}

// SaveResult is the mode-agnostic outcome of a save. Version is the new version id
// (a manifest hash in playground mode, a content token in workspace mode). Conflict
// is set when the workspace file changed on disk since BaseVersion, so the write
// was refused.
type SaveResult struct {
	Version  string
	Conflict bool
}

// HistoryEntry is one point in a file's history, mode-agnostic. Version is a
// manifest hash (playground) or a git commit hash (workspace); Label is empty for
// a version and the commit subject for a commit; At is when it was recorded.
type HistoryEntry struct {
	Version string    `json:"version"`
	Label   string    `json:"label"`
	At      time.Time `json:"at"`
}
