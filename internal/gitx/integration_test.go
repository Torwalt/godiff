package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// testRepo creates a temporary git repository with one initial commit.
func testRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "master")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "test")
	write(t, dir, "README.md", "hello\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "initial")

	repo, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return repo
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectNotARepo(t *testing.T) {
	if _, err := Detect(t.TempDir()); err != ErrNotARepo {
		t.Fatalf("err = %v, want ErrNotARepo", err)
	}
}

func TestDiscoverWorktreeAndStaged(t *testing.T) {
	repo := testRepo(t)
	write(t, repo.Root, "README.md", "changed\n")
	write(t, repo.Root, "db/queries/new.sql", "select 1;\n")
	git(t, repo.Root, "add", "db")

	staged, err := repo.Discover(Comparison{Kind: KindDiff, Args: []string{"--cached"}})
	if err != nil {
		t.Fatalf("staged: %v", err)
	}
	if len(staged) != 1 || staged[0].Path != "db/queries/new.sql" || staged[0].Status != 'A' {
		t.Errorf("staged = %+v", staged)
	}

	worktree, err := repo.Discover(Comparison{Kind: KindDiff})
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if len(worktree) != 1 || worktree[0].Path != "README.md" || worktree[0].Status != 'M' {
		t.Errorf("worktree = %+v", worktree)
	}
}

func TestDiscoverRangeRenameAndDelete(t *testing.T) {
	repo := testRepo(t)
	write(t, repo.Root, "pkg/a.go", "package a\n\nvar A = 1\n")
	write(t, repo.Root, "pkg/gone.go", "package a\n")
	git(t, repo.Root, "add", ".")
	git(t, repo.Root, "commit", "-m", "base")
	git(t, repo.Root, "checkout", "-b", "feature")
	git(t, repo.Root, "mv", "pkg/a.go", "pkg/b.go")
	git(t, repo.Root, "rm", "-q", "pkg/gone.go")
	git(t, repo.Root, "commit", "-m", "rename and delete")

	files, err := repo.Discover(Comparison{Kind: KindDiff, Args: []string{"master...HEAD"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	byPath := map[string]ChangedFile{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	if f := byPath["pkg/b.go"]; f.Status != 'R' || f.OldPath != "pkg/a.go" {
		t.Errorf("rename = %+v", f)
	}
	if f := byPath["pkg/gone.go"]; f.Status != 'D' {
		t.Errorf("delete = %+v", f)
	}
}

func TestDiscoverShowCommit(t *testing.T) {
	repo := testRepo(t)
	write(t, repo.Root, "proto/contract/v1/x.proto", "syntax\n")
	git(t, repo.Root, "add", ".")
	git(t, repo.Root, "commit", "-m", "add proto")

	files, err := repo.Discover(Comparison{Kind: KindShow, Args: []string{"HEAD"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 || files[0].Path != "proto/contract/v1/x.proto" || files[0].Status != 'A' {
		t.Errorf("files = %+v", files)
	}
}

func TestDisplayShowOmitsCommitMessage(t *testing.T) {
	repo := testRepo(t)
	write(t, repo.Root, "feature.txt", "content\n")
	git(t, repo.Root, "add", "feature.txt")
	git(t, repo.Root, "commit", "-m", "MESSAGE-MUST-NOT-APPEAR")

	cmd, cleanup, err := repo.DisplayCmd(Comparison{Kind: KindShow, Args: []string{"HEAD"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("MESSAGE-MUST-NOT-APPEAR")) {
		t.Errorf("display output includes commit message:\n%s", out)
	}
	if !bytes.Contains(out, []byte("diff --git a/feature.txt b/feature.txt")) {
		t.Errorf("display output is missing patch:\n%s", out)
	}
}

func TestDiscoverExclusions(t *testing.T) {
	repo := testRepo(t)
	write(t, repo.Root, "gen/out.pb.go", "generated\n")
	write(t, repo.Root, "src/main.go", "package main\n")

	git(t, repo.Root, "add", ".")
	files, err := repo.Discover(Comparison{
		Kind:    KindDiff,
		Args:    []string{"--cached"},
		Exclude: []string{":(exclude)gen"},
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 || files[0].Path != "src/main.go" {
		t.Errorf("files = %+v", files)
	}
}

func TestDiscoverInvalidRevision(t *testing.T) {
	repo := testRepo(t)
	_, err := repo.Discover(Comparison{Kind: KindDiff, Label: "bad", Args: []string{"nosuchbranch...HEAD"}})
	if err == nil {
		t.Fatal("expected error for invalid revision")
	}
}

func TestDiscoverEmptyComparison(t *testing.T) {
	repo := testRepo(t)
	files, err := repo.Discover(Comparison{Kind: KindDiff})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("files = %+v, want none", files)
	}
}

func TestLogPagePaginatesHeadHistory(t *testing.T) {
	repo := testRepo(t)
	for i := 1; i <= 12; i++ {
		write(t, repo.Root, "README.md", fmt.Sprintf("change %d\n", i))
		git(t, repo.Root, "add", "README.md")
		git(t, repo.Root, "commit", "-m", fmt.Sprintf("change %02d", i))
	}

	first, hasNext, err := repo.LogPage("", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 10 || !hasNext {
		t.Fatalf("first page len=%d hasNext=%v, want 10 true", len(first), hasNext)
	}
	if first[0].Subject != "change 12" || first[9].Subject != "change 03" {
		t.Errorf("first page bounds = %q..%q", first[0].Subject, first[9].Subject)
	}

	second, hasNext, err := repo.LogPage("", 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 3 || hasNext {
		t.Fatalf("second page len=%d hasNext=%v, want 3 false", len(second), hasNext)
	}
	if second[0].Subject != "change 02" || second[2].Subject != "initial" {
		t.Errorf("second page = %+v", second)
	}
	for _, commit := range append(first, second...) {
		if commit.SHA == "" || commit.ShortSHA == "" {
			t.Errorf("unexpected commit: %+v", commit)
		}
	}
}

func TestLogPageSearchesSubjectsAndResolvesSHA(t *testing.T) {
	repo := testRepo(t)
	write(t, repo.Root, "README.md", "needle\n")
	git(t, repo.Root, "add", "README.md")
	git(t, repo.Root, "commit", "-m", "Add Search Needle")

	matches, hasNext, err := repo.LogPage("search needle", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || hasNext || matches[0].Subject != "Add Search Needle" {
		t.Fatalf("subject search = %+v, hasNext=%v", matches, hasNext)
	}

	git(t, repo.Root, "checkout", "-b", "hidden")
	write(t, repo.Root, "README.md", "hidden\n")
	git(t, repo.Root, "add", "README.md")
	git(t, repo.Root, "commit", "-m", "Hidden branch commit")
	hidden, _, err := repo.LogPage("hidden branch", 0, 10)
	if err != nil || len(hidden) != 1 {
		t.Fatalf("hidden commit = %+v, err=%v", hidden, err)
	}
	git(t, repo.Root, "checkout", "master")

	bySHA, hasNext, err := repo.LogPage(hidden[0].ShortSHA, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(bySHA) != 1 || hasNext || bySHA[0].SHA != hidden[0].SHA {
		t.Fatalf("SHA search = %+v, hasNext=%v", bySHA, hasNext)
	}
}

func TestResolveCommit(t *testing.T) {
	repo := testRepo(t)
	commit, err := repo.ResolveCommit("master")
	if err != nil {
		t.Fatal(err)
	}
	if commit.SHA == "" || commit.ShortSHA == "" || commit.Subject != "initial" {
		t.Errorf("commit = %+v", commit)
	}
	if _, err := repo.ResolveCommit("not-a-revision"); err == nil {
		t.Fatal("expected invalid revision error")
	}
}

func TestDiscoverInclusiveCommitRange(t *testing.T) {
	repo := testRepo(t)
	write(t, repo.Root, "first.txt", "first\n")
	git(t, repo.Root, "add", "first.txt")
	git(t, repo.Root, "commit", "-m", "first change")
	first, err := repo.ResolveCommit("HEAD")
	if err != nil {
		t.Fatal(err)
	}

	write(t, repo.Root, "second.txt", "second\n")
	git(t, repo.Root, "add", "second.txt")
	git(t, repo.Root, "commit", "-m", "second change")
	newest, err := repo.ResolveCommit("HEAD")
	if err != nil {
		t.Fatal(err)
	}

	files, err := repo.Discover(Comparison{
		Kind: KindDiff,
		Args: []string{first.SHA + "^", newest.SHA},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "first.txt" || files[1].Path != "second.txt" {
		t.Errorf("range files = %+v", files)
	}
}

func TestBranchesSkipsCurrent(t *testing.T) {
	repo := testRepo(t)
	git(t, repo.Root, "branch", "feature/a")
	git(t, repo.Root, "switch", "-c", "stacked")

	got, err := repo.Branches()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] == "stacked" || got[1] == "stacked" {
		t.Errorf("branches = %v, want feature/a and master", got)
	}
}

func TestUntrackedComparisonLeavesIndexAlone(t *testing.T) {
	repo := testRepo(t)
	write(t, repo.Root, ".gitignore", "*.log\n")
	git(t, repo.Root, "add", ".gitignore")
	git(t, repo.Root, "commit", "-m", "ignore logs")
	git(t, repo.Root, "switch", "-c", "feature")
	write(t, repo.Root, "committed.txt", "committed\n")
	git(t, repo.Root, "add", "committed.txt")
	git(t, repo.Root, "commit", "-m", "feature work")
	write(t, repo.Root, "README.md", "edited\n")
	write(t, repo.Root, "new dir/untracked [x].txt", "untracked\n")
	write(t, repo.Root, "vendor/dep.txt", "excluded\n")
	write(t, repo.Root, "debug.log", "ignored\n")

	cmp := Comparison{
		Label:     "all",
		Kind:      KindDiff,
		Args:      []string{"--merge-base", "master"},
		Exclude:   []string{":(exclude)vendor"},
		Untracked: true,
	}
	files, err := repo.Discover(cmp)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]byte{}
	for _, f := range files {
		got[f.Path] = f.Status
	}
	want := map[string]byte{"committed.txt": 'A', "README.md": 'M', "new dir/untracked [x].txt": '?'}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("files = %q, want %q", got, want)
	}

	cmd, cleanup, err := repo.DisplayCmd(cmp, "new dir")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("+++ b/new dir/untracked [x].txt")) {
		t.Errorf("display output is missing the untracked file:\n%s", out)
	}
	var index string
	for _, e := range cmd.Env {
		if v, ok := strings.CutPrefix(e, "GIT_INDEX_FILE="); ok {
			index = v
		}
	}
	cleanup()
	if _, err := os.Stat(index); index == "" || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("temporary index %q survives cleanup: %v", index, err)
	}

	tracked, err := exec.Command("git", "-C", repo.Root, "ls-files").Output()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(tracked, []byte("untracked")) {
		t.Errorf("real index records the untracked file:\n%s", tracked)
	}
}

func TestUntrackedComparisonWithoutUntrackedFiles(t *testing.T) {
	repo := testRepo(t)
	write(t, repo.Root, "README.md", "edited\n")
	cmp := Comparison{Kind: KindDiff, Untracked: true}

	files, err := repo.Discover(cmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Status != 'M' {
		t.Errorf("files = %+v", files)
	}
	cmd, cleanup, err := repo.DisplayCmd(cmp, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if cmd.Env != nil {
		t.Errorf("env = %v, want inherited environment", cmd.Env)
	}
}

func TestWorktreesAndProgress(t *testing.T) {
	repo := testRepo(t)
	dir := t.TempDir()
	linked := filepath.Join(dir, "linked")
	detached := filepath.Join(dir, "detached")
	gone := filepath.Join(dir, "gone")
	git(t, repo.Root, "worktree", "add", "-q", "-b", "agent/task", linked)
	git(t, repo.Root, "worktree", "add", "-q", "--detach", detached)
	git(t, repo.Root, "worktree", "add", "-q", "--detach", gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	worktrees, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	branches := map[string]string{}
	for _, wt := range worktrees {
		if wt.Head == "" {
			t.Errorf("worktree without HEAD: %+v", wt)
		}
		branches[wt.Path] = wt.Branch
	}
	want := map[string]string{repo.Root: "master", linked: "agent/task", detached: ""}
	if !reflect.DeepEqual(branches, want) {
		t.Errorf("worktrees = %v, want %v", branches, want)
	}
	if worktrees[0].Path != repo.Root {
		t.Errorf("first worktree = %q, want the main one", worktrees[0].Path)
	}

	write(t, linked, "a.txt", "a\n")
	git(t, linked, "add", "a.txt")
	git(t, linked, "commit", "-m", "first")
	write(t, linked, "c.txt", "c\n")
	git(t, linked, "add", "c.txt")
	git(t, linked, "commit", "-m", "second")
	git(t, linked, "mv", "a.txt", "b.txt")
	write(t, linked, "README.md", "edited\n")
	write(t, linked, "notes/u.txt", "untracked\n")

	progress, err := (&Repo{Root: linked}).Progress("master")
	if err != nil {
		t.Fatal(err)
	}
	if progress != (Progress{Ahead: 2, Changed: 3}) {
		t.Errorf("progress = %+v, want 2 ahead and 3 changed", progress)
	}
}

func TestRemoveWorktreeDiscardsChangesKeepsBranch(t *testing.T) {
	repo := testRepo(t)
	dir := t.TempDir()
	linked := filepath.Join(dir, "linked")
	locked := filepath.Join(dir, "locked")
	git(t, repo.Root, "worktree", "add", "-q", "-b", "agent/task", linked)
	git(t, repo.Root, "worktree", "add", "-q", "--lock", "--detach", locked)
	write(t, linked, "README.md", "edited\n")
	write(t, linked, "notes/u.txt", "untracked\n")

	if err := repo.RemoveWorktree(linked); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(linked); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat removed worktree: %v", err)
	}
	git(t, repo.Root, "rev-parse", "--verify", "-q", "agent/task")

	if err := repo.RemoveWorktree(locked); err == nil {
		t.Error("removed a locked worktree")
	}
	worktrees, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 2 || worktrees[1].Path != locked {
		t.Errorf("worktrees = %+v, want main and locked", worktrees)
	}
}
