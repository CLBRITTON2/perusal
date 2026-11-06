package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMirrorAgentWorkTree(t *testing.T) {
	t.Run("showsTheAgentsEditsAsUnstagedChanges", showsTheAgentsEditsAsUnstagedChanges)
	t.Run("followsTheAgentsNextSnapshot", followsTheAgentsNextSnapshot)
	t.Run("keepsAFileTheOwnerEditedAndRefusesTheSnapshot", keepsAFileTheOwnerEditedAndRefusesTheSnapshot)
	t.Run("refusesASnapshotTakenOnAnotherCommit", refusesASnapshotTakenOnAnotherCommit)
	t.Run("refusesASnapshotNamingAPathInDotGit", refusesASnapshotNamingAPathInDotGit)
}

func TestSyncAgentCopy(t *testing.T) {
	t.Run("cleansWhatTheOwnerCommitted", cleansWhatTheOwnerCommitted)
	t.Run("keepsTheAgentsEditsOnTheOwnersNewCommit", keepsTheAgentsEditsOnTheOwnersNewCommit)
	t.Run("followsTheOwnersBranchSwitch", followsTheOwnersBranchSwitch)
	t.Run("leavesTheAgentOnACommitTheOwnerLacks", leavesTheAgentOnACommitTheOwnerLacks)
}

func showsTheAgentsEditsAsUnstagedChanges(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	writeFile(t, filepath.Join(agent, "new.txt"), "new\n")
	writeFile(t, filepath.Join(agent, "build.log"), "ignored\n")
	removeFile(t, filepath.Join(agent, "b.txt"))
	runSteps(t, agent, [][]string{{"add", "a.txt"}})

	mirrorErr := syncAndMirror(t, owner, agent)

	requireMirrored(t, mirrorErr)
	assertChangedFiles(t, owner, []FileStatus{
		{Path: "a.txt", IndexStatus: ' ', WorktreeStatus: 'M'},
		{Path: "b.txt", IndexStatus: ' ', WorktreeStatus: 'D'},
		{Path: "new.txt", IndexStatus: '?', WorktreeStatus: '?'},
	})
	assertFileHolds(t, filepath.Join(owner, "a.txt"), "agent\n")
	assertFileHolds(t, filepath.Join(owner, "new.txt"), "new\n")
	if _, err := os.Lstat(filepath.Join(owner, "build.log")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v for the agent's ignored build.log in the owner's copy, want it missing", err)
	}
}

func followsTheAgentsNextSnapshot(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	writeFile(t, filepath.Join(agent, "new.txt"), "new\n")
	removeFile(t, filepath.Join(agent, "b.txt"))
	if err := syncAndMirror(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	removeFile(t, filepath.Join(agent, "new.txt"))
	runSteps(t, agent, [][]string{{"restore", "b.txt"}})

	mirrorErr := syncAndMirror(t, owner, agent)

	requireMirrored(t, mirrorErr)
	assertChangedFiles(t, owner, []FileStatus{{Path: "a.txt", IndexStatus: ' ', WorktreeStatus: 'M'}})
}

func keepsAFileTheOwnerEditedAndRefusesTheSnapshot(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	if err := syncAndMirror(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(owner, "a.txt"), "owner\n")
	writeFile(t, filepath.Join(agent, "a.txt"), "agent again\n")

	mirrorErr := syncAndMirror(t, owner, agent)

	if mirrorErr == nil || !strings.Contains(mirrorErr.Error(), "a.txt") {
		t.Fatalf("got error %v, want git's refusal to overwrite a.txt", mirrorErr)
	}
	assertFileHolds(t, filepath.Join(owner, "a.txt"), "owner\n")
}

func refusesASnapshotTakenOnAnotherCommit(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	runSteps(t, agent, [][]string{{"commit", "--all", "--message", "Agent commit"}})
	writeFile(t, filepath.Join(agent, "b.txt"), "agent\n")
	if output, err := localAgentCopy(agent).Run(context.Background(), agentSnapshotScript(agent)); err != nil {
		t.Fatalf("snapshot %s: %v: %s", agent, err, output)
	}

	mirrorErr := Repository{Dir: owner}.MirrorAgentWorkTree(context.Background(), agent)

	if !errors.Is(mirrorErr, ErrAgentSnapshotRefused) {
		t.Fatalf("got error %v, want %v", mirrorErr, ErrAgentSnapshotRefused)
	}
	assertChangedFiles(t, owner, []FileStatus{})
}

func refusesASnapshotNamingAPathInDotGit(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	blob := gitOutputFromInput(t, agent, "[core]\n\thooksPath = planted\n", "hash-object", "-w", "--stdin")
	dotGit := gitOutputFromInput(t, agent, "100644 blob "+blob+"\tconfig\n", "mktree")
	tree := gitOutputFromInput(t, agent, "040000 tree "+dotGit+"\t.git\n", "mktree")
	commit := strings.TrimSpace(gitOutput(t, agent, "commit-tree", tree, "-p", "HEAD", "-m", "Plant .git/config"))
	runSteps(t, agent, [][]string{{"update-ref", "refs/perusal/snapshot", commit}})

	mirrorErr := Repository{Dir: owner}.MirrorAgentWorkTree(context.Background(), agent)

	if mirrorErr == nil || !strings.Contains(mirrorErr.Error(), "hasDotgit") {
		t.Fatalf("got error %v, want fsck's hasDotgit refusal", mirrorErr)
	}
	assertChangedFiles(t, owner, []FileStatus{})
}

// The owner's new commit is missing from the agent's copy, so this also covers the push.
func cleansWhatTheOwnerCommitted(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	writeFile(t, filepath.Join(agent, "new.txt"), "new\n")
	if err := syncAndMirror(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	runSteps(t, owner, [][]string{{"add", "a.txt"}, {"commit", "--message", "Take the agent's a.txt"}})

	mirrorErr := syncAndMirror(t, owner, agent)

	untracked := []FileStatus{{Path: "new.txt", IndexStatus: '?', WorktreeStatus: '?'}}
	requireMirrored(t, mirrorErr)
	assertChangedFiles(t, owner, untracked)
	assertChangedFiles(t, agent, untracked)
	assertSameHead(t, owner, agent)
}

func keepsTheAgentsEditsOnTheOwnersNewCommit(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	if err := syncAndMirror(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(owner, "b.txt"), "owner\n")
	runSteps(t, owner, [][]string{{"add", "b.txt"}, {"commit", "--message", "Owner commit"}})

	mirrorErr := syncAndMirror(t, owner, agent)

	edited := []FileStatus{{Path: "a.txt", IndexStatus: ' ', WorktreeStatus: 'M'}}
	requireMirrored(t, mirrorErr)
	assertChangedFiles(t, owner, edited)
	assertChangedFiles(t, agent, edited)
	assertFileHolds(t, filepath.Join(agent, "a.txt"), "agent\n")
	assertFileHolds(t, filepath.Join(agent, "b.txt"), "owner\n")
	assertSameHead(t, owner, agent)
}

func followsTheOwnersBranchSwitch(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(owner, "side.txt"), "side\n")
	runSteps(t, owner, [][]string{
		{"switch", "--create", "side"},
		{"add", "side.txt"},
		{"commit", "--message", "Side commit"},
	})

	mirrorErr := syncAndMirror(t, owner, agent)

	requireMirrored(t, mirrorErr)
	assertChangedFiles(t, agent, []FileStatus{})
	assertFileHolds(t, filepath.Join(agent, "side.txt"), "side\n")
	assertSameHead(t, owner, agent)
}

func leavesTheAgentOnACommitTheOwnerLacks(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	runSteps(t, agent, [][]string{{"commit", "--all", "--message", "Agent commit"}})
	agentCommit := revParse(t, agent, "HEAD")

	mirrorErr := syncAndMirror(t, owner, agent)

	if !errors.Is(mirrorErr, ErrAgentHeadUnknown) {
		t.Fatalf("got error %v, want %v", mirrorErr, ErrAgentHeadUnknown)
	}
	if head := revParse(t, agent, "HEAD"); head != agentCommit {
		t.Fatalf("got the agent's HEAD at %s, want it left at %s", head, agentCommit)
	}
	assertChangedFiles(t, owner, []FileStatus{})
}

// The agent's copy is a clone, as the VM's repositories are clones of the owner's from GitHub.
func ownerAndAgentCopies(t *testing.T) (string, string) {
	t.Helper()
	useTestIdentity(t)
	owner := t.TempDir()
	agent := filepath.Join(t.TempDir(), "agent")
	writeFile(t, filepath.Join(owner, "a.txt"), "a\n")
	writeFile(t, filepath.Join(owner, "b.txt"), "b\n")
	writeFile(t, filepath.Join(owner, ".gitignore"), "*.log\n")
	runSteps(t, owner, [][]string{
		{"init", "--initial-branch=main", owner},
		{"add", "a.txt", "b.txt", ".gitignore"},
		{"commit", "--message", "Start"},
		{"clone", owner, agent},
	})
	return owner, agent
}

// powershell.exe stands in for the VM's ssh login shell, and a local path for the scp-style ssh URL.
func localAgentCopy(agent string) AgentCopy {
	return AgentCopy{Dir: agent, Remote: agent, Run: func(ctx context.Context, script string) ([]byte, error) {
		command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		if err != nil {
			return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return output, nil
	}}
}

func syncAndMirror(t *testing.T, owner string, agent string) error {
	t.Helper()
	if err := (Repository{Dir: owner}).SyncAgentCopy(context.Background(), localAgentCopy(agent)); err != nil {
		return err
	}
	return Repository{Dir: owner}.MirrorAgentWorkTree(context.Background(), agent)
}

func assertSameHead(t *testing.T, owner string, agent string) {
	t.Helper()
	ownerHead := gitOutput(t, owner, "rev-parse", "HEAD", "--symbolic-full-name", "HEAD")
	if agentHead := gitOutput(t, agent, "rev-parse", "HEAD", "--symbolic-full-name", "HEAD"); agentHead != ownerHead {
		t.Fatalf("got the agent's HEAD at %q, want the owner's %q", agentHead, ownerHead)
	}
}

func gitOutputFromInput(t *testing.T, dir string, input string, args ...string) string {
	t.Helper()
	output, err := runGit(context.Background(), Repository{Dir: dir}, args, input, []string{})
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func requireMirrored(t *testing.T, mirrorErr error) {
	t.Helper()
	if mirrorErr != nil {
		t.Fatal(mirrorErr)
	}
}

func assertChangedFiles(t *testing.T, dir string, want []FileStatus) {
	t.Helper()
	workTree, err := Repository{Dir: dir}.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(workTree.Files, want) {
		t.Fatalf("got changed files %+v in %s, want %+v", workTree.Files, dir, want)
	}
}

func assertFileHolds(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil || string(content) != want {
		t.Fatalf("got %q (%v) in %s, want %q", content, err, path, want)
	}
}
