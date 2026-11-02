package ui

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
	m := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	m = update(t, m, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "main"}, stashes: threeStashes(),
		loadedAt: stashesLoadedAt()})

	next, loading := m.Update(tea.KeyPressMsg{Code: 'S', Text: "S"})
	m = asModel(t, next)

	if m.view != viewStashes || loading == nil {
		t.Fatalf("got view %d, load %v, want the stash pane open with its diff loading", m.view, loading)
	}
}

func rowsShowTheSummaryThenTheBranchAndAge(t *testing.T) {
	m := stashesOf(t)

	screen := ansi.Strip(m.renderScreen())

	for _, want := range []string{`Fix header\s+main · now`, `Try overview\s+main · 2h`, `Start\s+fix · 3d`} {
		if !regexp.MustCompile(want).MatchString(screen) {
			t.Fatalf("got screen:\n%s\nwant %q", screen, want)
		}
	}
}

func aLoadedStashListsItsFilesUnderIt(t *testing.T) {
	m := stashesOf(t)
	loading := m.loadSelectedStashDiff()
	m = update(t, m, commitDiffMsg{dir: "C:/dev/repo", hash: threeStashes()[0].Hash, contextLines: hunkContext,
		sections: []diff.FileSection{
			{Path: "a.go", FileDiff: diff.FileDiff{Change: diff.FileModified}},
			{Path: "b.go", FileDiff: diff.FileDiff{Change: diff.FileAdded}},
		}})

	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	want := []treeRow{{commit: 0, file: commitRow}, {commit: 0, file: 0}, {commit: 0, file: 1},
		{commit: 1, file: commitRow}, {commit: 2, file: commitRow}}
	if loading == nil || m.loadSelectedStashDiff() != nil || !reflect.DeepEqual(m.visibleStashRows(), want) ||
		m.commitPaneTitle() != "1a2b3c4 a.go" {
		t.Fatalf("got load %v, rows %v, title %q", loading, m.visibleStashRows(), m.commitPaneTitle())
	}
}

func clickingTheFoldedStashPaneOpensItAndARowSelectsIt(t *testing.T) {
	m := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	m = update(t, m, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "main"}, stashes: threeStashes(),
		loadedAt: stashesLoadedAt()})

	m = update(t, m, tea.MouseClickMsg{X: 3, Y: m.foldedPaneY(viewStashes), Button: tea.MouseLeft})
	m = update(t, m, tea.MouseClickMsg{X: 3, Y: 5, Button: tea.MouseLeft})

	if m.view != viewStashes || m.selectedStash != 1 {
		t.Fatalf("got view %d, selected stash %d, want the stash pane open on stash 1", m.view, m.selectedStash)
	}
}

func dropKeepsTheSelectionInPlaceAfterTheReload(t *testing.T) {
	m := selectRow(t, stashesOf(t), 1)
	m = update(t, m, tea.KeyPressMsg{Code: 'D', Text: "D"})
	popup := ansi.Strip(m.renderConfirmPopup())
	next, dropping := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)
	m = update(t, m, popupSuccessMsg{dir: "C:/dev/repo", notice: "dropped stash@{1}"})
	stashes := threeStashes()
	shifted := stashes[2]
	shifted.Ref = "stash@{1}"
	m = update(t, m, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "main"},
		stashes: []gitrepo.Stash{stashes[0], shifted}, loadedAt: stashesLoadedAt()})

	if !strings.Contains(popup, "drop stash@{1}") || !strings.Contains(popup, "On main: Try overview") {
		t.Fatalf("got popup:\n%s", popup)
	}
	if dropping == nil || m.footer.text != "dropped stash@{1}" || m.stashes[m.selectedStash].Hash != shifted.Hash {
		t.Fatalf("got drop %v, footer %q, selected %d, want the older stash selected in its place", dropping,
			m.footer.text, m.selectedStash)
	}
}

func popNamesTheStashAndReloadsTheStatus(t *testing.T) {
	m := stashesOf(t)
	_, popping := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	next, reloading := m.Update(stashPopMsg{dir: "C:/dev/repo", ref: "stash@{0}"})
	m = asModel(t, next)

	if popping == nil || reloading == nil || m.view != viewStashes || m.footer.text != "popped stash@{0}" {
		t.Fatalf("got pop %v, reload %v, view %d, footer %q", popping, reloading, m.view, m.footer.text)
	}
}

func stashesOf(t *testing.T) Model {
	t.Helper()
	m := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	m = update(t, m, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "main"}, stashes: threeStashes(),
		loadedAt: stashesLoadedAt()})
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	return update(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
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
