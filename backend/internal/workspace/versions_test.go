// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package workspace

import (
	"context"
	"testing"

	"github.com/stratorys/cueto/backend/internal/config"
)

// TestReadVersionReturnsRemovedFieldTextAsIs pins the Phase 0 decision: saved
// versions are readable as text as-is; the store never schema-validates on read
// (nor on write - validation is the transport's concern). The content below uses
// fields removed from the trimmed schema (role, region, policies). Re-evaluating
// it through the CUE engine would fail unification, but reading the stored blob
// must return the original bytes verbatim. If a future change makes reads validate,
// this breaks loudly.
func TestReadVersionReturnsRemovedFieldTextAsIs(t *testing.T) {
	const data = `package main

diagram: {
	policies: ["security"]
	nodes: gw: {
		id:     "gw"
		type:   "process"
		label:  "Gateway"
		role:   "gateway"
		region: "eu-west-1"
	}
}
`
	ctx := context.Background()
	w := New(config.Config{VersionsDir: t.TempDir(), CueDir: t.TempDir()})

	proj, err := w.CreateProject(ctx, "demo", "blank")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	version, err := w.SaveVersion(ctx, proj.ID, data)
	if err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}

	versions, err := w.ListVersions(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != version {
		t.Fatalf("versions = %+v, want the saved id %q", versions, version)
	}

	got, err := w.ReadVersion(ctx, proj.ID, version)
	if err != nil {
		t.Fatalf("ReadVersion: %v", err)
	}
	if got != data {
		t.Fatalf("ReadVersion returned altered text:\n--- got ---\n%s\n--- want ---\n%s", got, data)
	}
}
