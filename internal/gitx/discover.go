package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ChangedFile is one entry from changed-path discovery.
type ChangedFile struct {
	// Path is the repository-relative path (the new path for renames and
	// copies).
	Path string
	// OldPath is the source path for renames and copies, otherwise empty.
	OldPath string
	// Status is the git status letter: A, M, D, R, C, T, U, X, B.
	Status byte
}

// Discover runs the machine-readable changed-path invocation for cmp and
// parses the result. Output is captured, never paged, and NUL-delimited so
// unusual filenames survive.
func (r *Repo) Discover(cmp Comparison) ([]ChangedFile, error) {
	args := []string{"--no-pager", "-c", "core.quotePath=false", cmp.subcommand()}
	args = append(args, "--name-status", "-z", "--no-color", "-M", "-C")
	if cmp.Kind == KindShow {
		args = append(args, "--format=")
	}
	args = append(args, cmp.Args...)
	if len(cmp.Exclude) > 0 {
		args = append(args, "--")
		args = append(args, cmp.Exclude...)
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
		return nil, fmt.Errorf("git %s (%s): %s", cmp.subcommand(), cmp.Label, msg)
	}
	return ParseNameStatus(out.Bytes())
}

// ParseNameStatus parses `git diff --name-status -z` output. Records are
// NUL-delimited: a status field followed by one path, or two paths for
// renames and copies (score suffixes like R100 are accepted).
func ParseNameStatus(out []byte) ([]ChangedFile, error) {
	fields := strings.Split(string(out), "\x00")
	var files []ChangedFile
	for i := 0; i < len(fields); {
		status := fields[i]
		if status == "" { // trailing NUL
			i++
			continue
		}
		if i+1 >= len(fields) || fields[i+1] == "" {
			return nil, fmt.Errorf("unexpected --name-status output near %q", status)
		}
		code := status[0]
		f := ChangedFile{Status: code}
		if code == 'R' || code == 'C' {
			if i+2 >= len(fields) || fields[i+2] == "" {
				return nil, fmt.Errorf("missing rename target for %q %q", status, fields[i+1])
			}
			f.OldPath = fields[i+1]
			f.Path = fields[i+2]
			i += 3
		} else {
			f.Path = fields[i+1]
			i += 2
		}
		files = append(files, f)
	}
	return files, nil
}
