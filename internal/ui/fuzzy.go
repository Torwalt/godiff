package ui

import (
	"sort"
	"strings"

	"github.com/Torwalt/godiff/internal/tree"
)

// fuzzyMatch reports whether query is a case-insensitive subsequence of
// path and returns a score, lower is better. Matches at path segment
// starts are free, contiguous runs are cheap, and gaps are penalized, so
// "dq" ranks "db/queries" above "docs/quirks/x".
func fuzzyMatch(query, path string) (int, bool) {
	if query == "" {
		return len(path), true
	}
	q := strings.ToLower(query)
	t := strings.ToLower(path)
	score, qi, prev := 0, 0, -2
	for i := 0; i < len(t) && qi < len(q); i++ {
		if t[i] != q[qi] {
			continue
		}
		switch {
		case i == 0 || t[i-1] == '/':
		case prev == i-1:
			score++
		default:
			score += 4
		}
		prev = i
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	return score + len(t)/4, true
}

// rankDirs filters dirs by fuzzy-matching query against their paths and
// returns them best-first; ties break lexicographically.
func rankDirs(dirs []*tree.Node, query string) []*tree.Node {
	type scored struct {
		node  *tree.Node
		score int
	}
	var matches []scored
	for _, d := range dirs {
		if s, ok := fuzzyMatch(query, d.Path); ok {
			matches = append(matches, scored{d, s})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		return matches[i].node.Path < matches[j].node.Path
	})
	out := make([]*tree.Node, len(matches))
	for i, m := range matches {
		out[i] = m.node
	}
	return out
}
