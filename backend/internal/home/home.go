// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

// Package home owns cueto's standard on-disk location: the single root that
// holds the hand-edited config.cue, the machine-written state.json, and the
// default projects directory. The root resolves to $XDG_DATA_HOME/cueto when
// XDG_DATA_HOME is set, else ~/.cueto, so both the server and the CLI agree on
// where things live without any environment variable naming a project.
package home

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

const (
	configFile   = "config.cue"
	stateFile    = "state.json"
	projectsName = "projects"
)

// configSchema validates config.cue. Every field is optional; defaults live in
// the caller (the serve command), so an absent or empty file is a valid config.
const configSchema = `
#Config: {
	port?:           int & >0 & <65536
	maxBodyBytes?:   int & >0
	maxOutputBytes?: int & >0
	evalTimeoutMs?:  int & >0
	maxConcurrent?:  int & >0
}
`

// Home is a resolved cueto root directory.
type Home struct {
	root string
}

// DefaultRoot resolves the standard cueto root: $XDG_DATA_HOME/cueto when
// XDG_DATA_HOME is set (the XDG base-directory convention), else ~/.cueto.
func DefaultRoot() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "cueto"), nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(dir, ".cueto"), nil
}

// New returns a Home rooted at dir. The directory need not exist yet; Ensure
// creates it.
func New(root string) *Home {
	return &Home{root: root}
}

// Root returns the home root directory.
func (h *Home) Root() string { return h.root }

// ProjectsDir returns the default projects root under the home.
func (h *Home) ProjectsDir() string { return filepath.Join(h.root, projectsName) }

// Ensure creates the root and projects directories when missing.
func (h *Home) Ensure() error {
	return os.MkdirAll(h.ProjectsDir(), 0o755)
}

// Config is the hand-edited server configuration from config.cue. Zero values
// mean "not set"; the caller applies its defaults on top.
type Config struct {
	Port           int   `json:"port"`
	MaxBodyBytes   int64 `json:"maxBodyBytes"`
	MaxOutputBytes int   `json:"maxOutputBytes"`
	EvalTimeoutMs  int   `json:"evalTimeoutMs"`
	MaxConcurrent  int   `json:"maxConcurrent"`
}

// LoadConfig reads and validates config.cue. A missing file is an empty valid
// config; a file that fails the schema is an error naming the violation, so a
// typo surfaces at startup rather than as a silently ignored setting.
func (h *Home) LoadConfig() (Config, error) {
	content, err := os.ReadFile(filepath.Join(h.root, configFile))
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	ctx := cuecontext.New()
	schema := ctx.CompileString(configSchema).LookupPath(cue.ParsePath("#Config"))
	value := schema.Unify(ctx.CompileString(string(content), cue.Filename(configFile)))
	if err := value.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", configFile, err)
	}
	var cfg Config
	if err := value.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", configFile, err)
	}
	return cfg, nil
}

// State is the machine-written selection state. Selections maps an absolute
// projects root to the id of its current project, so a dev server pointed at a
// different root never clobbers the standard root's selection.
type State struct {
	Selections map[string]string `json:"selections"`
}

// ReadState returns the persisted state; a missing or empty file is an empty
// state, and a corrupt file is an error rather than a silent reset.
func (h *Home) ReadState() (State, error) {
	content, err := os.ReadFile(filepath.Join(h.root, stateFile))
	if errors.Is(err, os.ErrNotExist) {
		return State{Selections: map[string]string{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(content, &state); err != nil {
		return State{}, fmt.Errorf("%s: %w", stateFile, err)
	}
	if state.Selections == nil {
		state.Selections = map[string]string{}
	}
	return state, nil
}

// Selection returns the persisted current project id for a projects root, or ""
// when none (or when the state file is unreadable; selection is best-effort).
func (h *Home) Selection(projectsDir string) string {
	state, err := h.ReadState()
	if err != nil {
		return ""
	}
	return state.Selections[absKey(projectsDir)]
}

// SetSelection persists the current project id for a projects root, creating the
// home root when missing. It rewrites the whole file atomically (temp + rename)
// so a crash never leaves a torn state file.
func (h *Home) SetSelection(projectsDir, id string) error {
	state, err := h.ReadState()
	if err != nil {
		return err
	}
	state.Selections[absKey(projectsDir)] = id
	if err := os.MkdirAll(h.root, 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(h.root, stateFile+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(content, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(h.root, stateFile))
}

// absKey normalizes a projects root to an absolute path so the same directory
// always maps to the same selection regardless of how the caller spelled it.
func absKey(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}
