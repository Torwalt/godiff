// Package tree builds and navigates the synthetic directory tree derived
// from changed file paths. It is independent of git execution and the UI.
package tree

import (
	"slices"
	"sort"
	"strings"
)

// Kind distinguishes node types.
type Kind int

const (
	KindRoot Kind = iota
	KindDir
	KindFile
)

// FileEntry is the input to Build: one changed file with its git status.
type FileEntry struct {
	Path    string
	OldPath string
	Status  byte
}

// Node is one entry in the path tree. Path is repository-relative and empty
// for the root. Directory nodes are synthetic: git only reports files.
type Node struct {
	Name     string
	Path     string
	Kind     Kind
	Status   byte
	OldPath  string
	Children []*Node
	Parent   *Node
	Expanded bool
}

// Row is a visible line in the flattened tree. A chain of directories that
// each hold a lone subdirectory renders as one row, so Spine may hold
// several nodes; Node is its last element and owns the row's path and
// children. Label is the joined names ("proto/core/conditions").
type Row struct {
	Node     *Node
	Spine    []*Node
	Label    string
	Depth    int
	Expanded bool
}

// Contains reports whether n is rendered on this row, either as its node or
// as one of the directories folded into it.
func (r Row) Contains(n *Node) bool {
	return slices.Contains(r.Spine, n)
}

// Build constructs a sorted tree from changed file paths. The root is
// expanded; all directories start collapsed. Entry order does not matter.
func Build(entries []FileEntry) *Node {
	root := &Node{Kind: KindRoot, Expanded: true}
	dirs := map[string]*Node{"": root}

	dir := func(path string) *Node { return ensureDir(path, dirs) }
	for _, e := range entries {
		parentPath, name := splitPath(e.Path)
		parent := dir(parentPath)
		parent.Children = append(parent.Children, &Node{
			Name:    name,
			Path:    e.Path,
			Kind:    KindFile,
			Status:  e.Status,
			OldPath: e.OldPath,
			Parent:  parent,
		})
	}
	sortTree(root)
	return root
}

func ensureDir(path string, dirs map[string]*Node) *Node {
	if n, ok := dirs[path]; ok {
		return n
	}
	parentPath, name := splitPath(path)
	parent := ensureDir(parentPath, dirs)
	n := &Node{Name: name, Path: path, Kind: KindDir, Parent: parent}
	parent.Children = append(parent.Children, n)
	dirs[path] = n
	return n
}

func splitPath(path string) (parent, name string) {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[:i], path[i+1:]
	}
	return "", path
}

// sortTree orders every level: directories first, then files, each group
// lexicographic.
func sortTree(n *Node) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if (a.Kind == KindDir) != (b.Kind == KindDir) {
			return a.Kind == KindDir
		}
		return a.Name < b.Name
	})
	for _, c := range n.Children {
		sortTree(c)
	}
}

// folds reports whether n shares a row with its child: it holds exactly one
// child and that child is a directory. Such a chain is worth folding
// because it offers no choice — and because diffing any node of the chain
// covers the same files, so no diff target is lost.
func folds(n *Node) bool {
	return n.Kind == KindDir && len(n.Children) == 1 && n.Children[0].Kind == KindDir
}

// spine returns the directories rendered on one row, starting at n.
func spine(n *Node) []*Node {
	s := []*Node{n}
	for folds(n) {
		n = n.Children[0]
		s = append(s, n)
	}
	return s
}

func spineLabel(s []*Node) string {
	names := make([]string, len(s))
	for i, n := range s {
		names[i] = n.Name
	}
	return strings.Join(names, "/")
}

// Anchor returns the first directory of the row n is rendered on.
func Anchor(n *Node) *Node {
	for n.Kind == KindDir && n.Parent != nil && folds(n.Parent) {
		n = n.Parent
	}
	return n
}

// SetExpanded opens or closes the row n is rendered on. A folded chain is a
// single row, so its directories share one expansion state.
func SetExpanded(n *Node, expanded bool) {
	if n.Kind == KindRoot {
		n.Expanded = expanded
		return
	}
	for _, d := range spine(Anchor(n)) {
		d.Expanded = expanded
	}
}

// VisibleRows flattens the tree into the rows currently on screen: the root
// itself followed by children of every expanded node, depth-first, with
// lone-subdirectory chains folded onto one row.
func (n *Node) VisibleRows() []Row {
	rows := []Row{{Node: n, Spine: []*Node{n}, Expanded: n.Expanded}}
	var walk func(node *Node, depth int)
	walk = func(node *Node, depth int) {
		for _, c := range node.Children {
			if c.Kind == KindFile {
				rows = append(rows, Row{Node: c, Spine: []*Node{c}, Label: c.Name, Depth: depth})
				continue
			}
			s := spine(c)
			last := s[len(s)-1]
			// The anchor carries the state: a refresh may re-fold a chain
			// whose members were restored unevenly.
			rows = append(rows, Row{
				Node: last, Spine: s, Label: spineLabel(s), Depth: depth, Expanded: c.Expanded,
			})
			if c.Expanded {
				walk(last, depth+1)
			}
		}
	}
	if n.Expanded {
		walk(n, 1)
	}
	return rows
}

// ChildNames returns the row labels one level below n, directories suffixed
// with "/": the one-line preview of what expanding n would reveal.
func (n *Node) ChildNames() []string {
	names := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		if c.Kind == KindDir {
			names = append(names, spineLabel(spine(c))+"/")
			continue
		}
		names = append(names, c.Name)
	}
	return names
}

// Find returns the node with the given repository-relative path, or nil.
func (n *Node) Find(path string) *Node {
	if n.Path == path {
		return n
	}
	for _, c := range n.Children {
		if c.Path == path || strings.HasPrefix(path, c.Path+"/") || c.Path == "" {
			if found := c.Find(path); found != nil {
				return found
			}
		}
	}
	return nil
}

// FindClosest returns the node for path, or its nearest existing ancestor
// (ultimately the root).
func (n *Node) FindClosest(path string) *Node {
	for path != "" {
		if found := n.Find(path); found != nil {
			return found
		}
		path, _ = splitPath(path)
	}
	return n
}

// ExpandedPaths collects the paths of all expanded directories (excluding
// the root, which is always expanded).
func (n *Node) ExpandedPaths() map[string]bool {
	paths := map[string]bool{}
	var walk func(node *Node)
	walk = func(node *Node) {
		if node.Kind == KindDir && node.Expanded {
			paths[node.Path] = true
		}
		for _, c := range node.Children {
			walk(c)
		}
	}
	walk(n)
	return paths
}

// RestoreExpanded re-expands directories whose paths are in the set,
// silently skipping paths that no longer exist.
func (n *Node) RestoreExpanded(paths map[string]bool) {
	var walk func(node *Node)
	walk = func(node *Node) {
		if node.Kind == KindDir && paths[node.Path] {
			node.Expanded = true
		}
		for _, c := range node.Children {
			walk(c)
		}
	}
	walk(n)
}

// Dirs returns the addressable directory nodes in depth-first order: one
// per possible row, so folded chains contribute only their last node. Its
// path still carries every folded segment, so matching on any of them works.
func (n *Node) Dirs() []*Node {
	var dirs []*Node
	var walk func(node *Node)
	walk = func(node *Node) {
		if node.Kind == KindDir && !folds(node) {
			dirs = append(dirs, node)
		}
		for _, c := range node.Children {
			walk(c)
		}
	}
	walk(n)
	return dirs
}

// ExpandTo expands all ancestors of node so it becomes visible.
func ExpandTo(node *Node) {
	for p := node.Parent; p != nil; p = p.Parent {
		p.Expanded = true
	}
}
