package gitrepo

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkTreesListsTheMainOneFirstAndSkipsOneWhoseFolderIsGone(t *testing.T) {
	_, clone := pushedClone(t)
	side := filepath.Join(t.TempDir(), "side")
	detached := filepath.Join(t.TempDir(), "detached")
	gone := filepath.Join(t.TempDir(), "gone")
	runSteps(t, clone, [][]string{
		{"worktree", "add", "-b", "side", side},
		{"worktree", "add", "--detach", detached},
		{"worktree", "add", "-b", "gone", gone},
	})
	removeErr := os.RemoveAll(gone)
	require.NoError(t, removeErr)

	workTrees, err := Repository{Dir: side}.WorkTrees(context.Background())

	require.NoError(t, err)
	want := []WorkTree{
		{Path: filepath.ToSlash(clone), Branch: "main"},
		{Path: filepath.ToSlash(side), Branch: "side"},
		{Path: filepath.ToSlash(detached), Branch: ""},
	}
	require.Equal(t, want, workTrees)
}

func TestRemoveWorkTree(t *testing.T) {
	t.Run("removesTheWorkTreeItsUnmergedBranchAndItsTakenRef", removesTheWorkTreeItsUnmergedBranchAndItsTakenRef)
	t.Run("removesAWorkTreeNothingWasTakenInto", removesAWorkTreeNothingWasTakenInto)
	t.Run("refusesAWorkTreeWithUncommittedChangesAndKeepsItsBranch",
		refusesAWorkTreeWithUncommittedChangesAndKeepsItsBranch)
}

func removesTheWorkTreeItsUnmergedBranchAndItsTakenRef(t *testing.T) {
	clone, side := cloneWithSideWorkTree(t)
	writeFile(t, filepath.Join(side, "side.txt"), "side\n")
	runSteps(t, side, [][]string{
		{"add", "side.txt"},
		{"commit", "--message", "Side"},
		{"update-ref", "refs/perusal/taken/heads/side", "HEAD"},
	})

	err := Repository{Dir: clone}.RemoveWorkTree(context.Background(), WorkTree{Path: side, Branch: "side"})

	require.NoError(t, err)
	assertSideWorkTreeAndRefsGone(t, clone, side)
}

func removesAWorkTreeNothingWasTakenInto(t *testing.T) {
	clone, side := cloneWithSideWorkTree(t)

	err := Repository{Dir: clone}.RemoveWorkTree(context.Background(), WorkTree{Path: side, Branch: "side"})

	require.NoError(t, err)
	assertSideWorkTreeAndRefsGone(t, clone, side)
}

func refusesAWorkTreeWithUncommittedChangesAndKeepsItsBranch(t *testing.T) {
	clone, side := cloneWithSideWorkTree(t)
	untracked := filepath.Join(side, "side.txt")
	writeFile(t, untracked, "side\n")

	err := Repository{Dir: clone}.RemoveWorkTree(context.Background(), WorkTree{Path: side, Branch: "side"})

	require.Error(t, err)
	require.FileExists(t, untracked)
	refs := gitOutput(t, clone, "for-each-ref", "--format=%(refname)", "refs/heads/side")
	require.Equal(t, "refs/heads/side\n", refs)
}

func cloneWithSideWorkTree(t *testing.T) (string, string) {
	t.Helper()
	_, clone := pushedClone(t)
	side := filepath.Join(t.TempDir(), "side")
	runSteps(t, clone, [][]string{{"worktree", "add", "-b", "side", side}})
	return clone, side
}

func assertSideWorkTreeAndRefsGone(t *testing.T, clone string, side string) {
	t.Helper()
	_, statErr := os.Stat(side)
	require.ErrorIs(t, statErr, fs.ErrNotExist)
	refs := gitOutput(t, clone, "for-each-ref", "--format=%(refname)", "refs/heads/side", "refs/perusal")
	require.Empty(t, refs)
}
