package gitrepo

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestBranchesListsEachLocalBranchWithItsUpstream(t *testing.T) {
	clone, _ := cloneBehindOther(t)
	runSteps(t, clone, [][]string{
		{"-C", clone, "fetch"},
		{"-C", clone, "commit", "--allow-empty", "--message", "Local"},
		{"-C", clone, "switch", "--create", "feature"},
		{"-C", clone, "push", "--set-upstream", "origin", "feature"},
		{"-C", clone, "push", "origin", "--delete", "feature"},
		{"-C", clone, "switch", "--create", "fix/local"},
	})

	branches, err := Repository{Dir: clone}.Branches(context.Background())

	want := []Branch{
		{Name: "feature", Tracking: Tracking{Upstream: "origin/feature", UpstreamGone: true}},
		{Name: "fix/local", Current: true},
		{Name: "main", Tracking: Tracking{Upstream: "origin/main", Ahead: 1, Behind: 1}},
	}
	if err != nil || !reflect.DeepEqual(branches, want) {
		t.Fatalf("got %+v, %v, want %+v", branches, err, want)
	}
}

func TestBranchesNamesTheOtherWorkTreeABranchIsCheckedOutIn(t *testing.T) {
	_, clone := pushedClone(t)
	side := filepath.Join(t.TempDir(), "side")
	runSteps(t, clone, [][]string{{"-C", clone, "worktree", "add", "-b", "side", side}})

	branches, err := Repository{Dir: clone}.Branches(context.Background())

	want := []Branch{
		{Name: "main", Current: true, Tracking: Tracking{Upstream: "origin/main"}},
		{Name: "side", OtherWorkTree: filepath.ToSlash(side)},
	}
	if err != nil || !reflect.DeepEqual(branches, want) {
		t.Fatalf("got %+v, %v, want %+v", branches, err, want)
	}
}

func TestDeleteBranchDeletesABranchWithChangesTheDefaultBranchLacks(t *testing.T) {
	clone := cloneWithFeatureBranch(t)

	err := Repository{Dir: clone}.DeleteBranch(context.Background(), "feature")

	if err != nil || slices.Contains(branchNames(t, clone), "feature") {
		t.Fatalf("got %v, want feature deleted", err)
	}
}

func cloneWithFeatureBranch(t *testing.T) string {
	t.Helper()
	_, clone := pushedClone(t)
	runSteps(t, clone, [][]string{{"-C", clone, "switch", "--create", "feature"}})
	writeFile(t, filepath.Join(clone, "feature.txt"), "feature\n")
	runSteps(t, clone, [][]string{
		{"-C", clone, "add", "feature.txt"},
		{"-C", clone, "commit", "--message", "Feature"},
		{"-C", clone, "switch", "main"},
	})
	return clone
}

func branchNames(t *testing.T, dir string) []string {
	t.Helper()
	branches, err := Repository{Dir: dir}.Branches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, branch := range branches {
		names = append(names, branch.Name)
	}
	return names
}
