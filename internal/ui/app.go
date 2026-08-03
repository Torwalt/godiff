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
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Torwalt/godiff/internal/gitx"
	"github.com/Torwalt/godiff/internal/tree"
)

type screen int

const (
	screenSelector screen = iota
	screenLoading
	screenTree
)

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

	screen screen
	status string // last status or error message, shown on the active screen

	// selector state
	selCursor int

	// tree state
	active *gitx.Comparison
	root   *tree.Node
	rows   []tree.Row
	cursor int
	offset int

	width, height int
	spin          spinner.Model
	help          help.Model
}

// New builds the initial model. If startID is non-empty the matching
// comparison is discovered immediately instead of showing the selector.
func New(repo *gitx.Repo, comparisons []gitx.Comparison, startID string) Model {
	m := Model{
		repo:        repo,
		comparisons: comparisons,
		screen:      screenSelector,
		spin:        spinner.New(spinner.WithSpinner(spinner.Dot)),
		help:        help.New(),
		width:       80,
		height:      24,
	}
	if startID != "" {
		for i, c := range comparisons {
			if c.ID == startID {
				m.selCursor = i
				m.screen = screenLoading
				m.active = &m.comparisons[i]
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

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.clampScroll()
		return m, nil

	case spinner.TickMsg:
		if m.screen != screenLoading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case discoveredMsg:
		return m.onDiscovered(msg)

	case diffDoneMsg:
		if msg.err != nil {
			m.status = errorStyle.Render(fmt.Sprintf("pager/git exited with error: %v", msg.err))
		}
		return m, nil

	case tea.KeyMsg:
		switch m.screen {
		case screenSelector:
			return m.updateSelector(msg)
		case screenTree:
			return m.updateTree(msg)
		case screenLoading:
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) onDiscovered(msg discoveredMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = errorStyle.Render(msg.err.Error())
		if m.root == nil {
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
		if r.Node == n {
			return i
		}
	}
	return 0
}

func (m Model) updateSelector(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selCursor > 0 {
			m.selCursor--
		}
	case "down", "j":
		if m.selCursor < len(m.comparisons)-1 {
			m.selCursor++
		}
	case "enter":
		cmp := m.comparisons[m.selCursor]
		m.screen = screenLoading
		m.status = ""
		return m, tea.Batch(m.spin.Tick, m.discover(cmp, false))
	case "q", "esc", "ctrl+c":
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
		n := m.rows[m.cursor].Node
		if len(n.Children) > 0 && !n.Expanded {
			n.Expanded = true
			m.rows = m.root.VisibleRows()
		}
	case key.Matches(msg, keys.Collapse):
		n := m.rows[m.cursor].Node
		switch {
		case n.Expanded && n.Kind != tree.KindRoot:
			n.Expanded = false
			m.rows = m.root.VisibleRows()
		case n.Parent != nil && n.Parent.Kind != tree.KindRoot:
			m.cursor = m.rowIndex(n.Parent)
		}
	case key.Matches(msg, keys.Open):
		n := m.rows[m.cursor].Node
		cmd := m.repo.DisplayCmd(*m.active, n.Path)
		m.status = ""
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return diffDoneMsg{err: err}
		})
	case key.Matches(msg, keys.Refresh):
		m.screen = screenLoading
		return m, tea.Batch(m.spin.Tick, m.discover(*m.active, true))
	case key.Matches(msg, keys.Back):
		m.screen = screenSelector
		m.status = ""
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	}
	m.clampScroll()
	return m, nil
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
	case screenLoading:
		label := ""
		if m.active != nil {
			label = m.active.Label
		}
		return fmt.Sprintf("\n %s loading %s…\n", m.spin.View(), label)
	case screenTree:
		return m.viewTree()
	}
	return ""
}

func (m Model) viewSelector() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("godiff — select comparison"))
	b.WriteString("\n\n")
	for i, c := range m.comparisons {
		line := "  " + c.Label
		if i == m.selCursor {
			line = selectedStyle.Render("> " + c.Label)
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
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(m.rows[i], i == m.cursor))
		b.WriteString("\n")
	}
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) renderRow(r tree.Row, selected bool) string {
	n := r.Node
	indent := strings.Repeat("  ", r.Depth)

	var line string
	switch n.Kind {
	case tree.KindRoot:
		line = "(all changes)"
	case tree.KindDir:
		marker := "▸ "
		if n.Expanded {
			marker = "▾ "
		}
		line = indent + marker + dirStyle.Render(n.Name+"/")
	case tree.KindFile:
		name := n.Name
		if n.OldPath != "" {
			name += " ← " + n.OldPath
		}
		line = indent + styleStatus(n.Status) + " " + name
	}
	if selected {
		return selectedStyle.Render("›") + " " + line
	}
	return "  " + line
}

func (m Model) footer() string {
	var b strings.Builder
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	b.WriteString(m.help.View(keys))
	return b.String()
}
