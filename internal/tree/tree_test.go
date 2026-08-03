package tree

import (
	"strings"
	"testing"
)

func entries(paths ...string) []FileEntry {
	var out []FileEntry
	for _, p := range paths {
		out = append(out, FileEntry{Path: p, Status: 'M'})
	}
	return out
}

// expandAll opens every directory so the full structure is visible.
func expandAll(n *Node) {
	n.Expanded = true
	for _, c := range n.Children {
		if c.Kind != KindFile {
			expandAll(c)
		}
	}
}

// outline renders visible rows as "depth:name" for compact assertions.
func outline(root *Node) string {
	var parts []string
	for _, r := range root.VisibleRows() {
		if r.Node.Kind == KindRoot {
			parts = append(parts, "root")
			continue
		}
		name := r.Node.Name
		if r.Node.Kind == KindDir {
			name += "/"
		}
		parts = append(parts, strings.Repeat(" ", r.Depth)+name)
	}
	return strings.Join(parts, "\n")
}

func TestBuildSortsDirsFirstLexicographic(t *testing.T) {
	root := Build(entries(
		"zz.txt",
		"internal/app/x.go",
		"db/queries/q.sql",
		"db/migrations/m.sql",
		"aa.txt",
		"proto/contract/v1/c.proto",
	))
	expandAll(root)

	want := strings.TrimSpace(`
root
 db/
  migrations/
   m.sql
  queries/
   q.sql
 internal/
  app/
   x.go
 proto/
  contract/
   v1/
    c.proto
 aa.txt
 zz.txt`)
	if got := outline(root); got != want {
		t.Errorf("outline:\n%s\nwant:\n%s", got, want)
	}
}

func TestCollapsedByDefault(t *testing.T) {
	root := Build(entries("db/queries/q.sql", "top.txt"))
	rows := root.VisibleRows()
	var names []string
	for _, r := range rows[1:] {
		names = append(names, r.Node.Name)
	}
	if strings.Join(names, ",") != "db,top.txt" {
		t.Errorf("visible = %v, want only top level", names)
	}
}

func TestExpandCollapse(t *testing.T) {
	root := Build(entries("db/queries/q.sql"))
	db := root.Find("db")
	if db == nil || db.Kind != KindDir {
		t.Fatalf("db node = %+v", db)
	}
	db.Expanded = true
	if len(root.VisibleRows()) != 3 { // root, db, queries
		t.Errorf("rows = %d, want 3", len(root.VisibleRows()))
	}
	db.Expanded = false
	if len(root.VisibleRows()) != 2 {
		t.Errorf("rows = %d, want 2", len(root.VisibleRows()))
	}
}

func TestFindAndFindClosest(t *testing.T) {
	root := Build(entries("db/queries/q.sql", "db/migrations/m.sql"))

	if n := root.Find("db/queries/q.sql"); n == nil || n.Kind != KindFile {
		t.Errorf("Find file = %+v", n)
	}
	if n := root.Find("db/queries"); n == nil || n.Kind != KindDir {
		t.Errorf("Find dir = %+v", n)
	}
	if n := root.Find("nope"); n != nil {
		t.Errorf("Find missing = %+v", n)
	}
	// gone file falls back to its directory, then further up.
	if n := root.FindClosest("db/queries/gone.sql"); n == nil || n.Path != "db/queries" {
		t.Errorf("FindClosest = %+v", n)
	}
	if n := root.FindClosest("x/y/z"); n != root {
		t.Errorf("FindClosest to root = %+v", n)
	}
}

func TestExpandedPathsRoundTrip(t *testing.T) {
	root := Build(entries("db/queries/q.sql", "internal/app/x.go"))
	root.Find("db").Expanded = true
	root.Find("db/queries").Expanded = true

	rebuilt := Build(entries("db/queries/q.sql", "internal/app/x.go", "db/new.sql"))
	rebuilt.RestoreExpanded(root.ExpandedPaths())

	if !rebuilt.Find("db").Expanded || !rebuilt.Find("db/queries").Expanded {
		t.Error("expansion not restored")
	}
	if rebuilt.Find("internal").Expanded {
		t.Error("internal should stay collapsed")
	}
}

func TestExpandTo(t *testing.T) {
	root := Build(entries("a/b/c/d.txt"))
	n := root.Find("a/b/c/d.txt")
	ExpandTo(n)
	rows := root.VisibleRows()
	if rows[len(rows)-1].Node != n {
		t.Errorf("target not visible after ExpandTo: %s", outline(root))
	}
}

func TestRenameEntryKeepsOldPath(t *testing.T) {
	root := Build([]FileEntry{{Path: "new/b.go", OldPath: "old/a.go", Status: 'R'}})
	n := root.Find("new/b.go")
	if n == nil || n.OldPath != "old/a.go" || n.Status != 'R' {
		t.Errorf("node = %+v", n)
	}
}
