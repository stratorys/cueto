// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package assets carries the files a standalone cueto binary needs at runtime:
// the hand-owned diagram schema module, the demo project seeded on first run,
// and the built web UI. The schema and demo are byte-for-byte copies of cue/
// and examples/service-catalog, enforced by this package's drift tests, so the
// repo stays the single source of truth. webui/ holds a placeholder page that
// the release build replaces with the real frontend dist before compiling.
package assets

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:cueschema
var schemaFS embed.FS

//go:embed all:demo
var demoFS embed.FS

//go:embed all:webui
var webuiFS embed.FS

// DemoProjectID is the id the seeded demo project gets under a projects root.
const DemoProjectID = "service-catalog"

// Schema returns the embedded diagram schema module (cue.mod, diagram/,
// knowledge/), rooted so its top level is the module root.
func Schema() fs.FS {
	return mustSub(schemaFS, "cueschema")
}

// Demo returns the embedded demo project module, rooted at the module root.
func Demo() fs.FS {
	return mustSub(demoFS, "demo")
}

// WebUI returns the embedded web UI file tree (index.html at its root).
func WebUI() fs.FS {
	return mustSub(webuiFS, "webui")
}

// MaterializeSchema writes the embedded schema module under dst, overwriting
// existing files, so a standalone binary can hand the evaluator a real on-disk
// schema dir without shipping the repo. Files removed from the schema in a newer
// binary are not cleaned up; the loader only reads the packages it asks for.
func MaterializeSchema(dst string) error {
	return fs.WalkDir(Schema(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		content, err := fs.ReadFile(Schema(), path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o644)
	})
}

// mustSub roots an embedded tree at its top directory. The directory name is a
// compile-time constant matching the embed directive, so failure is impossible
// in a correctly built binary and would mean the binary itself is broken.
func mustSub(fsys embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
