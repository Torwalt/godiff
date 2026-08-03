// Package gitx runs git as an external process: repository detection,
// changed-path discovery, and construction of display commands. It owns no
// git semantics itself; git remains the source of truth.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Repo is a located git repository. All git commands run with the
// repository root as working directory so repo-relative paths can be used
// as pathspecs directly.
type Repo struct {
	Root string
}

// ErrNotARepo is returned by Detect when dir is not inside a git work tree.
var ErrNotARepo = errors.New("not inside a git repository")

// Detect locates the repository root containing dir.
func Detect(dir string) (*Repo, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "not a git repository") {
			return nil, ErrNotARepo
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("git executable not found: %w", err)
		}
		return nil, fmt.Errorf("git rev-parse: %s", firstLine(stderr.String()))
	}
	root := strings.TrimSpace(out.String())
	if root == "" {
		return nil, ErrNotARepo
	}
	return &Repo{Root: root}, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
