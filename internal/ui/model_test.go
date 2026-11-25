package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestFocusSwitch(t *testing.T) {
	t.Run("dropsFilesLoadedForThePreviousRepository", dropsFilesLoadedForThePreviousRepository)
	t.Run("rejectionClearsTheReviewAndThePopup", rejectionClearsTheReviewAndThePopup)
	t.Run("refocusingTheSameRepositoryKeepsTheSelection", refocusingTheSameRepositoryKeepsTheSelection)
	t.Run("opensTheFirstFocusedRepositoryWithUncommittedChanges", opensTheFirstFocusedRepositoryWithUncommittedChanges)
	t.Run("aFocusThatStillNamesTheReviewedRepositoryKeepsItAndShowsTheRefusals",
		aFocusThatStillNamesTheReviewedRepositoryKeepsItAndShowsTheRefusals)
	t.Run("aFocusWithADifferentFirstRepositoryPicksAgain", aFocusWithADifferentFirstRepositoryPicksAgain)
}

func TestTheOverviewListsTheFocusedRepositoriesFirstAndMarksThem(t *testing.T) {
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})

	model = update(t, model, overviewMsg{rows: []overviewRow{
		repositoryRow{FoundRepository: foundAt("elsewhere", "C:/dev/elsewhere")},
		repositoryRow{FoundRepository: foundAt("first", "C:/dev/first")},
		repositoryRow{FoundRepository: foundAt("second", "C:/dev/second")},
	}})
	got := overviewLines(model)

	branch := " " + branchGlyph + " detached HEAD clean"
	want := []string{"▌● " + folderGlyph + " first" + branch, "● " + folderGlyph + " second" + branch,
		folderGlyph + " elsewhere" + branch}
	require.Equal(t, want, got)
}

func TestPendingOverviewRow(t *testing.T) {
	t.Run("showsLoadingUntilItsRowArrives", showsLoadingUntilItsRowArrives)
	t.Run("leavesTheTableWithTheErrorWhenGitFails", leavesTheTableWithTheErrorWhenGitFails)
}

func showsLoadingUntilItsRowArrives(t *testing.T) {
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	next, loads := model.Update(overviewMsg{rows: []overviewRow{
		pendingRow{FoundRepository: foundAt("first", "C:/dev/first")},
		pendingRow{FoundRepository: foundAt("second", "C:/dev/second")},
	}})
	model = asModel(t, next)
	pending := overviewLines(model)

	model = update(t, model, overviewRowMsg{row: repositoryRow{FoundRepository: foundAt("first", "C:/dev/first")}})
	loaded := overviewLines(model)

	wantPending := []string{"▌● " + folderGlyph + " first loading...", "● " + folderGlyph + " second loading..."}
	wantLoaded := []string{"▌● " + folderGlyph + " first " + branchGlyph + " detached HEAD clean",
		"● " + folderGlyph + " second loading..."}
	require.NotNil(t, loads)
	require.Equal(t, wantPending, pending)
	require.Equal(t, wantLoaded, loaded)
}

func leavesTheTableWithTheErrorWhenGitFails(t *testing.T) {
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": false, "C:/dev/second": true})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	model = update(t, model, overviewMsg{rows: []overviewRow{
		pendingRow{FoundRepository: foundAt("first", "C:/dev/first")},
		pendingRow{FoundRepository: foundAt("second", "C:/dev/second")},
	}})

	model = update(t, model, overviewRowFailureMsg{dir: "C:/dev/first", err: errors.New("run git status in C:/dev/first")})
	lines := overviewLines(model)

	want := []string{"▌● " + folderGlyph + " second loading..."}
	require.Equal(t, want, lines)
	require.Equal(t, "skipped run git status in C:/dev/first", model.footer.text)
}

func overviewLines(model Model) []string {
	height := model.paneInnerHeight()
	lines := []string{}
	for _, line := range listLines(model.overview, model.overviewCells, "no repositories found", 118, height) {
		plain := ansi.Strip(line)
		words := strings.Fields(plain)
		lines = append(lines, strings.Join(words, " "))
	}
	return lines
}

func plainScreen(model Model) string {
	rendered := model.renderScreen()
	return ansi.Strip(rendered)
}

func plainHeader(model Model) string {
	rendered := model.renderHeader()
	return ansi.Strip(rendered)
}

func plainFooter(model Model) string {
	rendered := model.renderFooter()
	return ansi.Strip(rendered)
}

func plainConfirmPopup(model Model) string {
	rendered := model.renderConfirmPopup()
	return ansi.Strip(rendered)
}

func opensTheFirstFocusedRepositoryWithUncommittedChanges(t *testing.T) {
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": false, "C:/dev/second": true})

	require.Equal(t, "C:/dev/second", model.repository.Dir)
}

func aFocusThatStillNamesTheReviewedRepositoryKeepsItAndShowsTheRefusals(t *testing.T) {
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})

	model = update(t, model, focus.Focus{
		Repositories: []gitrepo.Repository{{Dir: "C:/dev/cwd"}, {Dir: "C:/dev/other"}, {Dir: "C:/dev/first"}},
		Refusals:     []error{errors.New("C:/dev/notes: no .git")},
	})

	require.Equal(t, "C:/dev/first", model.repository.Dir)
	require.True(t, model.footer.isError)
	require.Contains(t, model.footer.text, "C:/dev/notes: no .git")
}

func aFocusWithADifferentFirstRepositoryPicksAgain(t *testing.T) {
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	model = update(t, model, tea.BlurMsg{})
	focused := []gitrepo.Repository{{Dir: "C:/dev/other"}, {Dir: "C:/dev/first"}}

	model = update(t, model, focus.Focus{Repositories: focused, Refusals: []error{}})
	uncommitted := map[string]bool{"C:/dev/other": true, "C:/dev/first": true}
	model = update(t, model, uncommittedMsg{focused: focused, uncommitted: uncommitted})

	require.Equal(t, "C:/dev/other", model.repository.Dir)
}

func focusOnSeveral(t *testing.T, uncommitted map[string]bool) Model {
	t.Helper()
	focused := []gitrepo.Repository{{Dir: "C:/dev/cwd"}, {Dir: "C:/dev/first"}, {Dir: "C:/dev/second"}}
	model := New(findNothing, openAnything, nil, AgentHost{})
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model = update(t, model, focus.Focus{Repositories: focused, Refusals: []error{}})
	return update(t, model, uncommittedMsg{focused: focused, uncommitted: uncommitted})
}

func TestEnterInTheOverviewReviewsTheSelectedRepositoryAndKeepsTheFocus(t *testing.T) {
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	model = update(t, model, overviewMsg{rows: []overviewRow{
		repositoryRow{FoundRepository: foundAt("first", "C:/dev/first")},
		repositoryRow{FoundRepository: foundAt("org/elsewhere", "C:/dev/org/elsewhere")},
	}})
	screen := plainScreen(model)

	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	require.Contains(t, screen, "org/elsewhere")
	require.Contains(t, screen, "1 of 2")
	require.NotEqual(t, modeOverview, model.mode)
	require.Equal(t, "C:/dev/org/elsewhere", model.repository.Dir)
	require.Len(t, model.focused, 3)
}

func TestARefusedRepositoryIsAMutedRowWithItsSummaryAndEnterShowsTheReason(t *testing.T) {
	list := func(context.Context) (focus.Found, error) {
		return focus.Found{Repositories: []focus.FoundRepository{}, Refusals: []focus.FoundRefusal{
			{RelativePath: "notes", Err: fmt.Errorf("list C:/dev/notes: %w", fs.ErrPermission)},
			{RelativePath: "agents", Err: fmt.Errorf("%w: C:/dev/agents/.git has no deny", focus.ErrGitDirUnprotected)},
		}}, nil
	}
	model := New(list, openAnything, nil, AgentHost{})
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	found := overviewRequest{findRepositories: list}.load()
	model = update(t, model, found)
	screen := plainScreen(model)

	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)

	agentsIndex := strings.Index(screen, "agents")
	notesIndex := strings.Index(screen, "notes")
	wantFooter := "agents is refused: " + focus.ErrGitDirUnprotected.Error() + ": C:/dev/agents/.git has no deny"
	require.Contains(t, screen, folderGlyph+" agents  not write-denied to the agent")
	require.Less(t, agentsIndex, notesIndex, screen)
	require.Nil(t, cmd)
	require.Equal(t, modeOverview, model.mode)
	require.Equal(t, wantFooter, model.footer.text)
}

func TestPullInTheOverview(t *testing.T) {
	t.Run("theRowShowsPullingUntilThePulledRowReplacesIt", theRowShowsPullingUntilThePulledRowReplacesIt)
	t.Run("aRowWithNoUpstreamIsNotPulled", aRowWithNoUpstreamIsNotPulled)
	t.Run("aRowWhoseUpstreamWasDeletedIsExplained", aRowWhoseUpstreamWasDeletedIsExplained)
	t.Run("aFinishedRowIsSwitchedThoughItTracksNoUpstream", aFinishedRowIsSwitchedThoughItTracksNoUpstream)
}

func theRowShowsPullingUntilThePulledRowReplacesIt(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	model = asModel(t, next)
	pulling := plainScreen(model)
	pulled := repositoryRow{
		FoundRepository: foundAt("first", "C:/dev/first"),
		status: gitrepo.Status{
			Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main"}, HasCommits: true,
		},
	}

	model = update(t, model, syncedMsg{row: pulled, notice: "pulled first"})

	require.NotNil(t, cmd)
	require.Contains(t, pulling, "pulling...")
	require.Equal(t, pulled, model.overview.rows[0])
	require.Empty(t, model.syncing)
	require.Equal(t, "pulled first", model.footer.text)
}

func aRowWithNoUpstreamIsNotPulled(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	model = asModel(t, next)

	require.Nil(t, cmd)
	require.Empty(t, model.syncing)
	require.True(t, model.footer.isError)
}

func aRowWhoseUpstreamWasDeletedIsExplained(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	model = update(t, model, overviewMsg{rows: []overviewRow{repositoryRow{
		FoundRepository: foundAt("first", "C:/dev/first"),
		status: gitrepo.Status{
			Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix", UpstreamGone: true}, HasCommits: true,
		},
	}}})
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	model = asModel(t, next)

	require.Nil(t, cmd)
	require.Equal(t, "first tracks origin/fix, which was deleted from the remote, so there is nothing to pull",
		model.footer.text)
}

func aFinishedRowIsSwitchedThoughItTracksNoUpstream(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	model = update(t, model, overviewMsg{rows: []overviewRow{repositoryRow{
		FoundRepository: foundAt("first", "C:/dev/first"),
		status:          gitrepo.Status{Branch: "fix", HasCommits: true, Files: []gitrepo.FileStatus{}},
		defaultBranch:   originMain(),
		finished:        true,
	}}})
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	model = asModel(t, next)

	require.NotNil(t, cmd)
	require.Equal(t, "pulling...", model.syncing["C:/dev/first"])
	require.False(t, model.footer.isError)
}

func TestFetchInTheOverview(t *testing.T) {
	t.Run("fetchesOnlyTheSelectedRow", fetchesOnlyTheSelectedRow)
	t.Run("fetchAllFetchesEveryRepositoryAndNamesTheFailures", fetchAllFetchesEveryRepositoryAndNamesTheFailures)
}

func fetchesOnlyTheSelectedRow(t *testing.T) {
	overview := overviewWithTwoRepositories(t)

	next, cmd := overview.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	model := asModel(t, next)

	require.NotNil(t, cmd)
	require.Equal(t, map[string]string{"C:/dev/first": "fetching..."}, model.syncing)
}

func fetchAllFetchesEveryRepositoryAndNamesTheFailures(t *testing.T) {
	overview := overviewWithTwoRepositories(t)
	next, cmd := overview.Update(tea.KeyPressMsg{Code: 'F', Text: "F"})
	model := asModel(t, next)
	fetching := plainScreen(model)
	fetched := repositoryRow{
		FoundRepository: foundAt("first", "C:/dev/first"),
		status:          gitrepo.Status{Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main", Behind: 5}},
	}

	model = update(t, model, fetchedAllMsg{dirs: []string{"C:/dev/first", "C:/dev/org/elsewhere"},
		rows: []repositoryRow{fetched}, failures: []string{"run git fetch --prune in C:/dev/org/elsewhere: exit status 128"}})

	fetchingCount := strings.Count(fetching, "fetching...")
	require.NotNil(t, cmd)
	require.Equal(t, 2, fetchingCount, fetching)
	require.Equal(t, fetched, model.overview.rows[0])
	require.Empty(t, model.syncing)
	require.Equal(t, "fetch failed: run git fetch --prune in C:/dev/org/elsewhere: exit status 128", model.footer.text)
}

func TestPushInTheOverview(t *testing.T) {
	t.Run("aRepositoryNotUnderReviewClosesItsPopupOnItsResult", aRepositoryNotUnderReviewClosesItsPopupOnItsResult)
	t.Run("aBranchWithNoUpstreamIsPublished", aBranchWithNoUpstreamIsPublished)
}

func aRepositoryNotUnderReviewClosesItsPopupOnItsResult(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	model = update(t, model, overviewMsg{rows: []overviewRow{repositoryRow{
		FoundRepository: foundAt("second", "C:/dev/second"),
		status: gitrepo.Status{
			Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main", Ahead: 1}, HasCommits: true,
		},
	}}})
	model = update(t, model, tea.KeyPressMsg{Code: 'P', Text: "P"})
	popup := plainConfirmPopup(model)

	next, cmd := model.Update(popupSuccessMsg{dir: "C:/dev/second", notice: "pushed to origin/main"})
	model = asModel(t, next)

	require.Contains(t, popup, "push main to origin/main")
	require.NotEqual(t, "C:/dev/second", model.repository.Dir)
	require.Equal(t, noPopup, model.popup.kind)
	require.Equal(t, "pushed to origin/main", model.footer.text)
	require.NotNil(t, cmd)
}

func aBranchWithNoUpstreamIsPublished(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	model = update(t, model, overviewMsg{rows: []overviewRow{repositoryRow{
		FoundRepository:   foundAt("second", "C:/dev/second"),
		status:            gitrepo.Status{Branch: "feature", HasCommits: true},
		commitsOnNoRemote: 2,
	}}})
	screen := plainScreen(model)

	model = update(t, model, tea.KeyPressMsg{Code: 'P', Text: "P"})
	popup := plainConfirmPopup(model)

	require.Contains(t, screen, "↑2 unpublished")
	require.Contains(t, popup, "publish feature to origin")
}

func TestCherryPickInTheOverview(t *testing.T) {
	t.Run("cherryPickOffersTheWorkTreesBranchOntoItsMainClonesDefaultBranch",
		cherryPickOffersTheWorkTreesBranchOntoItsMainClonesDefaultBranch)
	t.Run("cherryPickRefusesAMainClone", cherryPickRefusesAMainClone)
}

func cherryPickOffersTheWorkTreesBranchOntoItsMainClonesDefaultBranch(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	model = update(t, model, overviewMsg{rows: []overviewRow{
		repositoryRow{
			FoundRepository: focus.FoundRepository{RelativePath: "repo", Repository: gitrepo.Repository{
				Dir: "C:/dev/repo", GitDir: filepath.FromSlash("C:/dev/repo/.git"),
			}},
			status:        gitrepo.Status{Branch: "main", HasCommits: true},
			defaultBranch: originMain(),
		},
		repositoryRow{
			FoundRepository: focus.FoundRepository{RelativePath: "worktrees/repo/fix", Repository: gitrepo.Repository{
				Dir: "C:/dev/worktrees/repo/fix", GitDir: filepath.FromSlash("C:/dev/repo/.git/worktrees/fix"),
			}},
			status: gitrepo.Status{Branch: "fix", HasCommits: true},
		},
	}})
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})

	next, cmd := model.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	model = asModel(t, next)
	popup := plainConfirmPopup(model)

	require.Nil(t, cmd)
	require.Contains(t, popup, "cherry-pick fix onto main")
	require.Equal(t, "C:/dev/repo", model.popup.dir)
}

func cherryPickRefusesAMainClone(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	model = update(t, model, overviewMsg{rows: []overviewRow{repositoryRow{
		FoundRepository: focus.FoundRepository{RelativePath: "repo", Repository: gitrepo.Repository{
			Dir: "C:/dev/repo", GitDir: "C:/dev/repo/.git",
		}},
		status:        gitrepo.Status{Branch: "fix", HasCommits: true},
		defaultBranch: originMain(),
	}}})

	next, cmd := model.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	model = asModel(t, next)

	require.Nil(t, cmd)
	require.Equal(t, noPopup, model.popup.kind)
	require.Contains(t, model.footer.text, "repo is a main clone")
}

func TestTheOverviewDetailsTheSelectedRepository(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	model = update(t, model, overviewMsg{rows: []overviewRow{
		repositoryRow{
			FoundRepository: foundAt("first", "C:/dev/first"),
			status: gitrepo.Status{
				Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main", Ahead: 1, Behind: 1}, HasCommits: true,
				Files: []gitrepo.FileStatus{{Path: "edited.go", IndexStatus: ' ', WorktreeStatus: 'M'}},
			},
			outgoing: []gitrepo.Commit{{ShortHash: "aaa1111", Summary: "Add the local change"}},
			incoming: []gitrepo.Commit{{ShortHash: "bbb2222", Summary: "Add the remote change"}},
		},
		repositoryRow{FoundRepository: foundAt("second", "C:/dev/second")},
	}})
	first := plainScreen(model)

	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	second := plainScreen(model)

	for _, want := range []string{"changes", " M edited.go", "outgoing", "aaa1111 Add the local change", "incoming",
		"bbb2222 Add the remote change"} {
		require.Contains(t, first, want)
	}
	require.NotContains(t, second, "edited.go")
	require.NotContains(t, second, "aaa1111")
}

func TestOverviewTableHeight(t *testing.T) {
	t.Run("aShortListTakesOneLinePerRow", aShortOverviewListTakesOneLinePerRow)
	t.Run("aLongListLeavesTheDetailsFiveRows", aLongOverviewListLeavesTheDetailsFiveRows)
	t.Run("aFilterKeepsTheHeight", aFilterKeepsTheOverviewTableHeight)
}

func aShortOverviewListTakesOneLinePerRow(t *testing.T) {
	model := overviewWithTwoRepositories(t)

	detailsTop := detailsTopBorderLine(model)

	require.Equal(t, headerHeight+1+2+1, detailsTop)
}

func aLongOverviewListLeavesTheDetailsFiveRows(t *testing.T) {
	rows := []overviewRow{}
	for index := range 40 {
		name := "repo" + strconv.Itoa(index)
		rows = append(rows, repositoryRow{FoundRepository: foundAt(name, "C:/dev/"+name)})
	}
	model := overviewWithTwoRepositories(t)
	model = update(t, model, overviewMsg{rows: rows})

	detailsTop := detailsTopBorderLine(model)

	require.Equal(t, model.height-footerHeight-(5+2), detailsTop)
}

func aFilterKeepsTheOverviewTableHeight(t *testing.T) {
	overview := overviewWithTwoRepositories(t)
	model := typeText(t, overview, "/elsewhere")

	detailsTop := detailsTopBorderLine(model)

	require.Equal(t, headerHeight+1+2+1, detailsTop)
}

func detailsTopBorderLine(model Model) int {
	screen := plainScreen(model)
	lines := strings.Split(screen, "\n")
	return slices.IndexFunc(lines, func(line string) bool { return strings.Contains(line, "─ changes ─") })
}

func TestMayBeFinished(t *testing.T) {
	t.Run("aBranchWhoseUpstreamWasDeletedMayBeFinished", aBranchWhoseUpstreamWasDeletedMayBeFinished)
	t.Run("aBranchThatNeverHadAnUpstreamMayBeFinished", aBranchThatNeverHadAnUpstreamMayBeFinished)
	t.Run("aBranchWhoseUpstreamStillExistsIsNotFinished", aBranchWhoseUpstreamStillExistsIsNotFinished)
	t.Run("aBranchWithChangesIsNotFinished", aBranchWithChangesIsNotFinished)
	t.Run("aDetachedHeadIsNotFinished", aDetachedHeadIsNotFinished)
	t.Run("theDefaultBranchWithItsRemoteGoneIsNotFinished", theDefaultBranchWithItsRemoteGoneIsNotFinished)
}

func aBranchWhoseUpstreamWasDeletedMayBeFinished(t *testing.T) {
	status := gitrepo.Status{Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix", UpstreamGone: true}}

	finished := mayBeFinished(status, originMain())

	require.True(t, finished)
}

func aBranchThatNeverHadAnUpstreamMayBeFinished(t *testing.T) {
	status := gitrepo.Status{Branch: "fix"}

	finished := mayBeFinished(status, originMain())

	require.True(t, finished)
}

func aBranchWhoseUpstreamStillExistsIsNotFinished(t *testing.T) {
	status := gitrepo.Status{Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix"}}

	finished := mayBeFinished(status, originMain())

	require.False(t, finished)
}

func aBranchWithChangesIsNotFinished(t *testing.T) {
	status := gitrepo.Status{Branch: "fix", Files: []gitrepo.FileStatus{{Path: "a.go", IndexStatus: ' ', WorktreeStatus: 'M'}}}

	finished := mayBeFinished(status, originMain())

	require.False(t, finished)
}

func aDetachedHeadIsNotFinished(t *testing.T) {
	status := gitrepo.Status{}

	finished := mayBeFinished(status, originMain())

	require.False(t, finished)
}

func theDefaultBranchWithItsRemoteGoneIsNotFinished(t *testing.T) {
	status := gitrepo.Status{Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main", UpstreamGone: true}}

	finished := mayBeFinished(status, originMain())

	require.False(t, finished)
}

func TestRefreshInTheOverviewListsAnAddedRepositoryAndKeepsTheSelection(t *testing.T) {
	model := overviewWithTwoRepositories(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	model = asModel(t, next)

	model = update(t, model, overviewMsg{rows: []overviewRow{
		pendingRow{FoundRepository: foundAt("added", "C:/dev/added")},
		pendingRow{FoundRepository: foundAt("first", "C:/dev/first")},
		pendingRow{FoundRepository: foundAt("org/elsewhere", "C:/dev/org/elsewhere")},
	}})
	selected, _ := model.overview.selectedRow()
	selectedPath := selected.relativePath()

	require.NotNil(t, cmd)
	require.Len(t, model.overview.rows, 3)
	require.Equal(t, "org/elsewhere", selectedPath)
}

func overviewWithTwoRepositories(t *testing.T) Model {
	t.Helper()
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	return update(t, model, overviewMsg{rows: []overviewRow{
		repositoryRow{
			FoundRepository: foundAt("first", "C:/dev/first"),
			status: gitrepo.Status{
				Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main", Behind: 2}, HasCommits: true,
			},
		},
		repositoryRow{FoundRepository: foundAt("org/elsewhere", "C:/dev/org/elsewhere")},
	}})
}

func foundAt(relativePath string, dir string) focus.FoundRepository {
	return focus.FoundRepository{RelativePath: relativePath, Repository: gitrepo.Repository{Dir: dir}}
}

func TestOverviewMouse(t *testing.T) {
	t.Run("theWheelMovesTheSelection", theWheelMovesTheOverviewSelection)
	t.Run("clickingARowSelectsIt", clickingAnOverviewRowSelectsIt)
	t.Run("clickingBelowTheLastRowKeepsTheSelection", clickingBelowTheLastOverviewRowKeepsTheSelection)
}

func theWheelMovesTheOverviewSelection(t *testing.T) {
	model := overviewOfWorkTrees(t)
	start := model.overview.selected

	model = update(t, model, tea.MouseWheelMsg{X: 10, Y: 25, Button: tea.MouseWheelDown})
	down := model.overview.selected
	model = update(t, model, tea.MouseWheelMsg{X: 10, Y: 25, Button: tea.MouseWheelUp})

	require.Equal(t, start+1, down)
	require.Equal(t, start, model.overview.selected)
}

func clickingAnOverviewRowSelectsIt(t *testing.T) {
	model := overviewOfWorkTrees(t)

	model = update(t, model, tea.MouseClickMsg{X: 10, Y: headerHeight + 1 + 2, Button: tea.MouseLeft})

	require.Equal(t, 2, model.overview.selected)
	require.Equal(t, modeOverview, model.mode)
}

func clickingBelowTheLastOverviewRowKeepsTheSelection(t *testing.T) {
	model := overviewOfWorkTrees(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})

	model = update(t, model, tea.MouseClickMsg{X: 10, Y: headerHeight + 1 + 4, Button: tea.MouseLeft})

	require.Equal(t, 1, model.overview.selected)
}

func TestOverviewFilter(t *testing.T) {
	t.Run("keepsTheRowsHoldingTheQuerysCharactersInOrderAndTitlesIt",
		keepsTheRowsHoldingTheQuerysCharactersInOrderAndTitlesIt)
	t.Run("everyWordMustMatchThePathOrTheBranch", everyWordMustMatchThePathOrTheBranch)
	t.Run("anUppercaseLetterMakesTheWordCaseSensitive", anUppercaseLetterMakesTheWordCaseSensitive)
	t.Run("arrowsMoveTheSelectionWhileTypingAndJTypesALetter", arrowsMoveTheSelectionWhileTypingAndJTypesALetter)
	t.Run("aPasteFiltersTheRows", aPasteFiltersTheRows)
	t.Run("deleteActsOnTheFilteredRowAndTheReloadStaysFiltered", deleteActsOnTheFilteredRowAndTheReloadStaysFiltered)
	t.Run("aRowFillingInMatchesOnItsBranch", aRowFillingInMatchesOnItsBranch)
	t.Run("aHiddenPendingRowThatFailsIsSkipped", aHiddenPendingRowThatFailsIsSkipped)
	t.Run("theFooterShowsTheQueryWhileTypingThenTheOverviewKeys", theFooterShowsTheQueryWhileTypingThenTheOverviewKeys)
	t.Run("enterWhileTypingOpensTheSelectedRow", enterWhileTypingOpensTheSelectedRow)
	t.Run("escWhileTypingShowsEveryRow", escWhileTypingShowsEveryRow)
	t.Run("backspaceOnAnEmptyQueryStopsTyping", backspaceOnAnEmptyQueryStopsTyping)
	t.Run("escClearsAKeptFilterThenClosesTheOverview", escClearsAKeptFilterThenClosesTheOverview)
	t.Run("aQueryMatchingNothingSaysSo", aQueryMatchingNothingSaysSo)
	t.Run("reopeningTheOverviewShowsEveryRow", reopeningTheOverviewShowsEveryRow)
	t.Run("fetchAllFetchesTheRowsTheFilterHides", fetchAllFetchesTheRowsTheFilterHides)
}

func keepsTheRowsHoldingTheQuerysCharactersInOrderAndTitlesIt(t *testing.T) {
	overview := overviewOfWorkTrees(t)

	model := typeText(t, overview, "/dwfix")
	lines := overviewLines(model)
	screen := plainScreen(model)
	height := model.paneInnerHeight()
	styledLines := listLines(model.overview, model.overviewCells, "", 118, height)
	unselected := styledLines[1]
	highlightedFix := searchMatchStyle.Render("fix")

	want := []string{"▌ " + folderGlyph + " worktrees/desktop-edge-win/fix-proxy " + branchGlyph + " fix-proxy clean",
		folderGlyph + " worktrees/desktop-edge-win/fix-dns " + branchGlyph + " fix-dns clean"}
	require.Equal(t, want, lines)
	require.Contains(t, screen, "repositories /dwfix")
	require.Contains(t, unselected, highlightedFix)
}

func everyWordMustMatchThePathOrTheBranch(t *testing.T) {
	overview := overviewOfWorkTrees(t)

	model := typeText(t, overview, "/edge dns")
	lines := overviewLines(model)

	want := []string{"▌ " + folderGlyph + " worktrees/desktop-edge-win/fix-dns " + branchGlyph + " fix-dns clean"}
	require.Equal(t, want, lines)
}

func anUppercaseLetterMakesTheWordCaseSensitive(t *testing.T) {
	model := overviewOfWorkTrees(t)
	model = update(t, model, overviewRowMsg{row: repositoryRow{
		FoundRepository: foundAt("ziti", "C:/dev/ziti"), status: gitrepo.Status{Branch: "Fix-docs", HasCommits: true},
	}})

	model = typeText(t, model, "/Fix")
	lines := overviewLines(model)

	want := []string{"▌ " + folderGlyph + " ziti " + branchGlyph + " Fix-docs clean"}
	require.Equal(t, want, lines)
}

func arrowsMoveTheSelectionWhileTypingAndJTypesALetter(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/fix")

	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	down, _ := model.overview.selectedRow()
	downPath := down.relativePath()
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	up, _ := model.overview.selectedRow()
	upPath := up.relativePath()
	model = typeText(t, model, "j")
	query := model.overviewQuery.Value()

	require.Equal(t, "worktrees/desktop-edge-win/fix-dns", downPath)
	require.Equal(t, "worktrees/desktop-edge-win/fix-proxy", upPath)
	require.Equal(t, "fixj", query)
	require.Empty(t, model.overview.rows)
}

func aPasteFiltersTheRows(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/")

	model = update(t, model, tea.PasteMsg{Content: "dns"})
	lines := overviewLines(model)

	want := []string{"▌ " + folderGlyph + " worktrees/desktop-edge-win/fix-dns " + branchGlyph + " fix-dns clean"}
	require.Equal(t, want, lines)
}

func aPasteIntoTheSearchFiltersTheLog(t *testing.T) {
	model := logOf(t, []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Message: "Add tree"},
		{Hash: strings.Repeat("2", 40), Message: "Fix wrap"},
	})
	model = update(t, model, tea.KeyPressMsg{Code: '/', Text: "/"})

	model = update(t, model, tea.PasteMsg{Content: "wrap"})
	rows := model.visibleLogRows()

	require.Equal(t, []treeRow{{commit: 1, file: commitRow}}, rows)
}

func deleteActsOnTheFilteredRowAndTheReloadStaysFiltered(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/fix")
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})

	next, check := model.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	model = asModel(t, next)
	require.NotNil(t, check)
	require.Empty(t, model.footer.text)
	checkMsg := check()
	failure, isFailure := checkMsg.(gitFailureMsg)
	require.True(t, isFailure, "got %#v, want the check to run in fix-dns, whose made up directory fails", checkMsg)
	require.ErrorContains(t, failure.err, "C:/dev/worktrees/desktop-edge-win/fix-dns")

	reloaded := slices.Delete(workTreeRows(), 2, 3)
	model = update(t, model, overviewMsg{rows: reloaded})
	lines := overviewLines(model)

	want := []string{"▌ " + folderGlyph + " worktrees/desktop-edge-win/fix-proxy " + branchGlyph + " fix-proxy clean"}
	require.Equal(t, want, lines)
}

func aRowFillingInMatchesOnItsBranch(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/release")
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})

	model = update(t, model, overviewRowMsg{row: repositoryRow{
		FoundRepository: foundAt("ziti", "C:/dev/ziti"), status: gitrepo.Status{Branch: "release", HasCommits: true},
	}})
	lines := overviewLines(model)

	want := []string{"▌ " + folderGlyph + " ziti " + branchGlyph + " release clean"}
	require.Equal(t, want, lines)
}

func theFooterShowsTheQueryWhileTypingThenTheOverviewKeys(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/dns")
	typing := plainFooter(model)

	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	kept := plainFooter(model)

	require.Contains(t, typing, " /dns")
	require.Contains(t, typing, "enter open  tab keep  esc clear")
	require.Contains(t, kept, "j/k select  s sort by work  h hide no work  / search  enter open")
}

func enterWhileTypingOpensTheSelectedRow(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/dns")

	next, open := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)

	require.Equal(t, modeBrowsing, model.mode)
	require.Equal(t, "C:/dev/worktrees/desktop-edge-win/fix-dns", model.repository.Dir)
	require.NotNil(t, open)
}

func aHiddenPendingRowThatFailsIsSkipped(t *testing.T) {
	model := overviewOfWorkTrees(t)
	model = update(t, model, overviewMsg{rows: []overviewRow{
		pendingRow{FoundRepository: foundAt("first", "C:/dev/first")},
		pendingRow{FoundRepository: foundAt("second", "C:/dev/second")},
	}})
	model = typeText(t, model, "/second")
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})

	model = update(t, model, overviewRowFailureMsg{dir: "C:/dev/first", err: errors.New("run git status in C:/dev/first")})
	footer := model.footer.text
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	lines := overviewLines(model)

	want := []string{"▌● " + folderGlyph + " second loading..."}
	require.Equal(t, want, lines)
	require.Equal(t, "skipped run git status in C:/dev/first", footer)
}

func escWhileTypingShowsEveryRow(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/dns")

	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	lines := overviewLines(model)
	typing := model.overviewQuery.Focused()

	require.Len(t, lines, 4)
	require.Equal(t, modeOverview, model.mode)
	require.False(t, typing)
}

func backspaceOnAnEmptyQueryStopsTyping(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/d")

	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	stillTyping := model.overviewQuery.Focused()
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	lines := overviewLines(model)
	typing := model.overviewQuery.Focused()

	require.True(t, stillTyping)
	require.Len(t, lines, 4)
	require.Equal(t, modeOverview, model.mode)
	require.False(t, typing)
}

func escClearsAKeptFilterThenClosesTheOverview(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/dns")
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})

	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	cleared := overviewLines(model)
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})

	require.Len(t, cleared, 4)
	require.Equal(t, modeBrowsing, model.mode)
}

func aQueryMatchingNothingSaysSo(t *testing.T) {
	overview := overviewOfWorkTrees(t)

	model := typeText(t, overview, "/qq")
	screen := plainScreen(model)

	require.Contains(t, screen, "no repositories match")
}

func reopeningTheOverviewShowsEveryRow(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/dns")
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})

	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	model = update(t, model, overviewMsg{rows: workTreeRows()})
	lines := overviewLines(model)
	query := model.overviewQuery.Value()

	require.Len(t, lines, 4)
	require.Empty(t, query)
}

func fetchAllFetchesTheRowsTheFilterHides(t *testing.T) {
	overview := overviewOfWorkTrees(t)
	model := typeText(t, overview, "/dns")
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})

	model = update(t, model, tea.KeyPressMsg{Code: 'F', Text: "F"})

	require.Len(t, model.syncing, 4)
}

// overviewOfWorkTrees lists the main clone desktop-edge-win, its work trees fix-proxy and fix-dns, and ziti.
func overviewOfWorkTrees(t *testing.T) Model {
	t.Helper()
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	return update(t, model, overviewMsg{rows: workTreeRows()})
}

func workTreeRows() []overviewRow {
	workTree := func(name string) repositoryRow {
		return repositoryRow{
			FoundRepository: focus.FoundRepository{RelativePath: "worktrees/desktop-edge-win/" + name,
				Repository: gitrepo.Repository{Dir: "C:/dev/worktrees/desktop-edge-win/" + name,
					GitDir: filepath.FromSlash("C:/dev/desktop-edge-win/.git/worktrees/" + name)}},
			status: gitrepo.Status{Branch: name, HasCommits: true},
		}
	}
	return []overviewRow{
		repositoryRow{
			FoundRepository: focus.FoundRepository{RelativePath: "desktop-edge-win", Repository: gitrepo.Repository{
				Dir: "C:/dev/desktop-edge-win", GitDir: filepath.FromSlash("C:/dev/desktop-edge-win/.git"),
			}},
			status:        gitrepo.Status{Branch: "main", HasCommits: true},
			defaultBranch: originMain(),
		},
		workTree("fix-proxy"),
		workTree("fix-dns"),
		repositoryRow{FoundRepository: foundAt("ziti", "C:/dev/ziti"), status: gitrepo.Status{Branch: "main"}},
	}
}

func TestOverviewWork(t *testing.T) {
	t.Run("sortListsTheFocusedRowsThenTheRowsWithWorkThenTheRest", sortListsTheFocusedRowsThenTheRowsWithWorkThenTheRest)
	t.Run("sortKeepsTheSelectedRow", sortKeepsTheSelectedRow)
	t.Run("aRowGainingWorkAfterTheSortStaysInPlace", aRowGainingWorkAfterTheSortStaysInPlace)
	t.Run("refreshKeepsTheSortedOrder", refreshKeepsTheSortedOrder)
	t.Run("refreshListsNewRowsByPathAfterTheListedOnesAndDropsGoneOnes",
		refreshListsNewRowsByPathAfterTheListedOnesAndDropsGoneOnes)
	t.Run("reopeningTheOverviewListsTheFocusedRowsThenByPath", reopeningTheOverviewListsTheFocusedRowsThenByPath)
	t.Run("hideLeavesTheFocusedPendingAndWorkRowsAndRetitlesThePane",
		hideLeavesTheFocusedPendingAndWorkRowsAndRetitlesThePane)
	t.Run("hideAgainShowsEveryRow", hideAgainShowsEveryRow)
	t.Run("hideMovesTheSelectionToTheNearestRowLeft", hideMovesTheSelectionToTheNearestRowLeft)
	t.Run("aRowReloadedIntoNoWorkLeavesWhileHiding", aRowReloadedIntoNoWorkLeavesWhileHiding)
	t.Run("aRowLeavingBetweenTwoShownRowsSelectsTheOneBelow", aRowLeavingBetweenTwoShownRowsSelectsTheOneBelow)
	t.Run("aRowGainingWorkAppearsWhileHiding", aRowGainingWorkAppearsWhileHiding)
	t.Run("hidingSurvivesAFocusSwitchWithTheOverviewOpen", hidingSurvivesAFocusSwitchWithTheOverviewOpen)
	t.Run("aQueryWhileHidingTitlesThePaneWithBoth", aQueryWhileHidingTitlesThePaneWithBoth)
	t.Run("hidingEveryRowSaysNoRepositoriesWithWork", hidingEveryRowSaysNoRepositoriesWithWork)
	t.Run("reopeningTheOverviewStartsWithHidingOff", reopeningTheOverviewStartsWithHidingOff)
}

func sortListsTheFocusedRowsThenTheRowsWithWorkThenTheRest(t *testing.T) {
	model := overviewWithWork(t)

	model = update(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})
	paths := overviewPaths(model)

	want := []string{"first", "changed", "incoming", "outgoing", "alpha", "beta", "pending", "refused", "zeta"}
	require.Equal(t, want, paths)
}

func sortKeepsTheSelectedRow(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})

	model = update(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})
	selected, _ := model.overview.selectedRow()
	selectedPath := selected.relativePath()

	require.Equal(t, "alpha", selectedPath)
	require.Equal(t, 4, model.overview.selected)
}

func aRowGainingWorkAfterTheSortStaysInPlace(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})

	model = update(t, model, overviewRowMsg{row: rowWithChangedFile("zeta", "C:/dev/zeta")})
	paths := overviewPaths(model)

	want := []string{"first", "changed", "incoming", "outgoing", "alpha", "beta", "pending", "refused", "zeta"}
	require.Equal(t, want, paths)
}

func refreshKeepsTheSortedOrder(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})
	model = update(t, model, tea.KeyPressMsg{Code: 'r', Text: "r"})

	model = update(t, model, overviewMsg{rows: pendingRowsWithWork()})
	paths := overviewPaths(model)

	want := []string{"first", "changed", "incoming", "outgoing", "alpha", "beta", "pending", "refused", "zeta"}
	require.Equal(t, want, paths)
}

func refreshListsNewRowsByPathAfterTheListedOnesAndDropsGoneOnes(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})
	model = update(t, model, tea.KeyPressMsg{Code: 'r', Text: "r"})
	newRows := []overviewRow{pendingRow{FoundRepository: foundAt("aardvark", "C:/dev/aardvark")},
		pendingRow{FoundRepository: foundAt("abacus", "C:/dev/abacus")}}
	withoutBeta := slices.Delete(pendingRowsWithWork(), 1, 2)
	reloaded := append(newRows, withoutBeta...)

	model = update(t, model, overviewMsg{rows: reloaded})
	paths := overviewPaths(model)

	want := []string{"first", "changed", "incoming", "outgoing", "alpha", "pending", "refused", "zeta", "aardvark", "abacus"}
	require.Equal(t, want, paths)
}

func reopeningTheOverviewListsTheFocusedRowsThenByPath(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})

	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	model = update(t, model, overviewMsg{rows: pendingRowsWithWork()})
	paths := overviewPaths(model)

	want := []string{"first", "alpha", "beta", "changed", "incoming", "outgoing", "pending", "refused", "zeta"}
	require.Equal(t, want, paths)
}

// pendingRowsWithWork is what a refresh of rowsWithWork lists before any row loads.
func pendingRowsWithWork() []overviewRow {
	rows := []overviewRow{}
	for _, row := range rowsWithWork() {
		loaded, isLoaded := row.(repositoryRow)
		if isLoaded {
			rows = append(rows, pendingRow{FoundRepository: loaded.FoundRepository})
		} else {
			rows = append(rows, row)
		}
	}
	return rows
}

func hideLeavesTheFocusedPendingAndWorkRowsAndRetitlesThePane(t *testing.T) {
	model := overviewWithWork(t)
	shownFooter := plainFooter(model)

	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	paths := overviewPaths(model)
	screen := plainScreen(model)
	hidingFooter := plainFooter(model)

	want := []string{"first", "changed", "incoming", "outgoing", "pending"}
	require.Equal(t, want, paths)
	require.Contains(t, screen, "repositories with work")
	require.Contains(t, shownFooter, "j/k select  s sort by work  h hide no work  / search")
	require.Contains(t, hidingFooter, "j/k select  s sort by work  h show all  / search")
}

func hideAgainShowsEveryRow(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})

	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	paths := overviewPaths(model)
	screen := plainScreen(model)

	want := []string{"first", "alpha", "beta", "changed", "incoming", "outgoing", "pending", "refused", "zeta"}
	require.Equal(t, want, paths)
	require.NotContains(t, screen, "repositories with work")
}

func hideMovesTheSelectionToTheNearestRowLeft(t *testing.T) {
	model := overviewWithWork(t)
	model = typeText(t, model, "jj")

	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	selected, _ := model.overview.selectedRow()
	selectedPath := selected.relativePath()

	require.Equal(t, "changed", selectedPath)
}

func aRowReloadedIntoNoWorkLeavesWhileHiding(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})

	model = update(t, model, overviewRowMsg{row: repositoryRow{FoundRepository: foundAt("changed", "C:/dev/changed")}})
	paths := overviewPaths(model)
	selected, _ := model.overview.selectedRow()
	selectedPath := selected.relativePath()

	want := []string{"first", "incoming", "outgoing", "pending"}
	require.Equal(t, want, paths)
	require.Equal(t, "incoming", selectedPath)
}

func aRowLeavingBetweenTwoShownRowsSelectsTheOneBelow(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	model = typeText(t, model, "jj")

	model = update(t, model, overviewRowMsg{row: repositoryRow{FoundRepository: foundAt("incoming", "C:/dev/incoming")}})
	selected, _ := model.overview.selectedRow()
	selectedPath := selected.relativePath()

	require.Equal(t, "outgoing", selectedPath)
}

func hidingSurvivesAFocusSwitchWithTheOverviewOpen(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	model = update(t, model, tea.BlurMsg{})
	focused := []gitrepo.Repository{{Dir: "C:/dev/other"}, {Dir: "C:/dev/first"}}

	model = update(t, model, focus.Focus{Repositories: focused, Refusals: []error{}})
	paths := overviewPaths(model)
	screen := plainScreen(model)

	want := []string{"first", "changed", "incoming", "outgoing", "pending"}
	require.Equal(t, modeOverview, model.mode)
	require.True(t, model.hidingRowsWithNoWork)
	require.Equal(t, want, paths)
	require.Contains(t, screen, "repositories with work")
}

func aRowGainingWorkAppearsWhileHiding(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})

	model = update(t, model, overviewRowMsg{row: rowWithChangedFile("alpha", "C:/dev/alpha")})
	paths := overviewPaths(model)

	want := []string{"first", "alpha", "changed", "incoming", "outgoing", "pending"}
	require.Equal(t, want, paths)
}

func aQueryWhileHidingTitlesThePaneWithBoth(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})

	model = typeText(t, model, "/out")
	screen := plainScreen(model)

	require.Contains(t, screen, "repositories with work /out")
}

func hidingEveryRowSaysNoRepositoriesWithWork(t *testing.T) {
	model := overviewOfWorkTrees(t)

	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	screen := plainScreen(model)

	require.Empty(t, model.overview.rows)
	require.Contains(t, screen, "no repositories with work")
}

func reopeningTheOverviewStartsWithHidingOff(t *testing.T) {
	model := overviewWithWork(t)
	model = update(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})

	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	model = update(t, model, overviewMsg{rows: rowsWithWork()})
	paths := overviewPaths(model)

	require.False(t, model.hidingRowsWithNoWork)
	require.Len(t, paths, 9)
}

// overviewWithWork lists the focused first, clean alpha, beta and zeta, changed with an uncommitted file, outgoing and
// incoming with a commit each way, a pending row and a refused one, with first selected.
func overviewWithWork(t *testing.T) Model {
	t.Helper()
	model := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	return update(t, model, overviewMsg{rows: rowsWithWork()})
}

func rowsWithWork() []overviewRow {
	outgoing := repositoryRow{FoundRepository: foundAt("outgoing", "C:/dev/outgoing"),
		outgoing: []gitrepo.Commit{{ShortHash: "aaa1111", Summary: "Add the local change"}}}
	incoming := repositoryRow{FoundRepository: foundAt("incoming", "C:/dev/incoming"),
		incoming: []gitrepo.Commit{{ShortHash: "bbb2222", Summary: "Add the remote change"}}}
	return []overviewRow{
		repositoryRow{FoundRepository: foundAt("alpha", "C:/dev/alpha")},
		repositoryRow{FoundRepository: foundAt("beta", "C:/dev/beta")},
		rowWithChangedFile("changed", "C:/dev/changed"),
		repositoryRow{FoundRepository: foundAt("first", "C:/dev/first")},
		incoming,
		outgoing,
		pendingRow{FoundRepository: foundAt("pending", "C:/dev/pending")},
		refusedRow{FoundRefusal: focus.FoundRefusal{RelativePath: "refused", Err: fs.ErrPermission}},
		repositoryRow{FoundRepository: foundAt("zeta", "C:/dev/zeta")},
	}
}

func rowWithChangedFile(relativePath string, dir string) repositoryRow {
	return repositoryRow{FoundRepository: foundAt(relativePath, dir), status: gitrepo.Status{
		Files: []gitrepo.FileStatus{{Path: "edited.go", IndexStatus: ' ', WorktreeStatus: 'M'}},
	}}
}

func overviewPaths(model Model) []string {
	paths := []string{}
	for _, row := range model.overview.rows {
		paths = append(paths, row.relativePath())
	}
	return paths
}

func typeText(t *testing.T, model Model, text string) Model {
	t.Helper()
	for _, character := range text {
		model = update(t, model, tea.KeyPressMsg{Code: character, Text: string(character)})
	}
	return model
}

func TestWholeFile(t *testing.T) {
	t.Run("dropsAFileDiffLoadedWithTheOtherContext", dropsAFileDiffLoadedWithTheOtherContext)
	t.Run("reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext", reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext)
	t.Run("keepsTheTopLineOnScreenBothWays", keepsTheTopLineOnScreenBothWays)
}

func keepsTheTopLineOnScreenBothWays(t *testing.T) {
	hunks := diff.FileDiff{
		Hunks: []diff.Hunk{contextHunk(10, 24, "line"), contextHunk(40, 54, "line"), contextHunk(70, 84, "line")}}
	wholeFile := diff.FileDiff{Hunks: []diff.Hunk{contextHunk(1, 100, "line")}}
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 16})
	model = update(t, model, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext, fileDiff: hunks})
	model.diffView.SetYOffset(20)

	model = update(t, model, tea.KeyPressMsg{Code: 'f', Text: "f"})
	model = update(t, model, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: wholeFileContext, fileDiff: wholeFile})
	wholeFileOffset := model.diffView.YOffset()
	model = update(t, model, tea.KeyPressMsg{Code: 'f', Text: "f"})
	model = update(t, model, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext, fileDiff: hunks})
	hunkOffset := model.diffView.YOffset()

	require.Equal(t, 43, wholeFileOffset)
	require.Equal(t, 20, hunkOffset)
}

func TestARewrapKeepsTheTopDiffLineOnScreen(t *testing.T) {
	longLines := diff.FileDiff{Hunks: []diff.Hunk{contextHunk(1, 60, strings.Repeat("word ", 30))}}
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 16})
	model = update(t, model, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext, fileDiff: longLines})
	diffLines := model.diffLines()
	lineTwenty := firstReaching(diffLines, sourceLine{section: 0, oldNumber: 0, newNumber: 20})
	model.diffView.SetYOffset(lineTwenty)
	model = update(t, model, tea.KeyPressMsg{Code: 'u', Text: "u"})
	afterUnified := topLineNumber(model)
	model = update(t, model, tea.WindowSizeMsg{Width: 80, Height: 16})
	afterResize := topLineNumber(model)
	model = update(t, model, tea.KeyPressMsg{Code: 'd', Text: "d"})
	afterSideBySide := topLineNumber(model)

	require.Equal(t, 20, afterUnified)
	require.Equal(t, 20, afterResize)
	require.Equal(t, 20, afterSideBySide)
}

func TestNAndShiftNJumpBetweenChangesAndWrapAround(t *testing.T) {
	hunk := contextHunk(1, 100, "line")
	for _, number := range []int{30, 95, 96} {
		hunk.Rows[number-1] = diff.Row{Old: diff.Line{Kind: diff.LineRemoved, Number: number, Text: "old"},
			New: diff.Line{Kind: diff.LineAdded, Number: number, Text: "new"}}
	}
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 16})
	model = update(t, model, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{hunk}}})

	got := []int{}
	for _, press := range []tea.KeyPressMsg{{Code: 'n', Text: "n"}, {Code: 'n', Text: "n"}, {Code: 'n', Text: "n"},
		{Code: 'N', Text: "N"}, {Code: 'N', Text: "N"}} {
		model = update(t, model, press)
		got = append(got, topLineNumber(model))
	}

	require.Equal(t, []int{27, 89, 27, 89, 27}, got)
}

func TestNSkipsAChangeAlreadyAtTheTopOfTheScreen(t *testing.T) {
	hunk := contextHunk(1, 100, "line")
	for _, number := range []int{4, 60} {
		hunk.Rows[number-1] = diff.Row{Old: diff.Line{Kind: diff.LineRemoved, Number: number, Text: "old"},
			New: diff.Line{Kind: diff.LineAdded, Number: number, Text: "new"}}
	}
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 16})
	model = update(t, model, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{hunk}}})

	model = update(t, model, tea.KeyPressMsg{Code: 'n', Text: "n"})
	topLine := topLineNumber(model)

	require.Equal(t, 57, topLine, "want three lines above the change at 60")
}

func TestTheFooterFitsAndQuestionMarkListsEveryKey(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	footer := plainFooter(model)

	model = update(t, model, tea.KeyPressMsg{Code: '?', Text: "?"})
	renderedKeyList := model.renderKeyListPopup()
	keyList := ansi.Strip(renderedKeyList)
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})

	require.NotContains(t, footer, "…")
	require.Contains(t, footer, "? all keys")
	for _, want := range []string{"discard", "next change", "unified", "overview", "quit"} {
		require.Contains(t, keyList, want)
	}
	require.Equal(t, noPopup, model.popup.kind)
}

func TestATakeFailureStaysInTheFooterAcrossKeysAndFocusSwitchesUntilItClears(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: unstagedFiles()})
	model = update(t, model, tea.BlurMsg{})
	model = update(t, model, TakeFailure{Reason: "claude/settings.json would be overwritten"})

	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	afterKey := plainFooter(model)
	model = update(t, model, focusOnOne("C:/dev/other"))
	afterFocusSwitch := plainFooter(model)
	switchedDir := model.repository.Dir
	model = update(t, model, TakeFailure{Reason: ""})
	afterTake := plainFooter(model)

	require.Equal(t, "C:/dev/other", switchedDir)
	require.Contains(t, afterKey, "claude/settings.json would be overwritten")
	require.Contains(t, afterFocusSwitch, "claude/settings.json would be overwritten")
	require.Contains(t, afterTake, "? all keys")
}

func TestAnOwnerStateFailureShowsBesideATakeFailureUntilAWriteSucceeds(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: unstagedFiles()})
	model = update(t, model, tea.BlurMsg{})
	model = update(t, model, TakeFailure{Reason: "a.go would be overwritten"})
	model = update(t, model, OwnerStateFailure{Reason: "owner state not written: ssh agent-host: the shell exited"})
	bothFailed := plainFooter(model)
	model = update(t, model, TakeFailure{Reason: ""})
	writeFailed := plainFooter(model)
	model = update(t, model, OwnerStateFailure{Reason: ""})
	written := plainFooter(model)

	require.Contains(t, bothFailed, "a.go would be overwritten, owner state not written: ssh agent-host")
	require.Contains(t, writeFailed, "✗ owner state not written: ssh agent-host")
	require.Contains(t, written, "? all keys")
}

func topLineNumber(model Model) int {
	lines := model.diffLines()
	offset := model.diffView.YOffset()
	return lines[offset].source.newNumber
}

func contextHunk(first int, last int, text string) diff.Hunk {
	hunk := diff.Hunk{Header: fmt.Sprintf("@@ -%d +%d @@", first, first)}
	for number := first; number <= last; number++ {
		line := diff.Line{Kind: diff.LineContext, Number: number, Text: text}
		hunk.Rows = append(hunk.Rows, diff.Row{Old: line, New: line})
	}
	return hunk
}

func dropsAFileDiffLoadedWithTheOtherContext(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	next, reload := model.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	model = asModel(t, next)

	model = update(t, model, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{{Header: "@@ -5 +5 @@"}}}})
	afterStaleLoad := model.fileDiff
	model = update(t, model, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: wholeFileContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{{Header: "@@ -1,9 +1,9 @@"}}}})

	require.NotNil(t, reload)
	require.Empty(t, afterStaleLoad.Hunks)
	require.Len(t, model.fileDiff.Hunks, 1)
	require.Equal(t, "@@ -1,9 +1,9 @@", model.fileDiff.Hunks[0].Header)
}

func reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext(t *testing.T) {
	commit := gitrepo.Commit{Hash: strings.Repeat("1", 40), Message: "Add tree"}
	model := logOf(t, []gitrepo.Commit{commit})
	model = update(t, model, commitDiffMsg{dir: "C:/dev/repo", hash: commit.Hash, contextLines: hunkContext,
		sections: []diff.FileSection{{Path: "a.go"}}})
	hunkContextLoad := model.loadSelectedCommitDiff()

	model = update(t, model, tea.KeyPressMsg{Code: 'f', Text: "f"})
	wholeFileLoad := model.loadSelectedCommitDiff()
	model = update(t, model, tea.KeyPressMsg{Code: 'f', Text: "f"})
	backToHunksLoad := model.loadSelectedCommitDiff()

	require.Nil(t, hunkContextLoad)
	require.NotNil(t, wholeFileLoad)
	require.Nil(t, backToHunksLoad)
}

func TestDiscardOpensOnTheSelectedFile(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})

	model = update(t, model, tea.KeyPressMsg{Code: 'x', Text: "x"})
	popup := plainConfirmPopup(model)

	require.Equal(t, confirmPopup, model.popup.kind)
	require.Contains(t, popup, "discard b.go")
	require.Contains(t, popup, "back to HEAD")
}

func findNothing(context.Context) (focus.Found, error) {
	return focus.Found{Repositories: []focus.FoundRepository{}, Refusals: []focus.FoundRefusal{}}, nil
}

func TestHeaderNamesLocationAndCountsBranchesAndWorkTrees(t *testing.T) {
	t.Run("theMainWorkTreeHeaderNamesTheCloneAndCountsRemoteBranches",
		theMainWorkTreeHeaderNamesTheCloneAndCountsRemoteBranches)
	t.Run("aLinkedWorkTreeHeaderNamesItsPathAndOmitsRemoteBranches",
		aLinkedWorkTreeHeaderNamesItsPathAndOmitsRemoteBranches)
}

func theMainWorkTreeHeaderNamesTheCloneAndCountsRemoteBranches(t *testing.T) {
	header := headerReviewing(t, "C:/dev/repo", 37)

	trimmed := strings.TrimRight(header, " ")
	startsWithLocation := strings.HasPrefix(header, " "+folderGlyph+" repo  ")
	endsWithCounts := strings.HasSuffix(trimmed, "3 branches (37 remote) · 2 work trees")
	require.True(t, startsWithLocation, header)
	require.True(t, endsWithCounts, header)
}

func aLinkedWorkTreeHeaderNamesItsPathAndOmitsRemoteBranches(t *testing.T) {
	header := headerReviewing(t, "C:/dev/worktrees/repo/side", 0)

	trimmed := strings.TrimRight(header, " ")
	startsWithLocation := strings.HasPrefix(header, " "+folderGlyph+" repo/side  ")
	endsWithCounts := strings.HasSuffix(trimmed, "3 branches · 2 work trees")
	require.True(t, startsWithLocation, header)
	require.True(t, endsWithCounts, header)
}

func headerReviewing(t *testing.T, dir string, remoteBranchCount int) string {
	t.Helper()
	workTrees := []gitrepo.WorkTree{{Path: "C:/dev/repo"}, {Path: "C:/dev/worktrees/repo/side"}}
	model := New(findNothing, openAnything, nil, AgentHost{})
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model = update(t, model, focusOnOne(dir))
	model = update(t, model, statusMsg{dir: dir, status: gitrepo.Status{Branch: "main"}, branchCount: 3,
		remoteBranchCount: remoteBranchCount, workTrees: workTrees})
	return plainHeader(model)
}

func focusOnOne(dir string) focus.Focus {
	return focus.Focus{Repositories: []gitrepo.Repository{{Dir: dir}}, Refusals: []error{}}
}

func reviewingRepository(t *testing.T, status gitrepo.Status) Model {
	t.Helper()
	model := New(findNothing, openAnything, nil, AgentHost{})
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model = update(t, model, focusOnOne("C:/dev/repo"))
	return update(t, model, statusMsg{dir: "C:/dev/repo", status: status})
}

func TestAFocusChangeWhilePerusalsTerminalHasFocus(t *testing.T) {
	t.Run("waitsWhileOnlyADiffIsOpenAndTheHeaderNamesIt", waitsWhileOnlyADiffIsOpenAndTheHeaderNamesIt)
	t.Run("appliesOnBlurAndClosesTheCommitPopupAndItsDraft", appliesOnBlurAndClosesTheCommitPopupAndItsDraft)
	t.Run("appliesAtOnceWhileBlurredAcrossSwitches", appliesAtOnceWhileBlurredAcrossSwitches)
	t.Run("waitsAgainOnceTheTerminalRegainsFocus", waitsAgainOnceTheTerminalRegainsFocus)
	t.Run("appliesOnlyTheNewestOne", appliesOnlyTheNewestOne)
	t.Run("aRejectionWaitsAsRefusedAndShowsItsReasonOnlyOnBlur", aRejectionWaitsAsRefusedAndShowsItsReasonOnlyOnBlur)
	t.Run("aFocusAfterAWaitingRejectionReplacesIt", aFocusAfterAWaitingRejectionReplacesIt)
	t.Run("isDroppedByEnterOnAnotherRepositoryInTheOverview", isDroppedByEnterOnAnotherRepositoryInTheOverview)
}

func TestTheViewAsksTheTerminalToReportFocusAndBlur(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: unstagedFiles()})

	view := model.View()

	require.True(t, view.ReportFocus)
}

func waitsWhileOnlyADiffIsOpenAndTheHeaderNamesIt(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: unstagedFiles()})

	model = update(t, model, focusOnOne("C:/dev/other"))
	header := plainHeader(model)
	trimmed := strings.TrimRight(header, " ")
	namesNextFocus := strings.HasSuffix(trimmed, "   next focus: other")

	require.Equal(t, "C:/dev/repo", model.repository.Dir)
	require.True(t, namesNextFocus, header)
}

func appliesOnBlurAndClosesTheCommitPopupAndItsDraft(t *testing.T) {
	model := typingCommitDraft(t, "fix")
	model = update(t, model, focusOnOne("C:/dev/other"))
	waiting := model

	model = update(t, model, tea.BlurMsg{})
	header := plainHeader(model)
	draft := model.summaryBox.Value()

	require.Equal(t, commitPopup, waiting.popup.kind)
	require.Equal(t, "C:/dev/repo", waiting.repository.Dir)
	require.Equal(t, "C:/dev/other", model.repository.Dir)
	require.Equal(t, noPopup, model.popup.kind)
	require.Empty(t, draft)
	require.NotContains(t, header, "next focus")
}

func appliesAtOnceWhileBlurredAcrossSwitches(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: unstagedFiles()})
	model = update(t, model, tea.BlurMsg{})

	model = update(t, model, focusOnOne("C:/dev/first"))
	first := model.repository.Dir
	model = update(t, model, focusOnOne("C:/dev/second"))

	require.Equal(t, "C:/dev/first", first)
	require.Equal(t, "C:/dev/second", model.repository.Dir)
}

func waitsAgainOnceTheTerminalRegainsFocus(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: unstagedFiles()})
	model = update(t, model, tea.BlurMsg{})
	model = update(t, model, tea.FocusMsg{})

	model = update(t, model, focusOnOne("C:/dev/other"))

	require.Equal(t, "C:/dev/repo", model.repository.Dir)
}

func appliesOnlyTheNewestOne(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: unstagedFiles()})
	model = update(t, model, focusOnOne("C:/dev/first"))
	model = update(t, model, focusOnOne("C:/dev/second"))
	header := plainHeader(model)
	trimmed := strings.TrimRight(header, " ")
	namesSecond := strings.HasSuffix(trimmed, "next focus: second")

	model = update(t, model, tea.BlurMsg{})

	require.True(t, namesSecond, header)
	require.Equal(t, "C:/dev/second", model.repository.Dir)
}

func aRejectionWaitsAsRefusedAndShowsItsReasonOnlyOnBlur(t *testing.T) {
	model := typingCommitDraft(t, "fix")
	model = update(t, model, FocusRejection{Reason: "outside the dev directory"})
	waiting := model
	waitingHeader := plainHeader(waiting)
	trimmedHeader := strings.TrimRight(waitingHeader, " ")
	namesRefused := strings.HasSuffix(trimmedHeader, "   next focus: refused")
	waitingScreen := plainScreen(waiting)

	model = update(t, model, tea.BlurMsg{})
	screen := plainScreen(model)

	require.Equal(t, "C:/dev/repo", waiting.repository.Dir)
	require.Equal(t, commitPopup, waiting.popup.kind)
	require.True(t, namesRefused, waitingHeader)
	require.NotContains(t, waitingScreen, "outside the dev directory")
	require.Empty(t, model.repository.Dir)
	require.Equal(t, noPopup, model.popup.kind)
	require.Contains(t, screen, "outside the dev directory")
}

func aFocusAfterAWaitingRejectionReplacesIt(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: unstagedFiles()})
	model = update(t, model, FocusRejection{Reason: "outside the dev directory"})
	model = update(t, model, focusOnOne("C:/dev/other"))

	model = update(t, model, tea.BlurMsg{})

	require.Equal(t, "C:/dev/other", model.repository.Dir)
	require.Empty(t, model.rejection)
}

func isDroppedByEnterOnAnotherRepositoryInTheOverview(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: unstagedFiles()})
	model = update(t, model, tea.KeyPressMsg{Code: 'o', Text: "o"})
	model = update(t, model, overviewMsg{rows: []overviewRow{
		repositoryRow{FoundRepository: foundAt("repo", "C:/dev/repo")},
		repositoryRow{FoundRepository: foundAt("elsewhere", "C:/dev/elsewhere")},
	}})
	model = update(t, model, focusOnOne("C:/dev/other"))

	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	header := plainHeader(model)

	require.Equal(t, "C:/dev/elsewhere", model.repository.Dir)
	require.Nil(t, model.waitingFocus)
	require.NotContains(t, header, "next focus")
}

func TestAFocusThatKeepsTheReviewedRepositoryAppliesAtOnceAndDropsAWaitingRejection(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	model = update(t, model, FocusRejection{Reason: "outside the dev directory"})

	model = update(t, model, focus.Focus{Repositories: []gitrepo.Repository{{Dir: "C:/dev/repo"}, {Dir: "C:/dev/other"}},
		Refusals: []error{}})
	header := plainHeader(model)

	require.Equal(t, "C:/dev/repo", model.repository.Dir)
	require.Len(t, model.focused, 2)
	require.Nil(t, model.waitingFocus)
	require.NotContains(t, header, "next focus")
}

func typingCommitDraft(t *testing.T, summary string) Model {
	t.Helper()
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	model = update(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})
	return typeText(t, model, summary)
}

func unstagedFiles() []gitrepo.FileStatus {
	return []gitrepo.FileStatus{{Path: "a.go", IndexStatus: ' ', WorktreeStatus: 'M'}}
}

func TestFullscreenDiffCoversTheListsSoAClickThereSelectsNothing(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})

	model = update(t, model, tea.KeyPressMsg{Code: 'd', Text: "d"})
	model = update(t, model, tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})
	diffWidth := model.diffInnerWidth()

	require.Equal(t, 118, diffWidth)
	require.Equal(t, 0, model.selectedChange)
}

func TestADivergedBranchOffersAForcePush(t *testing.T) {
	model := pressPush(t, gitrepo.Status{
		Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix", Ahead: 2, Behind: 1},
	})

	popup := plainConfirmPopup(model)

	require.Equal(t, confirmPopup, model.popup.kind)
	require.Contains(t, popup, "replaces 1 commit on origin/fix with 2 commits")
}

func TestABranchTrackingAnotherNameIsPublishedNotPushedThere(t *testing.T) {
	model := pressPush(t, gitrepo.Status{
		Branch: "enrollment-links", Tracking: gitrepo.Tracking{Upstream: "origin/main", Ahead: 1}, HasCommits: true,
	})

	popup := plainConfirmPopup(model)

	require.Equal(t, confirmPopup, model.popup.kind)
	require.Contains(t, popup, "publish enrollment-links to origin")
	require.NotContains(t, popup, "origin/main")
}

func pressPush(t *testing.T, status gitrepo.Status) Model {
	t.Helper()
	model := reviewingRepository(t, status)
	return update(t, model, tea.KeyPressMsg{Code: 'P', Text: "P"})
}

func TestRightClickingAnOlderPushedCommitOffersToAmendItWithAForcePushWarning(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{
		Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix", Ahead: 1}, Files: twoFiles(), HasCommits: true,
	})
	model = update(t, model, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Parents: []string{strings.Repeat("2", 40)}},
		{Hash: strings.Repeat("2", 40), ShortHash: "2222222", Summary: "Fix wrap"},
	}})
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})

	model = update(t, model, tea.MouseClickMsg{X: 3, Y: 4, Button: tea.MouseRight})
	popup := plainConfirmPopup(model)

	require.Equal(t, confirmPopup, model.popup.kind)
	require.Contains(t, popup, "amend 2222222")
	require.Contains(t, popup, "with 1 file staged")
	require.Contains(t, popup, "rewrites every commit above it")
	require.Contains(t, popup, "the next push is a force push")
}

func TestRewordFillsInTheSelectedMessageAndEscBringsBackTheCommitDraft(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Branch: "fix", HasCommits: true})
	model = update(t, model, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Parents: []string{strings.Repeat("2", 40)}},
		{Hash: strings.Repeat("2", 40), ShortHash: "2222222", Summary: "Fix wrap", Message: "Fix wrap\n\nKeep words whole"},
	}})
	model = update(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = typeText(t, model, "WIP")
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})

	model = update(t, model, tea.KeyPressMsg{Code: 'r', Text: "r"})
	renderedPopup := model.renderRewordPopup()
	popup := ansi.Strip(renderedPopup)
	summary := model.summaryBox.Value()
	description := model.descriptionBox.Value()
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	draftSummary := model.summaryBox.Value()
	draftDescription := model.descriptionBox.Value()

	require.Equal(t, "Fix wrap", summary)
	require.Equal(t, "Keep words whole", description)
	require.Contains(t, popup, "reword 2222222")
	require.Contains(t, popup, "rewrites every commit above it")
	require.Equal(t, noPopup, model.popup.kind)
	require.Equal(t, "WIP", draftSummary)
	require.Empty(t, draftDescription)
}

func TestCommitMessageFields(t *testing.T) {
	t.Run("summaryWrapsALongLineInsteadOfScrollingIt", summaryWrapsALongLineInsteadOfScrollingIt)
	t.Run("summaryJoinsPastedLinesWithSpaces", summaryJoinsPastedLinesWithSpaces)
	t.Run("ctrlAThenBackspaceClearsOnlyTheFocusedField", ctrlAThenBackspaceClearsOnlyTheFocusedField)
}

func summaryWrapsALongLineInsteadOfScrollingIt(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Branch: "fix", HasCommits: true})
	model = update(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})
	summary := strings.Repeat("word ", 20) + "end"
	model = typeText(t, model, summary)

	renderedPopup := model.renderCommitPopup()
	popup := ansi.Strip(renderedPopup)
	typed := model.summaryBox.Value()
	rows := model.summaryBox.Height()

	require.Equal(t, summary, typed)
	require.Contains(t, popup, "word word")
	require.Contains(t, popup, "end")
	require.Equal(t, 2, rows)
}

func summaryJoinsPastedLinesWithSpaces(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Branch: "fix", HasCommits: true})
	model = update(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})

	model = update(t, model, tea.PasteMsg{Content: "Fix wrap\r\nin the\nsummary"})
	summary := model.summaryBox.Value()

	require.Equal(t, "Fix wrap in the summary", summary)
}

func ctrlAThenBackspaceClearsOnlyTheFocusedField(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Branch: "fix", HasCommits: true})
	model = update(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = typeText(t, model, "Fix wrap")
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model = typeText(t, model, "Keep words whole")
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model = typeText(t, model, "in the diff")

	model = update(t, model, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	description := model.descriptionBox.Value()
	summaryKept := model.summaryBox.Value()
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model = update(t, model, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	summary := model.summaryBox.Value()

	require.Empty(t, description)
	require.Equal(t, "Fix wrap", summaryKept)
	require.Empty(t, summary)
}

func TestLog(t *testing.T) {
	t.Run("selectionFollowsTheCommitWhenNewOnesArriveAbove", selectionFollowsTheCommitWhenNewOnesArriveAbove)
	t.Run("expandingACommitListsItsFilesUnderIt", expandingACommitListsItsFilesUnderIt)
	t.Run("searchKeepsOnlyMatchingMessagesIgnoringCase", searchKeepsOnlyMatchingMessagesIgnoringCase)
	t.Run("anUppercaseLetterMakesTheSearchCaseSensitive", anUppercaseLetterMakesTheSearchCaseSensitive)
	t.Run("searchNeedsEveryWordInAnyOrder", searchNeedsEveryWordInAnyOrder)
	t.Run("searchLooksAtTheAuthor", searchLooksAtTheAuthor)
	t.Run("searchLooksAtTheRefs", searchLooksAtTheRefs)
	t.Run("searchLooksAtTheHash", searchLooksAtTheHash)
	t.Run("aPasteIntoTheSearchFiltersTheLog", aPasteIntoTheSearchFiltersTheLog)
	t.Run("clickingTheFoldedLogOpensIt", clickingTheFoldedLogOpensIt)
	t.Run("clickingTheFoldedChangesDuringASearchClosesIt", clickingTheFoldedChangesDuringASearchClosesIt)
	t.Run("backspaceOnAnEmptySearchClosesIt", backspaceOnAnEmptySearchClosesIt)
	t.Run("nearingTheEndOfAFullPageLoadsTheNextOne", nearingTheEndOfAFullPageLoadsTheNextOne)
	t.Run("aShortLogIsTheWholeHistory", aShortLogIsTheWholeHistory)
	t.Run("aReloadReadWithTheOldLimitIsDropped", aReloadReadWithTheOldLimitIsDropped)
	t.Run("searchLoadsTheWholeHistory", searchLoadsTheWholeHistory)
}

func clickingTheFoldedLogOpensIt(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Files: twoFiles(), HasCommits: true})
	model = update(t, model, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40)}, {Hash: strings.Repeat("2", 40)},
	}})

	model = update(t, model, tea.MouseClickMsg{X: 3, Y: 27, Button: tea.MouseLeft})
	model = update(t, model, tea.MouseClickMsg{X: 3, Y: 4, Button: tea.MouseLeft})

	require.Equal(t, viewLog, model.view)
	require.Equal(t, 1, model.selectedCommit)
}

func clickingTheFoldedChangesDuringASearchClosesIt(t *testing.T) {
	model := logOf(t, []gitrepo.Commit{{Hash: strings.Repeat("1", 40)}})
	model = update(t, model, tea.KeyPressMsg{Code: '/', Text: "/"})
	foldedChangesY := model.foldedPaneY(viewChanges)

	model = update(t, model, tea.MouseClickMsg{X: 3, Y: foldedChangesY, Button: tea.MouseLeft})

	require.Equal(t, viewChanges, model.view)
	require.Equal(t, modeBrowsing, model.mode)
}

func backspaceOnAnEmptySearchClosesIt(t *testing.T) {
	model := logOf(t, []gitrepo.Commit{{Hash: strings.Repeat("1", 40)}})
	model = update(t, model, tea.KeyPressMsg{Code: '/', Text: "/"})
	model = update(t, model, tea.KeyPressMsg{Code: 'a', Text: "a"})

	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	afterDeleting := model.mode
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})

	require.Equal(t, modeSearching, afterDeleting)
	require.Equal(t, modeBrowsing, model.mode)
}

func expandingACommitListsItsFilesUnderIt(t *testing.T) {
	first := gitrepo.Commit{Hash: strings.Repeat("1", 40), Message: "Add tree"}
	second := gitrepo.Commit{Hash: strings.Repeat("2", 40), Message: "Fix wrap"}
	model := logOf(t, []gitrepo.Commit{first, second})
	model = update(t, model, commitDiffMsg{
		dir: "C:/dev/repo", hash: first.Hash, contextLines: hunkContext, sections: []diff.FileSection{
			{Path: "a.go", FileDiff: diff.FileDiff{Change: diff.FileAdded}},
			{Path: "b.go", FileDiff: diff.FileDiff{Change: diff.FileModified}},
		}})

	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	rows := model.visibleLogRows()
	title := model.commitPaneTitle()

	want := []treeRow{{commit: 0, file: commitRow}, {commit: 0, file: 0}, {commit: 0, file: 1},
		{commit: 1, file: commitRow}}
	require.Equal(t, want, rows)
	require.Equal(t, 1, model.selectedCommitFile)
	require.Equal(t, " b.go", title)
}

func searchKeepsOnlyMatchingMessagesIgnoringCase(t *testing.T) {
	model := logOf(t, []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Message: "Add tree"},
		{Hash: strings.Repeat("2", 40), Message: "Fix WRAP of long lines"},
	})

	model = typeSearch(t, model, "wr.p")
	rows := model.visibleLogRows()

	require.Equal(t, []treeRow{{commit: 1, file: commitRow}}, rows)
	require.Equal(t, 1, model.selectedCommit)
}

func anUppercaseLetterMakesTheSearchCaseSensitive(t *testing.T) {
	model := logOf(t, []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Message: "fix tree"},
		{Hash: strings.Repeat("2", 40), Message: "Fix wrap"},
	})

	model = typeSearch(t, model, "Fix")
	rows := model.visibleLogRows()

	require.Equal(t, []treeRow{{commit: 1, file: commitRow}}, rows)
}

func searchNeedsEveryWordInAnyOrder(t *testing.T) {
	model := logOf(t, []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Message: "Fix wrap"},
		{Hash: strings.Repeat("2", 40), Message: "Fix the log paging"},
		{Hash: strings.Repeat("3", 40), Message: "log tree"},
	})

	model = typeSearch(t, model, "log fix")
	rows := model.visibleLogRows()

	require.Equal(t, []treeRow{{commit: 1, file: commitRow}}, rows)
}

func searchLooksAtTheAuthor(t *testing.T) {
	log := logOf(t, authorRefAndHashCommits())

	model := typeSearch(t, log, "ada")
	rows := model.visibleLogRows()

	require.Equal(t, []treeRow{{commit: 0, file: commitRow}}, rows)
}

func searchLooksAtTheRefs(t *testing.T) {
	log := logOf(t, authorRefAndHashCommits())

	model := typeSearch(t, log, "v1")
	rows := model.visibleLogRows()

	require.Equal(t, []treeRow{{commit: 1, file: commitRow}}, rows)
}

func searchLooksAtTheHash(t *testing.T) {
	log := logOf(t, authorRefAndHashCommits())

	model := typeSearch(t, log, "3333")
	rows := model.visibleLogRows()

	require.Equal(t, []treeRow{{commit: 2, file: commitRow}}, rows)
}

// authorRefAndHashCommits share one message, so only the author Ada, the tag v1 or the hash 3333 tells them apart.
func authorRefAndHashCommits() []gitrepo.Commit {
	return []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Author: "Ada", Message: "Add tree"},
		{Hash: strings.Repeat("2", 40), Refs: []string{"tag: v1"}, Message: "Add tree"},
		{Hash: strings.Repeat("3", 40), Message: "Add tree"},
	}
}

func logOf(t *testing.T, commits []gitrepo.Commit) Model {
	t.Helper()
	model := New(findNothing, openAnything, nil, AgentHost{})
	model = update(t, model, focusOnOne("C:/dev/repo"))
	model = update(t, model, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: commits})
	return update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
}

func typeSearch(t *testing.T, model Model, pattern string) Model {
	t.Helper()
	model = update(t, model, tea.KeyPressMsg{Code: '/', Text: "/"})
	model = typeText(t, model, pattern)
	return update(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
}

func selectionFollowsTheCommitWhenNewOnesArriveAbove(t *testing.T) {
	first := gitrepo.Commit{Hash: strings.Repeat("1", 40)}
	second := gitrepo.Commit{Hash: strings.Repeat("2", 40)}
	added := gitrepo.Commit{Hash: strings.Repeat("3", 40)}
	model := logOf(t, []gitrepo.Commit{first, second})
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})

	model = update(t, model, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{added, first, second}})
	selected := model.selectedCommitHash()

	require.Equal(t, second.Hash, selected)
}

func dropsFilesLoadedForThePreviousRepository(t *testing.T) {
	model := New(findNothing, openAnything, nil, AgentHost{})
	model = update(t, model, focusOnOne("C:/dev/old"))
	model = update(t, model, tea.BlurMsg{})
	model = update(t, model, focusOnOne("C:/dev/new"))

	model = update(t, model, statusMsg{dir: "C:/dev/old", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})

	require.Empty(t, model.status.Files)
	require.Empty(t, model.status.Branch)
}

func rejectionClearsTheReviewAndThePopup(t *testing.T) {
	model := New(findNothing, openAnything, nil, AgentHost{})
	model = update(t, model, focusOnOne("C:/dev/repo"))
	model = update(t, model, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})
	model = update(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})
	model = update(t, model, tea.BlurMsg{})

	model = update(t, model, FocusRejection{Reason: "outside\nthe dev directory"})

	require.Empty(t, model.repository.Dir)
	require.Empty(t, model.status.Files)
	require.Equal(t, noPopup, model.popup.kind)
	require.Equal(t, "outside | the dev directory", model.rejection)
}

func refocusingTheSameRepositoryKeepsTheSelection(t *testing.T) {
	model := New(findNothing, openAnything, nil, AgentHost{})
	model = update(t, model, focusOnOne("C:/dev/repo"))
	model = update(t, model, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})
	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})

	model = update(t, model, focusOnOne("C:/dev/repo"))

	require.Equal(t, 1, model.selectedChange)
	require.Len(t, model.status.Files, 2)
}

func update(t *testing.T, model Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := model.Update(msg)
	return asModel(t, next)
}

func asModel(t *testing.T, next tea.Model) Model {
	t.Helper()
	nextModel, isModel := next.(Model)
	require.True(t, isModel, "got %T, want Model", next)
	return nextModel
}

func twoFiles() []gitrepo.FileStatus {
	return []gitrepo.FileStatus{
		{Path: "a.go", IndexStatus: ' ', WorktreeStatus: 'M'},
		{Path: "b.go", IndexStatus: 'M', WorktreeStatus: ' '},
	}
}

func nearingTheEndOfAFullPageLoadsTheNextOne(t *testing.T) {
	model := logOf(t, numberedCommits(logPageSize))
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})

	next, _ := selectLogRow(t, model, logPageSize-1)

	require.Equal(t, 2*logPageSize, next.logLimit)
}

func aShortLogIsTheWholeHistory(t *testing.T) {
	model := logOf(t, numberedCommits(3))
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})

	next, _ := selectLogRow(t, model, 2)

	require.Equal(t, logPageSize, next.logLimit)
}

func aReloadReadWithTheOldLimitIsDropped(t *testing.T) {
	model := logOf(t, numberedCommits(logPageSize))
	model = update(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	model, _ = selectLogRow(t, model, logPageSize-1)

	model = update(t, model, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: numberedCommits(2)})

	require.Len(t, model.commits, logPageSize)
	require.Equal(t, logPageSize-1, model.selectedCommit)
}

func searchLoadsTheWholeHistory(t *testing.T) {
	model := logOf(t, numberedCommits(logPageSize))

	model = typeSearch(t, model, "commit")

	require.Equal(t, wholeHistory, model.logLimit)
}

func numberedCommits(count int) []gitrepo.Commit {
	commits := []gitrepo.Commit{}
	for index := range count {
		commits = append(commits, gitrepo.Commit{Hash: fmt.Sprintf("%040d", index), Message: fmt.Sprintf("commit %d", index)})
	}
	return commits
}

func selectLogRow(t *testing.T, model Model, index int) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := model.selectLogRow(index)
	return asModel(t, next), cmd
}
