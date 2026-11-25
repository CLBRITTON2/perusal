package gitrepo

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTakeAgentCommits(t *testing.T) {
	t.Run("showsTheAgentsCommitAsUnstagedChanges", showsTheAgentsCommitAsUnstagedChanges)
	t.Run("takesOnlyWhatTheNextCommitChangesAfterTheOwnerCommits", takesOnlyWhatTheNextCommitChangesAfterTheOwnerCommits)
	t.Run("keepsAFileTheOwnerEditedAndRefusesTheCommit", keepsAFileTheOwnerEditedAndRefusesTheCommit)
	t.Run("skipsTheFetchWhenTheAgentsHeadIsTaken", skipsTheFetchWhenTheAgentsHeadIsTaken)
	t.Run("refusesACommitNamingAPathInDotGit", refusesACommitNamingAPathInDotGit)
	t.Run("refusesACommitWritingOverMaximumTakenBytes", refusesACommitWritingOverMaximumTakenBytes)
}

func refusesACommitWritingOverMaximumTakenBytes(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	blob := gitOutputFromInput(t, agent, strings.Repeat("0", maximumTakenBytes+1), "hash-object", "-w", "--stdin")
	tree := gitOutputFromInput(t, agent, "100644 blob "+blob+"\tlarge.txt\n", "mktree")
	commit := strings.TrimSpace(gitOutput(t, agent, "commit-tree", tree, "-p", "HEAD", "-m", "Add large.txt"))
	runSteps(t, agent, [][]string{{"update-ref", "refs/heads/main", commit}})

	changed, err := takeAgentCommits(t, owner, agent)

	require.ErrorIs(t, err, ErrTakenCommitsTooLarge)
	require.False(t, changed)
	assertChangedFiles(t, owner, []FileStatus{})
}

func TestAddWorkTreeFromAgent(t *testing.T) {
	t.Run("addsTheAgentsBranchAtWhereItLeftOriginsDefaultBranch", addsTheAgentsBranchAtWhereItLeftOriginsDefaultBranch)
	t.Run("refusesADetachedHead", refusesADetachedHead)
}

func addsTheAgentsBranchAtWhereItLeftOriginsDefaultBranch(t *testing.T) {
	owner, agent := ownerAndAgentClonesOfOrigin(t)
	branchPoint := revParse(t, agent, "HEAD")
	runSteps(t, agent, [][]string{{"switch", "--create", "claude/x"}})
	writeFile(t, filepath.Join(agent, "agent.txt"), "agent\n")
	commitAll(t, agent, "Agent")
	writeFile(t, filepath.Join(owner, "a.txt"), "moved on\n")
	commitAll(t, owner, "Origin moves on")
	runSteps(t, owner, [][]string{{"push", "origin", "main"}})
	workTree := filepath.Join(t.TempDir(), "claude-x")
	agentHead := readAgentHead(t, agent)

	base, err := Repository{Dir: owner}.AddWorkTreeFromAgent(context.Background(), agent, agentHead, workTree)

	require.NoError(t, err)
	require.Equal(t, branchPoint, base)
	workTreeHead := revParse(t, workTree, "HEAD")
	require.Equal(t, branchPoint, workTreeHead)
	branchOutput := gitOutput(t, workTree, "branch", "--show-current")
	branch := strings.TrimSpace(branchOutput)
	require.Equal(t, "claude/x", branch)
	_, takeErr := takeAgentCommits(t, workTree, agent)
	require.NoError(t, takeErr)
	agentFile := filepath.Join(workTree, "agent.txt")
	assertFileHolds(t, agentFile, "agent\n")
}

func refusesADetachedHead(t *testing.T) {
	owner, agent := ownerAndAgentClonesOfOrigin(t)
	runSteps(t, agent, [][]string{{"switch", "--detach", "HEAD"}})
	workTree := filepath.Join(t.TempDir(), "detached")
	agentHead := readAgentHead(t, agent)

	_, err := Repository{Dir: owner}.AddWorkTreeFromAgent(context.Background(), agent, agentHead, workTree)

	require.ErrorContains(t, err, "detached HEAD")
	_, statErr := os.Stat(workTree)
	require.ErrorIs(t, statErr, fs.ErrNotExist)
}

// Both are clones of a bare origin, as the laptop's and the VM's are clones from GitHub.
func ownerAndAgentClonesOfOrigin(t *testing.T) (string, string) {
	t.Helper()
	origin, _ := ownerAndAgentCopies(t)
	bare := filepath.Join(t.TempDir(), "origin.git")
	owner := filepath.Join(t.TempDir(), "owner")
	agent := filepath.Join(t.TempDir(), "agent")
	runSteps(t, origin, [][]string{
		{"clone", "--bare", origin, bare},
		{"clone", bare, owner},
		{"clone", bare, agent},
	})
	return owner, agent
}

func TestReadAgentHeads(t *testing.T) {
	_, agent := ownerAndAgentCopies(t)
	notAClone := t.TempDir()

	heads, err := ReadAgentHeads(context.Background(), runLocalScript, []string{notAClone, agent})

	require.NoError(t, err)
	require.ErrorContains(t, heads[0].Err, notAClone)
	agentCommit := revParse(t, agent, "HEAD")
	want := AgentHead{Commit: agentCommit, Branch: "refs/heads/main"}
	require.Equal(t, want, heads[1])
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

	require.NoError(t, err)
	require.True(t, changed)
	assertChangedFiles(t, owner, []FileStatus{
		{Path: "a.txt", IndexStatus: ' ', WorktreeStatus: 'M'},
		{Path: "b.txt", IndexStatus: ' ', WorktreeStatus: 'D'},
		{Path: "new.txt", IndexStatus: '?', WorktreeStatus: '?'},
	})
	aFile := filepath.Join(owner, "a.txt")
	assertFileHolds(t, aFile, "agent\n")
	ownerHeadAfter := revParse(t, owner, "HEAD")
	require.Equal(t, ownerHead, ownerHeadAfter)
}

func takesOnlyWhatTheNextCommitChangesAfterTheOwnerCommits(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "First")
	_, firstErr := takeAgentCommits(t, owner, agent)
	require.NoError(t, firstErr)
	commitAll(t, owner, "Owner takes the first")
	writeFile(t, filepath.Join(agent, "b.txt"), "agent\n")
	commitAll(t, agent, "Second")

	_, err := takeAgentCommits(t, owner, agent)

	require.NoError(t, err)
	assertChangedFiles(t, owner, []FileStatus{{Path: "b.txt", IndexStatus: ' ', WorktreeStatus: 'M'}})
}

func keepsAFileTheOwnerEditedAndRefusesTheCommit(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "First")
	_, firstErr := takeAgentCommits(t, owner, agent)
	require.NoError(t, firstErr)
	writeFile(t, filepath.Join(owner, "a.txt"), "owner\n")
	writeFile(t, filepath.Join(agent, "a.txt"), "agent again\n")
	writeFile(t, filepath.Join(agent, "b.txt"), "agent\n")
	commitAll(t, agent, "Second")

	changed, err := takeAgentCommits(t, owner, agent)

	require.ErrorContains(t, err, "a.txt")
	require.False(t, changed)
	aFile := filepath.Join(owner, "a.txt")
	assertFileHolds(t, aFile, "owner\n")
	bFile := filepath.Join(owner, "b.txt")
	assertFileHolds(t, bFile, "b\n")
}

func skipsTheFetchWhenTheAgentsHeadIsTaken(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "Agent")
	_, firstErr := takeAgentCommits(t, owner, agent)
	require.NoError(t, firstErr)
	missing := filepath.Join(t.TempDir(), "missing")
	agentHead := readAgentHead(t, agent)

	changed, err := Repository{Dir: owner}.TakeAgentCommits(context.Background(), missing, agentHead)

	require.NoError(t, err)
	require.False(t, changed)
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

	require.ErrorContains(t, err, "hasDotgit")
	require.False(t, changed)
	assertChangedFiles(t, owner, []FileStatus{})
}

func takeAgentCommits(t *testing.T, owner string, agent string) (bool, error) {
	t.Helper()
	agentHead := readAgentHead(t, agent)
	return Repository{Dir: owner}.TakeAgentCommits(context.Background(), agent, agentHead)
}

func readAgentHead(t *testing.T, agent string) AgentHead {
	t.Helper()
	heads, err := ReadAgentHeads(context.Background(), runLocalScript, []string{agent})
	require.NoError(t, err)
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
	require.NoError(t, err)
	return strings.TrimSpace(string(output))
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	err := os.Remove(path)
	require.NoError(t, err)
}

func assertChangedFiles(t *testing.T, dir string, want []FileStatus) {
	t.Helper()
	workTree, err := Repository{Dir: dir}.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, workTree.Files)
}

func TestReadAgentWorkTree(t *testing.T) {
	t.Run("readsAMissingCopyAsNotExisting", readsAMissingCopyAsNotExisting)
	t.Run("readsAnUntrackedFileAsUncommitted", readsAnUntrackedFileAsUncommitted)
	t.Run("readsACleanCopyWithItsHead", readsACleanCopyWithItsHead)
}

func readsAMissingCopyAsNotExisting(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	agentWorkTree, err := ReadAgentWorkTree(context.Background(), runLocalScript, missing)

	require.NoError(t, err)
	want := AgentWorkTree{Dir: missing}
	require.Equal(t, want, agentWorkTree)
}

func readsAnUntrackedFileAsUncommitted(t *testing.T) {
	_, agent := ownerAndAgentCopies(t)
	dirty := filepath.Join(t.TempDir(), "dirty")
	runSteps(t, agent, [][]string{{"worktree", "add", "-b", "dirty", dirty}})
	writeFile(t, filepath.Join(dirty, "new.txt"), "new\n")
	head := revParse(t, agent, "HEAD")

	agentWorkTree, err := ReadAgentWorkTree(context.Background(), runLocalScript, dirty)

	require.NoError(t, err)
	want := AgentWorkTree{Dir: dirty, Exists: true, Uncommitted: true,
		Head: AgentHead{Commit: head, Branch: "refs/heads/dirty"}}
	require.Equal(t, want, agentWorkTree)
}

func readsACleanCopyWithItsHead(t *testing.T) {
	_, agent := ownerAndAgentCopies(t)
	head := revParse(t, agent, "HEAD")

	agentWorkTree, err := ReadAgentWorkTree(context.Background(), runLocalScript, agent)

	require.NoError(t, err)
	want := AgentWorkTree{Dir: agent, Exists: true, Head: AgentHead{Commit: head, Branch: "refs/heads/main"}}
	require.Equal(t, want, agentWorkTree)
}

func TestHasTaken(t *testing.T) {
	t.Run("isTrueForACommitInTheOwnersBranch", isTrueForACommitInTheOwnersBranch)
	t.Run("isTrueForTheCommitTheLastTakeTook", isTrueForTheCommitTheLastTakeTook)
	t.Run("isFalseForACommitTheOwnerLacks", isFalseForACommitTheOwnerLacks)
	t.Run("isFalseForAFetchedCommitOutsideTheOwnersBranch", isFalseForAFetchedCommitOutsideTheOwnersBranch)
}

func isTrueForACommitInTheOwnersBranch(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	agentHead := readAgentHead(t, agent)

	taken, err := Repository{Dir: owner}.HasTaken(context.Background(), agentHead)

	require.NoError(t, err)
	require.True(t, taken)
}

func isTrueForTheCommitTheLastTakeTook(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "Agent")
	_, takeErr := takeAgentCommits(t, owner, agent)
	require.NoError(t, takeErr)
	agentHead := readAgentHead(t, agent)

	taken, err := Repository{Dir: owner}.HasTaken(context.Background(), agentHead)

	require.NoError(t, err)
	require.True(t, taken)
}

func isFalseForACommitTheOwnerLacks(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "Agent")
	agentHead := readAgentHead(t, agent)

	taken, err := Repository{Dir: owner}.HasTaken(context.Background(), agentHead)

	require.NoError(t, err)
	require.False(t, taken)
}

func isFalseForAFetchedCommitOutsideTheOwnersBranch(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "Agent")
	runSteps(t, owner, [][]string{{"fetch", agent, "main:refs/fetched/main"}})
	agentHead := readAgentHead(t, agent)

	taken, err := Repository{Dir: owner}.HasTaken(context.Background(), agentHead)

	require.NoError(t, err)
	require.False(t, taken)
}

func TestRemoveAgentWorkTree(t *testing.T) {
	t.Run("removesTheCopyAndItsBranch", removesTheCopyAndItsBranch)
	t.Run("refusesACopyWhoseHeadMovedSinceItWasRead", refusesACopyWhoseHeadMovedSinceItWasRead)
}

func removesTheCopyAndItsBranch(t *testing.T) {
	agent, side := cloneWithSideWorkTree(t)
	agentWorkTree, readErr := ReadAgentWorkTree(context.Background(), runLocalScript, side)
	require.NoError(t, readErr)

	err := RemoveAgentWorkTree(context.Background(), runLocalScript, agent, agentWorkTree)

	require.NoError(t, err)
	assertSideWorkTreeAndRefsGone(t, agent, side)
}

func refusesACopyWhoseHeadMovedSinceItWasRead(t *testing.T) {
	agent, side := cloneWithSideWorkTree(t)
	agentWorkTree, readErr := ReadAgentWorkTree(context.Background(), runLocalScript, side)
	require.NoError(t, readErr)
	writeFile(t, filepath.Join(side, "side.txt"), "side\n")
	commitAll(t, side, "Side")
	moved := revParse(t, side, "HEAD")

	err := RemoveAgentWorkTree(context.Background(), runLocalScript, agent, agentWorkTree)

	require.ErrorContains(t, err, "HEAD moved to "+moved)
	sideFile := filepath.Join(side, "side.txt")
	assertFileHolds(t, sideFile, "side\n")
}
