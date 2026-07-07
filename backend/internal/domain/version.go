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
// data.cue. It is a placeholder in the re-layout step and becomes the on-disk
// version model once the manifest/blob store lands.
type Manifest struct {
	Entries []ManifestEntry `json:"entries"`
}

// ManifestEntry binds one filename in a version to the content hash of its body.
type ManifestEntry struct {
	Name string `json:"name"`
	Blob string `json:"blob"`
}
