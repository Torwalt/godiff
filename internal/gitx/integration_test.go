package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
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
