// godiff is a terminal navigator for git changes: pick a comparison, browse
// changed paths as a tree, and open focused diffs through git's configured
// pager (delta).
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Torwalt/godiff/internal/config"
	"github.com/Torwalt/godiff/internal/gitx"
	"github.com/Torwalt/godiff/internal/ui"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "godiff:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		comparison   = flag.String("comparison", "", "start directly in the comparison with this id")
		base         = flag.String("base", "", "override the configured base branch for this run")
		repoPath     = flag.String("repo", ".", "path inside the git repository to open")
		noRepoConfig = flag.Bool("no-repo-config", false, "ignore the repository's "+config.RepoConfigName)
		printConfig  = flag.Bool("print-config", false, "print the effective configuration and exit")
		showVersion  = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("godiff", version)
		return nil
	}

	repo, err := gitx.Detect(*repoPath)
	if err != nil {
		return err
	}

	cfg, err := config.Load(repo.Root, config.Options{
		BaseBranch:   *base,
		NoRepoConfig: *noRepoConfig,
	})
	if err != nil {
		return err
	}

	if *printConfig {
		printEffectiveConfig(repo, cfg)
		return nil
	}

	if *comparison != "" {
		if _, ok := cfg.Comparison(*comparison); !ok {
			return fmt.Errorf("unknown comparison %q (use -print-config to list)", *comparison)
		}
	}

	m := ui.New(repo, cfg.Comparisons, cfg.Exclude, *comparison)
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func printEffectiveConfig(repo *gitx.Repo, cfg *config.Config) {
	fmt.Println("repository:", repo.Root)
	fmt.Println("base branch:", cfg.BaseBranch)
	fmt.Println("comparisons:")
	for _, c := range cfg.Comparisons {
		kind := "diff"
		if c.Kind == gitx.KindShow {
			kind = "show"
		}
		fmt.Printf("  %-12s %-30s git %s %s\n", c.ID, c.Label, kind, strings.Join(c.Args, " "))
		for _, e := range c.Exclude {
			fmt.Printf("  %-12s   exclude %s\n", "", e)
		}
	}
}
