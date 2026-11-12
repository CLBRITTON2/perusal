package gitrepo

import (
	"context"
	"encoding/json"
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

// AgentCopy is the agent's clone of a repository. Run runs a PowerShell script where the clone lives, as
// agenthost.Session.Run does over ssh, and Remote is the clone's git URL as seen from the owner's copy.
type AgentCopy struct {
	Dir    string
	Remote string
	Run    func(ctx context.Context, script string) ([]byte, error)
}

// AgentSnapshot is what snapshotting one agent copy found: its HEAD, its branch (empty when detached) and the
// refs/perusal/snapshot commit, or the error that stopped it.
type AgentSnapshot struct {
	Head   string
	Branch string
	Commit string
	Err    error
}

// The JSON agentSnapshotScript prints for each clone.
type agentSnapshotOutput struct {
	Dir    string `json:"dir"`
	Head   string `json:"head"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	Error  string `json:"error"`
}

// SnapshotAgentCopies commits the work tree of each of dirs, the agent's clones, to refs/perusal/snapshot there, with
// untracked files and without ignored ones. It stages through a temporary index, so the agent's own index stays as it
// is. All clones go in one script run, and one clone's failure goes in its AgentSnapshot
// without stopping the others.
func SnapshotAgentCopies(ctx context.Context, run func(ctx context.Context, script string) ([]byte, error),
	dirs []string) ([]AgentSnapshot, error) {
	if len(dirs) == 0 {
		return []AgentSnapshot{}, nil
	}
	output, err := run(ctx, agentSnapshotScript(dirs))
	if err != nil {
		return nil, fmt.Errorf("snapshot the agent's work trees: %w", err)
	}
	var outputs []agentSnapshotOutput
	if err := json.Unmarshal(output, &outputs); err != nil {
		return nil, fmt.Errorf("snapshot the agent's work trees: read %q: %w", output, err)
	}
	if len(outputs) != len(dirs) {
		return nil, fmt.Errorf("snapshot the agent's work trees: got %d snapshots for %d clones: %q", len(outputs),
			len(dirs), output)
	}
	snapshots := make([]AgentSnapshot, len(dirs))
	for index, dir := range dirs {
		snapshotOutput := outputs[index]
		switch {
		case snapshotOutput.Dir != dir:
			return nil, fmt.Errorf("snapshot the agent's work trees: got %s in place of %s", snapshotOutput.Dir, dir)
		case snapshotOutput.Error != "":
			snapshots[index] = AgentSnapshot{Err: fmt.Errorf("snapshot the agent's work tree of %s: %s", dir,
				snapshotOutput.Error)}
		case !objectName.MatchString(snapshotOutput.Head) || !objectName.MatchString(snapshotOutput.Commit):
			snapshots[index] = AgentSnapshot{Err: fmt.Errorf(
				"snapshot the agent's work tree of %s: want its HEAD and snapshot commit, got %q and %q", dir,
				snapshotOutput.Head, snapshotOutput.Commit)}
		default:
			snapshots[index] = AgentSnapshot{Head: snapshotOutput.Head, Branch: snapshotOutput.Branch,
				Commit: snapshotOutput.Commit, Err: nil}
		}
	}
	return snapshots, nil
}

// SyncAgentCopy returns the snapshot commit to mirror. When the agent's HEAD or branch in snapshot differs from r's,
// as after the owner commits, pulls or switches branches, it pushes r's HEAD to refs/perusal/owner in agent and moves
// agent's work tree, HEAD, branch and index onto it, then snapshots again. On the same branch the agent's uncommitted
// edits come along, merged in (r's side wins where the two conflict). On a branch switch they stay behind in
// agentEditsRef of the old branch, and the edits saved for the new branch, if any, come back merged onto it, so no edit
// crosses branches. It refuses to move agent off a commit r lacks, such as one the agent made, so no agent commit is
// dropped.
func (r Repository) SyncAgentCopy(ctx context.Context, agent AgentCopy, snapshot AgentSnapshot) (string, error) {
	if snapshot.Err != nil {
		return "", snapshot.Err
	}
	head, err := r.revParse(ctx, "HEAD")
	if err != nil {
		return "", err
	}
	branch, err := r.headBranch(ctx)
	if err != nil {
		return "", err
	}
	if snapshot.Head == head && snapshot.Branch == branch {
		return snapshot.Commit, nil
	}
	hasAgentHead, err := r.gitSucceeds(ctx, "rev-parse", "--verify", "--quiet", snapshot.Head+"^{commit}")
	if err != nil {
		return "", err
	}
	if !hasAgentHead {
		return "", fmt.Errorf("%w: the agent's copy of %s is on %s", ErrAgentHeadUnknown, r.Dir, snapshot.Head)
	}
	if err := r.run(ctx, "push", "--quiet", "--no-verify", agent.Remote, "+"+head+":refs/perusal/owner"); err != nil {
		return "", err
	}
	output, err := agent.Run(ctx, agentMoveScript(agent.Dir, snapshot.Head, snapshot.Branch, head, branch))
	if err != nil {
		return "", fmt.Errorf("move the agent's copy of %s onto %s: %w", r.Dir, head, err)
	}
	moved := strings.TrimSpace(string(output))
	if !objectName.MatchString(moved) {
		return "", fmt.Errorf("move the agent's copy of %s onto %s: want the new snapshot commit, got %q", r.Dir, head,
			output)
	}
	return moved, nil
}

// HeadAndBranch is HEAD's commit and the ref HEAD points to (HEAD itself when detached), from one git run, so a
// caller can tell cheaply whether either moved.
func (r Repository) HeadAndBranch(ctx context.Context) (string, error) {
	output, err := r.git(ctx, "rev-parse", "HEAD", "--symbolic-full-name", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
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

// agentSnapshotScript prints a JSON array of agentSnapshotOutput, one per dir in order.
func agentSnapshotScript(dirs []string) string {
	quotedDirs := make([]string, len(dirs))
	for index, dir := range dirs {
		quotedDirs[index] = agenthost.QuotedString(dir)
	}
	return strings.Join([]string{
		agentScriptPrelude(),
		"$snapshots = @(foreach ($repoDir in @(" + strings.Join(quotedDirs, ", ") + ")) { try { " +
			"$commit = Save-Snapshot; $head = Invoke-Git rev-parse HEAD; " +
			"$branch = [string](git -C $repoDir symbolic-ref --quiet HEAD); " +
			"[pscustomobject]@{ dir = $repoDir; head = $head; branch = $branch; commit = $commit; error = '' } " +
			"} catch { [pscustomobject]@{ dir = $repoDir; head = ''; branch = ''; commit = ''; " +
			"error = $_.Exception.Message } } })",
		"ConvertTo-Json -InputObject $snapshots -Compress",
	}, "; ")
}

// agentEditsRef holds the agent's uncommitted edits on branch (empty when detached) while another branch is checked
// out, as a snapshot commit whose parent is the commit they were made on.
func agentEditsRef(branch string) string {
	if branch == "" {
		return "refs/perusal/edits/detached"
	}
	return "refs/perusal/edits/" + strings.TrimPrefix(branch, "refs/")
}

// agentMoveScript checks that the agent's HEAD is still from, so a commit made since the snapshot is not dropped. The
// move starts from a fresh snapshot, and the refresh of the temporary index lets read-tree refuse a file the agent wrote
// since then. It prints the snapshot commit taken after the move.
func agentMoveScript(dir string, from string, fromBranch string, to string, branch string) string {
	var moveHead string
	if branch == "" {
		moveHead = "$null = Invoke-Git update-ref --no-deref HEAD " + to
	} else {
		moveHead = "$null = Invoke-Git update-ref " + agenthost.QuotedString(branch) + " " + to + "; " +
			"$null = Invoke-Git symbolic-ref HEAD " + agenthost.QuotedString(branch)
	}
	steps := []string{
		agentScriptPrelude(),
		"$repoDir = " + agenthost.QuotedString(dir),
		"$head = Invoke-Git rev-parse HEAD",
		"if ($head -ne '" + from + "') { throw ('HEAD moved to ' + $head + ' since the snapshot') }",
		"$snapshot = Save-Snapshot",
	}
	leftRef := agenthost.QuotedString(agentEditsRef(fromBranch))
	savedRef := agenthost.QuotedString(agentEditsRef(branch))
	if fromBranch == branch {
		steps = append(steps, "$target = @(Invoke-Git merge-tree --write-tree --merge-base "+from+" -X theirs $snapshot "+
			to+")[0]")
	} else {
		steps = append(steps,
			// An edit-free snapshot drops the ref, so a stale set of edits never comes back.
			"if ((Invoke-Git rev-parse ($snapshot + '^{tree}')) -ne (Invoke-Git rev-parse '"+from+"^{tree}')) "+
				"{ $null = Invoke-Git update-ref "+leftRef+" $snapshot } "+
				"else { $null = Invoke-Git update-ref -d "+leftRef+" }",
			"$saved = [string](git -C $repoDir rev-parse --verify --quiet ("+savedRef+" + '^{commit}'))",
			"if ($saved) { $target = @(Invoke-Git merge-tree --write-tree --merge-base ($saved + '^') -X theirs $saved "+
				to+")[0] } else { $target = Invoke-Git rev-parse '"+to+"^{tree}' }")
	}
	steps = append(steps,
		"Invoke-WithTemporaryIndex { $null = Invoke-Git read-tree $snapshot; "+
			"$null = Invoke-Git update-index -q --refresh; $null = Invoke-Git read-tree -m -u $snapshot $target }",
		moveHead,
		"$null = Invoke-Git reset --quiet")
	if fromBranch != branch {
		steps = append(steps, "if ($saved) { $null = Invoke-Git update-ref -d "+savedRef+" }")
	}
	return strings.Join(append(steps, "Save-Snapshot"), "; ")
}

// Exit codes are checked by hand: with $ErrorActionPreference Stop, PowerShell 5.1 turns git's warnings on a
// redirected stderr into errors. Invoke-Git runs in the clone the caller's $repoDir names. The fixed dates make an
// unchanged work tree give the same commit, so the mirror after it skips the fetch and changes nothing.
func agentScriptPrelude() string {
	return strings.Join([]string{
		"$env:GIT_AUTHOR_NAME = 'perusal'",
		"$env:GIT_AUTHOR_EMAIL = 'perusal@localhost'",
		"$env:GIT_AUTHOR_DATE = '@0 +0000'",
		"$env:GIT_COMMITTER_NAME = 'perusal'",
		"$env:GIT_COMMITTER_EMAIL = 'perusal@localhost'",
		"$env:GIT_COMMITTER_DATE = '@0 +0000'",
		"function Invoke-Git { $output = git -C $repoDir @args; if ($LASTEXITCODE -ne 0) " +
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

// MirrorAgentWorkTree fetches refs/perusal/snapshot from remote, a git URL of the agent's copy of r, unless the last
// fetch already got snapshotCommit, and moves r's work tree from the snapshot it last mirrored, or HEAD the first time
// and after HEAD moved without perusal, to the fetched one. r's index stays as it is, so the agent's edits show as
// unstaged changes. It refuses a snapshot taken on another commit than r's HEAD, and git refuses to overwrite a file
// that differs from both the last mirrored snapshot and the fetched one, as one the owner edited does.
func (r Repository) MirrorAgentWorkTree(ctx context.Context, remote string, snapshotCommit string) error {
	lastFetched, err := r.optionalRevParse(ctx, "refs/perusal/agent-fetched")
	if err != nil {
		return err
	}
	if lastFetched != snapshotCommit {
		// fsck rejects a tree naming .git or another path git would refuse to check out.
		if err := r.run(ctx, "-c", "transfer.fsckObjects=true", "fetch", "--no-tags", "--no-write-fetch-head",
			"--no-recurse-submodules", remote, "+refs/perusal/snapshot:refs/perusal/agent-fetched"); err != nil {
			return err
		}
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
	lastMirrored, err := r.optionalRevParse(ctx, "refs/perusal/agent")
	if err != nil {
		return err
	}
	// A mirror on another commit is stale, left by a switch, commit or pull made outside perusal, which keeps the edits
	// it carried in the work tree as the owner's.
	mirrored := head
	if lastMirrored != "" {
		mirroredBase, err := r.revParse(ctx, lastMirrored+"^")
		if err != nil {
			return err
		}
		if mirroredBase == head {
			mirrored = lastMirrored
		}
	}
	if mirrored == snapshot {
		return nil
	}
	if err := r.moveWorkTree(ctx, mirrored, snapshot); err != nil {
		return err
	}
	return r.run(ctx, "update-ref", "refs/perusal/agent", snapshot)
}

// removeMirroredAgentEdits moves r's work tree from the last mirrored snapshot back to HEAD and forgets that snapshot,
// so a branch switch takes none of the agent's edits along. The agent's copy still holds them and keeps them for this
// branch. A file the owner edited since the mirror stops it. Without a mirror it does nothing.
func (r Repository) removeMirroredAgentEdits(ctx context.Context) error {
	lastMirrored, err := r.optionalRevParse(ctx, "refs/perusal/agent")
	if err != nil || lastMirrored == "" {
		return err
	}
	if err := r.moveWorkTree(ctx, lastMirrored, "HEAD"); err != nil {
		return fmt.Errorf("remove the agent's mirrored edits from %s before the switch: %w", r.Dir, err)
	}
	return r.run(ctx, "update-ref", "-d", "refs/perusal/agent")
}

// moveWorkTree moves r's work tree from commit from to commit to, leaving r's index as it is. A temporary index holds
// r's work tree, so read-tree updates a file that still matches from, keeps one that already matches to, as after the
// owner commits or pulls, and refuses one that matches neither.
func (r Repository) moveWorkTree(ctx context.Context, from string, to string) (moveErr error) {
	indexDir, err := os.MkdirTemp("", "perusal-mirror-")
	if err != nil {
		return fmt.Errorf("create a directory for the temporary index of %s: %w", r.Dir, err)
	}
	defer func() {
		if err := os.RemoveAll(indexDir); err != nil {
			moveErr = errors.Join(moveErr, fmt.Errorf("remove the temporary index of %s: %w", r.Dir, err))
		}
	}()
	temporaryIndex := []string{"GIT_INDEX_FILE=" + filepath.Join(indexDir, "index")}
	for _, args := range [][]string{
		{"read-tree", from},
		{"add", "--all"},
		{"read-tree", "-m", "-u", from, to},
	} {
		if _, err := runGit(ctx, r, args, "", temporaryIndex); err != nil {
			return err
		}
	}
	return nil
}

// optionalRevParse is the commit ref names, or empty when ref does not exist.
func (r Repository) optionalRevParse(ctx context.Context, ref string) (string, error) {
	hasRef, err := r.gitSucceeds(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil || !hasRef {
		return "", err
	}
	return r.revParse(ctx, ref)
}
