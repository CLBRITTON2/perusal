# perusal

A small terminal git client for reading diffs, a stripped-down [lazygit](https://github.com/jesseduffield/lazygit).
The diff gets most of the screen. It also stages, commits, amends, pushes and switches branches. On Windows, follow
mode tracks an LLM coding agent and shows the diffs of whichever repository it is working in.

perusal runs your local `git` for everything, with no GitHub API and no login.

## Install

Needs Go 1.27 or later and `git`. A [Nerd Font](https://www.nerdfonts.com) shows the glyphs.

```
go install github.com/CLBRITTON2/perusal/cmd/perusal@latest
```

From a clone, `make` installs it and `make test` tests it.

## Use

```
perusal              # review the repository in the current directory
perusal -repo <dir>  # review the repository <dir> is in
```

| Key | Does |
|---|---|
| `j` `k` | select |
| `tab` | switch between changes and log |
| `space` `a` | stage the file, or everything |
| `c` | commit |
| `P` | push |
| `d` `f` `u` | fullscreen, whole file, unified diff |
| `b` `w` | branches, work trees |
| `o` | overview of every repository (follow mode) |
| `?` | every key |
| `q` | quit |

The mouse works too.

## Show

```
perusal show -old <file> -new <file> -label <path> -note <file>
```

Shows the diff of two files under a note, read only. Suits a popup that shows a proposed change.

## Follow mode (Windows)

`perusal follow` reviews the repositories an agent running as its own account is working in, and switches as it moves.
It reads `%APPDATA%\perusal\config.json`:

```json
{
  "agentAccount": "claude",
  "focusFile": "C:\\Users\\claude\\dev\\.herdr-focus",
  "agentDir": "C:\\Users\\claude\\dev",
  "ownerDir": "C:\\Users\\you\\dev"
}
```

- `focusFile` lists the directories the agent is working in, one per line, written by something on the agent side
  such as a [herdr](https://github.com/herdrdev/herdr) plugin.
- `agentDir` and `ownerDir` are the same directory as the agent and you see it. perusal refuses paths outside it.
- `agentHost` replaces `agentAccount` when the agent works on another machine over ssh. perusal then brings the
  agent's commits into your work tree as unstaged changes.

Since git runs whatever a repository's `.git` tells it to, perusal refuses a repository whose `.git` the agent account
can write. From an elevated prompt, for each repository:

```
icacls C:\Users\you\dev\repo\.git /setowner you /T
icacls C:\Users\you\dev\repo\.git /deny "claude:(OI)(CI)(WD,AD,WEA,WA,DE,DC)"
icacls C:\Users\you\dev\repo\.git\* /reset /T
```

With `agentHost` set, no deny is needed.

## License

[Apache-2.0](LICENSE)
