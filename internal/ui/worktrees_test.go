package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestWorkTreeList(t *testing.T) {
	t.Run("namesEachWorkTreeWithItsBranchAndChanges", namesEachWorkTreeWithItsBranchAndChanges)
	t.Run("enterReviewsTheSelectedWorkTree", enterReviewsTheSelectedWorkTree)
	t.Run("enterOnARefusedWorkTreeShowsWhy", enterOnARefusedWorkTreeShowsWhy)
	t.Run("enterOnTheWorkTreeUnderReviewClosesTheList", enterOnTheWorkTreeUnderReviewClosesTheList)
}

func namesEachWorkTreeWithItsBranchAndChanges(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	next, loading := model.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	model = asModel(t, next)
	model = update(t, model, workTreesMsg{dir: "C:/dev/repo", rows: fourWorkTrees()})

	screen := plainScreen(model)

	markCount := strings.Count(screen, "*")
	markIndex := strings.Index(screen, "*")
	afterMark := strings.TrimLeft(screen[markIndex+1:], " ")
	repoMarked := strings.HasPrefix(afterMark, folderGlyph+" repo ")
	require.NotNil(t, loading)
	require.Equal(t, 1, markCount)
	require.True(t, repoMarked, screen)
	for _, want := range []string{
		branchGlyph + " main", "clean",
		folderGlyph + " repo-side", branchGlyph + " fix", "2 changed",
		folderGlyph + " repo/.claude/worktrees/agent", branchGlyph + " detached HEAD",
		folderGlyph + " D:/elsewhere/repo-out", "not write-denied to the agent",
	} {
		require.Contains(t, screen, want)
	}
}

func enterReviewsTheSelectedWorkTree(t *testing.T) {
	workTrees := workTreesOf(t)
	model := selectRow(t, workTrees, 1)

	next, reloading := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)

	require.NotNil(t, reloading)
	require.Equal(t, modeBrowsing, model.mode)
	require.Equal(t, "C:/dev/repo-side", model.repository.Dir)
}

func enterOnARefusedWorkTreeShowsWhy(t *testing.T) {
	workTrees := workTreesOf(t)
	model := selectRow(t, workTrees, 3)

	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)

	require.Nil(t, cmd)
	require.Equal(t, modeWorkTrees, model.mode)
	require.True(t, model.footer.isError)
	require.Equal(t, "D:/elsewhere/repo-out is refused: "+focus.ErrGitDirUnprotected.Error(), model.footer.text)
}

func enterOnTheWorkTreeUnderReviewClosesTheList(t *testing.T) {
	model := workTreesOf(t)

	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)

	require.Nil(t, cmd)
	require.Equal(t, modeBrowsing, model.mode)
	require.Equal(t, "C:/dev/repo", model.repository.Dir)
}

func TestEnterOnABranchInAnotherWorkTreeReviewsThatWorkTree(t *testing.T) {
	branches := branchesOf(t, gitrepo.Status{Branch: "fix", Files: twoFiles()}, originMain())
	model := selectRow(t, branches, 4)

	_, opening := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	opened := opening()
	next, reloading := model.Update(opened)
	model = asModel(t, next)

	require.NotNil(t, reloading)
	require.Equal(t, modeBrowsing, model.mode)
	require.Equal(t, "C:/dev/repo/.claude/worktrees/side", model.repository.Dir)
}

func TestMissingWorkTreeOffer(t *testing.T) {
	t.Run("cCreatesTheOfferedWorkTreeAndClosesThePopupWithItsBase", cCreatesTheOfferedWorkTreeAndClosesThePopupWithItsBase)
	t.Run("enterLeavesTheOfferOpenWithoutCreating", enterLeavesTheOfferOpenWithoutCreating)
	t.Run("escClosesTheOfferAndItIsNotMadeAgainThisRun", escClosesTheOfferAndItIsNotMadeAgainThisRun)
}

func cCreatesTheOfferedWorkTreeAndClosesThePopupWithItsBase(t *testing.T) {
	created := []string{}
	model := offeringMissingWorkTree(t, func(_ context.Context, missing focus.MissingWorkTree) (string, error) {
		created = append(created, missing.Dir)
		return "0123456789abcdef0123456789abcdef01234567", nil
	})
	popup := plainConfirmPopup(model)

	next, creating := model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = asModel(t, next)
	createdMsg := creating()
	model = update(t, model, createdMsg)

	require.Contains(t, popup, "create worktree worktrees/repo/claude-x")
	require.Equal(t, []string{"C:/dev/worktrees/repo/claude-x"}, created)
	require.Equal(t, noPopup, model.popup.kind)
	require.Equal(t, "created worktree worktrees/repo/claude-x at 0123456", model.footer.text)
}

func enterLeavesTheOfferOpenWithoutCreating(t *testing.T) {
	model := offeringMissingWorkTree(t, refuseToCreate)

	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)

	require.Nil(t, cmd)
	require.Equal(t, confirmPopup, model.popup.kind)
	require.False(t, model.popup.running)
}

func escClosesTheOfferAndItIsNotMadeAgainThisRun(t *testing.T) {
	model := offeringMissingWorkTree(t, refuseToCreate)

	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	model = update(t, model, focus.Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{},
		MissingWorkTrees: []focus.MissingWorkTree{missingClaudeX()}})
	model = update(t, model, pollTickMsg{})

	require.Equal(t, noPopup, model.popup.kind)
}

func offeringMissingWorkTree(t *testing.T, createWorkTree WorkTreeCreator) Model {
	t.Helper()
	model := New(findNothing, openAnything, createWorkTree, AgentHost{})
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model = update(t, model, focus.Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{},
		MissingWorkTrees: []focus.MissingWorkTree{missingClaudeX()}})
	model = update(t, model, pollTickMsg{})
	require.Equal(t, confirmPopup, model.popup.kind)
	return model
}

func missingClaudeX() focus.MissingWorkTree {
	return focus.MissingWorkTree{
		AgentDir:     "C:/agent/dev/worktrees/repo/claude-x",
		Dir:          "C:/dev/worktrees/repo/claude-x",
		RelativePath: "worktrees/repo/claude-x",
		Clone:        gitrepo.Repository{Dir: "C:/dev/repo"},
	}
}

func refuseToCreate(context.Context, focus.MissingWorkTree) (string, error) {
	return "", errors.New("refuseToCreate creates nothing")
}

func workTreesOf(t *testing.T) Model {
	t.Helper()
	model := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	model = update(t, model, tea.KeyPressMsg{Code: 'w', Text: "w"})
	return update(t, model, workTreesMsg{dir: "C:/dev/repo", rows: fourWorkTrees()})
}

func fourWorkTrees() []workTreeRow {
	return []workTreeRow{
		{
			workTree:     gitrepo.WorkTree{Path: "C:/dev/repo", Branch: "main"},
			repository:   gitrepo.Repository{Dir: "C:/dev/repo"},
			changedCount: 0,
		},
		{
			workTree:     gitrepo.WorkTree{Path: "C:/dev/repo-side", Branch: "fix"},
			repository:   gitrepo.Repository{Dir: "C:/dev/repo-side"},
			changedCount: 2,
		},
		{
			workTree:   gitrepo.WorkTree{Path: "C:/dev/repo/.claude/worktrees/agent", Branch: ""},
			repository: gitrepo.Repository{Dir: "C:/dev/repo/.claude/worktrees/agent"},
		},
		{
			workTree: gitrepo.WorkTree{Path: "D:/elsewhere/repo-out", Branch: "out"},
			refusal:  focus.ErrGitDirUnprotected,
		},
	}
}

func openAnything(_ context.Context, dir string) (gitrepo.Repository, error) {
	return gitrepo.Repository{Dir: dir}, nil
}

func TestDeleteWorkTree(t *testing.T) {
	t.Run("anUnmergedWorkTreeWarnsAndNamesTheAgentsCopy", anUnmergedWorkTreeWarnsAndNamesTheAgentsCopy)
	t.Run("aMergedWorkTreeWithNoAgentCopySaysItIsInTheDefaultBranch",
		aMergedWorkTreeWithNoAgentCopySaysItIsInTheDefaultBranch)
	t.Run("aFailedAgentRemoveStopsTheDeleteAndShowsGitsOutput", aFailedAgentRemoveStopsTheDeleteAndShowsGitsOutput)
	t.Run("aDeleteClosesThePopupAndReloadsTheList", aDeleteClosesThePopupAndReloadsTheList)
	t.Run("refusesTheMainWorkTreeWithoutRunningGit", refusesTheMainWorkTreeWithoutRunningGit)
	t.Run("refusesTheWorkTreeUnderReviewWithoutRunningGit", refusesTheWorkTreeUnderReviewWithoutRunningGit)
	t.Run("refusesADetachedHeadWithoutRunningGit", refusesADetachedHeadWithoutRunningGit)
	t.Run("refusesARefusedWorkTreeWithoutRunningGit", refusesARefusedWorkTreeWithoutRunningGit)
	t.Run("refusesUncommittedChanges", refusesUncommittedChanges)
	t.Run("refusesWhenTheDefaultBranchIsUnknown", refusesWhenTheDefaultBranchIsUnknown)
	t.Run("refusesAWorkTreeOnTheDefaultBranch", refusesAWorkTreeOnTheDefaultBranch)
	t.Run("refusesUncommittedChangesInTheAgentsCopy", refusesUncommittedChangesInTheAgentsCopy)
	t.Run("refusesTheAgentsCopyOnAnotherBranch", refusesTheAgentsCopyOnAnotherBranch)
	t.Run("refusesAgentCommitsPerusalHasNotTaken", refusesAgentCommitsPerusalHasNotTaken)
	t.Run("dropsACheckForThePreviousRepository", dropsACheckForThePreviousRepository)
	t.Run("dropsACheckThatArrivesAfterTheWorkTreesScreenClosed", dropsACheckThatArrivesAfterTheWorkTreesScreenClosed)
	t.Run("dropsACheckThatArrivesWhileAPopupIsOpen", dropsACheckThatArrivesWhileAPopupIsOpen)
}

func refusesTheMainWorkTreeWithoutRunningGit(t *testing.T) {
	model, cmd := pressDeleteOnWorkTree(t, 0, "C:/dev/repo")

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "repo is the main work tree, which perusal never deletes", model.footer.text)
}

func refusesTheWorkTreeUnderReviewWithoutRunningGit(t *testing.T) {
	model, cmd := pressDeleteOnWorkTree(t, 1, "C:/dev/repo-side")

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "repo-side is under review, so it cannot be deleted", model.footer.text)
}

func refusesADetachedHeadWithoutRunningGit(t *testing.T) {
	model, cmd := pressDeleteOnWorkTree(t, 2, "C:/dev/repo")

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "repo/.claude/worktrees/agent has a detached HEAD, so it cannot be deleted", model.footer.text)
}

func refusesARefusedWorkTreeWithoutRunningGit(t *testing.T) {
	model, cmd := pressDeleteOnWorkTree(t, 3, "C:/dev/repo")

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "D:/elsewhere/repo-out is refused: "+focus.ErrGitDirUnprotected.Error(), model.footer.text)
}

func pressDeleteOnWorkTree(t *testing.T, moves int, underReview string) (Model, tea.Cmd) {
	t.Helper()
	workTrees := workTreesOf(t)
	model := selectRow(t, workTrees, moves)
	model.repository.Dir = underReview
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	return asModel(t, next), cmd
}

func refusesUncommittedChanges(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)
	check.uncommitted = true

	model := receiveDeleteCheckOnRepoSide(t, check)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "repo-side has uncommitted changes, so it cannot be deleted", model.footer.text)
}

func refusesWhenTheDefaultBranchIsUnknown(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)
	check.defaultBranch = gitrepo.RemoteBranch{}

	model := receiveDeleteCheckOnRepoSide(t, check)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "origin's default branch is unknown, so perusal cannot rule out that fix is it", model.footer.text)
}

func refusesAWorkTreeOnTheDefaultBranch(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)
	check.defaultBranch = gitrepo.RemoteBranch{Ref: "origin/fix", Name: "fix"}

	model := receiveDeleteCheckOnRepoSide(t, check)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "fix is the default branch, which perusal never deletes", model.footer.text)
}

func refusesUncommittedChangesInTheAgentsCopy(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)
	check.request.agentWorkTree.Uncommitted = true

	model := receiveDeleteCheckOnRepoSide(t, check)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "repo-side has uncommitted changes in claude-vm, so it cannot be deleted", model.footer.text)
}

func refusesTheAgentsCopyOnAnotherBranch(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)
	check.request.agentWorkTree.Head.Branch = "refs/heads/other"

	model := receiveDeleteCheckOnRepoSide(t, check)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "repo-side is on other in claude-vm, so it cannot be deleted", model.footer.text)
}

func refusesAgentCommitsPerusalHasNotTaken(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)
	check.agentTaken = false

	model := receiveDeleteCheckOnRepoSide(t, check)

	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "claude-vm has commits perusal has not taken, so repo-side cannot be deleted", model.footer.text)
}

func receiveDeleteCheckOnRepoSide(t *testing.T, check workTreeDeleteCheckMsg) Model {
	t.Helper()
	workTrees := workTreesOf(t)
	model := selectRow(t, workTrees, 1)
	return update(t, model, check)
}

func dropsACheckForThePreviousRepository(t *testing.T) {
	workTrees := workTreesOf(t)
	model := selectRow(t, workTrees, 1)
	model.repository.Dir = "C:/dev/other"

	model = update(t, model, repoSideDeleteCheck(removeNothing))

	deleteTitle := titleStyle.Render("delete repo-side")
	require.NotContains(t, model.popup.confirmation.lines, deleteTitle)
	require.Empty(t, model.footer.text)
}

func dropsACheckThatArrivesAfterTheWorkTreesScreenClosed(t *testing.T) {
	workTrees := workTreesOf(t)
	model := selectRow(t, workTrees, 1)
	model.mode = modeBrowsing

	model = update(t, model, repoSideDeleteCheck(removeNothing))

	deleteTitle := titleStyle.Render("delete repo-side")
	require.NotContains(t, model.popup.confirmation.lines, deleteTitle)
	require.Empty(t, model.footer.text)
}

func dropsACheckThatArrivesWhileAPopupIsOpen(t *testing.T) {
	workTrees := workTreesOf(t)
	model := selectRow(t, workTrees, 1)
	model.popup.kind = confirmPopup

	model = update(t, model, repoSideDeleteCheck(removeNothing))

	deleteTitle := titleStyle.Render("delete repo-side")
	require.NotContains(t, model.popup.confirmation.lines, deleteTitle)
	require.Empty(t, model.footer.text)
}

func anUnmergedWorkTreeWarnsAndNamesTheAgentsCopy(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)

	model := receiveDeleteCheckOnRepoSide(t, check)

	want := []string{
		titleStyle.Render("delete repo-side"),
		warningStyle.Render("origin/main lacks some of fix"),
		textStyle.Render("also deletes the copy in claude-vm"),
	}
	require.Equal(t, confirmPopup, model.popup.kind)
	require.Equal(t, want, model.popup.confirmation.lines)
}

func aMergedWorkTreeWithNoAgentCopySaysItIsInTheDefaultBranch(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)
	check.request.agentHost = AgentHost{}
	check.request.agentWorkTree = gitrepo.AgentWorkTree{}
	check.mergedIntoDefault = true

	model := receiveDeleteCheckOnRepoSide(t, check)

	want := []string{titleStyle.Render("delete repo-side"), textStyle.Render("fix is in origin/main")}
	require.Equal(t, confirmPopup, model.popup.kind)
	require.Equal(t, want, model.popup.confirmation.lines)
}

func aFailedAgentRemoveStopsTheDeleteAndShowsGitsOutput(t *testing.T) {
	ownerClones := []string{}
	check := repoSideDeleteCheck(func(_ context.Context, ownerClone string, _ gitrepo.AgentWorkTree) error {
		ownerClones = append(ownerClones, ownerClone)
		return errors.New("fatal: cannot remove a locked working tree")
	})
	model := receiveDeleteCheckOnRepoSide(t, check)

	next, deleting := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)
	popup := plainConfirmPopup(model)
	deleted := deleting()
	model = update(t, model, deleted)

	require.Contains(t, popup, "deleting...")
	require.Equal(t, []string{"C:/dev/repo"}, ownerClones)
	require.Equal(t, confirmPopup, model.popup.kind)
	require.Equal(t, "fatal: cannot remove a locked working tree", model.popup.err)
}

func aDeleteClosesThePopupAndReloadsTheList(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)
	model := receiveDeleteCheckOnRepoSide(t, check)
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	next, reloading := model.Update(workTreeDeletedMsg{
		success: popupSuccessMsg{dir: "C:/dev/repo", notice: "deleted repo-side"},
	})
	model = asModel(t, next)
	reloadMsg := reloading()
	batch, isBatch := reloadMsg.(tea.BatchMsg)
	require.True(t, isBatch)
	reloads := []string{}
	for _, cmd := range batch {
		cmdMsg := cmd()
		if failure, isWorkTreeList := cmdMsg.(workTreesFailureMsg); isWorkTreeList {
			reloads = append(reloads, failure.dir)
		}
	}

	require.Equal(t, noPopup, model.popup.kind)
	require.Equal(t, modeWorkTrees, model.mode)
	require.Equal(t, "deleted repo-side", model.footer.text)
	require.Equal(t, []string{"C:/dev/repo"}, reloads)
}

func TestDeleteWorkTreeInTheOverview(t *testing.T) {
	t.Run("aLinkedWorkTreeRunsTheCheck", aLinkedWorkTreeRunsTheCheck)
	t.Run("aCheckOpensThePopupOnTheMainClone", aCheckOpensThePopupOnTheMainClone)
	t.Run("aDeleteReloadsTheOverview", aDeleteReloadsTheOverview)
	t.Run("overviewRefusesAMainCloneWithoutRunningGit", overviewRefusesAMainCloneWithoutRunningGit)
	t.Run("overviewRefusesTheWorkTreeUnderReviewWithoutRunningGit",
		overviewRefusesTheWorkTreeUnderReviewWithoutRunningGit)
	t.Run("overviewRefusesADetachedHeadWithoutRunningGit", overviewRefusesADetachedHeadWithoutRunningGit)
	t.Run("overviewRefusesAWorkTreeWhoseMainCloneIsNotLoadedWithoutRunningGit",
		overviewRefusesAWorkTreeWhoseMainCloneIsNotLoadedWithoutRunningGit)
	t.Run("overviewRefusesARefusedRowWithoutRunningGit", overviewRefusesARefusedRowWithoutRunningGit)
}

func overviewRefusesAMainCloneWithoutRunningGit(t *testing.T) {
	mainClone := repositoryRow{
		FoundRepository: focus.FoundRepository{RelativePath: "other", Repository: gitrepo.Repository{
			Dir: "C:/dev/other", GitDir: filepath.FromSlash("C:/dev/other/.git"),
		}},
		status: gitrepo.Status{Branch: "main", HasCommits: true},
	}

	model, cmd := pressDeleteInOverview(t, mainClone)

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "other is the main work tree, which perusal never deletes", model.footer.text)
}

func overviewRefusesTheWorkTreeUnderReviewWithoutRunningGit(t *testing.T) {
	underReview := linkedWorkTreeRow("C:/dev/first", "fix")

	model, cmd := pressDeleteInOverview(t, underReview)

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "worktrees/repo/fix is under review, so it cannot be deleted", model.footer.text)
}

func overviewRefusesADetachedHeadWithoutRunningGit(t *testing.T) {
	detached := linkedWorkTreeRow("C:/dev/worktrees/repo/fix", "")

	model, cmd := pressDeleteInOverview(t, detached)

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "worktrees/repo/fix has a detached HEAD, so it cannot be deleted", model.footer.text)
}

func overviewRefusesAWorkTreeWhoseMainCloneIsNotLoadedWithoutRunningGit(t *testing.T) {
	orphan := repositoryRow{
		FoundRepository: focus.FoundRepository{RelativePath: "worktrees/gone/fix", Repository: gitrepo.Repository{
			Dir: "C:/dev/worktrees/gone/fix", GitDir: filepath.FromSlash("C:/dev/gone/.git/worktrees/fix"),
		}},
		status: gitrepo.Status{Branch: "fix", HasCommits: true},
	}

	model, cmd := pressDeleteInOverview(t, orphan)

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "worktrees/gone/fix's main clone is not loaded in the overview", model.footer.text)
}

func overviewRefusesARefusedRowWithoutRunningGit(t *testing.T) {
	refused := refusedRow{FoundRefusal: focus.FoundRefusal{RelativePath: "worktrees/repo/fix",
		Err: focus.ErrGitDirUnprotected}}

	model, cmd := pressDeleteInOverview(t, refused)

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.True(t, model.footer.isError)
	require.Equal(t, "worktrees/repo/fix is refused: "+focus.ErrGitDirUnprotected.Error(), model.footer.text)
}

func pressDeleteInOverview(t *testing.T, target overviewRow) (Model, tea.Cmd) {
	t.Helper()
	model := overviewWithMainClone(t, target)
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	return asModel(t, next), cmd
}

// aLinkedWorkTreeRunsTheCheck runs the check's first git call, the work tree's status, which fails on the made up
// directory and so shows the check runs in the work tree.
func aLinkedWorkTreeRunsTheCheck(t *testing.T) {
	linked := linkedWorkTreeRow("C:/dev/worktrees/repo/fix", "fix")
	model := overviewWithMainClone(t, linked)

	next, cmd := model.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	model = asModel(t, next)

	require.NotNil(t, cmd)
	require.Empty(t, model.footer.text)
	checkMsg := cmd()
	failure, isFailure := checkMsg.(gitFailureMsg)
	require.True(t, isFailure)
	require.ErrorContains(t, failure.err, "C:/dev/worktrees/repo/fix")
}

func aCheckOpensThePopupOnTheMainClone(t *testing.T) {
	linked := linkedWorkTreeRow("C:/dev/worktrees/repo/fix", "fix")
	model := overviewWithMainClone(t, linked)

	model = update(t, model, overviewFixCheck())

	want := []string{titleStyle.Render("delete worktrees/repo/fix"), warningStyle.Render("origin/main lacks some of fix")}
	require.Equal(t, confirmPopup, model.popup.kind)
	require.Equal(t, "C:/dev/repo", model.popup.dir)
	require.Equal(t, want, model.popup.confirmation.lines)
}

func aDeleteReloadsTheOverview(t *testing.T) {
	linked := linkedWorkTreeRow("C:/dev/worktrees/repo/fix", "fix")
	model := overviewWithMainClone(t, linked)
	model = update(t, model, overviewFixCheck())
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	next, reloading := model.Update(workTreeDeletedMsg{
		success: popupSuccessMsg{dir: "C:/dev/repo", notice: "deleted worktrees/repo/fix"},
	})
	model = asModel(t, next)
	reloadMsg := reloading()
	batch, isBatch := reloadMsg.(tea.BatchMsg)
	require.True(t, isBatch)
	overviewReloads := 0
	for _, cmd := range batch {
		if cmd == nil {
			continue
		}
		cmdMsg := cmd()
		if _, isOverview := cmdMsg.(overviewMsg); isOverview {
			overviewReloads++
		}
	}

	require.Equal(t, noPopup, model.popup.kind)
	require.Equal(t, modeOverview, model.mode)
	require.Equal(t, "deleted worktrees/repo/fix", model.footer.text)
	require.Equal(t, 1, overviewReloads)
}

// overviewWithMainClone lists target first, then the main clone of C:/dev/repo, while C:/dev/first is under review.
func overviewWithMainClone(t *testing.T, target overviewRow) Model {
	t.Helper()
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	return update(t, model, overviewMsg{rows: []overviewRow{
		target,
		repositoryRow{
			FoundRepository: focus.FoundRepository{RelativePath: "repo", Repository: gitrepo.Repository{
				Dir: "C:/dev/repo", GitDir: filepath.FromSlash("C:/dev/repo/.git"),
			}},
			status:        gitrepo.Status{Branch: "main", HasCommits: true},
			defaultBranch: originMain(),
		},
	}})
}

// linkedWorkTreeRow is the work tree fix of C:/dev/repo at dir, on branch.
func linkedWorkTreeRow(dir string, branch string) repositoryRow {
	return repositoryRow{
		FoundRepository: focus.FoundRepository{RelativePath: "worktrees/repo/fix", Repository: gitrepo.Repository{
			Dir: dir, GitDir: filepath.FromSlash("C:/dev/repo/.git/worktrees/fix"),
		}},
		status: gitrepo.Status{Branch: branch, HasCommits: true},
	}
}

// overviewFixCheck passes every check of an overview delete of worktrees/repo/fix, which has no agent copy.
func overviewFixCheck() workTreeDeleteCheckMsg {
	return workTreeDeleteCheckMsg{
		request: workTreeDeleteRequest{
			repository: gitrepo.Repository{Dir: "C:/dev/repo"},
			row: workTreeRow{
				workTree:   gitrepo.WorkTree{Path: "C:/dev/worktrees/repo/fix", Branch: "fix"},
				repository: gitrepo.Repository{Dir: "C:/dev/worktrees/repo/fix"},
			},
			folder:        "worktrees/repo/fix",
			ownerClone:    "C:/dev/repo",
			agentHost:     AgentHost{},
			agentWorkTree: gitrepo.AgentWorkTree{},
			screen:        modeOverview,
			reviewedDir:   "C:/dev/first",
		},
		uncommitted:       false,
		defaultBranch:     originMain(),
		mergedIntoDefault: false,
		agentTaken:        false,
	}
}

// repoSideDeleteCheck passes every check of a delete of repo-side, which has an agent copy on claude-vm.
func repoSideDeleteCheck(removeAgentWorkTree func(ctx context.Context, ownerClone string,
	agentWorkTree gitrepo.AgentWorkTree) error) workTreeDeleteCheckMsg {
	workTrees := fourWorkTrees()
	return workTreeDeleteCheckMsg{
		request: workTreeDeleteRequest{
			repository: gitrepo.Repository{Dir: "C:/dev/repo"},
			row:        workTrees[1],
			folder:     "repo-side",
			ownerClone: "C:/dev/repo",
			agentHost:  AgentHost{Name: "claude-vm", ReadWorkTree: nil, RemoveWorkTree: removeAgentWorkTree},
			agentWorkTree: gitrepo.AgentWorkTree{
				Dir:    "C:/agent/dev/repo-side",
				Exists: true,
				Head:   gitrepo.AgentHead{Branch: "refs/heads/fix", Commit: "0123456789abcdef0123456789abcdef01234567"},
			},
			screen:      modeWorkTrees,
			reviewedDir: "C:/dev/repo",
		},
		uncommitted:       false,
		defaultBranch:     originMain(),
		mergedIntoDefault: false,
		agentTaken:        true,
	}
}

func removeNothing(context.Context, string, gitrepo.AgentWorkTree) error {
	return errors.New("removeNothing removes nothing")
}
