package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Torwalt/godiff/internal/tree"
)

const (
	cursorWidth = 2  // the "› " / "  " prefix every row carries
	ghostGap    = 2  // minimum space between a row and its preview
	ghostMin    = 12 // below this, a preview says too little to be worth it
	ghostAlign  = 90 // narrower terminals place previews inline instead
)

// ghostGutter is the column where previews start, shared by all rows on
// screen so they read as one column. Zero means previews sit inline right
// after each row, which avoids squeezing them on narrow terminals.
func ghostGutter(bodies []string, width int) int {
	if width < ghostAlign {
		return 0
	}
	col := 0
	for _, body := range bodies {
		col = max(col, lipgloss.Width(body)+ghostGap)
	}
	return min(col, max(width-cursorWidth-ghostMin, 0))
}

// ghost renders the dim preview of a collapsed directory's contents, padded
// out to the gutter. Expanded rows show their children literally, so a
// preview there would only repeat the lines below it.
func (m Model) ghost(r tree.Row, bodyWidth, gutter int) string {
	if r.Node.Kind != tree.KindDir || r.Expanded {
		return ""
	}
	col := max(gutter, bodyWidth+ghostGap)
	text := fitNames(r.Node.ChildNames(), m.width-cursorWidth-col)
	if text == "" {
		return ""
	}
	return strings.Repeat(" ", col-bodyWidth) + ghostStyle.Render(text)
}

// fitNames joins as many names as budget allows, reporting the rest as
// "+n". It returns "" when not even one name fits, since a bare count is
// not worth a column.
func fitNames(names []string, budget int) string {
	if budget < ghostMin {
		return ""
	}
	var b strings.Builder
	shown := 0
	for _, name := range names {
		width := lipgloss.Width(name)
		if shown > 0 {
			width++ // separating space
		}
		if rest := len(names) - shown - 1; rest > 0 {
			width += 2 + len(strconv.Itoa(rest)) // " +n"
		}
		if lipgloss.Width(b.String())+width > budget {
			break
		}
		if shown > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(name)
		shown++
	}
	if shown == 0 {
		return ""
	}
	if rest := len(names) - shown; rest > 0 {
		b.WriteString(" +" + strconv.Itoa(rest))
	}
	return b.String()
}
