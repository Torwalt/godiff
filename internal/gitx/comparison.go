package gitx

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
// are applied to both discovery and display.
type Comparison struct {
	ID      string
	Label   string
	Kind    Kind
	Args    []string
	Exclude []string
}

// subcommand returns the git subcommand for the comparison kind.
func (c Comparison) subcommand() string {
	if c.Kind == KindShow {
		return "show"
	}
	return "diff"
}
