package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestDeleteBranch(t *testing.T) {
	t.Run("aMergedBranchIsDeletedAfterAConfirmAndTheListReloadsInPlace",
		aMergedBranchIsDeletedAfterAConfirmAndTheListReloadsInPlace)
	t.Run("anUnmergedBranchIsDeletedAfterAWarning", anUnmergedBranchIsDeletedAfterAWarning)
	t.Run("refusesTheCurrentBranch", refusesTheCurrentBranch)
	t.Run("refusesTheDefaultBranch", refusesTheDefaultBranch)
	t.Run("refusesABranchWhenTheDefaultBranchIsUnknown", refusesABranchWhenTheDefaultBranchIsUnknown)
	t.Run("refusesABranchCheckedOutInAnotherWorkTree", refusesABranchCheckedOutInAnotherWorkTree)
	t.Run("refusesARemoteBranch", refusesARemoteBranch)
}

func aMergedBranchIsDeletedAfterAConfirmAndTheListReloadsInPlace(t *testing.T) {
	branches := branchesOf(t, gitrepo.Status{}, originMain())
	model := selectRow(t, branches, 2)
	screen := plainScreen(model)
	model = update(t, model, tea.KeyPressMsg{Code: 'D', Text: "D"})
	popup := plainConfirmPopup(model)
	next, deleting := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)
	next, reloading := model.Update(popupSuccessMsg{dir: "C:/dev/repo", notice: "deleted done"})
	model = asModel(t, next)
	rows := sixBranches()
	model = update(t, model, branchesMsg{
		dir: "C:/dev/repo", rows: []branchRow{rows[0], rows[1], rows[3]}, defaultBranch: originMain()})
	selected := model.branches.rows[model.branches.selected]

	require.Contains(t, screen, "origin/done deleted")
	require.Contains(t, screen, "merged into origin/main")
	require.Contains(t, screen, "in work tree side")
	require.Contains(t, popup, "delete done")
	require.Contains(t, popup, "origin/main has all its changes")
	require.NotNil(t, deleting)
	require.NotNil(t, reloading)
	require.Equal(t, noPopup, model.popup.kind)
	require.Equal(t, "deleted done", model.footer.text)
	require.Equal(t, "wip", selected.branch.Name)
}

func anUnmergedBranchIsDeletedAfterAWarning(t *testing.T) {
	branches := branchesOf(t, gitrepo.Status{}, originMain())
	model := selectRow(t, branches, 3)
	model = update(t, model, tea.KeyPressMsg{Code: 'D', Text: "D"})
	popup := plainConfirmPopup(model)

	_, deleting := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.Contains(t, popup, "delete wip")
	require.Contains(t, popup, "origin/main lacks some of its changes")
	require.NotNil(t, deleting)
}

func refusesTheCurrentBranch(t *testing.T) {
	model := pressDeleteOnRow(t, originMain(), 0)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "fix is checked out, so it cannot be deleted", model.footer.text)
}

func refusesTheDefaultBranch(t *testing.T) {
	model := pressDeleteOnRow(t, originMain(), 1)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "main is the default branch, which perusal never deletes", model.footer.text)
}

func refusesABranchWhenTheDefaultBranchIsUnknown(t *testing.T) {
	model := pressDeleteOnRow(t, gitrepo.RemoteBranch{}, 2)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "origin's default branch is unknown, so perusal cannot rule out that done is it", model.footer.text)
}

func refusesABranchCheckedOutInAnotherWorkTree(t *testing.T) {
	model := pressDeleteOnRow(t, originMain(), 4)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "side is checked out in C:/dev/repo/.claude/worktrees/side, so it cannot be deleted", model.footer.text)
}

func refusesARemoteBranch(t *testing.T) {
	model := pressDeleteOnRow(t, originMain(), 5)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "origin/fix-x is a remote branch, which perusal never deletes", model.footer.text)
}

func pressDeleteOnRow(t *testing.T, defaultBranch gitrepo.RemoteBranch, moves int) Model {
	t.Helper()
	branches := branchesOf(t, gitrepo.Status{}, defaultBranch)
	model := selectRow(t, branches, moves)
	return update(t, model, tea.KeyPressMsg{Code: 'D', Text: "D"})
}

func TestTheCurrentBranchMergedMark(t *testing.T) {
	t.Run("theCurrentBranchShowsMergedWhenClean", theCurrentBranchShowsMergedWhenClean)
	t.Run("theCurrentBranchHidesMergedWithUncommittedChanges", theCurrentBranchHidesMergedWithUncommittedChanges)
}

func theCurrentBranchShowsMergedWhenClean(t *testing.T) {
	model := currentBranchMergedIntoDefault(t, []gitrepo.FileStatus{})

	screen := plainScreen(model)

	require.Contains(t, screen, "merged into origin/main")
}

func theCurrentBranchHidesMergedWithUncommittedChanges(t *testing.T) {
	model := currentBranchMergedIntoDefault(t, twoFiles())

	screen := plainScreen(model)

	require.NotContains(t, screen, "merged into origin/main")
}

func currentBranchMergedIntoDefault(t *testing.T, files []gitrepo.FileStatus) Model {
	t.Helper()
	model := reviewingRepository(t, gitrepo.Status{Branch: "fix", Files: files})
	model = update(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	return update(t, model, branchesMsg{
		dir:           "C:/dev/repo",
		rows:          []branchRow{{branch: gitrepo.Branch{Name: "fix", Current: true}, mergedIntoDefault: true}},
		defaultBranch: originMain(),
	})
}

func TestSwitchBranch(t *testing.T) {
	t.Run("toALocalBranchIsRefusedWithUncommittedChanges", toALocalBranchIsRefusedWithUncommittedChanges)
	t.Run("toARemoteBranchIsRefusedWithUncommittedChanges", toARemoteBranchIsRefusedWithUncommittedChanges)
	t.Run("closesTheListAndReloadsTheStatus", closesTheListAndReloadsTheStatus)
	t.Run("toARemoteBranchCreatesItsLocalBranch", toARemoteBranchCreatesItsLocalBranch)
}

func toALocalBranchIsRefusedWithUncommittedChanges(t *testing.T) {
	branches := branchesOf(t, gitrepo.Status{Branch: "fix", Files: twoFiles()}, originMain())
	model := selectRow(t, branches, 3)

	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)

	require.Nil(t, cmd)
	require.True(t, model.footer.isError)
	require.Equal(t, modeBranches, model.mode)
}

func toARemoteBranchIsRefusedWithUncommittedChanges(t *testing.T) {
	branches := branchesOf(t, gitrepo.Status{Branch: "fix", Files: twoFiles()}, originMain())
	model := selectRow(t, branches, 5)

	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)

	require.Nil(t, cmd)
	require.True(t, model.footer.isError)
	require.Equal(t, modeBranches, model.mode)
}

func toARemoteBranchCreatesItsLocalBranch(t *testing.T) {
	branches := branchesOf(t, gitrepo.Status{Branch: "fix"}, originMain())
	model := selectRow(t, branches, 5)
	screen := plainScreen(model)

	_, switching := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.Contains(t, screen, "origin/fix-x")
	require.NotNil(t, switching)
}

func TestFetchInTheBranchListReloadsIt(t *testing.T) {
	model := branchesOf(t, gitrepo.Status{Branch: "fix"}, originMain())

	next, fetching := model.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	model = asModel(t, next)
	fetchingText := model.footer.text
	next, reloading := model.Update(branchesFetchedMsg{dir: "C:/dev/repo"})
	model = asModel(t, next)

	require.NotNil(t, fetching)
	require.Equal(t, "fetching...", fetchingText)
	require.NotNil(t, reloading)
	require.Equal(t, modeBranches, model.mode)
	require.Equal(t, "fetched", model.footer.text)
}

func closesTheListAndReloadsTheStatus(t *testing.T) {
	branches := branchesOf(t, gitrepo.Status{Branch: "fix"}, originMain())
	model := selectRow(t, branches, 3)

	next, switching := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)
	next, reloading := model.Update(branchSwitchMsg{dir: "C:/dev/repo", branch: "wip"})
	model = asModel(t, next)

	require.NotNil(t, switching)
	require.NotNil(t, reloading)
	require.NotEqual(t, modeBranches, model.mode)
	require.Equal(t, "switched to wip", model.footer.text)
}

func TestNewBranch(t *testing.T) {
	t.Run("isCreatedFromTheDefaultBranchWithTheUncommittedFilesAndClosesTheList",
		isCreatedFromTheDefaultBranchWithTheUncommittedFilesAndClosesTheList)
	t.Run("isRefusedWhenTheDefaultBranchIsUnknown", isRefusedWhenTheDefaultBranchIsUnknown)
	t.Run("needsAName", needsAName)
	t.Run("showsGitsRefusalInThePopup", showsGitsRefusalInThePopup)
}

func isCreatedFromTheDefaultBranchWithTheUncommittedFilesAndClosesTheList(t *testing.T) {
	model := branchesOf(t, gitrepo.Status{Branch: "fix", Files: twoFiles()}, originMain())
	model = update(t, model, tea.KeyPressMsg{Code: 'n', Text: "n"})
	model = typeText(t, model, "feat")
	renderedPopup := model.renderNewBranchPopup()
	popup := ansi.Strip(renderedPopup)

	next, creating := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)
	running := model.popup.running
	next, reloading := model.Update(branchSwitchMsg{dir: "C:/dev/repo", branch: "feat"})
	model = asModel(t, next)

	require.Contains(t, popup, "new branch from origin/main")
	require.Contains(t, popup, "feat")
	require.Contains(t, popup, "takes along 2 uncommitted files")
	require.NotNil(t, creating)
	require.True(t, running)
	require.NotNil(t, reloading)
	require.Equal(t, noPopup, model.popup.kind)
	require.NotEqual(t, modeBranches, model.mode)
	require.Equal(t, "switched to feat", model.footer.text)
}

func isRefusedWhenTheDefaultBranchIsUnknown(t *testing.T) {
	model := branchesOf(t, gitrepo.Status{Branch: "fix"}, gitrepo.RemoteBranch{})

	model = update(t, model, tea.KeyPressMsg{Code: 'n', Text: "n"})

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
}

func needsAName(t *testing.T) {
	model := branchesOf(t, gitrepo.Status{Branch: "fix"}, originMain())
	model = update(t, model, tea.KeyPressMsg{Code: 'n', Text: "n"})

	next, creating := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)

	require.Nil(t, creating)
	require.Equal(t, newBranchPopup, model.popup.kind)
	require.Equal(t, "the name is required", model.popup.err)
}

func showsGitsRefusalInThePopup(t *testing.T) {
	model := branchesOf(t, gitrepo.Status{Branch: "fix"}, originMain())
	model = update(t, model, tea.KeyPressMsg{Code: 'n', Text: "n"})
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	model = update(t, model, popupFailureMsg{dir: "C:/dev/repo", err: errors.New("a branch named 'main' already exists")})

	require.Equal(t, newBranchPopup, model.popup.kind)
	require.False(t, model.popup.running)
	require.Contains(t, model.popup.err, "already exists")
}

func TestRemoteOnlyBranchesDropsEachOneALocalBranchShadows(t *testing.T) {
	local := []gitrepo.Branch{{Name: "main"}, {Name: "fix/y"}}
	remote := []gitrepo.RemoteBranch{
		{Ref: "origin/main", Name: "main"},
		{Ref: "origin/fix/x", Name: "fix/x"},
		{Ref: "team/fork/fix/y", Name: "fix/y"},
		{Ref: "team/fork/main-old", Name: "main-old"},
	}

	got := remoteOnlyBranches(local, remote)

	want := []gitrepo.RemoteBranch{remote[1], remote[3]}
	require.Equal(t, want, got)
}

func originMain() gitrepo.RemoteBranch {
	return gitrepo.RemoteBranch{Ref: "origin/main", Name: "main"}
}

func branchesOf(t *testing.T, status gitrepo.Status, defaultBranch gitrepo.RemoteBranch) Model {
	t.Helper()
	model := reviewingRepository(t, status)
	model = update(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	return update(t, model, branchesMsg{dir: "C:/dev/repo", rows: sixBranches(), defaultBranch: defaultBranch})
}

func selectRow(t *testing.T, model Model, moves int) Model {
	t.Helper()
	for range moves {
		model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	return model
}

func sixBranches() []branchRow {
	return []branchRow{
		{branch: gitrepo.Branch{Name: "fix", Current: true}},
		{branch: gitrepo.Branch{Name: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main"}}},
		{
			branch: gitrepo.Branch{
				Name: "done", Tracking: gitrepo.Tracking{Upstream: "origin/done", UpstreamGone: true},
			},
			mergedIntoDefault: true,
		},
		{branch: gitrepo.Branch{Name: "wip"}},
		{branch: gitrepo.Branch{Name: "side", OtherWorkTree: "C:/dev/repo/.claude/worktrees/side"}},
		{remoteBranch: gitrepo.RemoteBranch{Ref: "origin/fix-x", Name: "fix-x"}},
	}
}
