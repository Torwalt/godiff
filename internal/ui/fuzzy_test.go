package ui

import (
	"testing"

	"github.com/Torwalt/godiff/internal/tree"
)

func dirsOf(paths ...string) []*tree.Node {
	var entries []tree.FileEntry
	for _, p := range paths {
		entries = append(entries, tree.FileEntry{Path: p + "/f.go", Status: 'M'})
	}
	return tree.Build(entries).Dirs()
}

func rankedPaths(dirs []*tree.Node, query string) []string {
	var out []string
	for _, d := range rankDirs(dirs, query) {
		out = append(out, d.Path)
	}
	return out
}

func TestFuzzyMatchSubsequence(t *testing.T) {
	if _, ok := fuzzyMatch("dbq", "db/queries"); !ok {
		t.Error("dbq should match db/queries")
	}
	if _, ok := fuzzyMatch("xyz", "db/queries"); ok {
		t.Error("xyz should not match db/queries")
	}
	if _, ok := fuzzyMatch("QUER", "db/queries"); !ok {
		t.Error("match should be case-insensitive")
	}
	if _, ok := fuzzyMatch("", "anything"); !ok {
		t.Error("empty query matches everything")
	}
}

func TestRankDirsPrefersSegmentStarts(t *testing.T) {
	dirs := dirsOf("db/queries", "docs/quirks", "internal/dbq-helpers")

	got := rankedPaths(dirs, "dq")
	if len(got) == 0 || got[0] != "db/queries" {
		t.Errorf("dq ranking = %v, want db/queries first", got)
	}
}

func TestRankDirsFiltersAndEmptyQueryKeepsAll(t *testing.T) {
	dirs := dirsOf("proto/contract", "db/queries")
	// dirs also contains parents proto/ and db/

	if got := rankedPaths(dirs, "contract"); len(got) != 1 || got[0] != "proto/contract" {
		t.Errorf("filtered = %v", got)
	}
	if got := rankedPaths(dirs, ""); len(got) != len(dirs) {
		t.Errorf("empty query dropped candidates: %v", got)
	}
}

func TestDirsListsAllDirectoriesNoFiles(t *testing.T) {
	entries := []tree.FileEntry{
		{Path: "db/queries/q.sql", Status: 'M'},
		{Path: "top.txt", Status: 'M'},
	}
	dirs := tree.Build(entries).Dirs()
	var paths []string
	for _, d := range dirs {
		paths = append(paths, d.Path)
	}
	want := []string{"db", "db/queries"}
	if len(paths) != 2 || paths[0] != want[0] || paths[1] != want[1] {
		t.Errorf("dirs = %v, want %v", paths, want)
	}
}
