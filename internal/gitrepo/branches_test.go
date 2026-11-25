package gitrepo

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
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

	require.NoError(t, err)
	want := []Branch{
		{Name: "feature", Tracking: Tracking{Upstream: "origin/feature", UpstreamGone: true}},
		{Name: "fix/local", Current: true},
		{Name: "main", Tracking: Tracking{Upstream: "origin/main", Ahead: 1, Behind: 1}},
	}
	require.Equal(t, want, branches)
}

func TestBranchesNamesTheOtherWorkTreeABranchIsCheckedOutIn(t *testing.T) {
	_, clone := pushedClone(t)
	side := filepath.Join(t.TempDir(), "side")
	runSteps(t, clone, [][]string{{"-C", clone, "worktree", "add", "-b", "side", side}})

	branches, err := Repository{Dir: clone}.Branches(context.Background())

	require.NoError(t, err)
	want := []Branch{
		{Name: "main", Current: true, Tracking: Tracking{Upstream: "origin/main"}},
		{Name: "side", OtherWorkTree: filepath.ToSlash(side)},
	}
	require.Equal(t, want, branches)
}

func TestRemoteBranchesListsEachRemoteBranchButNotARemoteHEAD(t *testing.T) {
	clone := cloneWithTwoRemotes(t)

	branches, err := Repository{Dir: clone}.RemoteBranches(context.Background())

	require.NoError(t, err)
	want := []RemoteBranch{
		{Ref: "fork/fix/x", Name: "fix/x"},
		{Ref: "fork/main", Name: "main"},
		{Ref: "origin/fix/x", Name: "fix/x"},
		{Ref: "origin/main", Name: "main"},
	}
	require.Equal(t, want, branches)
}

func TestRemoteBranchesNamesABranchOfARemoteNamedWithASlash(t *testing.T) {
	remote, clone := pushedClone(t)
	runSteps(t, clone, [][]string{
		{"-C", clone, "remote", "add", "team/fork", remote},
		{"-C", clone, "fetch", "team/fork"},
	})

	branches, err := Repository{Dir: clone}.RemoteBranches(context.Background())

	require.NoError(t, err)
	want := []RemoteBranch{{Ref: "origin/main", Name: "main"}, {Ref: "team/fork/main", Name: "main"}}
	require.Equal(t, want, branches)
}

func TestSwitchTrackingCreatesTheBranchFromTheNamedRemoteBranch(t *testing.T) {
	clone := cloneWithTwoRemotes(t)
	repository := Repository{Dir: clone}

	err := repository.SwitchTracking(context.Background(), "fork/fix/x")

	require.NoError(t, err)
	branches, branchesErr := repository.Branches(context.Background())
	require.NoError(t, branchesErr)
	want := Branch{Name: "fix/x", Current: true, Tracking: Tracking{Upstream: "fork/fix/x"}}
	require.Contains(t, branches, want)
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

func TestCreateBranchStartsAtTheStartPointWithNoUpstreamAndKeepsUncommittedChanges(t *testing.T) {
	_, clone := pushedClone(t)
	runSteps(t, clone, [][]string{{"-C", clone, "commit", "--allow-empty", "--message", "Local"}})
	writeFile(t, filepath.Join(clone, "edit.txt"), "edit\n")
	repository := Repository{Dir: clone}

	err := repository.CreateBranch(context.Background(), "feature", "origin/main")

	require.NoError(t, err)
	branches, branchesErr := repository.Branches(context.Background())
	require.NoError(t, branchesErr)
	want := Branch{Name: "feature", Current: true}
	require.Contains(t, branches, want)
	head := revParse(t, clone, "HEAD")
	start := revParse(t, clone, "origin/main")
	require.Equal(t, start, head)
	uncommitted := gitOutput(t, clone, "status", "--porcelain")
	require.Equal(t, "?? edit.txt\n", uncommitted)
}

func TestCreateBranch(t *testing.T) {
	t.Run("createBranchRefusesATakenName", createBranchRefusesATakenName)
	t.Run("createBranchRefusesANameWithTwoDots", createBranchRefusesANameWithTwoDots)
	t.Run("createBranchRefusesANameWithASpace", createBranchRefusesANameWithASpace)
}

func createBranchRefusesATakenName(t *testing.T) {
	_, clone := pushedClone(t)

	err := Repository{Dir: clone}.CreateBranch(context.Background(), "main", "origin/main")

	require.Error(t, err)
}

func createBranchRefusesANameWithTwoDots(t *testing.T) {
	_, clone := pushedClone(t)

	err := Repository{Dir: clone}.CreateBranch(context.Background(), "bad..name", "origin/main")

	require.Error(t, err)
}

func createBranchRefusesANameWithASpace(t *testing.T) {
	_, clone := pushedClone(t)

	err := Repository{Dir: clone}.CreateBranch(context.Background(), "has space", "origin/main")

	require.Error(t, err)
}

func TestDeleteBranchDeletesABranchWithChangesTheDefaultBranchLacks(t *testing.T) {
	_, clone := pushedClone(t)
	runSteps(t, clone, [][]string{{"-C", clone, "switch", "--create", "feature"}})
	writeFile(t, filepath.Join(clone, "feature.txt"), "feature\n")
	runSteps(t, clone, [][]string{
		{"-C", clone, "add", "feature.txt"},
		{"-C", clone, "commit", "--message", "Feature"},
		{"-C", clone, "switch", "main"},
	})
	repository := Repository{Dir: clone}

	err := repository.DeleteBranch(context.Background(), "feature")

	require.NoError(t, err)
	branches, branchesErr := repository.Branches(context.Background())
	require.NoError(t, branchesErr)
	isFeature := func(branch Branch) bool { return branch.Name == "feature" }
	featureListed := slices.ContainsFunc(branches, isFeature)
	require.False(t, featureListed)
}
