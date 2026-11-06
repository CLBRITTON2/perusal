package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CLBRITTON2/perusal/internal/agenthost"
)

var ErrAgentSnapshotRefused = errors.New("refused the agent's work tree snapshot")

// SnapshotAgentWorkTree commits the work tree of the agent's copy of r on host to refs/perusal/snapshot there, with
// untracked files and without ignored ones. It stages through a temporary index, so the agent's index and branch stay
// as they are. host's ssh login shell must be Windows PowerShell, and the copy must sit at r.Dir there too.
func (r Repository) SnapshotAgentWorkTree(ctx context.Context, host string) error {
	if _, err := agenthost.Run(ctx, host, agentSnapshotScript(r.Dir)); err != nil {
		return fmt.Errorf("snapshot the agent's work tree of %s: %w", r.Dir, err)
	}
	return nil
}

// Exit codes are checked by hand: with $ErrorActionPreference Stop, PowerShell 5.1 turns git's warnings on a
// redirected stderr into errors. The fixed dates make an unchanged work tree give the same commit, so the fetch and
// mirror after it do nothing.
func agentSnapshotScript(dir string) string {
	return strings.Join([]string{
		"$index = Join-Path $env:TEMP ('perusal-' + [guid]::NewGuid() + '.index')",
		"$env:GIT_INDEX_FILE = $index",
		"$env:GIT_AUTHOR_NAME = 'perusal'",
		"$env:GIT_AUTHOR_EMAIL = 'perusal@localhost'",
		"$env:GIT_AUTHOR_DATE = '@0 +0000'",
		"$env:GIT_COMMITTER_NAME = 'perusal'",
		"$env:GIT_COMMITTER_EMAIL = 'perusal@localhost'",
		"$env:GIT_COMMITTER_DATE = '@0 +0000'",
		"function Invoke-Git { $output = git -C " + agenthost.QuotedString(dir) + " @args; if ($LASTEXITCODE -ne 0) " +
			"{ throw ('git ' + ($args -join ' ') + ' exited with ' + $LASTEXITCODE) }; $output }",
		"try { $null = Invoke-Git read-tree HEAD; $null = Invoke-Git add --all; $tree = Invoke-Git write-tree; " +
			"$commit = Invoke-Git commit-tree $tree -p HEAD -m 'perusal snapshot of the agent work tree'; " +
			"$null = Invoke-Git update-ref refs/perusal/snapshot $commit } " +
			"finally { if (Test-Path -LiteralPath $index) { Remove-Item -LiteralPath $index } }",
	}, "; ")
}

// MirrorAgentWorkTree fetches refs/perusal/snapshot from remote, a git URL of the agent's copy of r, and moves r's work
// tree from the snapshot it last mirrored, or HEAD the first time, to the fetched one. r's index stays as it is, so the
// agent's edits show as unstaged changes. It refuses a snapshot taken on another commit than r's HEAD, and git refuses
// to overwrite a file that differs from the last mirrored snapshot, as one the owner edited does.
func (r Repository) MirrorAgentWorkTree(ctx context.Context, remote string) (mirrorErr error) {
	// fsck rejects a tree naming .git or another path git would refuse to check out.
	if err := r.run(ctx, "-c", "transfer.fsckObjects=true", "fetch", "--no-tags", "--no-write-fetch-head",
		"--no-recurse-submodules", remote, "+refs/perusal/snapshot:refs/perusal/agent-fetched"); err != nil {
		return err
	}
	snapshot, err := r.revParse(ctx, "refs/perusal/agent-fetched")
	if err != nil {
		return err
	}
	snapshotBase, err := r.revParse(ctx, snapshot+"^")
	if err != nil {
		return err
	}
	head, err := r.revParse(ctx, "HEAD")
	if err != nil {
		return err
	}
	if snapshotBase != head {
		return fmt.Errorf("%w: the snapshot from %s holds the agent's edits on %s, but %s is at %s",
			ErrAgentSnapshotRefused, remote, snapshotBase, r.Dir, head)
	}
	hasMirrored, err := r.gitSucceeds(ctx, "rev-parse", "--verify", "--quiet", "refs/perusal/agent^{commit}")
	if err != nil {
		return err
	}
	var mirrored string
	if hasMirrored {
		mirrored, err = r.revParse(ctx, "refs/perusal/agent")
		if err != nil {
			return err
		}
	} else {
		mirrored = head
	}
	if mirrored == snapshot {
		return nil
	}
	indexDir, err := os.MkdirTemp("", "perusal-mirror-")
	if err != nil {
		return fmt.Errorf("create a directory for the temporary index of %s: %w", r.Dir, err)
	}
	defer func() {
		if err := os.RemoveAll(indexDir); err != nil {
			mirrorErr = errors.Join(mirrorErr, fmt.Errorf("remove the temporary index of %s: %w", r.Dir, err))
		}
	}()
	temporaryIndex := []string{"GIT_INDEX_FILE=" + filepath.Join(indexDir, "index")}
	// The refresh records which files still match mirrored, and -q keeps going past the ones that do not, so read-tree
	// refuses only when one of those is a file it changes.
	for _, args := range [][]string{
		{"read-tree", mirrored},
		{"update-index", "-q", "--refresh"},
		{"read-tree", "-m", "-u", mirrored, snapshot},
	} {
		if _, err := runGit(ctx, r, args, "", temporaryIndex); err != nil {
			return err
		}
	}
	return r.run(ctx, "update-ref", "refs/perusal/agent", snapshot)
}
