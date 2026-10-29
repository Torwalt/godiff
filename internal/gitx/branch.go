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

// BranchTips maps commits to the local branches pointing at them, for tips
// strictly between base and HEAD.
func (r *Repo) BranchTips(base string) (map[string][]string, error) {
	out, err := r.run(nil, nil, "for-each-ref", "--merged=HEAD", "--no-contains=HEAD", "--no-merged="+base,
		"--format=%(objectname) %(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	tips := map[string][]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if sha, name, ok := strings.Cut(line, " "); ok {
			tips[sha] = append(tips[sha], name)
		}
	}
	return tips, nil
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
