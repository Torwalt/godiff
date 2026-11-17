package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Review is the revision under review and the branch it builds on.
type Review struct {
	Rev  string // HEAD, or the remote copy of HEAD's branch
	Name string
	// Copy is HEAD's branch on a remote, when it differs from HEAD.
	Copy   *RemoteCopy
	Parent Parent
	// BaseFork is where Rev leaves the base.
	BaseFork string
}

// RemoteCopy is HEAD's branch on a remote.
type RemoteCopy struct {
	Ref, Name, Branch string
	Ahead, Behind     int // HEAD against Ref
	// Newer: Ref is ahead of HEAD, or diverged with the later commit date.
	Newer bool
}

// Parent is the branch the commits under review start from.
type Parent struct {
	Ref     string // full ref name, or the base as configured
	Name    string
	Commits int // in Ref..Rev
	Fork    Commit
}

type branchRef struct {
	sha, ref, name string
	behind         int
}

// Branches returns local and remote-tracking branch names, most recently
// committed first, without the checked-out branch.
func (r *Repo) Branches() ([]string, error) {
	cmd := exec.Command("git", "--no-pager", "for-each-ref", "--sort=-committerdate", "--exclude=refs/remotes/*/HEAD",
		"--format=%(HEAD)%(refname:short)", "refs/heads", "refs/remotes")
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

// BranchTips maps commits to the local and remote-tracking branches pointing
// at them, for tips strictly between base and rev.
func (r *Repo) BranchTips(base, rev string) (map[string][]string, error) {
	refs, err := r.branchesBelow(base, rev)
	if err != nil {
		return nil, err
	}
	tips := map[string][]string{}
	for _, b := range refs {
		if b.behind > 0 {
			tips[b.sha] = append(tips[b.sha], b.name)
		}
	}
	return tips, nil
}

// Review finds what to review and its parent. An empty rev reviews the newer
// of HEAD and its branch's remote copy. The parent is the nearest local or
// remote-tracking branch below rev that base has not merged, else base.
// Branches pointing at rev, and their namesakes, are rev's own and never
// its parent.
func (r *Repo) Review(base, rev string) (Review, error) {
	remote, err := r.remoteCopy()
	if err != nil {
		return Review{}, err
	}
	review := Review{Rev: rev, Name: rev, Copy: remote}
	if rev == "" {
		review.Rev, review.Name = "HEAD", "HEAD"
		if remote != nil && remote.Newer {
			review.Rev = remote.Ref
		}
	}
	if remote != nil && review.Rev == remote.Ref {
		review.Name = remote.Name
	}

	refs, err := r.branchesBelow(base, review.Rev)
	if err != nil {
		return Review{}, err
	}
	branchName := func(ref string) string {
		if rest, ok := strings.CutPrefix(ref, "refs/remotes/"); ok {
			_, name, _ := strings.Cut(rest, "/")
			return name
		}
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	own := map[string]bool{}
	for _, b := range refs {
		if b.behind == 0 {
			own[branchName(b.ref)] = true
		}
	}
	var nearest *branchRef
	for i, b := range refs {
		if b.behind > 0 && !own[branchName(b.ref)] && (nearest == nil || b.behind < nearest.behind) {
			nearest = &refs[i]
		}
	}

	mergeBase := func(ref string) (Commit, error) {
		out, err := r.run(nil, nil, "merge-base", ref, review.Rev)
		if err != nil {
			return Commit{}, err
		}
		return r.ResolveCommit(strings.TrimSpace(string(out)))
	}
	baseFork, err := mergeBase(base)
	if err != nil {
		return Review{}, err
	}
	review.BaseFork = baseFork.SHA
	if nearest != nil {
		fork, err := mergeBase(nearest.ref)
		if err != nil {
			return Review{}, err
		}
		review.Parent = Parent{Ref: nearest.ref, Name: nearest.name, Commits: nearest.behind, Fork: fork}
		return review, nil
	}
	out, err := r.run(nil, nil, "rev-list", "--count", base+".."+review.Rev)
	if err != nil {
		return Review{}, err
	}
	commits, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return Review{}, fmt.Errorf("git rev-list --count: %w", err)
	}
	review.Parent = Parent{Ref: base, Name: base, Commits: commits, Fork: baseFork}
	return review, nil
}

func (r *Repo) remoteCopy() (*RemoteCopy, error) {
	out, err := r.run(nil, nil, "for-each-ref", "--points-at=HEAD", "--format=%(committerdate:unix) %(refname:lstrip=2)", "refs/heads")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		date, branch, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		headDate, headDateErr := strconv.ParseInt(date, 10, 64)
		out, err := r.run(nil, nil, "for-each-ref", "--format=%(ahead-behind:HEAD) %(committerdate:unix) %(refname) %(refname:short)",
			"refs/remotes/*/"+branch)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 5 || strings.SplitN(fields[3], "/", 4)[3] != branch {
				continue
			}
			ahead, aheadErr := strconv.Atoi(fields[0])
			behind, behindErr := strconv.Atoi(fields[1])
			date, dateErr := strconv.ParseInt(fields[2], 10, 64)
			if err := errors.Join(aheadErr, behindErr, dateErr, headDateErr); err != nil {
				return nil, fmt.Errorf("git for-each-ref: %w", err)
			}
			if ahead == 0 && behind == 0 {
				return nil, nil
			}
			return &RemoteCopy{
				Ref: fields[3], Name: fields[4], Branch: branch,
				Ahead: behind, Behind: ahead,
				Newer: ahead > 0 && (behind == 0 || date > headDate),
			}, nil
		}
	}
	return nil, nil
}

func (r *Repo) branchesBelow(base, rev string) ([]branchRef, error) {
	out, err := r.run(nil, nil, "for-each-ref", "--merged="+rev, "--no-merged="+base, "--exclude=refs/remotes/*/HEAD",
		"--format=%(objectname) %(ahead-behind:"+rev+") %(refname) %(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var refs []branchRef
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 5 {
			continue
		}
		behind, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("git for-each-ref: unexpected ahead-behind %q", fields[1]+" "+fields[2])
		}
		refs = append(refs, branchRef{sha: fields[0], ref: fields[3], name: fields[4], behind: behind})
	}
	return refs, nil
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
