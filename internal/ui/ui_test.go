package ui_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Torwalt/godiff/internal/config"
	"github.com/Torwalt/godiff/internal/gitx"
	"github.com/Torwalt/godiff/internal/ui"
)

func newRepo(t *testing.T) (string, func(...string)) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "master")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
	return dir, git
}

func run(t *testing.T, model tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command to run")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		model, _ = model.Update(msg)
		return model
	}
	for _, c := range batch {
		if c != nil {
			model = run(t, model, c)
		}
	}
	return model
}

func press(t *testing.T, model tea.Model, r rune) tea.Model {
	t.Helper()
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return run(t, model, cmd)
}

func TestShowCommitMarksBranchesHeadBuildsOn(t *testing.T) {
	dir, git := newRepo(t)
	commit := func(subject string) { git("commit", "--allow-empty", "-m", subject) }
	commit("old work")
	git("branch", "old")
	commit("master work")
	git("checkout", "-b", "stack-1")
	commit("stack one work")
	git("checkout", "-b", "stack-2")
	commit("stack two work")
	git("branch", "stack-2-copy")
	git("checkout", "-b", "feature")
	commit("feature work")
	git("checkout", "--detach", "feature")
	git("update-ref", "refs/remotes/origin/stack-1", "stack-1")

	repo, err := gitx.Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	model := press(t, ui.New(repo, nil, nil, "master", ""), 'h')
	view := model.View()
	if !strings.Contains(selectedLine(view), "feature work") ||
		!strings.Contains(view, "stack two work  ◇ stack-2, stack-2-copy") || strings.Contains(view, "stack one work") {
		t.Errorf("branch commits do not stop at stack-2:\n%s", view)
	}

	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("work"), Paste: true})
	view = run(t, model, cmd).View()
	for subject, want := range map[string]string{
		"feature work":   "",
		"stack two work": "◇ stack-2, stack-2-copy",
		"stack one work": "◇ stack-1, origin/stack-1",
		"master work":    "◆ master",
		"old work":       "",
	} {
		i := strings.Index(view, subject)
		if i < 0 {
			t.Errorf("view misses %q:\n%s", subject, view)
			continue
		}
		rest, _, _ := strings.Cut(view[i+len(subject):], "\n")
		if got := strings.TrimSpace(rest); got != want {
			t.Errorf("%s: marker %q, want %q", subject, got, want)
		}
	}
}

func TestBranchChangesStartAtNearestBranch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   func(git func(...string))
		label   string
		files   []string
		without []string
	}{
		{
			name:    "nearest of two local branches",
			setup:   func(func(...string)) {},
			label:   "Branch changes since stack (2 commits)",
			files:   []string{"feature-1.txt", "feature-2.txt"},
			without: []string{"stack.txt", "lower.txt", "base.txt"},
		},
		{
			name: "remote-tracking branch only",
			setup: func(git func(...string)) {
				git("update-ref", "refs/remotes/origin/master", "master")
				git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/master")
				git("update-ref", "refs/remotes/origin/stack", "stack")
				git("branch", "-D", "stack")
			},
			label:   "Branch changes since origin/stack (2 commits)",
			files:   []string{"feature-1.txt", "feature-2.txt"},
			without: []string{"stack.txt"},
		},
		{
			name: "own branch behind HEAD on a remote",
			setup: func(git func(...string)) {
				git("update-ref", "refs/remotes/origin/feature", "feature~1")
				git("checkout", "--detach", "feature")
			},
			label:   "Branch changes since stack (2 commits)",
			files:   []string{"feature-1.txt", "feature-2.txt"},
			without: []string{"stack.txt"},
		},
		{
			name: "remote base ahead of the local one",
			setup: func(git func(...string)) {
				git("update-ref", "refs/remotes/origin/master", "stack")
				git("branch", "-D", "stack")
			},
			label:   "Branch changes since origin/master (2 commits)",
			files:   []string{"feature-1.txt", "feature-2.txt"},
			without: []string{"stack.txt"},
		},
		{
			name:    "no branch between base and HEAD",
			setup:   func(git func(...string)) { git("branch", "-D", "lower", "stack") },
			label:   "Branch changes since master (4 commits)",
			files:   []string{"lower.txt", "stack.txt", "feature-1.txt", "feature-2.txt"},
			without: []string{"base.txt"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, git := newRepo(t)
			commit := func(file string) {
				if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644); err != nil {
					t.Fatal(err)
				}
				git("add", file)
				git("commit", "-m", file)
			}
			commit("base.txt")
			git("checkout", "-b", "lower")
			commit("lower.txt")
			git("checkout", "-b", "stack")
			commit("stack.txt")
			git("checkout", "-b", "feature")
			commit("feature-1.txt")
			commit("feature-2.txt")
			tc.setup(git)

			repo, err := gitx.Detect(dir)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Merge(nil, config.Options{})
			if err != nil {
				t.Fatal(err)
			}
			model := tea.Model(ui.New(repo, cfg.Comparisons, nil, cfg.BaseBranch, ""))
			model = run(t, model, model.Init())
			if view := model.View(); !strings.Contains(view, tc.label+" (m)") {
				t.Errorf("selector misses %q:\n%s", tc.label, view)
			}

			view := press(t, model, 'm').View()
			if !strings.Contains(view, tc.label) {
				t.Errorf("tree title misses %q:\n%s", tc.label, view)
			}
			for _, f := range tc.files {
				if !strings.Contains(view, f) {
					t.Errorf("tree misses %s:\n%s", f, view)
				}
			}
			for _, f := range tc.without {
				if strings.Contains(view, f) {
					t.Errorf("tree shows %s:\n%s", f, view)
				}
			}
		})
	}
}

func TestBranchPickerListsBaseFirstAndRemoteBranches(t *testing.T) {
	dir, git := newRepo(t)
	git("commit", "--allow-empty", "-m", "base")
	git("checkout", "-b", "feature")
	git("commit", "--allow-empty", "-m", "feature")
	git("update-ref", "refs/remotes/origin/stack", "feature")
	git("update-ref", "refs/remotes/origin/master", "master")
	git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/master")

	repo, err := gitx.Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	view := press(t, ui.New(repo, nil, nil, "master", ""), 'b').View()
	lines := strings.Split(view, "\n")
	var branches []string
	for _, l := range lines {
		if l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), ">")); l == "master" || strings.HasPrefix(l, "origin") {
			branches = append(branches, l)
		}
	}
	if len(branches) != 3 || branches[0] != "master" {
		t.Errorf("branches = %q, want master first, then origin/master and origin/stack without origin/HEAD:\n%s", branches, view)
	}
}

func TestShowCommitListsBranchCommitsFromTheOldest(t *testing.T) {
	dir, git := newRepo(t)
	commit := func(subject, date string) {
		t.Setenv("GIT_COMMITTER_DATE", date)
		git("commit", "--allow-empty", "-m", subject)
	}
	commit("branched from", "2026-01-01T00:00:00")
	git("checkout", "-b", "feature")
	commit("first change", "2026-01-02T00:00:00")
	git("checkout", "master")
	commit("merged in", "2026-01-03T00:00:00")
	git("checkout", "feature")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-04T00:00:00")
	git("merge", "--no-ff", "-m", "merge master", "master")
	commit("second change", "2026-01-05T00:00:00")
	git("checkout", "master")
	commit("pulled later", "2026-01-06T00:00:00")
	git("checkout", "feature")

	repo, err := gitx.Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	model := press(t, ui.New(repo, nil, nil, "master", ""), 'h')
	view := model.View()
	for _, want := range []string{
		"show commit · HEAD since master (3 commits)",
		"second change\n",
		"merge master\n",
		"merged in  ◆ master",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view misses %q:\n%s", want, view)
		}
	}
	for _, master := range []string{"pulled later", "branched from"} {
		if strings.Contains(view, master) {
			t.Errorf("view lists %q of master:\n%s", master, view)
		}
	}
	if line := selectedLine(view); !strings.Contains(line, "first change") {
		t.Errorf("selected %q, want the oldest branch commit", line)
	}

	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if view := run(t, model, cmd).View(); !strings.Contains(view, "first change") {
		t.Errorf("enter does not open the oldest branch commit:\n%s", view)
	}

	git("checkout", "master")
	view = press(t, ui.New(repo, nil, nil, "master", ""), 'h').View()
	if line := selectedLine(view); !strings.Contains(line, "pulled later") || !strings.Contains(view, "branched from") {
		t.Errorf("on master: selected %q, want the full history from the top:\n%s", line, view)
	}
}

func TestReviewPicksTheNewerOfLocalAndRemoteBranch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(commit func(file, date string), git func(...string))
		header string
		files  []string
	}{
		{
			name: "local behind its remote copy",
			setup: func(commit func(string, string), git func(...string)) {
				commit("pushed.txt", "2026-02-01T00:00:00")
				git("update-ref", "refs/remotes/origin/feature", "feature")
				git("reset", "--hard", "HEAD~1")
			},
			header: "reviewing origin/feature · o: use local · local vs origin/feature: 1 behind",
			files:  []string{"pushed.txt"},
		},
		{
			name: "local ahead of its remote copy",
			setup: func(commit func(string, string), git func(...string)) {
				git("update-ref", "refs/remotes/origin/feature", "feature")
				commit("unpushed.txt", "2026-02-01T00:00:00")
			},
			header: "reviewing local feature · o: use origin/feature · local vs origin/feature: 1 ahead",
			files:  []string{"unpushed.txt"},
		},
		{
			name: "remote copy rebased later",
			setup: func(commit func(string, string), git func(...string)) {
				commit("local.txt", "2026-02-01T00:00:00")
				git("checkout", "-b", "remote", "HEAD~1")
				commit("remote.txt", "2026-03-01T00:00:00")
				git("update-ref", "refs/remotes/origin/feature", "remote")
				git("checkout", "feature")
				git("branch", "-D", "remote")
			},
			header: "reviewing origin/feature · o: use local · local vs origin/feature: 1 ahead, 1 behind",
			files:  []string{"remote.txt"},
		},
		{
			name: "local rebased later",
			setup: func(commit func(string, string), git func(...string)) {
				commit("local.txt", "2026-03-01T00:00:00")
				git("checkout", "-b", "remote", "HEAD~1")
				commit("remote.txt", "2026-02-01T00:00:00")
				git("update-ref", "refs/remotes/origin/feature", "remote")
				git("checkout", "feature")
				git("branch", "-D", "remote")
			},
			header: "reviewing local feature · o: use origin/feature · local vs origin/feature: 1 ahead, 1 behind",
			files:  []string{"local.txt"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, git := newRepo(t)
			commit := func(file, date string) {
				t.Setenv("GIT_COMMITTER_DATE", date)
				if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644); err != nil {
					t.Fatal(err)
				}
				git("add", file)
				git("commit", "-m", file)
			}
			commit("base.txt", "2026-01-01T00:00:00")
			git("checkout", "-b", "feature")
			commit("feature.txt", "2026-01-02T00:00:00")
			tc.setup(commit, git)

			repo, err := gitx.Detect(dir)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Merge(nil, config.Options{})
			if err != nil {
				t.Fatal(err)
			}
			model := tea.Model(ui.New(repo, cfg.Comparisons, nil, cfg.BaseBranch, ""))
			model = run(t, model, model.Init())
			view := model.View()
			if !strings.Contains(view, tc.header) {
				t.Errorf("selector misses %q:\n%s", tc.header, view)
			}
			if remote := strings.HasPrefix(tc.header, "reviewing origin"); remote != strings.Contains(view, "Working tree (local)") {
				t.Errorf("working tree marked local = %v, want %v:\n%s", !remote, remote, view)
			}

			tree := press(t, model, 'm').View()
			for _, f := range append(tc.files, "feature.txt") {
				if !strings.Contains(tree, f) {
					t.Errorf("branch changes miss %s:\n%s", f, tree)
				}
			}

			toggled := press(t, model, 'o')
			want := "reviewing origin/feature"
			if strings.HasPrefix(tc.header, want) {
				want = "reviewing local feature"
			}
			if view := toggled.View(); !strings.Contains(view, want) {
				t.Errorf("o does not switch to %q:\n%s", want, view)
			}
			if tree := press(t, toggled, 'm').View(); strings.Contains(tree, tc.files[0]) {
				t.Errorf("after o, branch changes still show %s:\n%s", tc.files[0], tree)
			}
		})
	}
}

func selectedLine(view string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, "> ") {
			return line
		}
	}
	return ""
}
