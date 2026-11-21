package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	workTrees, err := Repository{Dir: side}.WorkTrees(context.Background())

	want := []WorkTree{
		{Path: filepath.ToSlash(clone), Branch: "main"},
		{Path: filepath.ToSlash(side), Branch: "side"},
		{Path: filepath.ToSlash(detached), Branch: ""},
	}
	if err != nil || !reflect.DeepEqual(workTrees, want) {
		t.Fatalf("got %+v, %v, want %+v", workTrees, err, want)
	}
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

	assertWorkTreeAndRefsGone(t, err, clone, side)
}

func removesAWorkTreeNothingWasTakenInto(t *testing.T) {
	clone, side := cloneWithSideWorkTree(t)

	err := Repository{Dir: clone}.RemoveWorkTree(context.Background(), WorkTree{Path: side, Branch: "side"})

	assertWorkTreeAndRefsGone(t, err, clone, side)
}

func refusesAWorkTreeWithUncommittedChangesAndKeepsItsBranch(t *testing.T) {
	clone, side := cloneWithSideWorkTree(t)
	writeFile(t, filepath.Join(side, "side.txt"), "side\n")

	err := Repository{Dir: clone}.RemoveWorkTree(context.Background(), WorkTree{Path: side, Branch: "side"})

	if err == nil {
		t.Fatal("got no error, want git to refuse the untracked file")
	}
	if _, statErr := os.Stat(filepath.Join(side, "side.txt")); statErr != nil {
		t.Fatalf("got %v, want side.txt kept", statErr)
	}
	if refs := gitOutput(t, clone, "for-each-ref", "--format=%(refname)", "refs/heads/side"); refs != "refs/heads/side\n" {
		t.Fatalf("got refs %q, want side kept", refs)
	}
}

func cloneWithSideWorkTree(t *testing.T) (string, string) {
	t.Helper()
	_, clone := pushedClone(t)
	side := filepath.Join(t.TempDir(), "side")
	runSteps(t, clone, [][]string{{"worktree", "add", "-b", "side", side}})
	return clone, side
}

func assertWorkTreeAndRefsGone(t *testing.T, err error, clone string, side string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(side); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("got %v for %s, want it removed", statErr, side)
	}
	refs := gitOutput(t, clone, "for-each-ref", "--format=%(refname)", "refs/heads/side", "refs/perusal")
	if refs != "" {
		t.Fatalf("got refs %q, want side and its taken ref deleted", refs)
	}
}
