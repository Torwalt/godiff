# godiff

Interactive terminal navigator for git changes. Pick a comparison, browse the
changed files as a directory tree, and open focused diffs — rendered by your
existing git pager (delta), not by godiff itself.

godiff removes the repetitive parts of a diff-review loop:

- no more reconstructing `git diff master...HEAD -- some/dir ':(exclude)…'`,
- repository-specific exclusions (generated code, lockfiles) apply
  automatically to every diff,
- large changesets become navigable clusters instead of one flat patch.

Git stays the source of truth for diff semantics; delta keeps doing the
rendering, paging, and hyperlinking, so tmux copy mode and your
open-in-neovim workflow keep working unchanged.

## Usage

```
godiff                      # comparison selector
godiff 636c8908c            # jump straight to a commit
godiff -comparison branch   # jump straight into a comparison
godiff -base develop        # override the base branch for this run
godiff -print-config        # show effective comparisons and exclusions
```

Built-in choices: working tree, staged changes, branch against the configured
base (`master` by default), **Branch against…**, **All changes since** the
base, the current commit, **Show commit**, and **Worktree…**. Branch against…
(`b`) lists local branches, most recently committed first; type to
fuzzy-filter, and `enter` diffs `<branch>...HEAD`, which reviews a stacked
branch against the branch it was cut from. All changes since (`a`) diffs the
merge base with the working tree, so committed, staged, unstaged and untracked
work shows up as one tree. Show commit lists the history reachable from
`HEAD`, marks the configured base branch with `◆`, and opening one reviews
only that commit.

The working tree and All changes since comparisons include untracked files,
marked `?`. godiff diffs them through a throwaway copy of the index that
records them as intent-to-add; the real index is never touched.

### Worktrees

Worktree… (`t`) lists every checkout git has registered for the repository,
wherever it lives on disk (agent worktrees under `.claude/worktrees/`, temp
directories, siblings). Each row shows the branch (or detached SHA), commits
ahead of the base, uncommitted paths, and the path; `●` marks the active one.
Type to fuzzy-filter by branch or path; `enter` switches godiff into that
checkout and returns to the selector, so you pick how to review it: working
tree, staged, branch against the base, All changes since, or single commits.
Every comparison then runs in the chosen checkout.

`ctrl+d` asks to delete the highlighted worktree; only `y` confirms. Deletion
runs `git worktree remove --force`: uncommitted changes are discarded, the
branch stays. The main worktree, the active one, and locked ones cannot be
deleted.

Linked worktrees usually lack an ignored `.godiff.toml`, so godiff falls back
to the main worktree's.

### Keys

| Key | Action |
| --- | --- |
| `↑/k` `↓/j` | move selection |
| `→/l` / `←/h` | expand / collapse directory |
| `H` | collapse all directories and return to the top |
| `enter` | open: directory toggles its subclusters, file opens its diff, root opens the full diff |
| `o` | open diff for the selected node (root = full comparison, directory = subtree, file = single file) |
| `/` | fuzzy-search directory clusters; `enter` jumps to and opens the match, `esc`/`q` cancels |
| `c` | show the full commit messages of the comparison; `esc`/`q`/`b` returns to the tree |
| `space` | anchor or clear a commit range in Show commit |
| `w` `s` `m` `a` `c` | in the selector: open working tree, staged, branch, all changes since, current commit |
| `b` `h` `t` | in the selector: Branch against…, Show commit, Worktree… |
| `r` | refresh changed paths (keeps selection and expansion) |
| `esc`/`q`/`b` | back one screen |
| `?` | toggle full key help |
| `ctrl+c` | quit |

`c` in a file tree prints the comparison's commits as `git log` does, subject
and body: the single commit, every commit of a range, or the commits a branch
comparison adds (`<base>..HEAD` for `<base>...HEAD`). Working tree and staged
changes have none. `↑/k` `↓/j` scroll, `space`/`pgdn` and `pgup` page, `g`/`G`
jump to the top and bottom.

In Show commit, `↑/k` and `↓/j` scroll through commits, `/` searches commit
subjects, and `ctrl-v` starts a search with the clipboard contents. While
searching, ordinary keys edit the query, arrows choose a result, `esc`/`q` returns
to navigation, and `enter` opens the selected commit. A full or abbreviated SHA
can select a commit outside the current `HEAD` history. Returning from a
commit's file tree restores the same selection. Press `space` to anchor a range,
move to its other endpoint, and press `enter` to review the selected commits as
one inclusive diff; press `space` again to clear the range.

## Configuration

TOML, merged in order: built-in defaults → `~/.config/godiff/config.toml` →
`<repo>/.godiff.toml` → command-line flags. Repository entries override
global ones with the same `id`; exclusion lists are additive.

See [`godiff.example.toml`](godiff.example.toml) for the full schema.

## Installation

The primary packaging is the Nix flake:

```
nix run github:Torwalt/godiff        # try it
nix profile install github:Torwalt/godiff
```

or as a flake input in a NixOS / Home Manager configuration
(`inputs.godiff.url = "github:Torwalt/godiff";` then add
`inputs.godiff.packages.${system}.default` to your packages).

Plain Go also works: `go install github.com/Torwalt/godiff@latest`.

## Development

```
nix develop     # go, gopls, git, delta, golangci-lint, gofumpt
go test ./...
nix flake check
```

Layout: `internal/gitx` (git process integration), `internal/tree`
(synthetic path tree), `internal/config` (TOML merging), `internal/ui`
(Bubble Tea models). See `AGENTS.md` for the architecture rules that keep
those boundaries intact.
