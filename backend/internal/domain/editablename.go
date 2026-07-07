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

// The editable-name grammar is purely lexical - character classes, '/' segment
// separators, and a trailing .cue suffix - so it is validated with a small
// hand-written lexer and parser rather than a regular expression. The lexer only
// admits ASCII ident bytes, so unicode look-alikes are rejected by construction
// rather than by a separate normalization pass.

// reservedDiagramDir and reservedModuleDir are the two top-level directory names
// a client buffer may never target: the repo's own diagram schema package and
// the CUE module metadata directory. Only the first path segment is checked, so
// a nested dir of the same name (sub/diagram/x.cue) is unaffected. reservedModuleDir
// carries a dot and so can never lex to a valid directory segment; it is listed
// to match the stated reservation and to stay robust if the grammar changes.
const (
	reservedDiagramDir = "diagram"
	reservedModuleDir  = "cue.mod"
)

// tokKind enumerates the lexical atoms of an editable name.
type tokKind int

const (
	tokSegment tokKind = iota // a maximal run of ASCII [A-Za-z0-9_-]
	tokSep                    // a '/' path separator
	tokDot                    // a '.'
)

// token is one lexical atom with its source text and byte position.
type token struct {
	kind tokKind
	text string
	pos  int
}

// isIdentByte reports whether c is an allowed segment byte. The classes are ASCII
// only, which is what makes any non-ASCII rune a lexer error.
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

// lex tokenizes name into segment, separator, and dot tokens. It returns an error
// on the first byte that is none of those - a backslash, colon, space, control
// byte, or any byte of a non-ASCII rune - which is how separator tricks and
// unicode look-alikes are rejected before parsing.
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

// parseEditableName validates a token stream against the editable-name grammar:
// one or more '/'-separated segments, every non-final segment a single bare word,
// the final segment a word plus a ".cue" suffix. An empty group (from a leading,
// trailing, or doubled separator, or empty input) fails, as does a first segment
// that names a reserved directory.
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

	// The reservation guards directories, so it applies only when the first
	// segment is a directory (there is a segment after it). A root file such as
	// diagram.cue collides with nothing and stays valid.
	if len(groups) > 1 {
		first := groups[0][0].text
		if strings.EqualFold(first, reservedDiagramDir) || strings.EqualFold(first, reservedModuleDir) {
			return false
		}
	}
	return true
}

// validDirGroup accepts a single bare word: exactly one segment token, no dot.
func validDirGroup(g []token) bool {
	return len(g) == 1 && g[0].kind == tokSegment
}

// validFilenameGroup accepts a word plus a ".cue" suffix: segment, dot, segment
// where the trailing segment is exactly "cue".
func validFilenameGroup(g []token) bool {
	if len(g) != 3 {
		return false
	}
	if g[0].kind != tokSegment || g[1].kind != tokDot || g[2].kind != tokSegment {
		return false
	}
	return g[2].text == "cue"
}

// ValidEditableName reports whether name is a safe client-supplied CUE filename.
// It accepts a relative path whose every non-final segment is a bare word and
// whose final segment is a word with a .cue suffix, so files may live in
// subdirectories of the module root. It rejects any absolute path, traversal,
// separator trick, or non-ASCII byte, and reserves the two top-level directories
// cueto owns, cue.mod and diagram. The guard is what lets the overlay and the
// rewrite path accept client filenames without a client escaping the module root.
// It lives in domain because both the evaluation and authoring concerns enforce
// it, and evaluation must not depend on another concern to do so.
func ValidEditableName(name string) bool {
	tokens, err := lex(name)
	if err != nil {
		return false
	}
	return parseEditableName(tokens)
}
