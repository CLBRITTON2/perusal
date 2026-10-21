# perusal

A small terminal git client for reviewing and committing changes. It is built for the loop where an AI coding agent
writes the code and you read every line before it lands: a slimmed-down lazygit where the diff gets most of the screen.

## What it does

- Lists the working tree's changed files and shows each as a side-by-side diff, or one-sided for an added or deleted
  file. In a block that removes and adds as many lines, each pair marks the part that changed.
- Stages, unstages, commits, amends and pushes.
- Browses the log with a commit graph, expands a commit into its files, and searches by hash, author, ref or message.
- Refreshes every 2 seconds, so the review keeps up with edits as they happen.

perusal runs your local `git` for everything. There is no GitHub API and no login. A push uses whatever credentials
git already has, and fails instead of prompting when it has none.

## Install

Needs Go 1.27 or later and `git` on your `PATH`. A [Nerd Font](https://www.nerdfonts.com) shows the branch and folder
glyphs.

```
go install github.com/CLBRITTON2/perusal/cmd/perusal@latest
```

From a clone, `make` installs it and `make test` vets and tests it for Windows, Linux and macOS.

## Use

```
perusal              # review the repository the current directory is in
perusal -repo <dir>  # review the repository <dir> is in
```

| Key | Where | Does |
|---|---|---|
| `j` `k`, arrows | lists | select |
| `tab` | lists | switch between changes and log |
| `ctrl+d` `ctrl+u`, page keys | anywhere | scroll the diff |
| `space` | changes | stage or unstage the file |
| `a` | changes | stage or unstage everything |
| `x` `X` | changes | discard the file or every change, after a confirm. Untracked files are deleted, ignored ones stay |
| `d` | anywhere | show the diff fullscreen, `d` again brings the lists back |
| `c` | anywhere | open the commit popup |
| `P` | anywhere | open the push popup |
| `A` | log | amend the selected commit with the staged changes |
| `b` | anywhere | list the local branches, `enter` switches to one, `D` deletes one after a confirm |
| `o` | anywhere | in follow mode, open the overview of every repository under `ownerDir` |
| `p` | overview | fast-forward the selected repository to its upstream, or a finished branch to the default one |
| `enter` | log | expand the commit into its files |
| `/` | log | search, `enter` keeps it, `esc` clears it |
| `q` | anywhere | quit |

In the commit popup, `tab` switches between summary and description, `enter` commits from the summary, `ctrl+enter`
commits from either field, and `esc` cancels. The changes pane shows how many lines the staged changes add and
remove. The mouse works too: click a row to select it, click the folded pane to open it, right-click a commit to
amend it. The wheel moves the selection over a list and scrolls the diff anywhere else.

Amend folds only the staged changes into the commit and keeps its message. An older commit gets a fixup commit and an
autosquash rebase, so every commit above it is rewritten, and a rebase that stops is aborted and the fixup undone.
Below `HEAD`, perusal refuses a merge commit, the root commit and any commit with a merge above it.

When the branch and its upstream have diverged, as after amending a pushed commit, the push popup offers a force push
instead. It pushes with `--force-with-lease --force-if-includes`, so git refuses if the upstream moved since your last
fetch.

The branch list marks a branch merged when merging it into `origin`'s default branch would change nothing, which
counts a squash merge. `D` deletes any branch but the current and the default one, and its confirm warns when the
branch is not merged. `enter` refuses to switch while the work tree has uncommitted changes.

## Follow mode (Windows)

`perusal follow` is for an agent that runs as its own Windows account. It reviews the repositories the agent is
working in, and switches as the agent moves between them. It reads
`%APPDATA%\perusal\config.json`, where every key is required:

```json
{
  "agentAccount": "claude",
  "focusFile": "C:\\Users\\claude\\dev\\.herdr-focus",
  "agentDir": "C:\\Users\\claude\\dev",
  "ownerDir": "C:\\Users\\you\\dev"
}
```

- `agentAccount` is the Windows account the agent runs as.
- `focusFile` holds one absolute path per line, the directories the agent is working in, and is rewritten whenever
  they change. Something on the agent side has to write it, such as a herdr plugin. perusal opens the first
  repository with uncommitted changes, or the first one when all are clean. A new list that still names the
  repository under review keeps it open, and a path that fails a check below is shown in the footer.
- `agentDir` and `ownerDir` are one directory as the agent and you reach it. perusal maps a path under `agentDir`
  to the same path under `ownerDir`, and refuses anything outside it.

The overview (`o`) lists every repository under `ownerDir` with its branch, its uncommitted file count, how far it is
ahead of and behind its upstream, and how far it is behind `origin`'s default branch. The repositories the focus file
names come first, marked `●`. One that fails a check below is listed muted, and `enter` on it shows why. `enter`
reviews the selected one, `p` pulls it (fast-forward only, from its upstream rather than the default branch), and
`esc` goes back. A branch is finished when it is clean, `origin`'s default branch holds all its commits, and it has
no upstream or the remote deleted it. `p` then switches to the default branch and pulls that. A repository opened this
way stays open until the focus file changes to a list that does not name it.

perusal runs git as you in a repository the agent chose, and git runs that repository's hooks and config. So every
`.git` under `ownerDir` needs an inherited deny of `(OI)(CI)(WD,AD,WEA,WA,DE,DC)` for the agent account, and perusal
refuses any `.git` the agent owns or that lacks the deny, since a planted hook would otherwise run as you. For the
same reason the agent account must not be able to write your `%APPDATA%`.

## License

[Apache-2.0](LICENSE)
