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

// A SHA-1 or SHA-256 object name, as git prints it in full.
var objectName = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

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
		source := head.Branch
		if source == "" {
			source = "HEAD"
		}
		// fsck rejects a tree naming .git or another path git would refuse to check out.
		if err := r.run(ctx, "-c", "transfer.fsckObjects=true", "fetch", "--no-tags", "--no-write-fetch-head",
			"--no-recurse-submodules", remote, "+"+source+":refs/perusal/agent-head"); err != nil {
			return false, err
		}
		// The agent may have committed again since its HEAD was read.
		if commit, err = r.revParse(ctx, "refs/perusal/agent-head"); err != nil {
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
	if err := r.moveWorkTree(ctx, from, commit); err != nil {
		return false, fmt.Errorf("take the agent's commit %s on %s into %s: %w", commit[:7], headName(head.Branch),
			r.Dir, err)
	}
	if err := r.run(ctx, "update-ref", takenRef, commit); err != nil {
		return false, err
	}
	return from != commit, nil
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

// optionalRevParse is the commit ref names, or empty when ref does not exist.
func (r Repository) optionalRevParse(ctx context.Context, ref string) (string, error) {
	hasRef, err := r.gitSucceeds(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil || !hasRef {
		return "", err
	}
	return r.revParse(ctx, ref)
}
