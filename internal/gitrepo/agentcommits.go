package gitrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/CLBRITTON2/perusal/internal/agenthost"
)

// A SHA-1 or SHA-256 object name, as git prints it in full.
var objectName = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// maximumTakenBytes caps the file content one take writes into the owner's work tree. A fetch of a few megabytes can
// hold gigabytes of repeated bytes, since git compresses them.
const maximumTakenBytes = 100 << 20

// ErrTakenCommitsTooLarge is a take refused because the files it writes hold over maximumTakenBytes.
var ErrTakenCommitsTooLarge = errors.New("the agent's commits write too much into the work tree")

// AgentHead is the commit the agent's copy has checked out and the ref HEAD points to there (empty when detached), or
// the error that stopped reading them.
type AgentHead struct {
	Commit string
	Branch string
	Err    error
}

// The JSON agentHeadsScript prints for each copy.
type agentHeadOutput struct {
	Dir    string `json:"dir"`
	Head   string `json:"head"`
	Branch string `json:"branch"`
	Error  string `json:"error"`
}

// ReadAgentHeads reads the HEAD of each of dirs, the agent's copies, in one script run. One copy's failure goes in its
// AgentHead without stopping the others.
func ReadAgentHeads(ctx context.Context, run func(ctx context.Context, script string) ([]byte, error),
	dirs []string) ([]AgentHead, error) {
	if len(dirs) == 0 {
		return []AgentHead{}, nil
	}
	output, err := run(ctx, agentHeadsScript(dirs))
	if err != nil {
		return nil, fmt.Errorf("read the agent's HEADs: %w", err)
	}
	var outputs []agentHeadOutput
	if err := json.Unmarshal(output, &outputs); err != nil {
		return nil, fmt.Errorf("read the agent's HEADs: read %q: %w", output, err)
	}
	if len(outputs) != len(dirs) {
		return nil, fmt.Errorf("read the agent's HEADs: got %d for %d copies: %q", len(outputs), len(dirs), output)
	}
	heads := make([]AgentHead, len(dirs))
	for index, dir := range dirs {
		headOutput := outputs[index]
		switch {
		case headOutput.Dir != dir:
			return nil, fmt.Errorf("read the agent's HEADs: got %s in place of %s", headOutput.Dir, dir)
		case headOutput.Error != "":
			heads[index] = AgentHead{Err: fmt.Errorf("read the agent's HEAD in %s: %s", dir, headOutput.Error)}
		case !objectName.MatchString(headOutput.Head):
			heads[index] = AgentHead{Err: fmt.Errorf("read the agent's HEAD in %s: want a commit, got %q", dir,
				headOutput.Head)}
		case headOutput.Branch != "" && !strings.HasPrefix(headOutput.Branch, "refs/heads/"):
			heads[index] = AgentHead{Err: fmt.Errorf("read the agent's HEAD in %s: want a branch under refs/heads, got %q",
				dir, headOutput.Branch)}
		default:
			heads[index] = AgentHead{Commit: headOutput.Head, Branch: headOutput.Branch, Err: nil}
		}
	}
	return heads, nil
}

func agentHeadsScript(dirs []string) string {
	quotedDirs := make([]string, len(dirs))
	for index, dir := range dirs {
		quotedDirs[index] = agenthost.QuotedString(dir)
	}
	// Exit codes are checked by hand: with $ErrorActionPreference Stop, PowerShell 5.1 turns git's warnings on a
	// redirected stderr into errors.
	return strings.Join([]string{
		"function Invoke-Git { $output = git -C $repoDir @args; if ($LASTEXITCODE -ne 0) " +
			"{ throw ('git ' + ($args -join ' ') + ' exited with ' + $LASTEXITCODE) }; $output }",
		"$heads = @(foreach ($repoDir in @(" + strings.Join(quotedDirs, ", ") + ")) { try { " +
			"$head = Invoke-Git rev-parse HEAD; $branch = [string](git -C $repoDir symbolic-ref --quiet HEAD); " +
			"[pscustomobject]@{ dir = $repoDir; head = $head; branch = $branch; error = '' } " +
			"} catch { [pscustomobject]@{ dir = $repoDir; head = ''; branch = ''; error = $_.Exception.Message } } })",
		"ConvertTo-Json -InputObject $heads -Compress",
	}, "; ")
}

// agentTakenRef holds the agent's commit on branch (empty when detached) whose changes r's work tree last took.
func agentTakenRef(branch string) string {
	if branch == "" {
		return "refs/perusal/taken/detached"
	}
	return "refs/perusal/taken/" + strings.TrimPrefix(branch, "refs/")
}

// TakeAgentCommits brings the agent's commits on head's branch into r's work tree as unstaged changes, leaving r's
// index and HEAD as they are. It fetches head's branch from remote, a git URL of the agent's copy, and moves r's work
// tree from the agent's commit it last took on that branch, or the merge base with r's HEAD the first time, to the
// fetched one.
// It reports whether the work tree changed. git refuses the whole move when a file it changes differs from the commit
// it moves from, as one the owner edited does, and the next call retries it.
func (r Repository) TakeAgentCommits(ctx context.Context, remote string, head AgentHead) (bool, error) {
	if head.Err != nil {
		return false, head.Err
	}
	// The agent writes its own HEAD, so its branch goes into a refspec and a ref name only once git accepts it.
	if head.Branch != "" {
		if err := r.run(ctx, "check-ref-format", head.Branch); err != nil {
			return false, fmt.Errorf("take the agent's commits into %s: its branch %q: %w", r.Dir, head.Branch, err)
		}
	}
	takenRef := agentTakenRef(head.Branch)
	lastTaken, err := r.optionalRevParse(ctx, takenRef)
	if err != nil || lastTaken == head.Commit {
		return false, err
	}
	commit := head.Commit
	hasCommit, err := r.gitSucceeds(ctx, "rev-parse", "--verify", "--quiet", commit+"^{commit}")
	if err != nil {
		return false, err
	}
	if !hasCommit {
		if commit, err = r.fetchAgentHead(ctx, remote, head.Branch); err != nil {
			return false, err
		}
	}
	from := lastTaken
	if from == "" {
		output, err := r.git(ctx, "merge-base", "HEAD", commit)
		if err != nil {
			return false, fmt.Errorf("find where the agent's %s branched from %s: %w", headName(head.Branch), r.Dir, err)
		}
		from = strings.TrimSpace(string(output))
	}
	diffOutput, err := r.git(ctx, "diff-tree", "-r", "-z", "--no-renames", from, commit)
	if err != nil {
		return false, err
	}
	// Without renames each change is two fields: ":<old mode> <new mode> <old object> <new object> <status>", then
	// its path.
	diffFields := strings.Split(strings.TrimSuffix(string(diffOutput), "\x00"), "\x00")
	var writtenBlobs strings.Builder
	for index := 0; index+1 < len(diffFields); index += 2 {
		change := strings.Fields(diffFields[index])
		if len(change) != 5 {
			return false, fmt.Errorf("size the agent's commit %s in %s: read diff-tree change %q", commit[:7], r.Dir,
				diffFields[index])
		}
		// A deletion writes nothing, and a submodule's object is a commit in another repository.
		if change[4] != "D" && change[1] != "160000" {
			writtenBlobs.WriteString(change[3] + "\n")
		}
	}
	sizesOutput, err := runGit(ctx, r, []string{"cat-file", "--batch-check=%(objectsize)"}, writtenBlobs.String(),
		[]string{})
	if err != nil {
		return false, err
	}
	var writtenBytes int64
	for _, sizeText := range strings.Fields(string(sizesOutput)) {
		size, err := strconv.ParseInt(sizeText, 10, 64)
		if err != nil {
			return false, fmt.Errorf("size the agent's commit %s in %s: read cat-file size %q: %w", commit[:7], r.Dir,
				sizesOutput, err)
		}
		writtenBytes += size
	}
	if writtenBytes > maximumTakenBytes {
		return false, fmt.Errorf("take the agent's commit %s on %s into %s: %w: %d bytes, over %d", commit[:7],
			headName(head.Branch), r.Dir, ErrTakenCommitsTooLarge, writtenBytes, maximumTakenBytes)
	}
	if err := r.moveWorkTree(ctx, from, commit); err != nil {
		return false, fmt.Errorf("take the agent's commit %s on %s into %s: %w", commit[:7], headName(head.Branch),
			r.Dir, err)
	}
	if err := r.run(ctx, "update-ref", takenRef, commit); err != nil {
		return false, err
	}
	return from != commit, nil
}

// fetchAgentHead fetches branch, or HEAD when it is empty, from remote, a git URL of the agent's copy, and returns the
// fetched commit. branch must have passed check-ref-format.
func (r Repository) fetchAgentHead(ctx context.Context, remote string, branch string) (string, error) {
	source := branch
	if source == "" {
		source = "HEAD"
	}
	// fsck rejects a tree naming .git or another path git would refuse to check out.
	if err := r.run(ctx, "-c", "transfer.fsckObjects=true", "fetch", "--no-tags", "--no-write-fetch-head",
		"--no-recurse-submodules", remote, "+"+source+":refs/perusal/agent-head"); err != nil {
		return "", err
	}
	// The agent may have committed again since its HEAD was read.
	return r.revParse(ctx, "refs/perusal/agent-head")
}

// AddWorkTreeFromAgent adds a work tree of r at dir on a new branch named as head's, the agent's copy on remote, a git
// URL. The branch starts where the agent's branched from origin's default branch, a commit the owner already has from
// origin, and TakeAgentCommits then brings in the agent's commits. It returns that commit.
func (r Repository) AddWorkTreeFromAgent(ctx context.Context, remote string, head AgentHead, dir string) (string,
	error) {
	if head.Err != nil {
		return "", head.Err
	}
	if head.Branch == "" {
		return "", fmt.Errorf("add a work tree at %s: the agent's copy %s has a detached HEAD, so there is no branch "+
			"to name", dir, remote)
	}
	if err := r.run(ctx, "check-ref-format", head.Branch); err != nil {
		return "", fmt.Errorf("add a work tree at %s: the agent's branch %q: %w", dir, head.Branch, err)
	}
	if err := r.run(ctx, "fetch", "--prune", "origin"); err != nil {
		return "", err
	}
	defaultBranch, err := r.DefaultBranch(ctx)
	if err != nil {
		return "", err
	}
	if defaultBranch.Ref == "" {
		return "", fmt.Errorf("add a work tree at %s: origin's default branch is unset in %s, git remote set-head "+
			"origin --auto sets it", dir, r.Dir)
	}
	commit, err := r.fetchAgentHead(ctx, remote, head.Branch)
	if err != nil {
		return "", err
	}
	output, err := r.git(ctx, "merge-base", commit, defaultBranch.Ref)
	if err != nil {
		return "", fmt.Errorf("find where the agent's %s branched from %s in %s: %w", headName(head.Branch),
			defaultBranch.Ref, r.Dir, err)
	}
	base := strings.TrimSpace(string(output))
	if err := r.run(ctx, "worktree", "add", "-b", strings.TrimPrefix(head.Branch, "refs/heads/"), dir,
		base); err != nil {
		return "", err
	}
	return base, nil
}

// AgentWorkTree is the agent's copy of a work tree at Dir. Uncommitted and Head are read only when it exists.
type AgentWorkTree struct {
	Dir         string
	Exists      bool
	Uncommitted bool
	Head        AgentHead
}

func ReadAgentWorkTree(ctx context.Context, run func(ctx context.Context, script string) ([]byte, error),
	dir string) (AgentWorkTree, error) {
	quotedDir := agenthost.QuotedString(dir)
	output, err := run(ctx, "if (Test-Path -LiteralPath "+quotedDir+" -PathType Container) { "+
		"$changes = git --no-optional-locks -C "+quotedDir+" status --porcelain; "+
		"if ($LASTEXITCODE -ne 0) { throw ('git status exited with ' + $LASTEXITCODE) }; "+
		"if ($changes) { 'uncommitted' } else { 'clean' } } else { 'missing' }")
	if err != nil {
		return AgentWorkTree{}, fmt.Errorf("read the agent's work tree %s: %w", dir, err)
	}
	agentWorkTree := AgentWorkTree{Dir: dir, Exists: true, Uncommitted: false, Head: AgentHead{}}
	switch strings.TrimSpace(string(output)) {
	case "missing":
		agentWorkTree.Exists = false
		return agentWorkTree, nil
	case "uncommitted":
		agentWorkTree.Uncommitted = true
	case "clean":
	default:
		return AgentWorkTree{}, fmt.Errorf("read the agent's work tree %s: want missing, uncommitted or clean, got %q",
			dir, output)
	}
	heads, err := ReadAgentHeads(ctx, run, []string{dir})
	if err != nil {
		return AgentWorkTree{}, err
	}
	if heads[0].Err != nil {
		return AgentWorkTree{}, heads[0].Err
	}
	agentWorkTree.Head = heads[0]
	return agentWorkTree, nil
}

// HasTaken reports whether r holds head's commit: it is the commit the last take on its branch took, or it is in r's
// branch of the same name.
func (r Repository) HasTaken(ctx context.Context, head AgentHead) (bool, error) {
	lastTaken, err := r.optionalRevParse(ctx, agentTakenRef(head.Branch))
	if err != nil {
		return false, err
	}
	if lastTaken == head.Commit {
		return true, nil
	}
	hasCommit, err := r.gitSucceeds(ctx, "rev-parse", "--verify", "--quiet", head.Commit+"^{commit}")
	if err != nil || !hasCommit {
		return false, err
	}
	return r.IsAncestor(ctx, head.Commit, head.Branch)
}

// RemoveAgentWorkTree removes agentWorkTree from the agent's clone at clone and deletes its branch there. It refuses
// when the copy's HEAD moved since agentWorkTree was read, since the agent may have committed what was not taken.
func RemoveAgentWorkTree(ctx context.Context, run func(ctx context.Context, script string) ([]byte, error),
	clone string, agentWorkTree AgentWorkTree) error {
	quotedClone := agenthost.QuotedString(clone)
	quotedWorkTree := agenthost.QuotedString(agentWorkTree.Dir)
	quotedBranch := agenthost.QuotedString(strings.TrimPrefix(agentWorkTree.Head.Branch, "refs/heads/"))
	// Exit codes are checked by hand, as in agentHeadsScript. git's stderr reaches the error through the shell.
	script := strings.Join([]string{
		"$head = git -C " + quotedWorkTree + " rev-parse HEAD",
		"if ($head -ne " + agenthost.QuotedString(agentWorkTree.Head.Commit) + ") { throw ('HEAD moved to ' + $head) }",
		"git -C " + quotedClone + " worktree remove " + quotedWorkTree,
		"if ($LASTEXITCODE -ne 0) { throw ('git worktree remove exited with ' + $LASTEXITCODE) }",
		"git -C " + quotedClone + " branch --delete --force " + quotedBranch,
		"if ($LASTEXITCODE -ne 0) { throw ('git branch --delete exited with ' + $LASTEXITCODE) }",
	}, "; ")
	if _, err := run(ctx, script); err != nil {
		return fmt.Errorf("remove the agent's work tree %s from %s: %w", agentWorkTree.Dir, clone, err)
	}
	return nil
}

func headName(branch string) string {
	if branch == "" {
		return "detached HEAD"
	}
	return strings.TrimPrefix(branch, "refs/heads/")
}

// moveWorkTree moves r's work tree from commit from to commit to, leaving r's index as it is. A temporary index holds
// r's work tree, so read-tree updates a file that still matches from, keeps one that already matches to, as after the
// owner commits or pulls, and refuses one that matches neither.
func (r Repository) moveWorkTree(ctx context.Context, from string, to string) (moveErr error) {
	indexDir, err := os.MkdirTemp("", "perusal-take-")
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

func (r Repository) TakenCommit(ctx context.Context, branch string) (string, error) {
	return r.optionalRevParse(ctx, agentTakenRef("refs/heads/"+branch))
}

// optionalRevParse is the commit ref names, or empty when ref does not exist.
func (r Repository) optionalRevParse(ctx context.Context, ref string) (string, error) {
	hasRef, err := r.gitSucceeds(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil || !hasRef {
		return "", err
	}
	return r.revParse(ctx, ref)
}
