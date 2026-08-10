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

const logPageSize = 10

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
	fromLog    bool
}

type commitsMsg struct {
	page    int
	cursor  int
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
	baseBranch  string
	exclude     []string

	screen screen
	status string // last status or error message, shown on the active screen

	// selector state
	selCursor int

	// commit log state
	logCommits  []gitx.Commit
	logPage     int
	logCursor   int
	logHasNext  bool
	fromLogTree bool

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
func New(repo *gitx.Repo, comparisons []gitx.Comparison, baseBranch string, exclude []string, startID string) Model {
	m := Model{
		repo:        repo,
		comparisons: comparisons,
		baseBranch:  baseBranch,
		exclude:     append([]string(nil), exclude...),
		screen:      screenSelector,
		spin:        spinner.New(spinner.WithSpinner(spinner.Dot)),
		help:        help.New(),
		width:       80,
		height:      24,
	}
	if startID != "" {
		for i, entry := range m.selectorEntries() {
			if !entry.fromLog && comparisons[entry.comparison].ID == startID {
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

func (m Model) loadLogPage(page, cursor int) tea.Cmd {
	repo := m.repo
	base := m.baseBranch
	return func() tea.Msg {
		commits, hasNext, err := repo.LogPage(base, page*logPageSize, logPageSize)
		return commitsMsg{page: page, cursor: cursor, commits: commits, hasNext: hasNext, err: err}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.clampScroll()
		return m, nil

	case spinner.TickMsg:
		if m.screen != screenLoading && m.screen != screenLogLoading {
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

	if m.screen == screenSearch {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) onCommits(msg commitsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = errorStyle.Render(msg.err.Error())
		if len(m.logCommits) == 0 {
			m.screen = screenSelector
		} else {
			m.screen = screenLog
		}
		return m, nil
	}

	m.logCommits = msg.commits
	m.logPage = msg.page
	m.logCursor = min(msg.cursor, max(len(msg.commits)-1, 0))
	m.logHasNext = msg.hasNext
	m.status = ""
	m.screen = screenLog
	return m, nil
}

func (m Model) onDiscovered(msg discoveredMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = errorStyle.Render(msg.err.Error())
		if m.fromLogTree && !msg.refresh {
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
	case "l":
		for i, entry := range entries {
			if entry.fromLog {
				m.selCursor = i
				return m.startLog()
			}
		}
	default:
		for i, entry := range entries {
			if entry.fromLog {
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
	insertedLog := false
	for i, c := range m.comparisons {
		entries = append(entries, selectorEntry{comparison: i})
		if c.ID == "commit" {
			entries = append(entries, selectorEntry{fromLog: true})
			insertedLog = true
		}
	}
	if !insertedLog {
		entries = append(entries, selectorEntry{fromLog: true})
	}
	return entries
}

func (m Model) openSelectorEntry(entry selectorEntry) (tea.Model, tea.Cmd) {
	if entry.fromLog {
		return m.startLog()
	}
	return m.startComparison(m.comparisons[entry.comparison], false)
}

func (m Model) startComparison(cmp gitx.Comparison, fromLog bool) (tea.Model, tea.Cmd) {
	m.active = &cmp
	m.fromLogTree = fromLog
	m.screen = screenLoading
	m.status = ""
	return m, tea.Batch(m.spin.Tick, m.discover(cmp, false))
}

func (m Model) startLog() (tea.Model, tea.Cmd) {
	m.logCommits = nil
	m.logPage = 0
	m.logCursor = 0
	m.logHasNext = false
	m.fromLogTree = false
	m.screen = screenLogLoading
	m.status = ""
	return m, tea.Batch(m.spin.Tick, m.loadLogPage(0, 0))
}

func (m Model) updateLog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.logCursor > 0 {
			m.logCursor--
		} else if m.logPage > 0 {
			m.screen = screenLogLoading
			m.status = ""
			return m, tea.Batch(m.spin.Tick, m.loadLogPage(m.logPage-1, logPageSize-1))
		}
	case "down", "j":
		if m.logCursor < len(m.logCommits)-1 {
			m.logCursor++
		} else if m.logHasNext {
			m.screen = screenLogLoading
			m.status = ""
			return m, tea.Batch(m.spin.Tick, m.loadLogPage(m.logPage+1, 0))
		}
	case "enter":
		if m.logCursor < len(m.logCommits) {
			commit := m.logCommits[m.logCursor]
			cmp := gitx.Comparison{
				ID:      "log:" + commit.SHA,
				Label:   commit.ShortSHA + " " + commit.Subject,
				Kind:    gitx.KindShow,
				Args:    []string{commit.SHA},
				Exclude: append([]string(nil), m.exclude...),
			}
			return m.startComparison(cmp, true)
		}
	case "esc", "b":
		m.screen = screenSelector
		m.status = ""
	case "q", "ctrl+c":
		return m, tea.Quit
	}
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
		if m.fromLogTree {
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
		return fmt.Sprintf("\n %s loading commits against %s…\n", m.spin.View(), m.baseBranch)
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
		label := "From Log (l)"
		if !entry.fromLog {
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
	b.WriteString(titleStyle.Render("godiff — commits against " + m.baseBranch))
	b.WriteString("\n\n")
	if len(m.logCommits) == 0 {
		b.WriteString("  no commits\n")
	}
	for i, commit := range m.logCommits {
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
	b.WriteString(statusStyle.Render(fmt.Sprintf(
		"page %d · enter open · ↑/↓ move/page · esc back · q quit",
		m.logPage+1,
	)))
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
