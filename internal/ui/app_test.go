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
		[]string{":(exclude)vendor"},
		"master",
		"",
	)
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestSelectorIncludesShowCommitAsFifthEntry(t *testing.T) {
	m := testModel()
	entries := m.selectorEntries()
	if len(entries) != 5 || !entries[4].showCommit {
		t.Fatalf("entries = %+v, want Show commit fifth", entries)
	}
	if view := m.viewSelector(); !strings.Contains(view, "Show commit (h)") {
		t.Errorf("selector missing Show commit:\n%s", view)
	}

	model, cmd := m.updateSelector(runeKey('h'))
	updated := model.(Model)
	if updated.screen != screenLogLoading || updated.selCursor != 4 || cmd == nil {
		t.Errorf("after h: screen=%v cursor=%d cmd=%v", updated.screen, updated.selCursor, cmd)
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
	if updated.screen != screenLoading || !updated.fromCommitTree || cmd == nil {
		t.Fatalf("screen=%v fromCommit=%v cmd=%v", updated.screen, updated.fromCommitTree, cmd)
	}
	wantArgs := []string{"full-two"}
	if updated.active.Kind != gitx.KindShow || !reflect.DeepEqual(updated.active.Args, wantArgs) {
		t.Errorf("active = %+v", *updated.active)
	}
	if !reflect.DeepEqual(updated.active.Exclude, []string{":(exclude)vendor"}) {
		t.Errorf("exclude = %v", updated.active.Exclude)
	}
}

func TestLogRangeBuildsInclusiveDiffComparison(t *testing.T) {
	m := testModel()
	m.screen = screenLog
	m.logCommits = []gitx.Commit{
		{SHA: "full-newest", ShortSHA: "newest", Subject: "newest subject"},
		{SHA: "full-middle", ShortSHA: "middle", Subject: "middle subject"},
		{SHA: "full-oldest", ShortSHA: "oldest", Subject: "oldest subject"},
	}
	m.logAnchor = 0
	m.logCursor = 2

	cmp := m.logComparison()
	if cmp.Kind != gitx.KindDiff {
		t.Fatalf("kind = %v, want diff", cmp.Kind)
	}
	wantArgs := []string{"full-oldest^", "full-newest"}
	if !reflect.DeepEqual(cmp.Args, wantArgs) {
		t.Errorf("args = %v, want %v", cmp.Args, wantArgs)
	}
	if cmp.Label != "oldest…newest (3 commits)" {
		t.Errorf("label = %q", cmp.Label)
	}
	if !reflect.DeepEqual(cmp.Exclude, []string{":(exclude)vendor"}) {
		t.Errorf("exclude = %v", cmp.Exclude)
	}

	m.logAnchor, m.logCursor = 2, 0
	if got := m.logComparison().Args; !reflect.DeepEqual(got, wantArgs) {
		t.Errorf("reverse selection args = %v, want %v", got, wantArgs)
	}
}

func TestLogRangeAndBaseMarkers(t *testing.T) {
	m := testModel()
	m.screen = screenLog
	m.logCommits = []gitx.Commit{
		{SHA: "new", ShortSHA: "1111111", Subject: "new"},
		{SHA: "middle", ShortSHA: "2222222", Subject: "middle"},
		{SHA: "base", ShortSHA: "3333333", Subject: "base"},
	}
	m.baseSHA = "base"

	model, _ := m.updateLog(runeKey(' '))
	updated := model.(Model)
	updated.logCursor = 2
	view := updated.viewLog()
	for _, marker := range []string{"┌ start 1111111", "│       2222222", "└ end   3333333", "◆ master"} {
		if !strings.Contains(view, marker) {
			t.Errorf("view missing %q:\n%s", marker, view)
		}
	}

	model, _ = updated.updateLog(runeKey(' '))
	if got := model.(Model).logAnchor; got != -1 {
		t.Errorf("anchor after clearing = %d", got)
	}
}

func TestLogPaginationAndReturnFromTree(t *testing.T) {
	m := testModel()
	m.screen = screenLog
	m.logCommits = make([]gitx.Commit, commitBatchSize)
	m.logCursor = commitBatchSize - commitLoadMargin
	m.logHasNext = true

	model, cmd := m.updateLog(runeKey('j'))
	updated := model.(Model)
	if updated.screen != screenLog || !updated.logLoading || cmd == nil {
		t.Fatalf("lazy load: screen=%v loading=%v cmd=%v", updated.screen, updated.logLoading, cmd)
	}

	m.screen = screenTree
	m.fromCommitTree = true
	m.root = nil
	model, _ = m.updateTree(runeKey('b'))
	updated = model.(Model)
	if updated.screen != screenLog || updated.logCursor != commitBatchSize-commitLoadMargin {
		t.Errorf("back: screen=%v cursor=%d", updated.screen, updated.logCursor)
	}
}

func TestCommitPageResultAndError(t *testing.T) {
	m := testModel()
	m.logRequest = 2
	m.logCommits = []gitx.Commit{{SHA: "old"}}
	commits := []gitx.Commit{{SHA: "full", ShortSHA: "short", Subject: "subject"}}
	model, _ := m.onCommits(commitsMsg{request: 2, append: true, commits: commits})
	updated := model.(Model)
	if updated.screen != screenLog || len(updated.logCommits) != 2 || updated.logCommits[1].SHA != "full" {
		t.Errorf("page result: screen=%v commits=%+v", updated.screen, updated.logCommits)
	}

	model, _ = updated.onCommits(commitsMsg{request: 1, commits: []gitx.Commit{{SHA: "stale"}}})
	updated = model.(Model)
	if len(updated.logCommits) != 2 {
		t.Errorf("stale result replaced commits: %+v", updated.logCommits)
	}

	m = testModel()
	m.logRequest = 4
	model, _ = m.onCommits(commitsMsg{request: 4, commits: commits, baseSHA: "full"})
	updated = model.(Model)
	if updated.baseSHA != "full" {
		t.Errorf("base SHA = %q", updated.baseSHA)
	}

	m = testModel()
	m.screen = screenLogLoading
	model, _ = m.onCommits(commitsMsg{err: errors.New("bad log")})
	updated = model.(Model)
	if updated.screen != screenSelector || !strings.Contains(updated.status, "bad log") {
		t.Errorf("error: screen=%v status=%q", updated.screen, updated.status)
	}
}

func TestLeavingLogIgnoresPendingPage(t *testing.T) {
	m := testModel()
	m.screen = screenLog
	m.logRequest = 3
	m.logLoading = true

	model, _ := m.updateLog(runeKey('b'))
	updated := model.(Model)
	if updated.screen != screenSelector || updated.logLoading || updated.logRequest != 4 {
		t.Fatalf("leave: screen=%v loading=%v request=%d", updated.screen, updated.logLoading, updated.logRequest)
	}

	model, _ = updated.onCommits(commitsMsg{
		request: 3,
		commits: []gitx.Commit{{SHA: "late"}},
	})
	updated = model.(Model)
	if updated.screen != screenSelector || len(updated.logCommits) != 0 {
		t.Errorf("late page reopened picker: screen=%v commits=%+v", updated.screen, updated.logCommits)
	}
}

func TestLogSearchModeTreatsJKAsInput(t *testing.T) {
	m := testModel()
	m.screen = screenLog
	m.logInput = newLogInput()

	model, cmd := m.updateLog(runeKey('/'))
	updated := model.(Model)
	if updated.logMode != logSearch || cmd == nil {
		t.Fatalf("start search: mode=%v cmd=%v", updated.logMode, cmd)
	}

	model, cmd = updated.updateLog(runeKey('j'))
	updated = model.(Model)
	if got := updated.logInput.Value(); got != "j" {
		t.Fatalf("query = %q, want j", got)
	}
	if updated.logCursor != 0 || !updated.logLoading || cmd == nil {
		t.Errorf("search reload: cursor=%d loading=%v cmd=%v", updated.logCursor, updated.logLoading, cmd)
	}

	model, _ = updated.updateLog(tea.KeyMsg{Type: tea.KeyEsc})
	updated = model.(Model)
	if updated.logMode != logNavigate || updated.logInput.Value() != "j" {
		t.Errorf("escape: mode=%v query=%q", updated.logMode, updated.logInput.Value())
	}
}

func TestLogPasteEntersSearchAndFilteredEscapeClears(t *testing.T) {
	m := testModel()
	m.screen = screenLog
	m.logInput = newLogInput()

	model, cmd := m.updateLog(tea.KeyMsg{Type: tea.KeyCtrlV})
	updated := model.(Model)
	if updated.logMode != logSearch || cmd == nil {
		t.Fatalf("paste: mode=%v cmd=%v", updated.logMode, cmd)
	}

	m = testModel()
	m.screen = screenLog
	m.logInput = newLogInput()
	model, cmd = m.updateLog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abc123"), Paste: true})
	updated = model.(Model)
	if updated.logMode != logSearch || updated.logInput.Value() != "abc123" || !updated.logLoading || cmd == nil {
		t.Fatalf("terminal paste: mode=%v query=%q loading=%v cmd=%v", updated.logMode, updated.logInput.Value(), updated.logLoading, cmd)
	}

	updated.logMode = logNavigate
	updated.logInput.SetValue("needle")
	model, cmd = updated.updateLog(tea.KeyMsg{Type: tea.KeyEsc})
	updated = model.(Model)
	if updated.screen != screenLog || updated.logInput.Value() != "" || !updated.logLoading || cmd == nil {
		t.Errorf("clear: screen=%v query=%q loading=%v cmd=%v", updated.screen, updated.logInput.Value(), updated.logLoading, cmd)
	}
}
