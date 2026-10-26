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

func TestRemoteBranchesListsEachRemoteBranchButNotARemoteHEAD(t *testing.T) {
	clone := cloneWithTwoRemotes(t)

	branches, err := Repository{Dir: clone}.RemoteBranches(context.Background())

	want := []RemoteBranch{
		{Ref: "fork/fix/x", Name: "fix/x"},
		{Ref: "fork/main", Name: "main"},
		{Ref: "origin/fix/x", Name: "fix/x"},
		{Ref: "origin/main", Name: "main"},
	}
	if err != nil || !reflect.DeepEqual(branches, want) {
		t.Fatalf("got %+v, %v, want %+v", branches, err, want)
	}
}

func TestSwitchTrackingCreatesTheBranchFromTheNamedRemoteBranch(t *testing.T) {
	clone := cloneWithTwoRemotes(t)
	repository := Repository{Dir: clone}

	err := repository.SwitchTracking(context.Background(), "fork/fix/x")

	branches, branchesErr := repository.Branches(context.Background())
	want := Branch{Name: "fix/x", Current: true, Tracking: Tracking{Upstream: "fork/fix/x"}}
	if err != nil || branchesErr != nil || !slices.Contains(branches, want) {
		t.Fatalf("got %v, %+v (%v), want %+v", err, branches, branchesErr, want)
	}
}

func cloneWithTwoRemotes(t *testing.T) string {
	t.Helper()
	remote, clone := pushedClone(t)
	runSteps(t, clone, [][]string{
		{"-C", clone, "push", "origin", "main:fix/x"},
		{"-C", clone, "remote", "add", "fork", remote},
		{"-C", clone, "fetch", "fork"},
		{"-C", clone, "remote", "set-head", "fork", "main"},
	})
	return clone
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
