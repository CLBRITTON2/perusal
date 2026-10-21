package gitrepo

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestBranchesListsEachLocalBranchWithItsUpstream(t *testing.T) {
	work, _ := cloneBehindOther(t)
	runSteps(t, work, [][]string{
		{"-C", work, "fetch"},
		{"-C", work, "commit", "--allow-empty", "--message", "Local"},
		{"-C", work, "switch", "--create", "feature"},
		{"-C", work, "push", "--set-upstream", "origin", "feature"},
		{"-C", work, "push", "origin", "--delete", "feature"},
		{"-C", work, "switch", "--create", "fix/local"},
	})

	branches, err := Repository{Dir: work}.Branches(context.Background())

	want := []Branch{
		{Name: "feature", Upstream: "origin/feature", UpstreamGone: true},
		{Name: "fix/local", Current: true},
		{Name: "main", Upstream: "origin/main", Ahead: 1, Behind: 1},
	}
	if err != nil || !reflect.DeepEqual(branches, want) {
		t.Fatalf("got %+v, %v, want %+v", branches, err, want)
	}
}

func TestDeleteBranchDeletesABranchWithChangesTheDefaultBranchLacks(t *testing.T) {
	work := withFeatureBranch(t)

	err := Repository{Dir: work}.DeleteBranch(context.Background(), "feature")

	if err != nil || slices.Contains(branchNames(t, work), "feature") {
		t.Fatalf("got %v, want feature deleted", err)
	}
}

// work is back on main, and feature adds a file main lacks.
func withFeatureBranch(t *testing.T) string {
	t.Helper()
	_, work := pushedClone(t)
	runSteps(t, work, [][]string{{"-C", work, "switch", "--create", "feature"}})
	writeFile(t, filepath.Join(work, "feature.txt"), "feature\n")
	runSteps(t, work, [][]string{
		{"-C", work, "add", "feature.txt"},
		{"-C", work, "commit", "--message", "Feature"},
		{"-C", work, "switch", "main"},
	})
	return work
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
