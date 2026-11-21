# Release notes

Each release has a `## [<version>] - <date>` section, newest first. `scripts/release.ps1` publishes the section for
the tagged version as its GitHub release notes, and refuses to release a version without one. Changes since the last
release go under `## [Unreleased]`, renamed to the version when it is tagged.

## [Unreleased]

## [0.2.0] - 2026-11-21

- A stash pane under the log: `s` stashes the changes, `S` opens the pane, `enter` pops a stash and `D` drops one.
- `n` on the branch screen creates a branch from origin's default branch, taking uncommitted changes along. The
  branch list shows remote branches too, `enter` on one creates a local branch tracking it, and `f` fetches.
- `w` lists the repository's work trees and reviews one, and `D` deletes one with its branch, refusing uncommitted
  changes.
- `P` pushes a branch only to its own name on origin, so a branch that tracks `origin/main` is published instead of
  pushed to main.
- perusal reopens in the view it was quit in.
- `perusal show` diffs two files under a note in a read-only screen, for a popup that proposes a change. `i` shows
  the full note.
- `perusal -version` prints the version.
- The overview (`o`) shows the selected repository's changed files, outgoing and incoming commits, and fills in as
  each row loads. `f` and `F` fetch, `P` pushes, `R` rebases onto the default branch, `C` cherry-picks a work tree's
  commits onto the default branch in its main clone, `D` deletes a work tree, and `r` lists the repositories again.
- `/` filters the overview. `enter` opens the selected row, `tab` keeps the filter, and backspace on an empty query
  closes it (in log search too). The mouse wheel and a click select overview rows.
- Log search and the overview filter draw matches in bold blue, and a paste into the log search filters the log.
- Follow mode tracks an agent in a VM with `agentHost`: perusal takes the agent's new commits into your work tree as
  unstaged changes, over one persistent ssh session, and offers to create your copy of a work tree the agent added.
- Follow mode opens linked work trees, holds a handle in each checked `.git` so it cannot be swapped, and refuses a
  `.git` whose permissions or owner the agent can change.
- While perusal's terminal has focus, a focus file change that would switch away waits, shown as `next focus` in the
  header, and applies when you leave the window.
- `n` jumps correctly in a fullscreen diff, and diff, unstage and discard work in a repository with no commits.

## [0.1.0] - 2026-10-22

- First release.
