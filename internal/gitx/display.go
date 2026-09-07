package gitx

import "os/exec"

// DisplayCmd builds the interactive diff invocation for cmp, restricted to
// pathspec ("" means the whole comparison). The command is not started and
// has no streams attached; the caller hands it to Bubble Tea's ExecProcess,
// which connects it to the terminal so git can run its configured pager.
func (r *Repo) DisplayCmd(cmp Comparison, pathspec string) *exec.Cmd {
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
	return cmd
}
