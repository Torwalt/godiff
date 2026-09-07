package gitx

import (
	"reflect"
	"testing"
)

func TestParseNameStatus(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []ChangedFile
	}{
		{
			name: "empty",
			in:   "",
			want: nil,
		},
		{
			name: "modified and added",
			in:   "M\x00db/queries/foo.sql\x00A\x00internal/app/new.go\x00",
			want: []ChangedFile{
				{Path: "db/queries/foo.sql", Status: 'M'},
				{Path: "internal/app/new.go", Status: 'A'},
			},
		},
		{
			name: "rename with score",
			in:   "R086\x00old/name.go\x00new/name.go\x00",
			want: []ChangedFile{
				{Path: "new/name.go", OldPath: "old/name.go", Status: 'R'},
			},
		},
		{
			name: "copy with score",
			in:   "C075\x00a.txt\x00b.txt\x00",
			want: []ChangedFile{
				{Path: "b.txt", OldPath: "a.txt", Status: 'C'},
			},
		},
		{
			name: "filename with spaces and newline",
			in:   "D\x00dir with space/weird\nname.txt\x00",
			want: []ChangedFile{
				{Path: "dir with space/weird\nname.txt", Status: 'D'},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNameStatus([]byte(tt.in))
			if err != nil {
				t.Fatalf("ParseNameStatus: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseNameStatusMalformed(t *testing.T) {
	for _, in := range []string{"M\x00", "R100\x00only-one\x00"} {
		if _, err := ParseNameStatus([]byte(in)); err == nil {
			t.Errorf("ParseNameStatus(%q): expected error", in)
		}
	}
}

func TestDisplayCmdArgs(t *testing.T) {
	repo := &Repo{Root: "/tmp/x"}

	tests := []struct {
		name     string
		cmp      Comparison
		pathspec string
		want     []string
	}{
		{
			name:     "root of plain diff without exclusions",
			cmp:      Comparison{Kind: KindDiff},
			pathspec: "",
			want:     []string{"git", "diff"},
		},
		{
			name:     "directory pathspec",
			cmp:      Comparison{Kind: KindDiff, Args: []string{"master...HEAD"}},
			pathspec: "proto",
			want:     []string{"git", "diff", "master...HEAD", "--", "proto"},
		},
		{
			name:     "root with exclusions",
			cmp:      Comparison{Kind: KindDiff, Exclude: []string{":(exclude)vendor"}},
			pathspec: "",
			want:     []string{"git", "diff", "--", ":(exclude)vendor"},
		},
		{
			name:     "show single file",
			cmp:      Comparison{Kind: KindShow, Args: []string{"HEAD"}},
			pathspec: "db/queries/foo.sql",
			want:     []string{"git", "show", "--format=", "HEAD", "--", "db/queries/foo.sql"},
		},
		{
			name: "show format override",
			cmp: Comparison{
				Kind: KindShow,
				Args: []string{"--format=oneline", "HEAD"},
			},
			want: []string{"git", "show", "--format=", "--format=oneline", "HEAD"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := repo.DisplayCmd(tt.cmp, tt.pathspec)
			if !reflect.DeepEqual(cmd.Args, tt.want) {
				t.Errorf("args = %v, want %v", cmd.Args, tt.want)
			}
			if cmd.Dir != repo.Root {
				t.Errorf("dir = %q, want %q", cmd.Dir, repo.Root)
			}
		})
	}
}
