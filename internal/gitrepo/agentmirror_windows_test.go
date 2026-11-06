package gitrepo

import (
	"context"
	"errors"
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

func showsTheAgentsEditsAsUnstagedChanges(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	writeFile(t, filepath.Join(agent, "new.txt"), "new\n")
	writeFile(t, filepath.Join(agent, "build.log"), "ignored\n")
	removeFile(t, filepath.Join(agent, "b.txt"))
	runSteps(t, agent, [][]string{{"-C", agent, "add", "a.txt"}})

	mirrorErr := snapshotAndMirror(t, owner, agent)

	assertWorkTreeChanges(t, owner, mirrorErr, []FileStatus{
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
	if err := snapshotAndMirror(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	removeFile(t, filepath.Join(agent, "new.txt"))
	runSteps(t, agent, [][]string{{"-C", agent, "restore", "b.txt"}})

	mirrorErr := snapshotAndMirror(t, owner, agent)

	assertWorkTreeChanges(t, owner, mirrorErr, []FileStatus{{Path: "a.txt", IndexStatus: ' ', WorktreeStatus: 'M'}})
}

func keepsAFileTheOwnerEditedAndRefusesTheSnapshot(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	if err := snapshotAndMirror(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(owner, "a.txt"), "owner\n")
	writeFile(t, filepath.Join(agent, "a.txt"), "agent again\n")

	mirrorErr := snapshotAndMirror(t, owner, agent)

	if mirrorErr == nil || !strings.Contains(mirrorErr.Error(), "a.txt") {
		t.Fatalf("got error %v, want git's refusal to overwrite a.txt", mirrorErr)
	}
	assertFileHolds(t, filepath.Join(owner, "a.txt"), "owner\n")
}

func refusesASnapshotTakenOnAnotherCommit(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	runSteps(t, agent, [][]string{{"-C", agent, "commit", "--all", "--message", "Agent commit"}})
	writeFile(t, filepath.Join(agent, "b.txt"), "agent\n")

	mirrorErr := snapshotAndMirror(t, owner, agent)

	if !errors.Is(mirrorErr, ErrAgentSnapshotRefused) {
		t.Fatalf("got error %v, want %v", mirrorErr, ErrAgentSnapshotRefused)
	}
	assertWorkTreeChanges(t, owner, nil, []FileStatus{})
}

func refusesASnapshotNamingAPathInDotGit(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	blob := gitOutputFromInput(t, agent, "[core]\n\thooksPath = planted\n", "hash-object", "-w", "--stdin")
	dotGit := gitOutputFromInput(t, agent, "100644 blob "+blob+"\tconfig\n", "mktree")
	tree := gitOutputFromInput(t, agent, "040000 tree "+dotGit+"\t.git\n", "mktree")
	commit := strings.TrimSpace(gitOutput(t, agent, "commit-tree", tree, "-p", "HEAD", "-m", "Plant .git/config"))
	runSteps(t, agent, [][]string{{"-C", agent, "update-ref", "refs/perusal/snapshot", commit}})

	mirrorErr := Repository{Dir: owner}.MirrorAgentWorkTree(context.Background(), agent)

	if mirrorErr == nil || !strings.Contains(mirrorErr.Error(), "hasDotgit") {
		t.Fatalf("got error %v, want fsck's hasDotgit refusal", mirrorErr)
	}
	assertWorkTreeChanges(t, owner, nil, []FileStatus{})
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
		{"-C", owner, "add", "a.txt", "b.txt", ".gitignore"},
		{"-C", owner, "commit", "--message", "Start"},
		{"clone", owner, agent},
	})
	return owner, agent
}

// powershell.exe stands in for the VM's ssh login shell, and a local path for the scp-style ssh URL.
func snapshotAndMirror(t *testing.T, owner string, agent string) error {
	t.Helper()
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		agentSnapshotScript(agent)).CombinedOutput()
	if err != nil {
		t.Fatalf("snapshot %s: %v: %s", agent, err, output)
	}
	return Repository{Dir: owner}.MirrorAgentWorkTree(context.Background(), agent)
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

func assertWorkTreeChanges(t *testing.T, dir string, mirrorErr error, want []FileStatus) {
	t.Helper()
	status, statusErr := Repository{Dir: dir}.Status(context.Background())
	if mirrorErr != nil || statusErr != nil || !reflect.DeepEqual(status.Files, want) {
		t.Fatalf("got %+v (mirror %v, status %v), want %+v", status.Files, mirrorErr, statusErr, want)
	}
}

func assertFileHolds(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil || string(content) != want {
		t.Fatalf("got %q (%v) in %s, want %q", content, err, path, want)
	}
}
