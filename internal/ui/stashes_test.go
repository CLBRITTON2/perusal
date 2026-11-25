package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestStashPane(t *testing.T) {
	t.Run("rowsShowTheSummaryThenTheBranchAndAge", rowsShowTheSummaryThenTheBranchAndAge)
	t.Run("aLoadedStashListsItsFilesUnderIt", aLoadedStashListsItsFilesUnderIt)
	t.Run("clickingTheFoldedStashPaneOpensItAndARowSelectsIt", clickingTheFoldedStashPaneOpensItAndARowSelectsIt)
	t.Run("dropKeepsTheSelectionInPlaceAfterTheReload", dropKeepsTheSelectionInPlaceAfterTheReload)
	t.Run("popNamesTheStashAndReloadsTheStatus", popNamesTheStashAndReloadsTheStatus)
	t.Run("stashesKeyOpensTheStashPaneAndLoadsTheSelectedDiff", stashesKeyOpensTheStashPaneAndLoadsTheSelectedDiff)
}

func stashesKeyOpensTheStashPaneAndLoadsTheSelectedDiff(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	model = update(t, model, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "main"}, stashes: threeStashes(),
		loadedAt: stashesLoadedAt()})

	next, loading := model.Update(tea.KeyPressMsg{Code: 'S', Text: "S"})
	model = asModel(t, next)

	require.Equal(t, viewStashes, model.view)
	require.NotNil(t, loading)
}

func rowsShowTheSummaryThenTheBranchAndAge(t *testing.T) {
	model := stashesOf(t)

	screen := plainScreen(model)

	require.Regexp(t, `Fix header\s+main · now`, screen)
	require.Regexp(t, `Try overview\s+main · 2h`, screen)
	require.Regexp(t, `Start\s+fix · 3d`, screen)
}

func aLoadedStashListsItsFilesUnderIt(t *testing.T) {
	model := stashesOf(t)
	loading := model.loadSelectedStashDiff()
	stashes := threeStashes()
	model = update(t, model, commitDiffMsg{dir: "C:/dev/repo", hash: stashes[0].Hash, contextLines: hunkContext,
		sections: []diff.FileSection{
			{Path: "a.go", FileDiff: diff.FileDiff{Change: diff.FileModified}},
			{Path: "b.go", FileDiff: diff.FileDiff{Change: diff.FileAdded}},
		}})

	model = update(t, model, tea.KeyPressMsg{Code: 'j', Text: "j"})
	loadingAgain := model.loadSelectedStashDiff()
	rows := model.visibleStashRows()
	title := model.commitPaneTitle()

	want := []treeRow{{commit: 0, file: commitRow}, {commit: 0, file: 0}, {commit: 0, file: 1},
		{commit: 1, file: commitRow}, {commit: 2, file: commitRow}}
	require.NotNil(t, loading)
	require.Nil(t, loadingAgain)
	require.Equal(t, want, rows)
	require.Equal(t, "1a2b3c4 a.go", title)
}

func clickingTheFoldedStashPaneOpensItAndARowSelectsIt(t *testing.T) {
	model := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	model = update(t, model, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "main"}, stashes: threeStashes(),
		loadedAt: stashesLoadedAt()})
	foldedStashPaneY := model.foldedPaneY(viewStashes)

	model = update(t, model, tea.MouseClickMsg{X: 3, Y: foldedStashPaneY, Button: tea.MouseLeft})
	model = update(t, model, tea.MouseClickMsg{X: 3, Y: 5, Button: tea.MouseLeft})

	require.Equal(t, viewStashes, model.view)
	require.Equal(t, 1, model.selectedStash)
}

func dropKeepsTheSelectionInPlaceAfterTheReload(t *testing.T) {
	stashPane := stashesOf(t)
	model := selectRow(t, stashPane, 1)
	model = update(t, model, tea.KeyPressMsg{Code: 'D', Text: "D"})
	popup := plainConfirmPopup(model)
	next, dropping := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = asModel(t, next)
	model = update(t, model, popupSuccessMsg{dir: "C:/dev/repo", notice: "dropped stash@{1}"})
	stashes := threeStashes()
	shifted := stashes[2]
	shifted.Ref = "stash@{1}"
	model = update(t, model, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "main"},
		stashes: []gitrepo.Stash{stashes[0], shifted}, loadedAt: stashesLoadedAt()})
	selected := model.stashes[model.selectedStash]

	require.Contains(t, popup, "drop stash@{1}")
	require.Contains(t, popup, "On main: Try overview")
	require.NotNil(t, dropping)
	require.Equal(t, "dropped stash@{1}", model.footer.text)
	require.Equal(t, shifted.Hash, selected.Hash)
}

func popNamesTheStashAndReloadsTheStatus(t *testing.T) {
	model := stashesOf(t)
	_, popping := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	next, reloading := model.Update(stashPopMsg{dir: "C:/dev/repo", ref: "stash@{0}"})
	model = asModel(t, next)

	require.NotNil(t, popping)
	require.NotNil(t, reloading)
	require.Equal(t, viewStashes, model.view)
	require.Equal(t, "popped stash@{0}", model.footer.text)
}

func stashesOf(t *testing.T) Model {
	t.Helper()
	model := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	model = update(t, model, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "main"}, stashes: threeStashes(),
		loadedAt: stashesLoadedAt()})
	model = update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	return update(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
}

func threeStashes() []gitrepo.Stash {
	loadedAt := stashesLoadedAt()
	return []gitrepo.Stash{
		{Ref: "stash@{0}", Branch: "main", Commit: gitrepo.Commit{Hash: strings.Repeat("a", 40), ShortHash: "1a2b3c4",
			Summary: "Fix header", Message: "WIP on main: 5d6e7f8 Fix header",
			AuthoredAt: loadedAt.Add(-30 * time.Second)}},
		{Ref: "stash@{1}", Branch: "main", Commit: gitrepo.Commit{Hash: strings.Repeat("b", 40), ShortHash: "2b3c4d5",
			Summary: "Try overview", Message: "On main: Try overview", AuthoredAt: loadedAt.Add(-2 * time.Hour)}},
		{Ref: "stash@{2}", Branch: "fix", Commit: gitrepo.Commit{Hash: strings.Repeat("c", 40), ShortHash: "3c4d5e6",
			Summary: "Start", Message: "WIP on fix: 6e7f8a9 Start", AuthoredAt: loadedAt.Add(-3 * 24 * time.Hour)}},
	}
}

func stashesLoadedAt() time.Time {
	return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
}
