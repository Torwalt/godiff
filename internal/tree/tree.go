// Package tree builds and navigates the synthetic directory tree derived
// from changed file paths. It is independent of git execution and the UI.
package tree

import (
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

// Row is a visible line in the flattened tree.
type Row struct {
	Node  *Node
	Depth int
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

// VisibleRows flattens the tree into the rows currently on screen: the root
// itself followed by children of every expanded node, depth-first.
func (n *Node) VisibleRows() []Row {
	rows := []Row{{Node: n, Depth: 0}}
	var walk func(node *Node, depth int)
	walk = func(node *Node, depth int) {
		if !node.Expanded {
			return
		}
		for _, c := range node.Children {
			rows = append(rows, Row{Node: c, Depth: depth})
			walk(c, depth+1)
		}
	}
	walk(n, 1)
	return rows
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

// Dirs returns all directory nodes in depth-first order.
func (n *Node) Dirs() []*Node {
	var dirs []*Node
	var walk func(node *Node)
	walk = func(node *Node) {
		if node.Kind == KindDir {
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
