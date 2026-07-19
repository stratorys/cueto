// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stratorys/cueto/backend/internal/assets"
	"github.com/stratorys/cueto/backend/internal/home"
	"github.com/stratorys/cueto/backend/internal/projects"
)

// resolveModuleDir picks the module root for a subcommand, mirroring the web
// app's session rules so the CLI and the app always agree on "current":
//
//  1. an explicit -C dir wins (CI, arbitrary modules)
//  2. the working directory when it is itself a CUE module
//  3. -p id under the cueto home's projects root
//  4. the persisted selection (cueto use / the web app) when it still resolves
//  5. the only project under the projects root
//
// Anything else is an error that says exactly how to disambiguate.
func resolveModuleDir(cFlag, pFlag string) (string, error) {
	if cFlag != "" {
		return cFlag, nil
	}
	if pFlag == "" {
		if cwd, err := os.Getwd(); err == nil && isModuleDir(cwd) {
			return cwd, nil
		}
	}
	h, manager, err := homeProjects()
	if err != nil {
		return "", err
	}
	if pFlag != "" {
		dir, ok := manager.Resolve(pFlag)
		if !ok {
			return "", fmt.Errorf("unknown project %q under %s", pFlag, h.ProjectsDir())
		}
		return dir, nil
	}
	if id := h.Selection(h.ProjectsDir()); id != "" {
		if dir, ok := manager.Resolve(id); ok {
			return dir, nil
		}
	}
	ps, err := manager.List()
	if err != nil {
		return "", err
	}
	switch len(ps) {
	case 0:
		return "", fmt.Errorf("not inside a CUE module and no projects under %s: pass -C <dir>, or run `cueto serve` once to seed the demo project", h.ProjectsDir())
	case 1:
		dir, _ := manager.Resolve(ps[0].ID)
		return dir, nil
	}
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	return "", fmt.Errorf("multiple projects under %s and none selected: pass -p <id> or run `cueto use <id>` (projects: %s)", h.ProjectsDir(), strings.Join(ids, ", "))
}

// resolveSchemaDir picks the cueto diagram schema dir: an explicit -cue wins,
// else the embedded schema is materialized under <home>/schema, so an installed
// binary works from any directory without a repo checkout.
func resolveSchemaDir(cueFlag string) (string, error) {
	if cueFlag != "" {
		return cueFlag, nil
	}
	root, err := home.DefaultRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, serveSchemaDirName)
	if err := assets.MaterializeSchema(dir); err != nil {
		return "", fmt.Errorf("materialize schema: %w", err)
	}
	return dir, nil
}

// resolveDirs is the two-step resolution every subcommand runs: the module root
// (resolveModuleDir) and the schema dir (resolveSchemaDir).
func resolveDirs(cFlag, pFlag, cueFlag string) (string, string, error) {
	moduleDir, err := resolveModuleDir(cFlag, pFlag)
	if err != nil {
		return "", "", err
	}
	schemaDir, err := resolveSchemaDir(cueFlag)
	if err != nil {
		return "", "", err
	}
	return moduleDir, schemaDir, nil
}

// runProjects lists the projects under the cueto home, marking the current one
// with a star, so a CLI-only user can see what -p accepts and what is selected.
func runProjects(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: cueto projects")
	}
	h, manager, err := homeProjects()
	if err != nil {
		return err
	}
	ps, err := manager.List()
	if err != nil {
		return err
	}
	if len(ps) == 0 {
		fmt.Printf("no projects under %s (run `cueto serve` once to seed the demo)\n", h.ProjectsDir())
		return nil
	}
	current := h.Selection(h.ProjectsDir())
	for _, p := range ps {
		marker := " "
		if p.ID == current {
			marker = "*"
		}
		fmt.Printf("%s %s\n", marker, p.ID)
	}
	return nil
}

// runUse persists the current project for the cueto home, the same state the web
// app writes, so the app and every later CLI call agree on the default.
func runUse(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: cueto use <project-id>")
	}
	h, manager, err := homeProjects()
	if err != nil {
		return err
	}
	id := args[0]
	if _, ok := manager.Resolve(id); !ok {
		return fmt.Errorf("unknown project %q under %s (see `cueto projects`)", id, h.ProjectsDir())
	}
	if err := h.SetSelection(h.ProjectsDir(), id); err != nil {
		return err
	}
	fmt.Printf("current project is now %s\n", id)
	return nil
}

// homeProjects resolves the standard home and its projects manager.
func homeProjects() (*home.Home, *projects.Manager, error) {
	root, err := home.DefaultRoot()
	if err != nil {
		return nil, nil, err
	}
	h := home.New(root)
	return h, projects.New(h.ProjectsDir()), nil
}

// isModuleDir reports whether dir is a CUE module root (has a cue.mod dir).
func isModuleDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "cue.mod"))
	return err == nil && info.IsDir()
}
