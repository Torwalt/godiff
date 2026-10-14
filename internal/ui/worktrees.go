package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Torwalt/godiff/internal/gitx"
)

// worktreeItem is one row of the worktree picker.
type worktreeItem struct {
	gitx.Worktree
	name     string // branch, or the short SHA of a detached HEAD
	path     string // relative to the main worktree, or ~-prefixed
	progress string // commits ahead and uncommitted paths; empty when unknown
}

type worktreesMsg struct {
	items []worktreeItem
	err   error
}

type worktreeRemovedMsg struct {
	item worktreeItem
	err  error
}

func newWorktreeItem(wt gitx.Worktree, mainRoot, base string) worktreeItem {
	item := worktreeItem{Worktree: wt, name: wt.Branch, path: wt.Path}
	if item.name == "" {
		item.name = "detached " + wt.Head[:min(len(wt.Head), 7)]
	}
	home, _ := os.UserHomeDir()
	if rel, err := filepath.Rel(mainRoot, wt.Path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		item.path = rel
	} else if home != "" && strings.HasPrefix(wt.Path, home+string(filepath.Separator)) {
		item.path = "~" + strings.TrimPrefix(wt.Path, home)
	}
	p, err := (&gitx.Repo{Root: wt.Path}).Progress(base)
	if err != nil {
		return item
	}
	var parts []string
	if p.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d ahead", p.Ahead))
	}
	if p.Changed > 0 {
		parts = append(parts, fmt.Sprintf("%d changed", p.Changed))
	}
	item.progress = strings.Join(parts, " · ")
	if item.progress == "" {
		item.progress = "clean"
	}
	return item
}

func (m Model) startWorktrees() (tea.Model, tea.Cmd) {
	m.worktrees = nil
	m.worktreeMatches = nil
	m.worktreeCursor = 0
	m.worktreeConfirm = nil
	m.worktreeLoading = true
	m.worktreeInput = textinput.New()
	m.worktreeInput.Prompt = "/ "
	m.worktreeInput.Focus()
	m.screen = screenWorktrees
	m.status = ""
	repo, base := m.repo, m.baseBranch
	return m, tea.Batch(m.spin.Tick, textinput.Blink, func() tea.Msg {
		worktrees, err := repo.Worktrees()
		if err != nil {
			return worktreesMsg{err: err}
		}
		items := make([]worktreeItem, len(worktrees))
		var wg sync.WaitGroup
		for i, wt := range worktrees {
			wg.Go(func() { items[i] = newWorktreeItem(wt, worktrees[0].Path, base) })
		}
		wg.Wait()
		return worktreesMsg{items: items}
	})
}

func (m Model) onWorktrees(msg worktreesMsg) (tea.Model, tea.Cmd) {
	if !m.worktreeLoading {
		return m, nil
	}
	m.worktreeLoading = false
	if msg.err != nil {
		m.status = errorStyle.Render(msg.err.Error())
		if m.screen == screenWorktrees {
			m.screen = screenSelector
		}
		return m, nil
	}
	m.worktrees = msg.items
	m.worktreeMatches = rankWorktrees(m.worktrees, m.worktreeInput.Value())
	return m, nil
}

func (m Model) updateWorktrees(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.worktreeConfirm != nil {
		item := *m.worktreeConfirm
		m.worktreeConfirm = nil
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "y", "Y":
			m.worktreeRemoving = true
			m.status = statusStyle.Render("deleting " + item.name + "…")
			repo := m.repo
			return m, func() tea.Msg {
				return worktreeRemovedMsg{item: item, err: repo.RemoveWorktree(item.Path)}
			}
		}
		return m, nil
	}
	switch msg.String() {
	case "esc", "q":
		m.worktreeLoading = false
		m.screen = screenSelector
		m.status = ""
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "ctrl+p", "ctrl+k":
		if m.worktreeCursor > 0 {
			m.worktreeCursor--
		}
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		if m.worktreeCursor < len(m.worktreeMatches)-1 {
			m.worktreeCursor++
		}
		return m, nil
	case "enter":
		if m.worktreeCursor >= len(m.worktreeMatches) {
			return m, nil
		}
		return m.openWorktree(m.worktreeMatches[m.worktreeCursor])
	case "ctrl+d":
		if m.worktreeRemoving || m.worktreeCursor >= len(m.worktreeMatches) {
			return m, nil
		}
		item := m.worktreeMatches[m.worktreeCursor]
		switch item.Path {
		case m.worktrees[0].Path:
			m.status = errorStyle.Render("cannot delete the main worktree")
		case m.repo.Root:
			m.status = errorStyle.Render("cannot delete the active worktree; switch to another one first")
		default:
			m.worktreeConfirm = &item
			m.status = ""
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.worktreeInput, cmd = m.worktreeInput.Update(msg)
	m.worktreeMatches = rankWorktrees(m.worktrees, m.worktreeInput.Value())
	m.worktreeCursor = min(m.worktreeCursor, max(len(m.worktreeMatches)-1, 0))
	return m, cmd
}

func (m Model) onWorktreeRemoved(msg worktreeRemovedMsg) (tea.Model, tea.Cmd) {
	m.worktreeRemoving = false
	if msg.err != nil {
		m.status = errorStyle.Render(msg.err.Error())
		return m, nil
	}
	m.worktrees = slices.DeleteFunc(m.worktrees, func(it worktreeItem) bool { return it.Path == msg.item.Path })
	m.worktreeMatches = rankWorktrees(m.worktrees, m.worktreeInput.Value())
	m.worktreeCursor = min(m.worktreeCursor, max(len(m.worktreeMatches)-1, 0))
	m.status = statusStyle.Render("deleted " + msg.item.name)
	return m, nil
}

// openWorktree points every later git invocation at the chosen checkout and
// returns to the selector, so a large worktree is not diffed wholesale.
func (m Model) openWorktree(item worktreeItem) (tea.Model, tea.Cmd) {
	m.repo = &gitx.Repo{Root: item.Path}
	m.worktree = item.name
	m.root, m.rows = nil, nil
	m.selCursor = 0
	m.screen = screenSelector
	m.status = statusStyle.Render("switched to " + item.name)
	return m, nil
}

func (m Model) viewWorktrees() string {
	var b strings.Builder
	b.WriteString(m.title("worktrees"))
	b.WriteString("\n")
	b.WriteString(m.worktreeInput.View())
	b.WriteString("\n\n")

	switch {
	case m.worktreeLoading:
		b.WriteString("  " + m.spin.View() + " loading worktrees…\n")
	case len(m.worktreeMatches) == 0:
		b.WriteString("  no matching worktrees\n")
	}
	nameWidth, progressWidth := 0, 0
	for _, it := range m.worktreeMatches {
		nameWidth = max(nameWidth, lipgloss.Width(it.name))
		progressWidth = max(progressWidth, lipgloss.Width(it.progress))
	}
	visible := max(m.height-6, 1)
	offset := max(m.worktreeCursor-visible+1, 0)
	end := min(offset+visible, len(m.worktreeMatches))
	for i := offset; i < end; i++ {
		it := m.worktreeMatches[i]
		current := "  "
		if it.Path == m.repo.Root {
			current = "● "
		}
		label := current + padRight(it.name, nameWidth) + "  " + padRight(it.progress, progressWidth) + "  " + it.path
		line := "  " + label
		if i == m.worktreeCursor {
			line = selectedStyle.Render("> " + label)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	if it := m.worktreeConfirm; it != nil {
		prompt := "delete worktree " + it.name
		if it.progress != "" {
			prompt += " (" + it.progress + ")"
		}
		b.WriteString(errorStyle.Render(prompt + "? y/n"))
	} else {
		b.WriteString(statusStyle.Render("enter open · ctrl+d delete · ↑/↓ move · esc/q back · ● active"))
	}
	return b.String()
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}
