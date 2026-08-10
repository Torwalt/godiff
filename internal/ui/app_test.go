package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Torwalt/godiff/internal/gitx"
)

func testModel() Model {
	comparisons := []gitx.Comparison{
		{ID: "worktree", Label: "Working tree"},
		{ID: "staged", Label: "Staged changes"},
		{ID: "branch", Label: "Branch against master"},
		{ID: "commit", Label: "Current commit", Kind: gitx.KindShow},
	}
	return New(
		&gitx.Repo{Root: "/tmp/repo"},
		comparisons,
		"master",
		[]string{":(exclude)vendor"},
		"",
	)
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestSelectorIncludesFromLogAsFifthEntry(t *testing.T) {
	m := testModel()
	entries := m.selectorEntries()
	if len(entries) != 5 || !entries[4].fromLog {
		t.Fatalf("entries = %+v, want From Log fifth", entries)
	}
	if view := m.viewSelector(); !strings.Contains(view, "From Log (l)") {
		t.Errorf("selector missing From Log:\n%s", view)
	}

	model, cmd := m.updateSelector(runeKey('l'))
	updated := model.(Model)
	if updated.screen != screenLogLoading || updated.selCursor != 4 || cmd == nil {
		t.Errorf("after l: screen=%v cursor=%d cmd=%v", updated.screen, updated.selCursor, cmd)
	}
}

func TestLogSelectionBuildsShowComparison(t *testing.T) {
	m := testModel()
	m.screen = screenLog
	m.logCommits = []gitx.Commit{
		{SHA: "full-one", ShortSHA: "one", Subject: "first"},
		{SHA: "full-two", ShortSHA: "two", Subject: "second"},
	}
	m.logCursor = 1

	model, cmd := m.updateLog(tea.KeyMsg{Type: tea.KeyEnter})
	updated := model.(Model)
	if updated.screen != screenLoading || !updated.fromLogTree || cmd == nil {
		t.Fatalf("screen=%v fromLog=%v cmd=%v", updated.screen, updated.fromLogTree, cmd)
	}
	wantArgs := []string{"full-two"}
	if updated.active.Kind != gitx.KindShow || !reflect.DeepEqual(updated.active.Args, wantArgs) {
		t.Errorf("active = %+v", *updated.active)
	}
	if !reflect.DeepEqual(updated.active.Exclude, []string{":(exclude)vendor"}) {
		t.Errorf("exclude = %v", updated.active.Exclude)
	}
}

func TestLogPaginationAndReturnFromTree(t *testing.T) {
	m := testModel()
	m.screen = screenLog
	m.logCommits = make([]gitx.Commit, logPageSize)
	m.logCursor = logPageSize - 1
	m.logHasNext = true

	model, cmd := m.updateLog(runeKey('j'))
	updated := model.(Model)
	if updated.screen != screenLogLoading || cmd == nil {
		t.Fatalf("next page: screen=%v cmd=%v", updated.screen, cmd)
	}

	m.screen = screenTree
	m.fromLogTree = true
	m.root = nil
	model, _ = m.updateTree(runeKey('b'))
	updated = model.(Model)
	if updated.screen != screenLog || updated.logCursor != logPageSize-1 {
		t.Errorf("back: screen=%v cursor=%d", updated.screen, updated.logCursor)
	}
}

func TestCommitPageResultAndError(t *testing.T) {
	m := testModel()
	commits := []gitx.Commit{{SHA: "full", ShortSHA: "short", Subject: "subject"}}
	model, _ := m.onCommits(commitsMsg{page: 1, cursor: 9, commits: commits})
	updated := model.(Model)
	if updated.screen != screenLog || updated.logPage != 1 || updated.logCursor != 0 {
		t.Errorf("page result: screen=%v page=%d cursor=%d", updated.screen, updated.logPage, updated.logCursor)
	}

	m = testModel()
	model, _ = m.onCommits(commitsMsg{err: errors.New("bad base")})
	updated = model.(Model)
	if updated.screen != screenSelector || !strings.Contains(updated.status, "bad base") {
		t.Errorf("error: screen=%v status=%q", updated.screen, updated.status)
	}
}
