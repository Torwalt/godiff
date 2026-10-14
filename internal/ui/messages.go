package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type messagesMsg struct {
	text string
	err  error
}

func (m Model) startMessages() (tea.Model, tea.Cmd) {
	m.messages = viewport.New(0, 0)
	// b goes back, as on every other screen.
	m.messages.KeyMap.PageUp = key.NewBinding(key.WithKeys("pgup"))
	m.messagesText = ""
	m.messagesLoading = true
	m.screen = screenMessages
	m.status = ""
	repo, cmp := m.repo, *m.active
	return m, tea.Batch(m.spin.Tick, func() tea.Msg {
		text, err := repo.CommitMessages(cmp)
		return messagesMsg{text: text, err: err}
	})
}

func (m Model) onMessages(msg messagesMsg) (tea.Model, tea.Cmd) {
	if !m.messagesLoading {
		return m, nil
	}
	m.messagesLoading = false
	switch {
	case msg.err != nil:
		m.status = errorStyle.Render(msg.err.Error())
		m.screen = screenTree
	case strings.TrimSpace(msg.text) == "":
		m.status = statusStyle.Render("no commits in " + m.active.Label)
		m.screen = screenTree
	default:
		m.messagesText = strings.TrimRight(msg.text, "\n")
		m.layoutMessages()
	}
	m.clampScroll()
	return m, nil
}

func (m Model) updateMessages(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "b":
		m.messagesLoading = false
		m.screen = screenTree
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "g", "home":
		m.messages.GotoTop()
		return m, nil
	case "G", "end":
		m.messages.GotoBottom()
		return m, nil
	}
	var cmd tea.Cmd
	m.messages, cmd = m.messages.Update(msg)
	return m, cmd
}

// layoutMessages fits the viewport between title and footer and rewraps
// the messages to the window width.
func (m *Model) layoutMessages() {
	m.messages.Width = m.width
	m.messages.Height = max(m.height-4, 1)
	m.messages.SetContent(lipgloss.NewStyle().Width(m.width).Render(m.messagesText))
}

func (m Model) viewMessages() string {
	var b strings.Builder
	b.WriteString(m.title(m.active.Label + " · commits"))
	b.WriteString("\n\n")
	if m.messagesLoading {
		b.WriteString("  " + m.spin.View() + " loading commits…\n")
		return b.String()
	}
	b.WriteString(m.messages.View())
	b.WriteString("\n\n")
	b.WriteString(statusStyle.Render(fmt.Sprintf(
		"%3.f%% · ↑/k ↓/j scroll · space/pgdn pgup page · g/G top/bottom · esc/q/b back",
		m.messages.ScrollPercent()*100,
	)))
	return b.String()
}
