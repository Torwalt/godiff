package gitx

import (
	"fmt"
	"slices"
	"strings"
)

// Placeholders in Args and Label filled from a Review.
const (
	ParentPlaceholder = "{parent}"
	TipPlaceholder    = "{tip}"
)

// Kind selects how a comparison maps onto git subcommands.
type Kind int

const (
	// KindDiff compares two states with `git diff` (working tree, index,
	// revision ranges, branches).
	KindDiff Kind = iota
	// KindShow displays a single revision with `git show`.
	KindShow
)

// Comparison is a structured, shell-free description of one configured git
// comparison. Args are passed to git verbatim before the `--` separator;
// Exclude holds normalized git exclusion pathspecs (`:(exclude)...`) that
// are applied to both discovery and display. Untracked also reports
// untracked, non-ignored files as added; it only has an effect on diffs
// that read the working tree.
type Comparison struct {
	ID        string
	Label     string
	Kind      Kind
	Args      []string
	Exclude   []string
	Untracked bool
}

// subcommand returns the git subcommand for the comparison kind.
func (c Comparison) subcommand() string {
	if c.Kind == KindShow {
		return "show"
	}
	return "diff"
}

// UsesReview reports whether Args hold a Review placeholder.
func (c Comparison) UsesReview() bool {
	return slices.ContainsFunc(c.Args, func(a string) bool {
		return strings.Contains(a, ParentPlaceholder) || strings.Contains(a, TipPlaceholder)
	})
}

// WithReview fills the placeholders. When review is of a remote copy, the
// label names it, or marks the comparison local if it does not read it.
func (c Comparison) WithReview(review Review) Comparison {
	readsTip := slices.ContainsFunc(c.Args, func(a string) bool { return strings.Contains(a, TipPlaceholder) })
	args := make([]string, len(c.Args))
	for i, a := range c.Args {
		args[i] = strings.NewReplacer(ParentPlaceholder, review.Parent.Ref, TipPlaceholder, review.Rev).Replace(a)
	}
	c.Args = args
	c.Label = strings.ReplaceAll(c.Label, ParentPlaceholder, review.Parent.String())
	switch {
	case review.Rev == "HEAD":
	case readsTip:
		c.Label += " · " + review.Name
	default:
		c.Label += " (local)"
	}
	return c
}

// String names the parent with the commits above it.
func (p Parent) String() string {
	if p.Commits == 1 {
		return p.Name + " (1 commit)"
	}
	return fmt.Sprintf("%s (%d commits)", p.Name, p.Commits)
}
