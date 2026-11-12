package gitrepo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTakeAgentCommits(t *testing.T) {
	t.Run("showsTheAgentsCommitAsUnstagedChanges", showsTheAgentsCommitAsUnstagedChanges)
	t.Run("takesOnlyWhatTheNextCommitChangesAfterTheOwnerCommits", takesOnlyWhatTheNextCommitChangesAfterTheOwnerCommits)
	t.Run("keepsAFileTheOwnerEditedAndRefusesTheCommit", keepsAFileTheOwnerEditedAndRefusesTheCommit)
	t.Run("skipsTheFetchWhenTheAgentsHeadIsTaken", skipsTheFetchWhenTheAgentsHeadIsTaken)
	t.Run("refusesACommitNamingAPathInDotGit", refusesACommitNamingAPathInDotGit)
}

func TestReadAgentHeads(t *testing.T) {
	_, agent := ownerAndAgentCopies(t)
	notAClone := t.TempDir()

	heads, err := ReadAgentHeads(context.Background(), runLocalScript, []string{notAClone, agent})

	if err != nil {
		t.Fatal(err)
	}
	if heads[0].Err == nil || !strings.Contains(heads[0].Err.Error(), notAClone) {
		t.Fatalf("got error %v for %s, want one naming it", heads[0].Err, notAClone)
	}
	if heads[1] != (AgentHead{Commit: revParse(t, agent, "HEAD"), Branch: "refs/heads/main"}) {
		t.Fatalf("got %+v for %s, want its HEAD on main", heads[1], agent)
	}
}

func showsTheAgentsCommitAsUnstagedChanges(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	ownerHead := revParse(t, owner, "HEAD")
	runSteps(t, agent, [][]string{{"switch", "--create", "claude/x"}})
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	writeFile(t, filepath.Join(agent, "new.txt"), "new\n")
	removeFile(t, filepath.Join(agent, "b.txt"))
	commitAll(t, agent, "Agent")

	changed, err := takeAgentCommits(t, owner, agent)

	if err != nil || !changed {
		t.Fatalf("got changed %v, error %v, want the commit taken", changed, err)
	}
	assertChangedFiles(t, owner, []FileStatus{
		{Path: "a.txt", IndexStatus: ' ', WorktreeStatus: 'M'},
		{Path: "b.txt", IndexStatus: ' ', WorktreeStatus: 'D'},
		{Path: "new.txt", IndexStatus: '?', WorktreeStatus: '?'},
	})
	assertFileHolds(t, filepath.Join(owner, "a.txt"), "agent\n")
	if head := revParse(t, owner, "HEAD"); head != ownerHead {
		t.Fatalf("got the owner's HEAD at %s, want it left at %s", head, ownerHead)
	}
}

func takesOnlyWhatTheNextCommitChangesAfterTheOwnerCommits(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "First")
	if _, err := takeAgentCommits(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	commitAll(t, owner, "Owner takes the first")
	writeFile(t, filepath.Join(agent, "b.txt"), "agent\n")
	commitAll(t, agent, "Second")

	_, err := takeAgentCommits(t, owner, agent)

	if err != nil {
		t.Fatal(err)
	}
	assertChangedFiles(t, owner, []FileStatus{{Path: "b.txt", IndexStatus: ' ', WorktreeStatus: 'M'}})
}

func keepsAFileTheOwnerEditedAndRefusesTheCommit(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "First")
	if _, err := takeAgentCommits(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(owner, "a.txt"), "owner\n")
	writeFile(t, filepath.Join(agent, "a.txt"), "agent again\n")
	writeFile(t, filepath.Join(agent, "b.txt"), "agent\n")
	commitAll(t, agent, "Second")

	changed, err := takeAgentCommits(t, owner, agent)

	if err == nil || !strings.Contains(err.Error(), "a.txt") || changed {
		t.Fatalf("got changed %v, error %v, want the commit refused over a.txt", changed, err)
	}
	assertFileHolds(t, filepath.Join(owner, "a.txt"), "owner\n")
	assertFileHolds(t, filepath.Join(owner, "b.txt"), "b\n")
}

func skipsTheFetchWhenTheAgentsHeadIsTaken(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "Agent")
	if _, err := takeAgentCommits(t, owner, agent); err != nil {
		t.Fatal(err)
	}

	changed, err := Repository{Dir: owner}.TakeAgentCommits(context.Background(),
		filepath.Join(t.TempDir(), "missing"), readAgentHead(t, agent))

	if err != nil || changed {
		t.Fatalf("got changed %v, error %v, want nothing to do", changed, err)
	}
	assertChangedFiles(t, owner, []FileStatus{{Path: "a.txt", IndexStatus: ' ', WorktreeStatus: 'M'}})
}

func refusesACommitNamingAPathInDotGit(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	blob := gitOutputFromInput(t, agent, "[core]\n\thooksPath = planted\n", "hash-object", "-w", "--stdin")
	dotGit := gitOutputFromInput(t, agent, "100644 blob "+blob+"\tconfig\n", "mktree")
	tree := gitOutputFromInput(t, agent, "040000 tree "+dotGit+"\t.git\n", "mktree")
	commit := strings.TrimSpace(gitOutput(t, agent, "commit-tree", tree, "-p", "HEAD", "-m", "Plant .git/config"))
	runSteps(t, agent, [][]string{{"update-ref", "refs/heads/main", commit}})

	changed, err := takeAgentCommits(t, owner, agent)

	if err == nil || !strings.Contains(err.Error(), "hasDotgit") || changed {
		t.Fatalf("got changed %v, error %v, want fsck's hasDotgit refusal", changed, err)
	}
	assertChangedFiles(t, owner, []FileStatus{})
}

func takeAgentCommits(t *testing.T, owner string, agent string) (bool, error) {
	t.Helper()
	return Repository{Dir: owner}.TakeAgentCommits(context.Background(), agent, readAgentHead(t, agent))
}

func readAgentHead(t *testing.T, agent string) AgentHead {
	t.Helper()
	heads, err := ReadAgentHeads(context.Background(), runLocalScript, []string{agent})
	if err != nil {
		t.Fatal(err)
	}
	return heads[0]
}

func commitAll(t *testing.T, dir string, message string) {
	t.Helper()
	runSteps(t, dir, [][]string{{"add", "--all"}, {"commit", "--message", message}})
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

// powershell.exe stands in for the VM's ssh login shell, as agenthost.Session.Run runs a script there.
func runLocalScript(ctx context.Context, script string) ([]byte, error) {
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
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
