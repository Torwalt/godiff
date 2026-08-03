package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Expand   key.Binding
	Collapse key.Binding
	Open     key.Binding
	Refresh  key.Binding
	Back     key.Binding
	Quit     key.Binding
	Help     key.Binding
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Expand:   key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand")),
	Collapse: key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "collapse")),
	Open:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open diff")),
	Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Back:     key.NewBinding(key.WithKeys("esc", "b"), key.WithHelp("esc/b", "comparisons")),
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
}

// ShortHelp implements help.KeyMap for the tree screen.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Open, k.Refresh, k.Back, k.Quit, k.Help}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Expand, k.Collapse},
		{k.Open, k.Refresh, k.Back, k.Quit},
	}
}
