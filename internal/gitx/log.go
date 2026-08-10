package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Commit is one entry in a repository log.
type Commit struct {
	SHA      string
	ShortSHA string
	Subject  string
}

// LogPage returns a page of commits reachable from HEAD but not base, newest
// first. The extra record requested from git is used only to report whether a
// following page exists.
func (r *Repo) LogPage(base string, offset, limit int) ([]Commit, bool, error) {
	if offset < 0 || limit < 1 {
		return nil, false, fmt.Errorf("invalid log page: offset %d, limit %d", offset, limit)
	}

	rangeArg := base + "..HEAD"
	args := []string{
		"--no-pager", "log", "-z", "--format=%H%x00%h%x00%s",
		"--skip=" + strconv.Itoa(offset),
		"--max-count=" + strconv.Itoa(limit+1),
		rangeArg,
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Root
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := firstLine(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, false, fmt.Errorf("git log (%s): %s", rangeArg, msg)
	}

	commits, err := ParseLog(out.Bytes())
	if err != nil {
		return nil, false, err
	}
	hasNext := len(commits) > limit
	if hasNext {
		commits = commits[:limit]
	}
	return commits, hasNext, nil
}

// ParseLog parses the NUL-delimited full SHA, abbreviated SHA, and subject
// triples emitted by LogPage.
func ParseLog(out []byte) ([]Commit, error) {
	fields := strings.Split(string(out), "\x00")
	if len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%3 != 0 {
		return nil, fmt.Errorf("unexpected git log output: got %d fields", len(fields))
	}

	commits := make([]Commit, 0, len(fields)/3)
	for i := 0; i < len(fields); i += 3 {
		if fields[i] == "" || fields[i+1] == "" {
			return nil, fmt.Errorf("unexpected git log output: empty commit id")
		}
		commits = append(commits, Commit{
			SHA:      fields[i],
			ShortSHA: fields[i+1],
			Subject:  fields[i+2],
		})
	}
	return commits, nil
}
