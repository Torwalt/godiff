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
godiff -comparison branch   # jump straight into a comparison
godiff -base develop        # override the base branch for this run
godiff -print-config        # show effective comparisons and exclusions
```

Built-in comparisons: working tree, staged changes, branch against the
configured base (`master` by default), and the current commit.

### Keys

| Key | Action |
| --- | --- |
| `↑/k` `↓/j` | move selection |
| `→/l` / `←/h` | expand / collapse directory |
| `enter` | open diff for the selected node (root = full comparison, directory = subtree, file = single file) |
| `r` | refresh changed paths (keeps selection and expansion) |
| `esc`/`b` | back to the comparison selector |
| `?` | toggle full key help |
| `q` | quit |

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
