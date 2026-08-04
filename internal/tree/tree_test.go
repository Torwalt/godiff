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

// outline renders visible rows as indented labels for compact assertions.
func outline(root *Node) string {
	var parts []string
	for _, r := range root.VisibleRows() {
		if r.Node.Kind == KindRoot {
			parts = append(parts, "root")
			continue
		}
		label := r.Label
		if r.Node.Kind == KindDir {
			label += "/"
		}
		parts = append(parts, strings.Repeat(" ", r.Depth)+label)
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
 internal/app/
  x.go
 proto/contract/v1/
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
	var labels []string
	for _, r := range rows[1:] {
		labels = append(labels, r.Label)
	}
	if strings.Join(labels, ",") != "db/queries,top.txt" {
		t.Errorf("visible = %v, want only top level", labels)
	}
}

func TestExpandCollapse(t *testing.T) {
	root := Build(entries("db/queries/q.sql"))
	db := root.Find("db")
	if db == nil || db.Kind != KindDir {
		t.Fatalf("db node = %+v", db)
	}
	SetExpanded(db, true)
	if len(root.VisibleRows()) != 3 { // root, db/queries, q.sql
		t.Errorf("rows = %d, want 3", len(root.VisibleRows()))
	}
	SetExpanded(db, false)
	if len(root.VisibleRows()) != 2 {
		t.Errorf("rows = %d, want 2", len(root.VisibleRows()))
	}
}

// A lone-subdirectory chain is one row, addressed by its last node.
func TestFoldedChainIsOneRow(t *testing.T) {
	root := Build(entries("proto/core/conditions/structs/a.proto", "proto/options/v1/o.proto"))

	want := strings.TrimSpace(`
root
 proto/`)
	if got := outline(root); got != want {
		t.Errorf("outline:\n%s\nwant:\n%s", got, want)
	}

	SetExpanded(root.Find("proto"), true)
	want = strings.TrimSpace(`
root
 proto/
  core/conditions/structs/
  options/v1/`)
	if got := outline(root); got != want {
		t.Errorf("expanded outline:\n%s\nwant:\n%s", got, want)
	}

	rows := root.VisibleRows()
	chain := rows[2]
	if chain.Node.Path != "proto/core/conditions/structs" {
		t.Errorf("row node = %q, want the chain's last directory", chain.Node.Path)
	}
	// Every folded directory resolves to the row it renders on.
	for _, path := range []string{"proto/core", "proto/core/conditions", "proto/core/conditions/structs"} {
		if !chain.Contains(root.Find(path)) {
			t.Errorf("row does not contain %q", path)
		}
	}
}

// Expanding any member of a chain opens the whole row, and the state
// survives being set from the middle of the chain.
func TestSetExpandedCoversWholeChain(t *testing.T) {
	root := Build(entries("a/b/c/x.go"))
	SetExpanded(root.Find("a/b"), true)

	for _, path := range []string{"a", "a/b", "a/b/c"} {
		if !root.Find(path).Expanded {
			t.Errorf("%q not expanded", path)
		}
	}
	if got := outline(root); got != "root\n a/b/c/\n  x.go" {
		t.Errorf("outline:\n%s", got)
	}

	SetExpanded(root.Find("a/b/c"), false)
	if root.Find("a").Expanded {
		t.Error("collapsing the row left the chain head open")
	}
}

func TestAnchorSkipsFoldedParents(t *testing.T) {
	root := Build(entries("a/b/c/x.go", "top/one.go", "top/two.go"))

	if got := Anchor(root.Find("a/b/c")); got != root.Find("a") {
		t.Errorf("Anchor = %+v, want a", got)
	}
	// top holds two files, so it folds nothing and anchors itself.
	if got := Anchor(root.Find("top")); got != root.Find("top") {
		t.Errorf("Anchor(top) = %+v, want top", got)
	}
}

func TestChildNamesPreviewsFoldedRows(t *testing.T) {
	root := Build(entries(
		"proto/core/conditions/c.proto",
		"proto/options/v1/o.proto",
		"proto/gen.go",
	))
	got := strings.Join(root.Find("proto").ChildNames(), " ")
	if got != "core/conditions/ options/v1/ gen.go" {
		t.Errorf("ChildNames = %q", got)
	}
}

// Search offers one entry per possible row, but keeps the full path so a
// folded segment still matches.
func TestDirsSkipsFoldedDirectories(t *testing.T) {
	root := Build(entries("proto/core/conditions/c.proto", "db/a.sql", "db/b.sql"))
	var paths []string
	for _, d := range root.Dirs() {
		paths = append(paths, d.Path)
	}
	if strings.Join(paths, ",") != "db,proto/core/conditions" {
		t.Errorf("Dirs = %v", paths)
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
