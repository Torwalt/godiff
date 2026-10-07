package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Torwalt/godiff/internal/gitx"
	"github.com/Torwalt/godiff/internal/tree"
)

func testModel() Model {
	comparisons := []gitx.Comparison{
		{ID: "worktree", Label: "Working tree"},
		{ID: "staged", Label: "Staged changes"},
		{ID: "branch", Label: "Branch against master"},
		{ID: "all", Label: "All changes since master", Args: []string{"--merge-base", "master"}, Untracked: true},
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

func TestQActsLikeEscape(t *testing.T) {
	t.Run("selector", func(t *testing.T) {
		for _, msg := range []tea.KeyMsg{runeKey('q'), {Type: tea.KeyEsc}} {
			model, cmd := testModel().updateSelector(msg)
			if updated := model.(Model); updated.screen != screenSelector || cmd != nil {
				t.Errorf("key %q: screen=%v cmd=%v", msg.String(), updated.screen, cmd)
			}
		}
	})

	t.Run("loading", func(t *testing.T) {
		for _, msg := range []tea.KeyMsg{runeKey('q'), {Type: tea.KeyEsc}} {
			m := testModel()
			m.screen = screenLoading
			model, cmd := m.Update(msg)
			if updated := model.(Model); updated.screen != screenLoading || cmd != nil {
				t.Errorf("key %q: screen=%v cmd=%v", msg.String(), updated.screen, cmd)
			}
		}
	})

	t.Run("commit log", func(t *testing.T) {
		for _, msg := range []tea.KeyMsg{runeKey('q'), {Type: tea.KeyEsc}} {
			m := testModel()
			m.screen = screenLog
			model, cmd := m.updateLog(msg)
			if updated := model.(Model); updated.screen != screenSelector || cmd != nil {
				t.Errorf("key %q: screen=%v cmd=%v", msg.String(), updated.screen, cmd)
			}
		}
	})

	t.Run("commit log search", func(t *testing.T) {
		for _, msg := range []tea.KeyMsg{runeKey('q'), {Type: tea.KeyEsc}} {
			m := testModel()
			m.screen = screenLog
			m.logMode = logSearch
			m.logInput = newLogInput()
			m.logInput.Focus()
			model, cmd := m.updateLog(msg)
			if updated := model.(Model); updated.logMode != logNavigate || cmd != nil {
				t.Errorf("key %q: mode=%v cmd=%v", msg.String(), updated.logMode, cmd)
			}
		}
	})

	t.Run("tree", func(t *testing.T) {
		for _, msg := range []tea.KeyMsg{runeKey('q'), {Type: tea.KeyEsc}} {
			m := testModel()
			m.screen = screenTree
			m.root = tree.Build([]tree.FileEntry{{Path: "file.go", Status: 'M'}})
			m.rows = m.root.VisibleRows()
			model, cmd := m.updateTree(msg)
			if updated := model.(Model); updated.screen != screenSelector || cmd != nil {
				t.Errorf("key %q: screen=%v cmd=%v", msg.String(), updated.screen, cmd)
			}
		}
	})

	t.Run("search", func(t *testing.T) {
		for _, msg := range []tea.KeyMsg{runeKey('q'), {Type: tea.KeyEsc}} {
			m := testModel()
			m.screen = screenSearch
			model, cmd := m.updateSearch(msg)
			if updated := model.(Model); updated.screen != screenTree || cmd != nil {
				t.Errorf("key %q: screen=%v cmd=%v", msg.String(), updated.screen, cmd)
			}
		}
	})
}

func TestCollapseAllResetsTree(t *testing.T) {
	m := testModel()
	m.screen = screenTree
	m.height = 6
	m.root = tree.Build([]tree.FileEntry{
		{Path: "db/queries/q.sql", Status: 'M'},
		{Path: "db/migrations/m.sql", Status: 'M'},
		{Path: "internal/app/x.go", Status: 'M'},
	})
	fresh := len(m.root.VisibleRows())
	tree.ExpandTo(m.root.Find("db/queries/q.sql"))
	tree.ExpandTo(m.root.Find("internal/app/x.go"))
	m.rows = m.root.VisibleRows()
	m.cursor = len(m.rows) - 1
	m.clampScroll()

	model, cmd := m.updateTree(runeKey('H'))
	updated := model.(Model)
	if len(updated.rows) != fresh || updated.cursor != 0 || updated.offset != 0 || cmd != nil {
		t.Errorf("rows=%d want %d, cursor=%d offset=%d cmd=%v", len(updated.rows), fresh, updated.cursor, updated.offset, cmd)
	}
}

func TestSelectorIncludesPickers(t *testing.T) {
	m := testModel()
	entries := m.selectorEntries()
	if len(entries) != 8 || !entries[3].pickBranch || !entries[6].showCommit || !entries[7].pickWorktree {
		t.Fatalf("entries = %+v, want branch picker fourth, Show commit seventh and worktree picker last", entries)
	}
	view := m.viewSelector()
	for _, label := range []string{"Branch against… (b)", "All changes since master (a)", "Show commit (h)", "Worktree… (t)"} {
		if !strings.Contains(view, label) {
			t.Errorf("selector missing %q:\n%s", label, view)
		}
	}

	model, cmd := m.updateSelector(runeKey('h'))
	updated := model.(Model)
	if updated.screen != screenLogLoading || updated.selCursor != 6 || cmd == nil {
		t.Errorf("after h: screen=%v cursor=%d cmd=%v", updated.screen, updated.selCursor, cmd)
	}

	model, cmd = m.updateSelector(runeKey('b'))
	updated = model.(Model)
	if updated.screen != screenBranches || !updated.branchLoading || updated.selCursor != 3 || cmd == nil {
		t.Errorf("after b: screen=%v loading=%v cursor=%d cmd=%v", updated.screen, updated.branchLoading, updated.selCursor, cmd)
	}

	model, cmd = m.updateSelector(runeKey('t'))
	updated = model.(Model)
	if updated.screen != screenWorktrees || !updated.worktreeLoading || updated.selCursor != 7 || cmd == nil {
		t.Errorf("after t: screen=%v loading=%v cursor=%d cmd=%v", updated.screen, updated.worktreeLoading, updated.selCursor, cmd)
	}
}

func TestWorktreePickerOpensAllChangesInChosenCheckout(t *testing.T) {
	model, _ := testModel().updateSelector(runeKey('t'))
	model, _ = model.(Model).onWorktrees(worktreesMsg{items: []worktreeItem{
		{Worktree: gitx.Worktree{Path: "/tmp/repo", Branch: "master"}, name: "master", path: "/tmp/repo", progress: "clean"},
		{Worktree: gitx.Worktree{Path: "/tmp/repo/.claude/worktrees/sco-946", Branch: "sco-946-wt"}, name: "sco-946-wt", path: ".claude/worktrees/sco-946", progress: "5 ahead · 7 changed"},
		{Worktree: gitx.Worktree{Path: "/tmp/scratch/base", Head: "790b75a938b5"}, name: "detached 790b75a", path: "/tmp/scratch/base", progress: "2 changed"},
	}})
	m := model.(Model)
	if m.worktreeLoading || len(m.worktreeMatches) != 3 {
		t.Fatalf("loaded: loading=%v matches=%d", m.worktreeLoading, len(m.worktreeMatches))
	}
	view := m.viewWorktrees()
	for _, want := range []string{"● master", "5 ahead · 7 changed", ".claude/worktrees/sco-946", "detached 790b75a"} {
		if !strings.Contains(view, want) {
			t.Errorf("picker missing %q:\n%s", want, view)
		}
	}

	for _, r := range "946" {
		model, _ = m.updateWorktrees(runeKey(r))
		m = model.(Model)
	}
	if len(m.worktreeMatches) != 1 || m.worktreeMatches[0].name != "sco-946-wt" {
		t.Fatalf("matches = %+v", m.worktreeMatches)
	}

	model, cmd := m.updateWorktrees(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if m.screen != screenLoading || m.treeBack != screenWorktrees || cmd == nil {
		t.Fatalf("open: screen=%v back=%v cmd=%v", m.screen, m.treeBack, cmd)
	}
	if m.repo.Root != "/tmp/repo/.claude/worktrees/sco-946" || m.active.ID != "all" {
		t.Errorf("repo=%q active=%+v", m.repo.Root, *m.active)
	}
	if view := m.View(); !strings.Contains(view, "All changes since master @ sco-946-wt") {
		t.Errorf("loading view does not name the worktree:\n%s", view)
	}

	m.screen = screenTree
	model, _ = m.updateTree(runeKey('b'))
	m = model.(Model)
	if m.screen != screenWorktrees || m.worktreeInput.Value() != "946" {
		t.Errorf("back: screen=%v query=%q", m.screen, m.worktreeInput.Value())
	}
	if view := m.viewSelector(); !strings.Contains(view, "select comparison @ sco-946-wt") {
		t.Errorf("selector does not name the worktree:\n%s", view)
	}
}

func TestWorktreePickerError(t *testing.T) {
	model, _ := testModel().updateSelector(runeKey('t'))
	model, _ = model.(Model).onWorktrees(worktreesMsg{err: errors.New("no worktrees")})
	m := model.(Model)
	if m.screen != screenSelector || !strings.Contains(m.status, "no worktrees") {
		t.Errorf("error: screen=%v status=%q", m.screen, m.status)
	}
}

func TestBranchPickerFiltersAndOpensThreeDotDiff(t *testing.T) {
	model, _ := testModel().updateSelector(runeKey('b'))
	model, _ = model.(Model).onBranches(branchesMsg{branches: []string{"sco-961/salary-2-0", "master", "sco-935/recurring-read"}})
	m := model.(Model)
	if m.branchLoading || len(m.branchMatches) != 3 {
		t.Fatalf("loaded: loading=%v matches=%v", m.branchLoading, m.branchMatches)
	}

	for _, r := range "salary" {
		model, _ = m.updateBranches(runeKey(r))
		m = model.(Model)
	}
	if !reflect.DeepEqual(m.branchMatches, []string{"sco-961/salary-2-0"}) {
		t.Fatalf("matches = %v", m.branchMatches)
	}

	model, cmd := m.updateBranches(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if m.screen != screenLoading || m.treeBack != screenBranches || cmd == nil {
		t.Fatalf("open: screen=%v back=%v cmd=%v", m.screen, m.treeBack, cmd)
	}
	if !reflect.DeepEqual(m.active.Args, []string{"sco-961/salary-2-0...HEAD"}) || m.active.Kind != gitx.KindDiff {
		t.Errorf("active = %+v", *m.active)
	}
	if !reflect.DeepEqual(m.active.Exclude, []string{":(exclude)vendor"}) {
		t.Errorf("exclude = %v", m.active.Exclude)
	}

	m.screen = screenTree
	model, _ = m.updateTree(runeKey('b'))
	m = model.(Model)
	if m.screen != screenBranches || m.branchInput.Value() != "salary" {
		t.Errorf("back: screen=%v query=%q", m.screen, m.branchInput.Value())
	}
}

func TestBranchPickerError(t *testing.T) {
	model, _ := testModel().updateSelector(runeKey('b'))
	model, _ = model.(Model).onBranches(branchesMsg{err: errors.New("bad refs")})
	m := model.(Model)
	if m.screen != screenSelector || !strings.Contains(m.status, "bad refs") {
		t.Errorf("error: screen=%v status=%q", m.screen, m.status)
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
	if updated.screen != screenLoading || updated.treeBack != screenLog || cmd == nil {
		t.Fatalf("screen=%v back=%v cmd=%v", updated.screen, updated.treeBack, cmd)
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
	m.treeBack = screenLog
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
