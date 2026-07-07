// cueto
//
// Copyright: 2026, Lucas Jahier - Stratorys
// License: Mozilla Public License v2.0 (MPL v2.0)
// SPDX-License-Identifier: MPL-2.0

package domain

import (
	"fmt"
	"strings"
)

// The two top-level directories cueto owns, which a client buffer may never
// target. Only the first path segment is checked, so a nested sub/diagram/x.cue
// is unaffected.
const (
	reservedDiagramDir = "diagram"
	reservedModuleDir  = "cue.mod"
)

type tokKind int

const (
	tokSegment tokKind = iota // a maximal run of ASCII [A-Za-z0-9_-]
	tokSep                    // a '/' path separator
	tokDot                    // a '.'
)

type token struct {
	kind tokKind
	text string
	pos  int
}

// isIdentByte accepts ASCII segment bytes only, which is what makes any non-ASCII
// rune a lexer error.
func isIdentByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '_' || c == '-':
		return true
	default:
		return false
	}
}

// lex tokenizes name into segment, separator, and dot tokens, erroring on any
// other byte (backslash, colon, control, or a non-ASCII rune).
func lex(name string) ([]token, error) {
	var tokens []token
	for i := 0; i < len(name); {
		c := name[i]
		switch {
		case isIdentByte(c):
			start := i
			for i < len(name) && isIdentByte(name[i]) {
				i++
			}
			tokens = append(tokens, token{kind: tokSegment, text: name[start:i], pos: start})
		case c == '/':
			tokens = append(tokens, token{kind: tokSep, pos: i})
			i++
		case c == '.':
			tokens = append(tokens, token{kind: tokDot, pos: i})
			i++
		default:
			return nil, fmt.Errorf("illegal character %q at position %d", rune(c), i)
		}
	}
	return tokens, nil
}

// parseEditableName validates a token stream against the grammar: '/'-separated
// segments, every non-final segment a bare word, the final a word plus ".cue". An
// empty group (leading, trailing, or doubled separator, or empty input) fails.
func parseEditableName(tokens []token) bool {
	var groups [][]token
	current := []token{}
	for _, tok := range tokens {
		if tok.kind == tokSep {
			groups = append(groups, current)
			current = []token{}
			continue
		}
		current = append(current, tok)
	}
	groups = append(groups, current)

	for i, g := range groups {
		last := i == len(groups)-1
		if last {
			if !validFilenameGroup(g) {
				return false
			}
		} else if !validDirGroup(g) {
			return false
		}
	}

	// The reservation guards directories, so it applies only when the first segment
	// is a directory. A root file named diagram.cue collides with nothing.
	if len(groups) > 1 {
		first := groups[0][0].text
		if strings.EqualFold(first, reservedDiagramDir) || strings.EqualFold(first, reservedModuleDir) {
			return false
		}
	}
	return true
}

func validDirGroup(g []token) bool {
	return len(g) == 1 && g[0].kind == tokSegment
}

func validFilenameGroup(g []token) bool {
	if len(g) != 3 {
		return false
	}
	if g[0].kind != tokSegment || g[1].kind != tokDot || g[2].kind != tokSegment {
		return false
	}
	return g[2].text == "cue"
}

// ValidEditableName reports whether name is a safe client-supplied CUE filename: a
// relative path of bare-word segments ending in a .cue file, so files may live in
// subdirectories of the module root. It rejects any absolute path, traversal,
// separator trick, or non-ASCII byte, and reserves the cue.mod and diagram dirs.
// It is what lets the overlay and rewrite paths accept client filenames without a
// client escaping the module root. It lives in domain because both the evaluation
// and authoring concerns enforce it.
func ValidEditableName(name string) bool {
	tokens, err := lex(name)
	if err != nil {
		return false
	}
	return parseEditableName(tokens)
}
