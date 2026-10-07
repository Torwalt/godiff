package gitx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// untrackedIndex prepares a throwaway copy of the index in which untracked,
// non-ignored files are recorded as intent-to-add, so diffs that read the
// working tree report them as added. The real index is never written.
// env is nil when nothing is untracked; cleanup is always safe to call.
func (r *Repo) untrackedIndex(exclude []string) (env []string, untracked map[string]bool, cleanup func(), err error) {
	cleanup = func() {}
	args := []string{"-c", "core.quotePath=false", "ls-files", "--others", "--exclude-standard", "-z"}
	if len(exclude) > 0 {
		args = append(args, "--")
		args = append(args, exclude...)
	}
	list, err := r.run(nil, nil, args...)
	if err != nil {
		return nil, nil, cleanup, err
	}
	untracked = map[string]bool{}
	for _, path := range strings.Split(string(list), "\x00") {
		if path != "" {
			untracked[path] = true
		}
	}
	if len(untracked) == 0 {
		return nil, nil, cleanup, nil
	}

	indexPath, err := r.run(nil, nil, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return nil, nil, cleanup, err
	}
	dir, err := os.MkdirTemp("", "godiff-index-")
	if err != nil {
		return nil, nil, cleanup, err
	}
	cleanup = func() { os.RemoveAll(dir) }
	tmp := filepath.Join(dir, "index")
	// A repository without an index yet needs none copied; git creates it.
	data, err := os.ReadFile(strings.TrimSpace(string(indexPath)))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		cleanup()
		return nil, nil, func() {}, err
	}
	if err == nil {
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			cleanup()
			return nil, nil, func() {}, err
		}
	}

	env = []string{"GIT_INDEX_FILE=" + tmp}
	addEnv := append([]string{"GIT_LITERAL_PATHSPECS=1"}, env...)
	if _, err := r.run(addEnv, list, "add", "--intent-to-add", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
		cleanup()
		return nil, nil, func() {}, err
	}
	return env, untracked, cleanup, nil
}
