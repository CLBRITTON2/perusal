//go:build windows

package focus

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
	"github.com/stretchr/testify/require"
)

func TestReadOwnerStateKeepsEachFailedReadAndRefusedPathAsItsRefusal(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	mustGit(t, "-C", repository, "update-ref", "refs/perusal/taken/heads/trunk", "HEAD")
	headOutput := mustGit(t, "-C", repository, "rev-parse", "HEAD")
	head := strings.TrimSpace(headOutput)
	broken := filepath.Join(devDir, "broken")
	mustMkdir(t, broken)
	writeGitFile(t, broken, filepath.Join(devDir, "missing"))
	agentDir := agentDirHolding(t, "repo", "broken")
	rules := agentHostRules(t, agentDir, devDir)
	readOwnerWorkTree := func(ctx context.Context, found FoundRepository) (OwnerWorkTree, error) {
		if found.RelativePath == "repo-side" {
			return OwnerWorkTree{}, errors.New("git status failed")
		}
		return OwnerWorkTree{Branch: "trunk", Head: head, ChangedFiles: []string{}}, nil
	}
	sideSlashed := filepath.ToSlash(side)
	repositorySlashed := filepath.ToSlash(repository)
	focused := []gitrepo.Repository{{Dir: sideSlashed}, {Dir: repositorySlashed}}
	scan := scannedOwnerDir(t, rules, runLocalScript, emptyOwnerScan())

	state, _ := readOwnerState(context.Background(), readOwnerWorkTree, scan, focused, "a.go would be overwritten",
		map[string]stampedOwnerWorkTree{})

	require.Len(t, state.WorkTrees, 3)
	brokenRefusal := state.WorkTrees[0].Refusal
	require.NotEmpty(t, brokenRefusal)
	want := OwnerState{
		ReadAt:      state.ReadAt,
		TakeFailure: "a.go would be overwritten",
		Focused:     []string{side, repository},
		WorkTrees: []OwnerWorkTree{
			{OwnerPath: broken, AgentPath: filepath.Join(agentDir, "broken"), ChangedFiles: []string{},
				Refusal: brokenRefusal},
			{OwnerPath: repository, AgentPath: filepath.Join(agentDir, "repo"), Branch: "trunk", Head: head,
				ChangedFiles: []string{}, TakenCommit: head},
			{OwnerPath: side, AgentPath: filepath.Join(agentDir, "repo-side"), ChangedFiles: []string{},
				Refusal: "git status failed"},
		},
	}
	require.Equal(t, want, state)
}

func TestReadOwnerStateListsOnlyWorkTreesWhoseCloneIsOnTheAgentHostAfterAScanOfOneScript(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	gitInit(t, filepath.Join(devDir, "laptop-only"))
	agentDir := agentDirHolding(t, "repo")
	rules := agentHostRules(t, agentDir, devDir)
	scripts := 0
	countedRun := func(ctx context.Context, script string) ([]byte, error) {
		scripts++
		return runLocalScript(ctx, script)
	}
	readOwnerWorkTree := func(ctx context.Context, found FoundRepository) (OwnerWorkTree, error) {
		return OwnerWorkTree{ChangedFiles: []string{}}, nil
	}
	scan := scannedOwnerDir(t, rules, countedRun, emptyOwnerScan())

	state, _ := readOwnerState(context.Background(), readOwnerWorkTree, scan, []gitrepo.Repository{}, "",
		map[string]stampedOwnerWorkTree{})

	require.Equal(t, 1, scripts)
	ownerPaths := []string{}
	for _, workTree := range state.WorkTrees {
		ownerPaths = append(ownerPaths, workTree.OwnerPath)
	}
	require.Equal(t, []string{repository, side}, ownerPaths)
}

func TestReadOwnerStateReadsAgainOnlyTheWorkTreesWhoseGitStateFilesChanged(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	agentDir := agentDirHolding(t, "repo")
	rules := agentHostRules(t, agentDir, devDir)
	var readPathsLock sync.Mutex
	readPaths := []string{}
	readCount := 0
	readOwnerWorkTree := func(ctx context.Context, found FoundRepository) (OwnerWorkTree, error) {
		readPathsLock.Lock()
		defer readPathsLock.Unlock()
		readPaths = append(readPaths, found.RelativePath)
		readCount++
		subject := "read " + strconv.Itoa(readCount)
		return OwnerWorkTree{HeadSubject: subject, ChangedFiles: []string{}}, nil
	}
	scan := scannedOwnerDir(t, rules, runLocalScript, emptyOwnerScan())
	cache := map[string]stampedOwnerWorkTree{}
	readAgain := func() ([]string, []OwnerWorkTree) {
		t.Helper()
		readPaths = []string{}
		state, nextCache := readOwnerState(context.Background(), readOwnerWorkTree, scan, []gitrepo.Repository{}, "",
			cache)
		cache = nextCache
		slices.Sort(readPaths)
		return readPaths, state.WorkTrees
	}

	first, firstWorkTrees := readAgain()
	unchanged, unchangedWorkTrees := readAgain()
	err := os.WriteFile(filepath.Join(side, "staged.go"), []byte("package side\n"), 0o600)
	require.NoError(t, err)
	mustGit(t, "-C", side, "add", "staged.go")
	sideStaged, _ := readAgain()
	mustGit(t, "-C", repository, "update-ref", "refs/remotes/origin/trunk", "HEAD")
	refAdded, _ := readAgain()

	got := [][]string{first, unchanged, sideStaged, refAdded}
	want := [][]string{{"repo", "repo-side"}, {}, {"repo-side"}, {"repo", "repo-side"}}
	require.Equal(t, want, got)
	require.Equal(t, firstWorkTrees, unchangedWorkTrees)
}

func TestScanOwnerDirReusesTheRepositoryOpenedForAnUnchangedGitDir(t *testing.T) {
	devDir := t.TempDir()
	repository, _ := repositoryWithWorkTree(t, devDir)
	agentDir := agentDirHolding(t, "repo")
	rules := agentHostRules(t, agentDir, devDir)
	lastScan := scannedOwnerDir(t, rules, runLocalScript, emptyOwnerScan())
	// Open would find the side work tree's real top level, so only the last scan's repository carries this one.
	cachedSideDir := filepath.Join(devDir, "cached-side")
	lastScan.found.Repositories[1].Repository.Dir = cachedSideDir

	scan := scannedOwnerDir(t, rules, runLocalScript, lastScan)

	dirs := []string{}
	for _, found := range scan.found.Repositories {
		dirs = append(dirs, filepath.Clean(found.Repository.Dir))
	}
	require.Equal(t, []string{repository, cachedSideDir}, dirs)
}

func TestReadOwnerStateReadsAFailedWorkTreeAgainWhileItsGitStateFilesAreUnchanged(t *testing.T) {
	devDir := t.TempDir()
	repositoryWithWorkTree(t, devDir)
	agentDir := agentDirHolding(t, "repo")
	rules := agentHostRules(t, agentDir, devDir)
	var sideReadsLock sync.Mutex
	sideReads := 0
	readOwnerWorkTree := func(ctx context.Context, found FoundRepository) (OwnerWorkTree, error) {
		if found.RelativePath != "repo-side" {
			return OwnerWorkTree{ChangedFiles: []string{}}, nil
		}
		sideReadsLock.Lock()
		defer sideReadsLock.Unlock()
		sideReads++
		if sideReads == 1 {
			return OwnerWorkTree{}, errors.New("git status failed")
		}
		return OwnerWorkTree{ChangedFiles: []string{}}, nil
	}
	scan := scannedOwnerDir(t, rules, runLocalScript, emptyOwnerScan())
	_, cache := readOwnerState(context.Background(), readOwnerWorkTree, scan, []gitrepo.Repository{}, "",
		map[string]stampedOwnerWorkTree{})

	state, _ := readOwnerState(context.Background(), readOwnerWorkTree, scan, []gitrepo.Repository{}, "", cache)

	require.Equal(t, 2, sideReads)
	require.Len(t, state.WorkTrees, 2)
	require.Empty(t, state.WorkTrees[1].Refusal)
}

func TestScanOwnerDirFailsOnAWorkTreeWhoseCloneIsOutsideTheOwnerDir(t *testing.T) {
	devDir := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	gitInit(t, elsewhere)
	mustGit(t, "-C", elsewhere, "-c", "user.name=perusal", "-c", "user.email=perusal@example.com",
		"-c", "commit.gpgSign=false", "commit", "--allow-empty", "--message", "first")
	mustGit(t, "-C", elsewhere, "worktree", "add", "-b", "side", filepath.Join(devDir, "elsewhere-side"))
	agentDir := agentDirHolding(t)
	rules := agentHostRules(t, agentDir, devDir)
	scripts := 0
	countedRun := func(ctx context.Context, script string) ([]byte, error) {
		scripts++
		return runLocalScript(ctx, script)
	}

	_, err := scanOwnerDir(context.Background(), rules, countedRun, emptyOwnerScan())

	require.ErrorContains(t, err, "is not under")
	require.Zero(t, scripts)
}

func TestPairedWorkTrees(t *testing.T) {
	t.Run("pairsOnlyTheWorkTreesWhoseOwnAgentCopyExists", pairsOnlyTheWorkTreesWhoseOwnAgentCopyExists)
	t.Run("keepsOnlyTheRefusalsOfRefusedWorkTreesWhoseAgentCopyExists",
		keepsOnlyTheRefusalsOfRefusedWorkTreesWhoseAgentCopyExists)
}

func pairsOnlyTheWorkTreesWhoseOwnAgentCopyExists(t *testing.T) {
	devDir := t.TempDir()
	repository, _ := repositoryWithWorkTree(t, devDir)
	gitInit(t, filepath.Join(devDir, "laptop-only"))
	agentDir := agentDirHolding(t, "repo")
	rules := agentHostRules(t, agentDir, devDir)
	scan := scannedOwnerDir(t, rules, runLocalScript, emptyOwnerScan())

	paired := scan.paired()

	repositoryAgentDir := filepath.Join(agentDir, "repo")
	require.Equal(t, []string{repositoryAgentDir}, paired.agentDirs)
	require.Len(t, paired.repositories, 1)
	requireSameDirectory(t, paired.repositories[0].Dir, repository)
	require.Empty(t, paired.refusals)
}

func keepsOnlyTheRefusalsOfRefusedWorkTreesWhoseAgentCopyExists(t *testing.T) {
	devDir := t.TempDir()
	repositoryWithWorkTree(t, devDir)
	broken := filepath.Join(devDir, "broken")
	mustMkdir(t, broken)
	writeGitFile(t, broken, filepath.Join(devDir, "missing"))
	laptopOnlyBroken := filepath.Join(devDir, "laptop-only-broken")
	mustMkdir(t, laptopOnlyBroken)
	writeGitFile(t, laptopOnlyBroken, filepath.Join(devDir, "missing"))
	agentDir := agentDirHolding(t, "repo-side", "broken")
	rules := agentHostRules(t, agentDir, devDir)
	scan := scannedOwnerDir(t, rules, runLocalScript, emptyOwnerScan())

	paired := scan.paired()

	sideAgentDir := filepath.Join(agentDir, "repo-side")
	brokenGitDir := filepath.Join(devDir, "missing")
	require.Equal(t, []string{sideAgentDir}, paired.agentDirs)
	require.Len(t, paired.refusals, 1)
	require.Contains(t, paired.refusals[0], brokenGitDir)
}

func scannedOwnerDir(t *testing.T, rules Rules, run func(ctx context.Context, script string) ([]byte, error),
	last ownerScan) ownerScan {
	t.Helper()
	scan, err := scanOwnerDir(context.Background(), rules, run, last)
	require.NoError(t, err)
	return scan
}

func agentDirHolding(t *testing.T, clones ...string) string {
	t.Helper()
	agentDir := t.TempDir()
	for _, clone := range clones {
		mustMkdir(t, filepath.Join(agentDir, clone))
	}
	return agentDir
}

func agentHostRules(t *testing.T, agentDir string, ownerDir string) Rules {
	t.Helper()
	rules, err := NewRules(Config{FocusFile: "", AgentDir: agentDir, OwnerDir: ownerDir, AgentHost: "agent-vm"})
	require.NoError(t, err)
	t.Cleanup(func() {
		err := rules.held.release()
		require.NoError(t, err)
	})
	return rules
}

func TestWriteOwnerState(t *testing.T) {
	t.Run("writesFileNamesHoldingQuotesAsTheyAre", writesFileNamesHoldingQuotesAsTheyAre)
	t.Run("failsWhenTheFolderIsMissing", failsWhenTheFolderIsMissing)
}

func writesFileNamesHoldingQuotesAsTheyAre(t *testing.T) {
	file := filepath.Join(t.TempDir(), "owner-state.json")
	state := OwnerState{ReadAt: "2026-11-24T10:00:00-05:00", Focused: []string{}, WorkTrees: []OwnerWorkTree{
		{OwnerPath: `C:\dev\repo`, ChangedFiles: []string{"it's.go", "it’s <new>.go"}, ChangedFileCount: 2},
	}}

	err := writeOwnerState(context.Background(), runLocalScript, file, state)

	require.NoError(t, err)
	content, err := os.ReadFile(file)
	require.NoError(t, err)
	var written OwnerState
	err = json.Unmarshal(content, &written)
	require.NoError(t, err)
	require.Equal(t, state, written)
}

func failsWhenTheFolderIsMissing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "missing", "owner-state.json")
	emptyState := OwnerState{Focused: []string{}, WorkTrees: []OwnerWorkTree{}}

	err := writeOwnerState(context.Background(), runLocalScript, file, emptyState)

	require.ErrorContains(t, err, "Could not find a part of the path")
}

// runLocalScript stands in for the agent host's ssh login shell, failing as Session.Run does on a thrown error.
func runLocalScript(ctx context.Context, script string) ([]byte, error) {
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"$ErrorActionPreference = 'Stop'; "+script)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, errors.New(string(output))
	}
	return output, nil
}
