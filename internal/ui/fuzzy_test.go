package ui

import (
	"strings"
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

// Candidates are directories only, one per row: "db" folds into
// "db/queries" and so is not offered separately.
func TestDirsListsRowDirectoriesNoFiles(t *testing.T) {
	entries := []tree.FileEntry{
		{Path: "db/queries/q.sql", Status: 'M'},
		{Path: "internal/app/x.go", Status: 'M'},
		{Path: "top.txt", Status: 'M'},
	}
	dirs := tree.Build(entries).Dirs()
	var paths []string
	for _, d := range dirs {
		paths = append(paths, d.Path)
	}
	want := "db/queries,internal/app"
	if strings.Join(paths, ",") != want {
		t.Errorf("dirs = %v, want %v", paths, want)
	}
	// Folded segments stay searchable through the full path.
	if _, ok := fuzzyMatch("db", "db/queries"); !ok {
		t.Error("folded head no longer matches its row")
	}
}
