// Package ui contains the Bubble Tea models for godiff: comparison
// selection, changed-path loading, tree navigation, and the handoff to the
// external git/pager process.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Torwalt/godiff/internal/gitx"
	"github.com/Torwalt/godiff/internal/tree"
)

type screen int

const (
	screenSelector screen = iota
	screenLogLoading
	screenLog
	screenLoading
	screenTree
	screenSearch
)

const (
	commitBatchSize  = 50
	commitLoadMargin = 5
)

type logMode int

const (
	logNavigate logMode = iota
	logSearch
)

// selectorShortcuts maps built-in comparison IDs to a single key that both
// jumps to and opens that comparison. User-defined comparisons get none.
var selectorShortcuts = map[string]string{
	"worktree": "w",
	"staged":   "s",
	"branch":   "m",
	"commit":   "c",
}

type selectorEntry struct {
	comparison int
	showCommit bool
}

type commitsMsg struct {
	request uint64
	append  bool
	commits []gitx.Commit
	hasNext bool
	err     error
}

type discoveredMsg struct {
	cmp     gitx.Comparison
	files   []gitx.ChangedFile
	err     error
	refresh bool
}

type diffDoneMsg struct{ err error }

// Model is the root Bubble Tea model and explicit state machine.
type Model struct {
	repo        *gitx.Repo
	comparisons []gitx.Comparison
	exclude     []string

	screen screen
	status string // last status or error message, shown on the active screen

	// selector state
	selCursor int

	// commit log state
	logCommits     []gitx.Commit
	logCursor      int
	logOffset      int
	logHasNext     bool
	logLoading     bool
	logMode        logMode
	logInput       textinput.Model
	logRequest     uint64
	fromCommitTree bool

	// tree state
	active *gitx.Comparison
	root   *tree.Node
	rows   []tree.Row
	cursor int
	offset int

	// directory search state
	searchInput   textinput.Model
	searchMatches []*tree.Node
	searchCursor  int

	width, height int
	spin          spinner.Model
	help          help.Model
}

// New builds the initial model. If startID is non-empty the matching
// comparison is discovered immediately instead of showing the selector.
func New(repo *gitx.Repo, comparisons []gitx.Comparison, exclude []string, startID string) Model {
	m := Model{
		repo:        repo,
		comparisons: comparisons,
		exclude:     append([]string(nil), exclude...),
		screen:      screenSelector,
		spin:        spinner.New(spinner.WithSpinner(spinner.Dot)),
		help:        help.New(),
		width:       80,
		height:      24,
	}
	if startID != "" {
		for i, entry := range m.selectorEntries() {
			if !entry.showCommit && comparisons[entry.comparison].ID == startID {
				m.selCursor = i
				m.screen = screenLoading
				m.active = &m.comparisons[entry.comparison]
			}
		}
	}
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	if m.screen == screenLoading {
		return tea.Batch(m.spin.Tick, m.discover(*m.active, false))
	}
	return nil
}

// discover returns a command running changed-path discovery off the UI
// goroutine; the result comes back as a message to Update.
func (m Model) discover(cmp gitx.Comparison, refresh bool) tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		files, err := repo.Discover(cmp)
		return discoveredMsg{cmp: cmp, files: files, err: err, refresh: refresh}
	}
}

func (m Model) loadLogPage(query string, offset int, appendPage bool, request uint64) tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		commits, hasNext, err := repo.LogPage(query, offset, commitBatchSize)
		return commitsMsg{
			request: request,
			append:  appendPage,
			commits: commits,
			hasNext: hasNext,
			err:     err,
		}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.clampScroll()
		m.clampLogScroll()
		return m, nil

	case spinner.TickMsg:
		if m.screen != screenLoading && m.screen != screenLogLoading && !m.logLoading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case discoveredMsg:
		return m.onDiscovered(msg)

	case commitsMsg:
		return m.onCommits(msg)

	case diffDoneMsg:
		if msg.err != nil {
			m.status = errorStyle.Render(fmt.Sprintf("pager/git exited with error: %v", msg.err))
		}
		return m, nil

	case tea.KeyMsg:
		switch m.screen {
		case screenSelector:
			return m.updateSelector(msg)
		case screenLog:
			return m.updateLog(msg)
		case screenTree:
			return m.updateTree(msg)
		case screenSearch:
			return m.updateSearch(msg)
		case screenLoading, screenLogLoading:
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}
	}

	if m.screen == screenLog && m.logMode == logSearch {
		return m.updateLogInput(msg)
	}
	if m.screen == screenSearch {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) onCommits(msg commitsMsg) (tea.Model, tea.Cmd) {
	if msg.request != m.logRequest {
		return m, nil
	}
	m.logLoading = false
	if msg.err != nil {
		m.status = errorStyle.Render(msg.err.Error())
		if len(m.logCommits) == 0 {
			if m.screen == screenLogLoading {
				m.screen = screenSelector
			} else {
				m.screen = screenLog
			}
		} else {
			m.screen = screenLog
		}
		return m, nil
	}

	if msg.append {
		m.logCommits = append(m.logCommits, msg.commits...)
	} else {
		m.logCommits = msg.commits
		m.logCursor = 0
		m.logOffset = 0
	}
	m.logHasNext = msg.hasNext
	m.status = ""
	m.screen = screenLog
	m.clampLogScroll()
	return m, nil
}

func (m Model) onDiscovered(msg discoveredMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = errorStyle.Render(msg.err.Error())
		if m.fromCommitTree && !msg.refresh {
			m.screen = screenLog
		} else if m.root == nil {
			m.screen = screenSelector // discovery never succeeded: go back
		} else {
			m.screen = screenTree // keep the stale tree usable
		}
		return m, nil
	}

	entries := make([]tree.FileEntry, len(msg.files))
	for i, f := range msg.files {
		entries[i] = tree.FileEntry{Path: f.Path, OldPath: f.OldPath, Status: f.Status}
	}
	newRoot := tree.Build(entries)

	if msg.refresh && m.root != nil {
		newRoot.RestoreExpanded(m.root.ExpandedPaths())
		selected := newRoot.FindClosest(m.selectedPath())
		tree.ExpandTo(selected)
		m.root = newRoot
		m.rows = newRoot.VisibleRows()
		m.cursor = m.rowIndex(selected)
		m.status = statusStyle.Render(fmt.Sprintf("refreshed: %d changed files", len(msg.files)))
	} else {
		cmp := msg.cmp
		m.active = &cmp
		m.root = newRoot
		m.rows = newRoot.VisibleRows()
		m.cursor = 0
		m.offset = 0
		m.status = ""
	}
	m.screen = screenTree
	m.clampScroll()
	return m, nil
}

func (m *Model) selectedPath() string {
	if m.cursor < len(m.rows) {
		return m.rows[m.cursor].Node.Path
	}
	return ""
}

func (m *Model) rowIndex(n *tree.Node) int {
	for i, r := range m.rows {
		if r.Contains(n) {
			return i
		}
	}
	return 0
}

func (m Model) updateSelector(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	entries := m.selectorEntries()
	switch msg.String() {
	case "up", "k":
		if m.selCursor > 0 {
			m.selCursor--
		}
	case "down", "j":
		if m.selCursor < len(entries)-1 {
			m.selCursor++
		}
	case "enter":
		return m.openSelectorEntry(entries[m.selCursor])
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "h":
		for i, entry := range entries {
			if entry.showCommit {
				m.selCursor = i
				return m.startLog()
			}
		}
	default:
		for i, entry := range entries {
			if entry.showCommit {
				continue
			}
			c := m.comparisons[entry.comparison]
			if k, ok := selectorShortcuts[c.ID]; !ok || k != msg.String() {
				continue
			}
			m.selCursor = i
			return m.startComparison(c, false)
		}
	}
	return m, nil
}

func (m Model) selectorEntries() []selectorEntry {
	entries := make([]selectorEntry, 0, len(m.comparisons)+1)
	insertedShow := false
	for i, c := range m.comparisons {
		entries = append(entries, selectorEntry{comparison: i})
		if c.ID == "commit" {
			entries = append(entries, selectorEntry{showCommit: true})
			insertedShow = true
		}
	}
	if !insertedShow {
		entries = append(entries, selectorEntry{showCommit: true})
	}
	return entries
}

func (m Model) openSelectorEntry(entry selectorEntry) (tea.Model, tea.Cmd) {
	if entry.showCommit {
		return m.startLog()
	}
	return m.startComparison(m.comparisons[entry.comparison], false)
}

func (m Model) startComparison(cmp gitx.Comparison, fromCommit bool) (tea.Model, tea.Cmd) {
	m.active = &cmp
	m.fromCommitTree = fromCommit
	m.screen = screenLoading
	m.status = ""
	return m, tea.Batch(m.spin.Tick, m.discover(cmp, false))
}

func (m Model) startLog() (tea.Model, tea.Cmd) {
	m.logCommits = nil
	m.logCursor = 0
	m.logOffset = 0
	m.logHasNext = false
	m.logLoading = true
	m.logMode = logNavigate
	m.logInput = newLogInput()
	m.logRequest++
	m.fromCommitTree = false
	m.screen = screenLogLoading
	m.status = ""
	return m, tea.Batch(m.spin.Tick, m.loadLogPage("", 0, false, m.logRequest))
}

func (m Model) updateLog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.logMode == logSearch {
		switch msg.String() {
		case "esc":
			m.logMode = logNavigate
			m.logInput.Blur()
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		case "up", "ctrl+p", "ctrl+n", "down":
			if msg.String() == "up" || msg.String() == "ctrl+p" {
				m.moveLogCursor(-1)
			} else {
				m.moveLogCursor(1)
			}
			return m.maybeLoadMore()
		case "enter":
			return m.openLogCommit()
		}
		return m.updateLogInput(msg)
	}

	if msg.Paste {
		m.logMode = logSearch
		m.logInput.Focus()
		return m.updateLogInput(msg)
	}

	switch msg.String() {
	case "up", "k":
		m.moveLogCursor(-1)
	case "down", "j":
		m.moveLogCursor(1)
	case "enter":
		return m.openLogCommit()
	case "/":
		m.logMode = logSearch
		m.logInput.Focus()
		return m, textinput.Blink
	case "ctrl+v":
		m.logMode = logSearch
		m.logInput.Focus()
		return m.updateLogInput(msg)
	case "esc":
		if m.logInput.Value() != "" {
			m.logInput.Reset()
			return m.reloadLog()
		}
		return m.leaveLog()
	case "b":
		return m.leaveLog()
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m.maybeLoadMore()
}

func newLogInput() textinput.Model {
	input := textinput.New()
	input.Prompt = "/ "
	return input
}

func (m Model) updateLogInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.logInput.Value()
	var inputCmd tea.Cmd
	m.logInput, inputCmd = m.logInput.Update(msg)
	if m.logInput.Err != nil {
		m.status = errorStyle.Render(fmt.Sprintf("paste failed: %v", m.logInput.Err))
	}
	if m.logInput.Value() == before {
		return m, inputCmd
	}
	updated, loadCmd := m.reloadLog()
	return updated, tea.Batch(inputCmd, loadCmd)
}

func (m Model) reloadLog() (tea.Model, tea.Cmd) {
	m.logRequest++
	m.logCommits = nil
	m.logCursor = 0
	m.logOffset = 0
	m.logHasNext = false
	m.logLoading = true
	m.status = ""
	return m, tea.Batch(
		m.spin.Tick,
		m.loadLogPage(m.logInput.Value(), 0, false, m.logRequest),
	)
}

func (m Model) maybeLoadMore() (tea.Model, tea.Cmd) {
	m.clampLogScroll()
	if m.logLoading || !m.logHasNext || len(m.logCommits)-m.logCursor > commitLoadMargin {
		return m, nil
	}
	m.logLoading = true
	return m, tea.Batch(
		m.spin.Tick,
		m.loadLogPage(m.logInput.Value(), len(m.logCommits), true, m.logRequest),
	)
}

func (m *Model) moveLogCursor(delta int) {
	m.logCursor = min(max(m.logCursor+delta, 0), max(len(m.logCommits)-1, 0))
	m.clampLogScroll()
}

func (m Model) openLogCommit() (tea.Model, tea.Cmd) {
	if m.logCursor >= len(m.logCommits) {
		return m, nil
	}
	commit := m.logCommits[m.logCursor]
	m.logMode = logNavigate
	m.logInput.Blur()
	m.logRequest++
	m.logLoading = false
	cmp := gitx.Comparison{
		ID:      "show:" + commit.SHA,
		Label:   commit.ShortSHA + " " + commit.Subject,
		Kind:    gitx.KindShow,
		Args:    []string{commit.SHA},
		Exclude: append([]string(nil), m.exclude...),
	}
	return m.startComparison(cmp, true)
}

func (m Model) leaveLog() (tea.Model, tea.Cmd) {
	m.logRequest++
	m.logLoading = false
	m.screen = screenSelector
	m.status = ""
	return m, nil
}

func (m Model) updateTree(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case key.Matches(msg, keys.Expand):
		r := m.rows[m.cursor]
		if len(r.Node.Children) > 0 && !r.Expanded {
			tree.SetExpanded(r.Node, true)
			m.rows = m.root.VisibleRows()
		}
	case key.Matches(msg, keys.Collapse):
		r := m.rows[m.cursor]
		// The parent row is the one above the folded chain, not the chain's
		// own second-to-last directory.
		parent := tree.Anchor(r.Node).Parent
		switch {
		case r.Expanded && r.Node.Kind != tree.KindRoot:
			tree.SetExpanded(r.Node, false)
			m.rows = m.root.VisibleRows()
		case parent != nil && parent.Kind != tree.KindRoot:
			m.cursor = m.rowIndex(parent)
		}
	case key.Matches(msg, keys.Open):
		r := m.rows[m.cursor]
		if r.Node.Kind == tree.KindDir {
			tree.SetExpanded(r.Node, !r.Expanded)
			m.rows = m.root.VisibleRows()
			break
		}
		return m.openDiff(r.Node)
	case key.Matches(msg, keys.Diff):
		return m.openDiff(m.rows[m.cursor].Node)
	case key.Matches(msg, keys.Search):
		return m.startSearch()
	case key.Matches(msg, keys.Refresh):
		m.screen = screenLoading
		return m, tea.Batch(m.spin.Tick, m.discover(*m.active, true))
	case key.Matches(msg, keys.Back):
		if m.fromCommitTree {
			m.screen = screenLog
		} else {
			m.screen = screenSelector
		}
		m.status = ""
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	}
	m.clampScroll()
	return m, nil
}

func (m Model) openDiff(n *tree.Node) (tea.Model, tea.Cmd) {
	cmd := m.repo.DisplayCmd(*m.active, n.Path)
	m.status = ""
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return diffDoneMsg{err: err}
	})
}

func (m Model) startSearch() (tea.Model, tea.Cmd) {
	m.searchInput = textinput.New()
	m.searchInput.Prompt = "/ "
	m.searchInput.Focus()
	m.searchMatches = rankDirs(m.root.Dirs(), "")
	m.searchCursor = 0
	m.screen = screenSearch
	m.status = ""
	return m, textinput.Blink
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.screen = screenTree
		return m, nil
	case "up", "ctrl+p", "ctrl+k":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		if m.searchCursor < len(m.searchMatches)-1 {
			m.searchCursor++
		}
		return m, nil
	case "enter":
		if m.searchCursor < len(m.searchMatches) {
			n := m.searchMatches[m.searchCursor]
			tree.ExpandTo(n)
			tree.SetExpanded(n, true)
			m.rows = m.root.VisibleRows()
			m.cursor = m.rowIndex(n)
			m.clampScroll()
		}
		m.screen = screenTree
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.searchMatches = rankDirs(m.root.Dirs(), m.searchInput.Value())
	if m.searchCursor >= len(m.searchMatches) {
		m.searchCursor = max(len(m.searchMatches)-1, 0)
	}
	return m, cmd
}

// clampScroll keeps the cursor inside the visible window.
func (m *Model) clampScroll() {
	visible := m.treeHeight()
	if visible < 1 {
		visible = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) clampLogScroll() {
	visible := m.logHeight()
	if m.logCursor < m.logOffset {
		m.logOffset = m.logCursor
	}
	if m.logCursor >= m.logOffset+visible {
		m.logOffset = m.logCursor - visible + 1
	}
	maxOffset := max(len(m.logCommits)-visible, 0)
	m.logOffset = min(max(m.logOffset, 0), maxOffset)
}

func (m Model) logHeight() int {
	reserved := 4 // title, spacing, footer, and its leading newline
	if m.logMode == logSearch || m.logInput.Value() != "" {
		reserved++
	}
	if m.status != "" {
		reserved++
	}
	return max(m.height-reserved, 1)
}

// treeHeight is the number of rows available for the tree between header
// and footer.
func (m *Model) treeHeight() int {
	return m.height - 2 - strings.Count(m.footer(), "\n") - 1
}

// View implements tea.Model.
func (m Model) View() string {
	switch m.screen {
	case screenSelector:
		return m.viewSelector()
	case screenLogLoading:
		return fmt.Sprintf("\n %s loading commits…\n", m.spin.View())
	case screenLog:
		return m.viewLog()
	case screenLoading:
		label := ""
		if m.active != nil {
			label = m.active.Label
		}
		return fmt.Sprintf("\n %s loading %s…\n", m.spin.View(), label)
	case screenTree:
		return m.viewTree()
	case screenSearch:
		return m.viewSearch()
	}
	return ""
}

func (m Model) viewSearch() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("godiff — search directories"))
	b.WriteString("\n")
	b.WriteString(m.searchInput.View())
	b.WriteString("\n\n")

	if len(m.searchMatches) == 0 {
		b.WriteString("  no matching directories\n")
	}
	visible := m.height - 5
	if visible < 1 {
		visible = 1
	}
	offset := 0
	if m.searchCursor >= visible {
		offset = m.searchCursor - visible + 1
	}
	end := min(offset+visible, len(m.searchMatches))
	for i := offset; i < end; i++ {
		line := "  " + m.searchMatches[i].Path + "/"
		if i == m.searchCursor {
			line = selectedStyle.Render("> " + m.searchMatches[i].Path + "/")
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("enter jump · ↑/↓ move · esc cancel"))
	return b.String()
}

func (m Model) viewSelector() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("godiff — select comparison"))
	b.WriteString("\n\n")
	for i, entry := range m.selectorEntries() {
		label := "Show commit (h)"
		if !entry.showCommit {
			c := m.comparisons[entry.comparison]
			label = c.Label
			if k, ok := selectorShortcuts[c.ID]; ok {
				label += " (" + k + ")"
			}
		}
		line := "  " + label
		if i == m.selCursor {
			line = selectedStyle.Render("> " + label)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	b.WriteString(statusStyle.Render("enter open · ↑/↓ move · q quit"))
	return b.String()
}

func (m Model) viewLog() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("godiff — show commit"))
	b.WriteString("\n")
	if m.logMode == logSearch {
		b.WriteString(m.logInput.View())
		b.WriteString("\n")
	} else if m.logInput.Value() != "" {
		b.WriteString(statusStyle.Render("filter: " + m.logInput.Value()))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if len(m.logCommits) == 0 {
		if m.logLoading {
			b.WriteString("  " + m.spin.View() + " searching…\n")
		} else {
			b.WriteString("  no matching commits\n")
		}
	}
	end := min(m.logOffset+m.logHeight(), len(m.logCommits))
	for i := m.logOffset; i < end; i++ {
		commit := m.logCommits[i]
		label := commit.ShortSHA + " " + commit.Subject
		line := "  " + label
		if i == m.logCursor {
			line = selectedStyle.Render("> " + label)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	if m.logMode == logSearch {
		b.WriteString(statusStyle.Render("enter open · ↑/↓ choose · esc navigate · ctrl-v paste"))
	} else if m.logInput.Value() != "" {
		b.WriteString(statusStyle.Render("enter open · ↑/k ↓/j move · / edit · esc clear · b back"))
	} else {
		b.WriteString(statusStyle.Render("enter open · ↑/k ↓/j move · / search · ctrl-v paste · esc back"))
	}
	return b.String()
}

func (m Model) viewTree() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("godiff — " + m.active.Label))
	b.WriteString("\n")

	if len(m.root.Children) == 0 {
		b.WriteString("\n  no changes\n\n")
		b.WriteString(m.footer())
		return b.String()
	}

	visible := m.treeHeight()
	end := min(m.offset+visible, len(m.rows))
	bodies := make([]string, end-m.offset)
	for i := range bodies {
		bodies[i] = rowBody(m.rows[m.offset+i])
	}
	gutter := ghostGutter(bodies, m.width)
	for i := m.offset; i < end; i++ {
		body := bodies[i-m.offset]
		line := body + m.ghost(m.rows[i], lipgloss.Width(body), gutter)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("›") + " " + line)
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	b.WriteString(m.footer())
	return b.String()
}

func rowBody(r tree.Row) string {
	n := r.Node
	indent := strings.Repeat("  ", r.Depth)

	switch n.Kind {
	case tree.KindDir:
		marker := "▸ "
		if r.Expanded {
			marker = "▾ "
		}
		return indent + marker + dirStyle.Render(r.Label+"/")
	case tree.KindFile:
		name := r.Label
		if n.OldPath != "" {
			name += " ← " + n.OldPath
		}
		return indent + styleStatus(n.Status) + " " + name
	default:
		return "(all changes)"
	}
}

func (m Model) footer() string {
	var b strings.Builder
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	b.WriteString(m.help.View(keys))
	return b.String()
}
