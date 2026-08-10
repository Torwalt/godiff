package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestMergeDefaults(t *testing.T) {
	cfg, err := Merge(nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseBranch != "master" {
		t.Errorf("base = %q", cfg.BaseBranch)
	}
	ids := make([]string, len(cfg.Comparisons))
	for i, c := range cfg.Comparisons {
		ids[i] = c.ID
	}
	want := []string{"worktree", "staged", "branch", "commit"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}

	branch, _ := cfg.Comparison("branch")
	if !reflect.DeepEqual(branch.Args, []string{"master...HEAD"}) {
		t.Errorf("branch args = %v", branch.Args)
	}
}

func TestMergeBaseBranchPrecedence(t *testing.T) {
	global := File{BaseBranch: "main"}
	repo := File{BaseBranch: "develop"}

	cfg, err := Merge([]File{global, repo}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseBranch != "develop" {
		t.Errorf("base = %q, want repo override", cfg.BaseBranch)
	}
	branch, _ := cfg.Comparison("branch")
	if branch.Label != "Branch against develop" {
		t.Errorf("label = %q", branch.Label)
	}

	cfg, err = Merge([]File{global, repo}, Options{BaseBranch: "release"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseBranch != "release" {
		t.Errorf("base = %q, want CLI override", cfg.BaseBranch)
	}
}

func TestMergeOverridesBuiltinAndAddsCustom(t *testing.T) {
	repo := File{Comparisons: []ComparisonFile{
		{ID: "branch", Label: "vs {base}", Args: []string{"{base}..HEAD"}},
		{ID: "last3", Label: "Last 3 commits", Kind: "diff", Args: []string{"HEAD~3..HEAD"}},
	}}

	cfg, err := Merge([]File{repo}, Options{BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Comparisons) != 5 {
		t.Fatalf("len = %d, want 4 builtins + 1 custom", len(cfg.Comparisons))
	}
	branch, _ := cfg.Comparison("branch")
	if branch.Label != "vs main" || !reflect.DeepEqual(branch.Args, []string{"main..HEAD"}) {
		t.Errorf("branch = %+v, want {base} substituted", branch)
	}
	if _, ok := cfg.Comparison("last3"); !ok {
		t.Error("custom comparison missing")
	}
}

func TestMergeExclusions(t *testing.T) {
	global := File{Exclude: []string{"vendor"}}
	repo := File{
		Exclude: []string{":(exclude,glob)**/*.pb.go"},
		Comparisons: []ComparisonFile{
			{ID: "worktree", Label: "wt", Exclude: []string{"mocks"}},
		},
	}

	cfg, err := Merge([]File{global, repo}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	wt, _ := cfg.Comparison("worktree")
	want := []string{":(exclude)vendor", ":(exclude,glob)**/*.pb.go", ":(exclude)mocks"}
	if !reflect.DeepEqual(wt.Exclude, want) {
		t.Errorf("exclude = %v, want %v", wt.Exclude, want)
	}
	if !reflect.DeepEqual(cfg.Exclude, want[:2]) {
		t.Errorf("shared exclude = %v, want %v", cfg.Exclude, want[:2])
	}
	// comparisons without their own excludes still get the shared ones
	staged, _ := cfg.Comparison("staged")
	if !reflect.DeepEqual(staged.Exclude, want[:2]) {
		t.Errorf("staged exclude = %v", staged.Exclude)
	}
}

func TestMergeInvalidComparisons(t *testing.T) {
	for _, f := range []File{
		{Comparisons: []ComparisonFile{{ID: "x", Kind: "patch"}}},
		{Comparisons: []ComparisonFile{{Label: "no id"}}},
		{Comparisons: []ComparisonFile{{ID: "s", Kind: "show"}}}, // show without revision
	} {
		if _, err := Merge([]File{f}, Options{}); err == nil {
			t.Errorf("Merge(%+v): expected error", f)
		}
	}
}

func TestLoadRepoConfig(t *testing.T) {
	root := t.TempDir()
	content := `
base_branch = "main"
exclude = ["gen"]

[[comparisons]]
id = "review"
label = "Review vs {base}"
args = ["{base}...HEAD"]
`
	if err := os.WriteFile(filepath.Join(root, RepoConfigName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "no-such-config"))

	cfg, err := Load(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	review, ok := cfg.Comparison("review")
	if !ok {
		t.Fatal("review comparison missing")
	}
	if review.Label != "Review vs main" {
		t.Errorf("label = %q", review.Label)
	}
	if !reflect.DeepEqual(review.Exclude, []string{":(exclude)gen"}) {
		t.Errorf("exclude = %v", review.Exclude)
	}

	// NoRepoConfig skips the file entirely.
	cfg, err = Load(root, Options{NoRepoConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Comparison("review"); ok {
		t.Error("repo config should have been skipped")
	}
}

func TestLoadMalformedConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, RepoConfigName), []byte("base_branch = ["), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "no-such-config"))
	if _, err := Load(root, Options{}); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestComparisonFileTOMLShape(t *testing.T) {
	// guards the documented schema
	var f File
	if err := toml.Unmarshal([]byte(`
[[comparisons]]
id = "x"
kind = "show"
args = ["HEAD~1"]
exclude = ["a", ":(exclude)b"]
`), &f); err != nil {
		t.Fatal(err)
	}
	if f.Comparisons[0].Kind != "show" || len(f.Comparisons[0].Exclude) != 2 {
		t.Errorf("parsed = %+v", f.Comparisons[0])
	}
	if _, err := Merge([]File{f}, Options{}); err != nil {
		t.Errorf("Merge: %v", err)
	}
}
