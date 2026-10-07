package gitx

import (
	"os"
	"os/exec"
)

// DisplayCmd builds the interactive diff invocation for cmp, restricted to
// pathspec ("" means the whole comparison). The command is not started and
// has no streams attached; the caller hands it to Bubble Tea's ExecProcess,
// which connects it to the terminal so git can run its configured pager.
// cleanup removes the temporary index of an Untracked comparison and must
// run once the command has exited.
func (r *Repo) DisplayCmd(cmp Comparison, pathspec string) (*exec.Cmd, func(), error) {
	env, cleanup := []string(nil), func() {}
	if cmp.Untracked {
		var err error
		env, _, cleanup, err = r.untrackedIndex(cmp.Exclude)
		if err != nil {
			return nil, nil, err
		}
	}
	args := []string{cmp.subcommand()}
	if cmp.Kind == KindShow {
		args = append(args, "--format=")
	}
	args = append(args, cmp.Args...)
	if pathspec != "" || len(cmp.Exclude) > 0 {
		args = append(args, "--")
		if pathspec != "" {
			args = append(args, pathspec)
		}
		args = append(args, cmp.Exclude...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Root
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd, cleanup, nil
}
