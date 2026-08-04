package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Torwalt/godiff/internal/tree"
)

func TestFitNamesTruncatesWithRemainder(t *testing.T) {
	names := []string{"api/", "web/", "worker/", "x/", "y/"}
	tests := []struct {
		budget int
		want   string
	}{
		{40, "api/ web/ worker/ x/ y/"},
		{20, "api/ web/ worker/ +2"}, // exactly fills the budget
		{14, "api/ web/ +3"},
		{11, ""}, // below ghostMin: not worth a column
	}
	for _, tc := range tests {
		got := fitNames(names, tc.budget)
		if got != tc.want {
			t.Errorf("fitNames(budget=%d) = %q, want %q", tc.budget, got, tc.want)
		}
		if lipgloss.Width(got) > tc.budget {
			t.Errorf("fitNames(budget=%d) = %q, over budget", tc.budget, got)
		}
	}
}

func TestFitNamesShowsNothingRatherThanBareCount(t *testing.T) {
	if got := fitNames([]string{strings.Repeat("a", 40) + "/"}, 20); got != "" {
		t.Errorf("fitNames = %q, want empty when no name fits", got)
	}
}

func TestGhostGutterAlignsThenFallsBackInline(t *testing.T) {
	bodies := []string{"  ▸ db/", "  ▸ proto/core/"}
	if got := ghostGutter(bodies, 120); got != lipgloss.Width(bodies[1])+ghostGap {
		t.Errorf("gutter = %d, want widest body + gap", got)
	}
	if got := ghostGutter(bodies, 80); got != 0 {
		t.Errorf("gutter = %d, want inline previews on a narrow terminal", got)
	}
	// A viewport of long rows must still leave room for a preview.
	long := []string{"  ▸ " + strings.Repeat("a", 200) + "/"}
	if got := ghostGutter(long, 120); got != 120-cursorWidth-ghostMin {
		t.Errorf("gutter = %d, want clamped to leave preview room", got)
	}
}

func TestGhostOnlyPreviewsCollapsedDirs(t *testing.T) {
	root := tree.Build([]tree.FileEntry{
		{Path: "proto/core/conditions/c.proto", Status: 'M'},
		{Path: "proto/options/v1/o.proto", Status: 'M'},
		{Path: "top.txt", Status: 'M'},
	})
	m := Model{width: 120, root: root}
	rows := root.VisibleRows()

	proto := rows[1]
	ghost := m.ghost(proto, lipgloss.Width(rowBody(proto)), 0)
	if !strings.Contains(ghost, "core/conditions/") || !strings.Contains(ghost, "options/v1/") {
		t.Errorf("collapsed proto ghost = %q, want its folded child rows", ghost)
	}

	tree.SetExpanded(proto.Node, true)
	expanded := root.VisibleRows()[1]
	if got := m.ghost(expanded, lipgloss.Width(rowBody(expanded)), 0); got != "" {
		t.Errorf("expanded ghost = %q, want none", got)
	}
	// Files have nothing to preview.
	rows = root.VisibleRows()
	file := rows[len(rows)-1]
	if file.Node.Kind != tree.KindFile {
		t.Fatalf("last row = %+v, want the top-level file", file.Node)
	}
	if got := m.ghost(file, lipgloss.Width(rowBody(file)), 0); got != "" {
		t.Errorf("file ghost = %q, want none", got)
	}
}

func TestGhostPadsToGutter(t *testing.T) {
	root := tree.Build([]tree.FileEntry{{Path: "db/a.sql", Status: 'M'}})
	m := Model{width: 120, root: root}
	row := root.VisibleRows()[1]
	body := rowBody(row)

	ghost := m.ghost(row, lipgloss.Width(body), 40)
	if lipgloss.Width(body+ghost) != 40+lipgloss.Width("a.sql") {
		t.Errorf("ghost starts at column %d, want the gutter at 40",
			lipgloss.Width(body+ghost)-lipgloss.Width("a.sql"))
	}
}
