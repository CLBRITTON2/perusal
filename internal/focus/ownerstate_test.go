//go:build windows

package focus

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestReadOwnerStateKeepsEachFailedReadAndRefusedPathAsItsRefusal(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	mustGit(t, "-C", repository, "update-ref", "refs/perusal/taken/heads/trunk", "HEAD")
	head := strings.TrimSpace(mustGit(t, "-C", repository, "rev-parse", "HEAD"))
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

	state, err := readOwnerState(context.Background(), rules, runLocalScript, readOwnerWorkTree,
		[]gitrepo.Repository{{Dir: filepath.ToSlash(side)}, {Dir: filepath.ToSlash(repository)}}, "a.go would be overwritten")

	if err != nil || len(state.WorkTrees) != 3 || state.WorkTrees[0].Refusal == "" {
		t.Fatalf("got %+v, %v, want broken refused first", state, err)
	}
	want := OwnerState{
		ReadAt:      state.ReadAt,
		TakeFailure: "a.go would be overwritten",
		Focused:     []string{side, repository},
		WorkTrees: []OwnerWorkTree{
			{OwnerPath: broken, AgentPath: filepath.Join(agentDir, "broken"), ChangedFiles: []string{},
				Refusal: state.WorkTrees[0].Refusal},
			{OwnerPath: repository, AgentPath: filepath.Join(agentDir, "repo"), Branch: "trunk", Head: head,
				ChangedFiles: []string{}, TakenCommit: head},
			{OwnerPath: side, AgentPath: filepath.Join(agentDir, "repo-side"), ChangedFiles: []string{},
				Refusal: "git status failed"},
		},
	}
	if !reflect.DeepEqual(state, want) {
		t.Fatalf("got\n%+v\nwant\n%+v", state, want)
	}
}

func TestReadOwnerStateListsOnlyWorkTreesWhoseCloneIsOnTheAgentHostInOneScript(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	gitInit(t, filepath.Join(devDir, "laptop-only"))
	rules := agentHostRules(t, agentDirHolding(t, "repo"), devDir)
	scripts := 0
	countedRun := func(ctx context.Context, script string) ([]byte, error) {
		scripts++
		return runLocalScript(ctx, script)
	}
	readOwnerWorkTree := func(ctx context.Context, found FoundRepository) (OwnerWorkTree, error) {
		return OwnerWorkTree{ChangedFiles: []string{}}, nil
	}

	state, err := readOwnerState(context.Background(), rules, countedRun, readOwnerWorkTree, []gitrepo.Repository{}, "")

	ownerPaths := []string{}
	for _, workTree := range state.WorkTrees {
		ownerPaths = append(ownerPaths, workTree.OwnerPath)
	}
	if err != nil || scripts != 1 || !slices.Equal(ownerPaths, []string{repository, side}) {
		t.Fatalf("got %v in %d scripts, %v, want only %s and %s in one", ownerPaths, scripts, err, repository, side)
	}
}

func TestReadOwnerStateFailsOnAWorkTreeWhoseCloneIsOutsideTheOwnerDir(t *testing.T) {
	devDir := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	gitInit(t, elsewhere)
	mustGit(t, "-C", elsewhere, "-c", "user.name=perusal", "-c", "user.email=perusal@example.com",
		"-c", "commit.gpgSign=false", "commit", "--allow-empty", "--message", "first")
	mustGit(t, "-C", elsewhere, "worktree", "add", "-b", "side", filepath.Join(devDir, "elsewhere-side"))
	rules := agentHostRules(t, agentDirHolding(t), devDir)
	scripts := 0
	countedRun := func(ctx context.Context, script string) ([]byte, error) {
		scripts++
		return runLocalScript(ctx, script)
	}
	readOwnerWorkTree := func(ctx context.Context, found FoundRepository) (OwnerWorkTree, error) {
		return OwnerWorkTree{ChangedFiles: []string{}}, nil
	}

	state, err := readOwnerState(context.Background(), rules, countedRun, readOwnerWorkTree, []gitrepo.Repository{}, "")

	if err == nil || !strings.Contains(err.Error(), "is not under") || scripts != 0 {
		t.Fatalf("got %+v, %v after %d scripts, want the mapping error before any script", state, err, scripts)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rules.held.release(); err != nil {
			t.Error(err)
		}
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

	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var written OwnerState
	if err := json.Unmarshal(content, &written); err != nil || !reflect.DeepEqual(written, state) {
		t.Fatalf("got %s, %v, want %+v", content, err, state)
	}
}

func failsWhenTheFolderIsMissing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "missing", "owner-state.json")

	err := writeOwnerState(context.Background(), runLocalScript, file, OwnerState{Focused: []string{},
		WorkTrees: []OwnerWorkTree{}})

	if err == nil || !strings.Contains(err.Error(), "Could not find a part of the path") {
		t.Fatalf("got %v, want the missing folder", err)
	}
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
