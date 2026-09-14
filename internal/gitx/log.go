package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"unicode"
)

// Commit is one entry in a repository log.
type Commit struct {
	SHA      string
	ShortSHA string
	Subject  string
}

// LogPage returns a page of commits reachable from HEAD, newest first. A
// non-empty query searches subjects case-insensitively; an unambiguous
// hexadecimal object ID resolves directly, even when it is not reachable from
// HEAD. The extra record requested from git is used only to report whether a
// following page exists.
func (r *Repo) LogPage(query string, offset, limit int) ([]Commit, bool, error) {
	if offset < 0 || limit < 1 {
		return nil, false, fmt.Errorf("invalid log page: offset %d, limit %d", offset, limit)
	}

	query = strings.TrimSpace(query)
	if isHexObjectID(query) {
		if commit, err := r.ResolveCommit(query); err == nil {
			if offset > 0 {
				return nil, false, nil
			}
			return []Commit{commit}, false, nil
		}
	}

	args := []string{
		"--no-pager", "log", "-z", "--format=%H%x00%h%x00%s",
		"--skip=" + strconv.Itoa(offset),
		"--max-count=" + strconv.Itoa(limit+1),
	}
	if query != "" {
		args = append(args, "--regexp-ignore-case", "--fixed-strings", "--grep="+query)
	}
	args = append(args, "HEAD")
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
		return nil, false, fmt.Errorf("git log (HEAD): %s", msg)
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

// ResolveCommit resolves revision to a commit and returns its log metadata.
func (r *Repo) ResolveCommit(revision string) (Commit, error) {
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return Commit{}, fmt.Errorf("empty revision")
	}
	args := []string{
		"--no-pager", "show", "-s", "-z", "--format=%H%x00%h%x00%s",
		revision + "^{commit}",
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
		return Commit{}, fmt.Errorf("git resolve %s: %s", revision, msg)
	}
	commits, err := ParseLog(out.Bytes())
	if err != nil || len(commits) != 1 {
		if err != nil {
			return Commit{}, err
		}
		return Commit{}, fmt.Errorf("git resolve %s: expected one commit, got %d", revision, len(commits))
	}
	return commits[0], nil
}

func isHexObjectID(s string) bool {
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			return false
		}
	}
	return true
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
