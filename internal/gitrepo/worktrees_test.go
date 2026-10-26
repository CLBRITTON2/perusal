package gitrepo

import (
	"context"
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
