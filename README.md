# perusal

A small terminal git client for reviewing and committing changes. It is built for the loop where an AI coding agent
writes the code and you read every line before it lands: a slimmed-down lazygit where the diff gets most of the screen.

## What it does

- Lists the working tree's changed files and shows each as a side-by-side diff, or one-sided for an added or deleted
  file.
- Stages, unstages, commits and pushes.
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
| `d` | anywhere | show the diff fullscreen, `d` again brings the lists back |
| `c` | anywhere | open the commit popup |
| `P` | anywhere | open the push popup |
| `enter` | log | expand the commit into its files |
| `/` | log | search, `enter` keeps it, `esc` clears it |
| `q` | anywhere | quit |

In the commit popup, `tab` switches between summary and description, `enter` commits from the summary,
`ctrl+enter` commits from either field, and `esc` cancels. The mouse works too: click a row to select it, click the
folded pane to open it. The wheel moves the selection over a list and scrolls the diff anywhere else.

## Follow mode (Windows)

`perusal follow` is for an agent that runs as its own Windows account. It reviews whichever repository the
agent is working in, and switches as the agent moves between repositories. It reads
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
- `focusFile` holds one absolute path, the directory the agent is working in, and is rewritten whenever that
  changes. Something on the agent side has to write it, such as a herdr plugin.
- `agentDir` and `ownerDir` are one directory as the agent and you reach it. perusal maps a path under `agentDir`
  to the same path under `ownerDir`, and refuses anything outside it.

perusal runs git as you in a repository the agent chose, and git runs that repository's hooks and config. So every
`.git` under `ownerDir` needs an inherited deny of `(OI)(CI)(WD,AD,WEA,WA,DE,DC)` for the agent account, and perusal
refuses any `.git` the agent owns or that lacks the deny, since a planted hook would otherwise run as you. For the
same reason the agent account must not be able to write your `%APPDATA%`.

## License

[Apache-2.0](LICENSE)
