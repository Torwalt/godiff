# AGENTS.md

Shared guidance for AI coding agents (Cursor, Claude, etc.) working in this
repo. This file is committed and is the team-shared source of truth; root
`CLAUDE.md` and `.cursor/` are git-ignored, per-developer, and should just
import this file.

## What this project is

godiff is a small Bubble Tea TUI for navigating git changes by comparison
scope, directory cluster, and file, opening diffs through git's configured
pager (delta). It deliberately owns orchestration only. The review loop it
optimizes: select comparison → identify path cluster → inspect through
delta → return → inspect next cluster.

## Ownership boundaries

- **git** owns revision resolution, index/worktree semantics, merge bases,
  rename detection, pathspec matching, and diff generation.
- **delta** (the user's configured git pager) owns diff rendering, paging,
  search, and hyperlinks.
- **godiff** owns configuration, comparison selection, changed-path
  discovery, tree construction/navigation, and launching git as an external
  process.

Never parse full git patches. Never render diffs inside Bubble Tea. Never
invoke a shell — comparisons are structured argument lists, not command
strings.

## Package boundaries

- `internal/gitx` — git process integration. Two invocation classes:
  *discovery* (captured, `--name-status -z`, never paged) and *display*
  (streams attached to the terminal via Bubble Tea's `ExecProcess`, git runs
  its own pager). Must not import Bubble Tea.
- `internal/tree` — synthetic directory tree from changed paths: build,
  sort (dirs first, lexicographic), expand/collapse, flatten, selection
  restore. Must not import gitx or Bubble Tea.
- `internal/config` — TOML defaults/global/repo/CLI merging and validation.
  Produces `gitx.Comparison` values.
- `internal/ui` — Bubble Tea models only. State is an explicit state
  machine (selector → loading → tree → external process); background work
  returns messages to `Update`, never mutates the model from goroutines.

## Conventions

- Exclusions are git pathspecs (`:(exclude)…`), normalized in config,
  applied to both discovery and display so hidden paths never reappear.
- All git commands run with the repository root as working directory so
  repo-relative paths work as pathspecs.
- Keep the dependency set small (bubbletea, bubbles, lipgloss, BurntSushi
  toml). No go-git, no libgit2, no PTY libraries.
- Non-goals (do not add): staging, committing, branch management, hunk
  selection, embedded rendering, mouse support, plugins, API integrations.
- Prefer simple, explicit code; no speculative abstractions or
  configurability. Touch only what the task requires and match existing
  style. Comments explain *why*, not *what* — no narration, no essays.

## Verification

```
go test ./...                 # unit + integration tests (temp git repos)
go build -o /dev/null ./...   # compile check without stray binaries
go vet ./... && gofmt -l .
nix develop                   # go, gopls, git, delta, linters (direnv auto-loads)
nix flake check               # sandboxed build + tests
```

For TUI changes, validate the terminal handoff manually in tmux + kitty:
open a diff, use delta, quit, confirm the tree restores at the same node.
A scripted variant: run godiff in a detached tmux session, drive it with
`tmux send-keys`, inspect with `tmux capture-pane -p`.
