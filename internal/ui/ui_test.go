package ui_test

import (
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Torwalt/godiff/internal/gitx"
	"github.com/Torwalt/godiff/internal/ui"
)

func TestShowCommitMarksBranchesHeadBuildsOn(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	commit := func(subject string) { git("commit", "--allow-empty", "-m", subject) }
	git("init", "-b", "master")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
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

	repo, err := gitx.Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	var model tea.Model = ui.New(repo, nil, nil, "master", "")
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("h did not start loading commits")
	}
	for _, c := range batch {
		model, _ = model.Update(c())
	}

	view := model.View()
	for subject, want := range map[string]string{
		"feature work":   "",
		"stack two work": "◇ stack-2, stack-2-copy",
		"stack one work": "◇ stack-1",
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
