// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package domain

import "time"

// SaveRequest is the input to a workspace save. Scope is the workspace-relative
// file path. Data is the buffer text. BaseVersion is the optimistic-concurrency
// token the client loaded; empty means "creating a new file".
type SaveRequest struct {
	Scope       string
	Data        string
	BaseVersion string
}

// SaveResult is the outcome of a save. Version is the new content token. Conflict
// is set when the workspace file changed on disk since BaseVersion, so the write
// was refused.
type SaveResult struct {
	Version  string
	Conflict bool
}

// HistoryEntry is one point in a file's git history. Version is the commit hash;
// Label is the commit subject; At is when it was recorded.
type HistoryEntry struct {
	Version string    `json:"version"`
	Label   string    `json:"label"`
	At      time.Time `json:"at"`
}
