package gitx

import (
	"strconv"
	"strings"
)

// Worktree is one checkout registered with the repository: the main
// working tree or a linked one, wherever it lives on disk.
type Worktree struct {
	Path   string
	Head   string // commit SHA
	Branch string // short branch name; empty when detached
}

// Worktrees lists the repository's checkouts from git's central registry,
// main working tree first. Bare entries and prunable ones (whose directory
// is gone) are skipped.
func (r *Repo) Worktrees() ([]Worktree, error) {
	out, err := r.run(nil, nil, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	// Records are NUL-separated attributes terminated by an empty one.
	var worktrees []Worktree
	var cur Worktree
	skip := false
	for _, field := range strings.Split(string(out), "\x00") {
		switch {
		case field == "":
			if cur.Path != "" && !skip {
				worktrees = append(worktrees, cur)
			}
			cur, skip = Worktree{}, false
		case strings.HasPrefix(field, "worktree "):
			cur.Path = strings.TrimPrefix(field, "worktree ")
		case strings.HasPrefix(field, "HEAD "):
			cur.Head = strings.TrimPrefix(field, "HEAD ")
		case strings.HasPrefix(field, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(field, "branch "), "refs/heads/")
		case field == "bare", strings.HasPrefix(field, "prunable"):
			skip = true
		}
	}
	return worktrees, nil
}

// Progress summarizes a checkout's work relative to a base revision.
type Progress struct {
	Ahead   int // commits on HEAD that base lacks
	Changed int // uncommitted paths, untracked files included
}

// Progress counts the commits HEAD has on top of base and the uncommitted
// paths in the working tree.
func (r *Repo) Progress(base string) (Progress, error) {
	count, err := r.run(nil, nil, "rev-list", "--count", base+"..HEAD")
	if err != nil {
		return Progress{}, err
	}
	ahead, err := strconv.Atoi(strings.TrimSpace(string(count)))
	if err != nil {
		return Progress{}, err
	}
	status, err := r.run(nil, nil, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return Progress{}, err
	}
	changed := 0
	fields := strings.Split(string(status), "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 3 {
			continue
		}
		changed++
		// Renames and copies carry their source path as the next field.
		if strings.ContainsAny(entry[:2], "RC") {
			i++
		}
	}
	return Progress{Ahead: ahead, Changed: changed}, nil
}
