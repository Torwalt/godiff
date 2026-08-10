// Package config loads and merges godiff configuration: built-in defaults,
// the global XDG config file, the repository config file, and command-line
// overrides, in that order of precedence.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Torwalt/godiff/internal/gitx"
)

// RepoConfigName is the repository configuration file, looked up at the
// repository root.
const RepoConfigName = ".godiff.toml"

// File is the on-disk TOML schema, shared by global and repo config.
type File struct {
	// BaseBranch is the base for the built-in branch comparison and the
	// {base} placeholder.
	BaseBranch string `toml:"base_branch"`
	// Exclude lists exclusion pathspecs applied to every comparison.
	// Plain patterns are wrapped as `:(exclude)<pattern>`; patterns that
	// already start with `:` are passed to git verbatim.
	Exclude []string `toml:"exclude"`
	// Comparisons adds new comparisons or overrides built-ins by id.
	Comparisons []ComparisonFile `toml:"comparisons"`
}

// ComparisonFile is one [[comparisons]] entry.
type ComparisonFile struct {
	ID      string   `toml:"id"`
	Label   string   `toml:"label"`
	Kind    string   `toml:"kind"` // "diff" or "show"
	Args    []string `toml:"args"`
	Exclude []string `toml:"exclude"`
}

// Options are command-line overrides.
type Options struct {
	BaseBranch   string // overrides base_branch when non-empty
	NoRepoConfig bool   // skip the repository config file
}

// Config is the merged, validated result.
type Config struct {
	BaseBranch  string
	Exclude     []string
	Comparisons []gitx.Comparison
}

// Comparison returns the comparison with the given id.
func (c *Config) Comparison(id string) (gitx.Comparison, bool) {
	for _, cmp := range c.Comparisons {
		if cmp.ID == id {
			return cmp, true
		}
	}
	return gitx.Comparison{}, false
}

// Load reads the global and repository config files and merges them with
// built-in defaults and opts. A missing file is not an error; a malformed
// one is.
func Load(repoRoot string, opts Options) (*Config, error) {
	var files []File
	globalPath, err := globalConfigPath()
	if err == nil {
		f, err := readFile(globalPath)
		if err != nil {
			return nil, err
		}
		if f != nil {
			files = append(files, *f)
		}
	}
	if !opts.NoRepoConfig {
		f, err := readFile(filepath.Join(repoRoot, RepoConfigName))
		if err != nil {
			return nil, err
		}
		if f != nil {
			files = append(files, *f)
		}
	}
	return Merge(files, opts)
}

func globalConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "godiff", "config.toml"), nil
}

func readFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &f, nil
}

// Merge combines defaults with the given config files (lowest precedence
// first) and command-line options.
func Merge(files []File, opts Options) (*Config, error) {
	base := "master"
	var globalExclude []string
	for _, f := range files {
		if f.BaseBranch != "" {
			base = f.BaseBranch
		}
		globalExclude = append(globalExclude, f.Exclude...)
	}
	if opts.BaseBranch != "" {
		base = opts.BaseBranch
	}

	comparisons := builtins(base)
	for _, f := range files {
		for _, cf := range f.Comparisons {
			cmp, err := toComparison(cf, base)
			if err != nil {
				return nil, err
			}
			if i := indexByID(comparisons, cmp.ID); i >= 0 {
				comparisons[i] = cmp
			} else {
				comparisons = append(comparisons, cmp)
			}
		}
	}

	normalizedGlobal := normalizeExcludes(globalExclude)
	for i := range comparisons {
		comparisons[i].Exclude = append(normalizedGlobal, comparisons[i].Exclude...)
	}
	return &Config{
		BaseBranch:  base,
		Exclude:     append([]string(nil), normalizedGlobal...),
		Comparisons: comparisons,
	}, nil
}

func builtins(base string) []gitx.Comparison {
	return []gitx.Comparison{
		{ID: "worktree", Label: "Working tree", Kind: gitx.KindDiff},
		{ID: "staged", Label: "Staged changes", Kind: gitx.KindDiff, Args: []string{"--cached"}},
		{ID: "branch", Label: "Branch against " + base, Kind: gitx.KindDiff, Args: []string{base + "...HEAD"}},
		{ID: "commit", Label: "Current commit", Kind: gitx.KindShow, Args: []string{"HEAD"}},
	}
}

func toComparison(cf ComparisonFile, base string) (gitx.Comparison, error) {
	if cf.ID == "" {
		return gitx.Comparison{}, errors.New("comparison without id")
	}
	var kind gitx.Kind
	switch cf.Kind {
	case "diff", "":
		kind = gitx.KindDiff
	case "show":
		kind = gitx.KindShow
	default:
		return gitx.Comparison{}, fmt.Errorf("comparison %q: unknown kind %q (want diff or show)", cf.ID, cf.Kind)
	}
	if kind == gitx.KindShow && len(cf.Args) == 0 {
		return gitx.Comparison{}, fmt.Errorf("comparison %q: kind show requires a revision in args", cf.ID)
	}
	args := make([]string, len(cf.Args))
	for i, a := range cf.Args {
		args[i] = strings.ReplaceAll(a, "{base}", base)
	}
	label := cf.Label
	if label == "" {
		label = cf.ID
	}
	return gitx.Comparison{
		ID:      cf.ID,
		Label:   strings.ReplaceAll(label, "{base}", base),
		Kind:    kind,
		Args:    args,
		Exclude: normalizeExcludes(cf.Exclude),
	}, nil
}

// normalizeExcludes turns plain patterns into git exclusion pathspecs while
// passing explicit pathspecs (starting with ":") through untouched.
func normalizeExcludes(patterns []string) []string {
	var out []string
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, ":") {
			out = append(out, p)
		} else {
			out = append(out, ":(exclude)"+p)
		}
	}
	return out
}

func indexByID(cmps []gitx.Comparison, id string) int {
	for i, c := range cmps {
		if c.ID == id {
			return i
		}
	}
	return -1
}
