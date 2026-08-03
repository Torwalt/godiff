package ui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	dirStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	statusStyle   = lipgloss.NewStyle().Faint(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	statusColors = map[byte]lipgloss.Style{
		'A': lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		'M': lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		'D': lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		'R': lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		'C': lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
	}
)

func styleStatus(code byte) string {
	if s, ok := statusColors[code]; ok {
		return s.Render(string(code))
	}
	return string(code)
}
