# perusal

A small terminal git client for reading diffs, a stripped-down [lazygit](https://github.com/jesseduffield/lazygit).
The diff gets most of the screen and the working tree is reread every 2 seconds. It also stages, commits, amends,
pushes, browses the log and switches branches, among other things.

On Windows, follow mode also tracks an LLM coding agent that runs as its own user account: `perusal follow` shows
the diffs of whichever repository the agent is working in. lazygit, [gitui](https://github.com/gitui-org/gitui) and
[tig](https://github.com/jonas/tig) cover most of the same ground and more.

perusal runs your local `git` for everything. There is no GitHub API and no login: a push uses whatever credentials
git already has.

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
| `f` | anywhere | show the whole file around the changes, `f` again shows only the changed parts |
| `u` | anywhere | show the diff unified, removed lines above added ones, `u` again shows it side by side |
| `n` `N` | anywhere | jump the diff to the next or previous change, wrapping around at either end |
| `c` | anywhere | open the commit popup |
| `P` | anywhere | push the branch to its own name on `origin`, or publish it there when it tracks anything else |
| `A` | log | amend the selected commit with the staged changes |
| `r` | log | reword the selected commit's message in the commit popup |
| `b` | anywhere | list the branches, `enter` switches to one, `D` deletes one after a confirm, `f` fetches |
| `w` | anywhere | list the repository's work trees, `enter` reviews one |
| `o` | anywhere | in follow mode, open the overview of every repository under `ownerDir` |
| `f` `F` | overview | fetch the selected repository or every one, dropping the remote branches that were deleted |
| `p` | overview | fast-forward the selected repository to its upstream, or a finished branch to the default one |
| `P` | overview | push or publish the selected repository's branch |
| `enter` | log | expand the commit into its files |
| `/` | log | search, `enter` keeps it, `esc` clears it |
| `?` | anywhere | list every key, `?` or `esc` closes it |
| `q` | anywhere | quit |

In the commit popup, `tab` switches between summary and description, `enter` commits from the summary, `ctrl+enter`
commits from either field, and `esc` cancels. The changes pane shows how many lines the staged changes add and
remove. The mouse works too: click a row to select it, click the folded pane to open it, right-click a commit to
amend it. The wheel moves the selection over a list and scrolls the diff anywhere else.

The top line names the repository and its branch, then counts its local branches, its remote branches and its work
trees, as in `4 branches (37 remote) · 2 work trees`. In a linked work tree, made by `git worktree add`, it also names
the repository's main one, as in `work tree of repo`.

On quit, perusal saves which list was showing and the `d`, `f` and `u` toggles to `perusal\state.json` in the user
config directory (`%APPDATA%` on Windows), and the next launch opens the same way.

Amend folds only the staged changes into the commit and keeps its message. An older commit gets a fixup commit and an
autosquash rebase, so every commit above it is rewritten, and a rebase that stops is aborted and the fixup undone.

Reword changes only the message. Below `HEAD`, perusal recreates every commit above it with the same files and
author, signed if `commit.gpgSign` is on, so staged and unstaged changes stay as they are. Below `HEAD`, amend and
reword refuse a merge commit, the root commit and any commit with a merge above it.

`P` pushes a branch only to its own name on `origin`. A branch that tracks anything else, such as one created from
`origin/main`, is unpublished, and `P` publishes it to its own name and tracks that instead.

When the branch and its upstream have diverged, as after amending a pushed commit, the push popup offers a force push
instead. It pushes with `--force-with-lease --force-if-includes`, so git refuses if the upstream moved since your last
fetch.

The branch list marks a branch merged when merging it into `origin`'s default branch would change nothing, which
counts a squash merge. That check sees only commits, so the current branch is not marked while the work tree has
uncommitted changes, and a branch checked out in another work tree shows that work tree's name instead. `D` deletes
any branch but the default one and those checked out in a work tree, and its confirm warns when the branch is not
merged. `enter` refuses to switch while the work tree has uncommitted changes, and on a branch checked out in another
work tree it reviews that work tree instead.

Below the local branches, the list shows muted each remote branch that has no local branch of the same name, such as
`origin/fix-x`. `enter` on one creates the local branch tracking it with `git switch --track`, and `D` refuses it. The
list is only as fresh as the last fetch, so `f` fetches with `--prune` and reloads it.

The work tree list shows each work tree's folder, its branch and its uncommitted file count, and marks the one under
review with `*`. In follow mode, one that fails a check below is listed muted, and `enter` on it shows why.

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
  they change. Something on the agent side has to write it, such as a [herdr](https://github.com/herdrdev/herdr)
  plugin, and its first line is where the agent's focused session works, such as its pane's directory. perusal opens
  the first repository with uncommitted changes, or the first one when all are clean. A new list that starts with the
  same repository and still names the one under review keeps it open, and a path that fails a check below is shown in
  the footer.
- `agentDir` and `ownerDir` are one directory as the agent and you reach it. perusal maps a path under `agentDir`
  to the same path under `ownerDir`, and refuses anything outside it.

The overview (`o`) lists every repository under `ownerDir` with its branch, its uncommitted file count, how far it is
ahead of and behind its upstream, and how far it is behind `origin`'s default branch. The repositories the focus file
names come first, marked `●`. One that fails a check below is listed muted, and `enter` on it shows why. `enter`
reviews the selected one, `f` fetches it and `F` fetches them all (`git fetch --prune`), `p` pulls it (fast-forward
only, from its upstream rather than the default branch), `P` pushes it, and `esc` goes back. A branch is finished when
it is clean, `origin`'s default branch holds all its commits, and it has no upstream or the remote deleted it. `p` then
switches to the default branch and pulls that. An unpublished branch shows `↑N unpublished`, the commits no
remote-tracking branch holds. Below the table, the selected repository's changed files are listed beside the commits
`P` would push and the commits `p` would pull. perusal fetches only on `f` and `F`, so those last ones are as of the
last fetch. A repository opened this way stays open until the focus file starts with a different repository or no
longer names it.

perusal runs git as you in a repository the agent chose, and git runs whatever that repository's `.git` tells it to:
hooks, and config such as `core.fsmonitor`, `core.hooksPath` or `core.sshCommand` that names a program. An agent
that can write `.git` could plant one and have git run it as you. So perusal checks `.git` and everything in it, and
refuses the repository if the agent account owns any of it, if any of it is a link or junction, if any of it grants
write access ahead of a deny of `(OI)(CI)(WD,AD,WEA,WA,DE,DC)` for that account, or if any of it lets that account or
a group other than SYSTEM and Administrators change its permissions or owner. git then runs with that `.git` pinned,
so it never searches for another one. For the same reason the agent account must not be able to write your
`%APPDATA%`, where the config lives.

A linked work tree, made by `git worktree add`, has a `.git` file instead, which the agent can rewrite. perusal opens
it only when that file names a folder in the `worktrees` of a `.git` that passes the checks above, and that folder
names the work tree back. git then runs with that folder pinned, so a later rewrite of the file changes nothing. The
overview lists a work tree beside its repository, such as `repo-side` next to `repo`, but not one nested inside it,
such as one under `repo\.claude\worktrees`. The focus file still opens that one.

The agent can still rename the work tree folder that holds a checked `.git` and plant its own `.git` at the old path.
So perusal keeps a handle open in each `.git` it checked, or in a linked work tree's folder there, until it exits.
Windows refuses to rename a folder with a handle open inside it, so while perusal runs neither the agent nor you can
move such a repository or rename any folder above it.

From an elevated prompt, for each repository under `ownerDir`, take ownership if the agent cloned it, add the deny,
then reset everything inside `.git` to the inherited ACL, which drops any grant made directly on it:

```
icacls C:\Users\you\dev\repo\.git /setowner you /T
icacls C:\Users\you\dev\repo\.git /deny "claude:(OI)(CI)(WD,AD,WEA,WA,DE,DC)"
icacls C:\Users\you\dev\repo\.git\* /reset /T
```

The deny lists the write bits one by one because `(W)` includes `SYNCHRONIZE`, which every open requests, so it would
block reads too and git would no longer see the repository.

## License

[Apache-2.0](LICENSE)
