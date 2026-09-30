package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Branches returns local branch names, most recently committed first,
// without the checked-out branch.
func (r *Repo) Branches() ([]string, error) {
	cmd := exec.Command("git", "--no-pager", "for-each-ref",
		"--sort=-committerdate", "--format=%(HEAD)%(refname:short)", "refs/heads")
	cmd.Dir = r.Root
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := firstLine(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git for-each-ref: %s", msg)
	}
	return ParseBranches(out.Bytes()), nil
}

// ParseBranches parses `%(HEAD)%(refname:short)` lines, dropping the one
// marked `*` as checked out.
func ParseBranches(out []byte) []string {
	var branches []string
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" || line[0] == '*' {
			continue
		}
		branches = append(branches, strings.TrimPrefix(line, " "))
	}
	return branches
}
