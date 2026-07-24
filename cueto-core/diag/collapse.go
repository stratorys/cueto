package diag

import (
	"regexp"
	"sort"
	"strings"

	cueerrors "cuelang.org/go/cue/errors"
)

// conflictRE extracts the two operands of a CUE "conflicting values X and Y" message.
// Operands are whitespace-free (quoted strings, type names, or literals).
var conflictRE = regexp.MustCompile(`conflicting values (\S+) and (\S+)`)

// groupByPath partitions the error list by failing path, preserving first-seen order of
// both groups and siblings within a group.
func groupByPath(list []cueerrors.Error) [][]cueerrors.Error {
	index := make(map[string]int)
	var groups [][]cueerrors.Error
	for _, e := range list {
		key := strings.Join(e.Path(), ".")
		if i, ok := index[key]; ok {
			groups[i] = append(groups[i], e)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, []cueerrors.Error{e})
	}
	return groups
}

// hasEmptyDisjunction reports whether any sibling is the empty-disjunction summary.
func hasEmptyDisjunction(group []cueerrors.Error) bool {
	for _, e := range group {
		if strings.Contains(e.Error(), "empty disjunction") {
			return true
		}
	}
	return false
}

// collapseDisjunction folds an empty-disjunction group into one diagnostic. When the
// conflicting siblings reveal a single common offending value and the remaining
// operands are quoted string disjuncts, the result is a NonexistentReference carrying
// the sorted alternatives; otherwise it degrades to a plain EmptyDisjunction.
func collapseDisjunction(group []cueerrors.Error) Diagnostic {
	path := ""
	for _, e := range group {
		if p := strings.Join(e.Path(), "."); p != "" {
			path = p
			break
		}
	}
	raw := renderRaw(group)
	origins := originsOf(group)

	offender, alternatives, ok := disjunctAlternatives(group)
	if !ok {
		return Diagnostic{
			Class:   EmptyDisjunction,
			Message: strings.TrimSpace(path + ": no disjunct matched"),
			Origins: origins,
			Raw:     raw,
		}
	}

	return Diagnostic{
		Class:        NonexistentReference,
		Message:      path + ": " + offender + " is not a known reference; expected one of " + strings.Join(alternatives, ", "),
		Origins:      origins,
		Alternatives: alternatives,
		Raw:          raw,
	}
}

// disjunctAlternatives inspects the conflicting-values siblings and returns the single
// value common to every pair (the offending value) and the sorted set of the other
// operands (the legal disjuncts), both unquoted. It reports ok=false unless exactly one
// operand is shared across all pairs and every alternative is a quoted string.
func disjunctAlternatives(group []cueerrors.Error) (offender string, alternatives []string, ok bool) {
	var pairs [][2]string
	for _, e := range group {
		m := conflictRE.FindStringSubmatch(e.Error())
		if m == nil {
			continue
		}
		pairs = append(pairs, [2]string{m[1], m[2]})
	}
	if len(pairs) == 0 {
		return "", nil, false
	}

	// The offending value is present in every conflict pair; intersect operand sets.
	common := map[string]int{}
	for _, p := range pairs {
		seen := map[string]bool{p[0]: true, p[1]: true}
		for v := range seen {
			common[v]++
		}
	}
	var shared []string
	for v, n := range common {
		if n == len(pairs) {
			shared = append(shared, v)
		}
	}
	if len(shared) != 1 {
		return "", nil, false
	}
	offender = shared[0]

	altSet := map[string]bool{}
	for _, p := range pairs {
		other := p[0]
		if other == offender {
			other = p[1]
		}
		if !isQuoted(other) {
			return "", nil, false
		}
		altSet[unquote(other)] = true
	}
	for a := range altSet {
		alternatives = append(alternatives, a)
	}
	sort.Strings(alternatives)
	return unquote(offender), alternatives, true
}

// renderRaw joins a group's native error messages, preserving CUE's own wording.
func renderRaw(group []cueerrors.Error) string {
	lines := make([]string, 0, len(group))
	for _, e := range group {
		lines = append(lines, e.Error())
	}
	return strings.Join(lines, "\n")
}

func isQuoted(s string) bool {
	return len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"'
}

func unquote(s string) string {
	if isQuoted(s) {
		return s[1 : len(s)-1]
	}
	return s
}
