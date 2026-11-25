package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/perusal/internal/focus"
)

func TestReadOwnerWorkTree(t *testing.T) {
	t.Run("aSquashMergedBranchWithAGoneUpstreamNamesTheBranchItIsMergedInto",
		aSquashMergedBranchWithAGoneUpstreamNamesTheBranchItIsMergedInto)
	t.Run("aSquashMergedBranchWithALiveUpstreamNamesTheBranchItIsMergedInto",
		aSquashMergedBranchWithALiveUpstreamNamesTheBranchItIsMergedInto)
	t.Run("aFreshBranchAtOriginsMainIsNotMerged", aFreshBranchAtOriginsMainIsNotMerged)
	t.Run("aBranchWithAChangeOriginsMainLacksIsNotMerged", aBranchWithAChangeOriginsMainLacksIsNotMerged)
	t.Run("listsTheFirstFiftyChangedFilesAndCountsThemAll", listsTheFirstFiftyChangedFilesAndCountsThemAll)
}

func aSquashMergedBranchWithAGoneUpstreamNamesTheBranchItIsMergedInto(t *testing.T) {
	clone := squashMergedBranchWithAGoneUpstream(t)
	workRepository := foundAt("work", clone)

	workTree, err := ReadOwnerWorkTree(context.Background(), workRepository)

	require.NoError(t, err)
	head := gitIn(t, clone, "rev-parse", "HEAD")
	want := focus.OwnerWorkTree{Branch: "fix", Head: head, HeadSubject: "Fix the parser",
		ChangedFiles: []string{}, BehindDefault: 1, MergedInto: "origin/main"}
	require.Equal(t, want, workTree)
}

func aSquashMergedBranchWithALiveUpstreamNamesTheBranchItIsMergedInto(t *testing.T) {
	clone := squashMergedBranchWithALiveUpstream(t)
	workRepository := foundAt("work", clone)

	workTree, err := ReadOwnerWorkTree(context.Background(), workRepository)

	require.NoError(t, err)
	require.Equal(t, "origin/fix", workTree.Upstream)
	require.Equal(t, "origin/main", workTree.MergedInto)
}

func aFreshBranchAtOriginsMainIsNotMerged(t *testing.T) {
	clone := squashMergedBranchWithALiveUpstream(t)
	gitIn(t, clone, "switch", "--create", "fresh", "origin/main")
	workRepository := foundAt("work", clone)

	workTree, err := ReadOwnerWorkTree(context.Background(), workRepository)

	require.NoError(t, err)
	require.Equal(t, "fresh", workTree.Branch)
	require.Empty(t, workTree.MergedInto)
}

func aBranchWithAChangeOriginsMainLacksIsNotMerged(t *testing.T) {
	clone := squashMergedBranchWithALiveUpstream(t)
	lexerPath := filepath.Join(clone, "lexer.go")
	err := os.WriteFile(lexerPath, []byte("package parser\n"), 0o600)
	require.NoError(t, err)
	gitIn(t, clone, "add", "lexer.go")
	gitIn(t, clone, "commit", "--message", "Add the lexer")
	workRepository := foundAt("work", clone)

	workTree, err := ReadOwnerWorkTree(context.Background(), workRepository)

	require.NoError(t, err)
	require.Empty(t, workTree.MergedInto)
}

func listsTheFirstFiftyChangedFilesAndCountsThemAll(t *testing.T) {
	clone := squashMergedBranchWithAGoneUpstream(t)
	for index := range 51 {
		name := fmt.Sprintf("new%02d.txt", index)
		path := filepath.Join(clone, name)
		err := os.WriteFile(path, []byte("new\n"), 0o600)
		require.NoError(t, err)
	}
	workRepository := foundAt("work", clone)

	workTree, err := ReadOwnerWorkTree(context.Background(), workRepository)

	require.NoError(t, err)
	require.Len(t, workTree.ChangedFiles, 50)
	require.Equal(t, "new49.txt", workTree.ChangedFiles[49])
	require.Equal(t, 51, workTree.ChangedFileCount)
	require.Equal(t, "origin/main", workTree.MergedInto)
}

func squashMergedBranchWithAGoneUpstream(t *testing.T) string {
	t.Helper()
	clone := squashMergedBranchWithALiveUpstream(t)
	gitIn(t, clone, "push", "--delete", "origin", "fix")
	gitIn(t, clone, "fetch", "--prune")
	return clone
}

func squashMergedBranchWithALiveUpstream(t *testing.T) string {
	t.Helper()
	for name, value := range map[string]string{
		"GIT_AUTHOR_NAME": "perusal", "GIT_AUTHOR_EMAIL": "perusal@example.com",
		"GIT_COMMITTER_NAME": "perusal", "GIT_COMMITTER_EMAIL": "perusal@example.com",
		"GIT_CONFIG_COUNT": "2", "GIT_CONFIG_KEY_0": "commit.gpgsign", "GIT_CONFIG_VALUE_0": "false",
		"GIT_CONFIG_KEY_1": "core.hooksPath", "GIT_CONFIG_VALUE_1": t.TempDir(),
	} {
		t.Setenv(name, value)
	}
	parent := t.TempDir()
	remote := filepath.Join(parent, "remote.git")
	clone := filepath.Join(parent, "work")
	gitIn(t, parent, "init", "--bare", "--initial-branch=main", remote)
	gitIn(t, parent, "clone", remote, clone)
	gitIn(t, clone, "commit", "--allow-empty", "--message", "Start")
	gitIn(t, clone, "push", "origin", "main")
	gitIn(t, clone, "remote", "set-head", "origin", "main")
	gitIn(t, clone, "switch", "--create", "fix")
	commitParser(t, clone, "Fix the parser")
	gitIn(t, clone, "push", "--set-upstream", "origin", "fix")
	gitIn(t, clone, "switch", "main")
	commitParser(t, clone, "Fix the parser (#1)")
	gitIn(t, clone, "push", "origin", "main")
	gitIn(t, clone, "switch", "fix")
	return clone
}

func commitParser(t *testing.T, clone string, message string) {
	t.Helper()
	parserPath := filepath.Join(clone, "parser.go")
	err := os.WriteFile(parserPath, []byte("package parser\n"), 0o600)
	require.NoError(t, err)
	gitIn(t, clone, "add", "parser.go")
	gitIn(t, clone, "commit", "--message", message)
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	gitArgs := append([]string{"-C", dir}, args...)
	command := exec.Command("git", gitArgs...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git %s\n%s", strings.Join(args, " "), output)
	return strings.TrimSpace(string(output))
}
