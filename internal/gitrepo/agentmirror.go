package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CLBRITTON2/perusal/internal/agenthost"
)

var (
	ErrAgentSnapshotRefused = errors.New("refused the agent's work tree snapshot")
	ErrAgentHeadUnknown     = errors.New("left the agent's copy on a commit missing from yours")
)

// A SHA-1 or SHA-256 object name, as git prints it in full.
var objectName = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// AgentCopy is the agent's clone of a repository. Run runs a Windows PowerShell script where the clone lives, as
// agenthost.Run does over ssh, and Remote is the clone's git URL as seen from the owner's copy.
type AgentCopy struct {
	Dir    string
	Remote string
	Run    func(ctx context.Context, script string) ([]byte, error)
}

// SyncAgentCopy commits agent's work tree to refs/perusal/snapshot there, with untracked files and without ignored ones.
// It stages through a temporary index, so the agent's own index stays as it is. When agent's HEAD or branch differs
// from r's, as after the owner commits, pulls or switches branches, it then pushes r's HEAD to refs/perusal/owner in
// agent, moves agent's work tree onto it with the agent's uncommitted edits merged in (r's side wins where the two
// conflict), moves agent's HEAD and branch there, resets agent's index to it, and snapshots again. It refuses to move
// agent off a commit r lacks, such as one the agent made, so no agent commit is dropped.
func (r Repository) SyncAgentCopy(ctx context.Context, agent AgentCopy) error {
	head, err := r.revParse(ctx, "HEAD")
	if err != nil {
		return err
	}
	branch, err := r.headBranch(ctx)
	if err != nil {
		return err
	}
	output, err := agent.Run(ctx, agentSnapshotScript(agent.Dir))
	if err != nil {
		return fmt.Errorf("snapshot the agent's work tree of %s: %w", r.Dir, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) < 1 || len(fields) > 2 || !objectName.MatchString(fields[0]) {
		return fmt.Errorf("snapshot the agent's work tree of %s: want its HEAD and branch, got %q", r.Dir, output)
	}
	agentHead := fields[0]
	var agentBranch string
	if len(fields) == 2 {
		agentBranch = fields[1]
	} else {
		agentBranch = ""
	}
	if agentHead == head && agentBranch == branch {
		return nil
	}
	hasAgentHead, err := r.gitSucceeds(ctx, "rev-parse", "--verify", "--quiet", agentHead+"^{commit}")
	if err != nil {
		return err
	}
	if !hasAgentHead {
		return fmt.Errorf("%w: the agent's copy of %s is on %s", ErrAgentHeadUnknown, r.Dir, agentHead)
	}
	if err := r.run(ctx, "push", "--quiet", "--no-verify", agent.Remote, "+"+head+":refs/perusal/owner"); err != nil {
		return err
	}
	if _, err := agent.Run(ctx, agentMoveScript(agent.Dir, agentHead, head, branch)); err != nil {
		return fmt.Errorf("move the agent's copy of %s onto %s: %w", r.Dir, head, err)
	}
	return nil
}

// headBranch is the ref HEAD points to, such as refs/heads/main, or empty when HEAD is detached.
func (r Repository) headBranch(ctx context.Context) (string, error) {
	output, err := r.git(ctx, "symbolic-ref", "--quiet", "HEAD")
	switch {
	case isExitCodeOne(err):
		return "", nil
	case err != nil:
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// agentSnapshotScript prints the agent's HEAD and, unless it is detached, its branch.
func agentSnapshotScript(dir string) string {
	return strings.Join([]string{
		agentScriptPrelude(dir),
		"$null = Save-Snapshot",
		"$head = Invoke-Git rev-parse HEAD",
		"$branch = git -C " + agenthost.QuotedString(dir) + " symbolic-ref --quiet HEAD",
		"$head",
		"$branch",
	}, "; ")
}

// agentMoveScript checks that the agent's HEAD is still from, so a commit made since the snapshot is not dropped. The
// merge runs on a fresh snapshot, and the refresh of the temporary index lets read-tree refuse a file the agent wrote
// since then.
func agentMoveScript(dir string, from string, to string, branch string) string {
	var moveHead string
	if branch == "" {
		moveHead = "$null = Invoke-Git update-ref --no-deref HEAD " + to
	} else {
		moveHead = "$null = Invoke-Git update-ref " + agenthost.QuotedString(branch) + " " + to + "; " +
			"$null = Invoke-Git symbolic-ref HEAD " + agenthost.QuotedString(branch)
	}
	return strings.Join([]string{
		agentScriptPrelude(dir),
		"$head = Invoke-Git rev-parse HEAD",
		"if ($head -ne '" + from + "') { throw ('HEAD moved to ' + $head + ' since the snapshot') }",
		"$snapshot = Save-Snapshot",
		"$merged = @(Invoke-Git merge-tree --write-tree --merge-base " + from + " -X theirs $snapshot " + to + ")[0]",
		"Invoke-WithTemporaryIndex { $null = Invoke-Git read-tree $snapshot; " +
			"$null = Invoke-Git update-index -q --refresh; $null = Invoke-Git read-tree -m -u $snapshot $merged }",
		moveHead,
		"$null = Invoke-Git reset --quiet",
		"$null = Save-Snapshot",
	}, "; ")
}

// Exit codes are checked by hand: with $ErrorActionPreference Stop, PowerShell 5.1 turns git's warnings on a
// redirected stderr into errors. The fixed dates make an unchanged work tree give the same commit, so the fetch and
// mirror after it do nothing.
func agentScriptPrelude(dir string) string {
	return strings.Join([]string{
		"$env:GIT_AUTHOR_NAME = 'perusal'",
		"$env:GIT_AUTHOR_EMAIL = 'perusal@localhost'",
		"$env:GIT_AUTHOR_DATE = '@0 +0000'",
		"$env:GIT_COMMITTER_NAME = 'perusal'",
		"$env:GIT_COMMITTER_EMAIL = 'perusal@localhost'",
		"$env:GIT_COMMITTER_DATE = '@0 +0000'",
		"function Invoke-Git { $output = git -C " + agenthost.QuotedString(dir) + " @args; if ($LASTEXITCODE -ne 0) " +
			"{ throw ('git ' + ($args -join ' ') + ' exited with ' + $LASTEXITCODE) }; $output }",
		"function Invoke-WithTemporaryIndex([scriptblock] $steps) { " +
			"$index = Join-Path $env:TEMP ('perusal-' + [guid]::NewGuid() + '.index'); $env:GIT_INDEX_FILE = $index; " +
			"try { & $steps } finally { Remove-Item Env:GIT_INDEX_FILE; " +
			"if (Test-Path -LiteralPath $index) { Remove-Item -LiteralPath $index } } }",
		"function Save-Snapshot { Invoke-WithTemporaryIndex { $null = Invoke-Git read-tree HEAD; " +
			"$null = Invoke-Git add --all; $tree = Invoke-Git write-tree; " +
			"$commit = Invoke-Git commit-tree $tree -p HEAD -m 'perusal snapshot of the agent work tree'; " +
			"$null = Invoke-Git update-ref refs/perusal/snapshot $commit; $commit } }",
	}, "; ")
}

// MirrorAgentWorkTree fetches refs/perusal/snapshot from remote, a git URL of the agent's copy of r, and moves r's work
// tree from the snapshot it last mirrored, or HEAD the first time, to the fetched one. r's index stays as it is, so the
// agent's edits show as unstaged changes. It refuses a snapshot taken on another commit than r's HEAD, and git refuses
// to overwrite a file that differs from both the last mirrored snapshot and the fetched one, as one the owner edited
// does.
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
	// The temporary index holds r's work tree, so read-tree updates a file that still matches mirrored, keeps one that
	// already matches the snapshot, as after the owner commits or pulls, and refuses one that matches neither.
	for _, args := range [][]string{
		{"read-tree", mirrored},
		{"add", "--all"},
		{"read-tree", "-m", "-u", mirrored, snapshot},
	} {
		if _, err := runGit(ctx, r, args, "", temporaryIndex); err != nil {
			return err
		}
	}
	return r.run(ctx, "update-ref", "refs/perusal/agent", snapshot)
}
