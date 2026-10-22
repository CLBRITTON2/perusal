package ui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestFocusSwitch(t *testing.T) {
	t.Run("dropsFilesLoadedForThePreviousRepository", dropsFilesLoadedForThePreviousRepository)
	t.Run("rejectionClearsTheReviewAndThePopup", rejectionClearsTheReviewAndThePopup)
	t.Run("refocusingTheSameRepositoryKeepsTheSelection", refocusingTheSameRepositoryKeepsTheSelection)
	t.Run("opensTheFirstFocusedRepositoryWithUncommittedChanges", opensTheFirstFocusedRepositoryWithUncommittedChanges)
	t.Run("aFocusThatStillNamesTheReviewedRepositoryKeepsItAndShowsTheRefusals",
		aFocusThatStillNamesTheReviewedRepositoryKeepsItAndShowsTheRefusals)
}

func TestTheOverviewListsTheFocusedRepositoriesFirstAndMarksThem(t *testing.T) {
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})

	m = update(t, m, overviewLoadedMsg{rows: []overviewRow{
		{listed: ListedRepository{Name: "elsewhere", Repository: gitrepo.Repository{Dir: "C:/dev/elsewhere"}}},
		{listed: ListedRepository{Name: "first", Repository: gitrepo.Repository{Dir: "C:/dev/first"}}},
		{listed: ListedRepository{Name: "second", Repository: gitrepo.Repository{Dir: "C:/dev/second"}}},
	}, problems: []string{}})

	lines := m.overviewLines(118)
	got := []string{}
	for _, line := range lines {
		got = append(got, strings.Join(strings.Fields(ansi.Strip(line)), " "))
	}
	branch := " " + branchGlyph + " detached HEAD clean"
	want := []string{"▌● " + folderGlyph + " first" + branch, "● " + folderGlyph + " second" + branch,
		folderGlyph + " elsewhere" + branch}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func opensTheFirstFocusedRepositoryWithUncommittedChanges(t *testing.T) {
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": false, "C:/dev/second": true})

	if m.repository.Dir != "C:/dev/second" {
		t.Fatalf("got %s, want C:/dev/second", m.repository.Dir)
	}
}

func aFocusThatStillNamesTheReviewedRepositoryKeepsItAndShowsTheRefusals(t *testing.T) {
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})

	m = update(t, m, RepositoriesFocused{
		Repositories: []gitrepo.Repository{{Dir: "C:/dev/other"}, {Dir: "C:/dev/first"}},
		Refusals:     []string{"C:/dev/notes: no .git"},
	})

	if m.repository.Dir != "C:/dev/first" || !m.footerIsError || !strings.Contains(m.footerText, "C:/dev/notes: no .git") {
		t.Fatalf("got %s, footer %q, want C:/dev/first and the refusal", m.repository.Dir, m.footerText)
	}
}

// The focus lists cwd, first and second, in that order.
func focusOnSeveral(t *testing.T, uncommitted map[string]bool) Model {
	t.Helper()
	focused := []gitrepo.Repository{{Dir: "C:/dev/cwd"}, {Dir: "C:/dev/first"}, {Dir: "C:/dev/second"}}
	m := update(t, New(listNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, RepositoriesFocused{Repositories: focused, Refusals: []string{}})
	return update(t, m, uncommittedLoadedMsg{focused: focused, uncommitted: uncommitted})
}

func TestEnterInTheOverviewReviewsTheSelectedRepositoryAndKeepsTheFocus(t *testing.T) {
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = update(t, m, overviewLoadedMsg{rows: []overviewRow{
		{listed: ListedRepository{Name: "first", Repository: gitrepo.Repository{Dir: "C:/dev/first"}}},
		{listed: ListedRepository{Name: "org/elsewhere", Repository: gitrepo.Repository{Dir: "C:/dev/org/elsewhere"}}},
	}, problems: []string{}})
	screen := ansi.Strip(m.renderScreen())

	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if !strings.Contains(screen, "org/elsewhere") || !strings.Contains(screen, "1 of 2") {
		t.Fatalf("got screen %q, want both rows with first selected", screen)
	}
	if m.overview.open || m.repository.Dir != "C:/dev/org/elsewhere" || len(m.focused) != 3 {
		t.Fatalf("got overview open %v, %s, %d focused, want it closed on org/elsewhere with 3 focused",
			m.overview.open, m.repository.Dir, len(m.focused))
	}
}

func TestARefusedRepositoryIsAMutedRowWithItsSummaryAndEnterShowsTheReason(t *testing.T) {
	list := func(context.Context) (Listing, error) {
		return Listing{Repositories: []ListedRepository{}, Refusals: []RefusedRepository{
			{Name: "notes", Summary: "access denied", Reason: "list C:/dev/notes: access denied"},
			{Name: "agents", Summary: "not write-denied to the agent", Reason: "C:/dev/agents/.git has no deny"},
		}}, nil
	}
	m := update(t, New(list), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = update(t, m, overviewRequest{list: list}.load())
	screen := ansi.Strip(m.renderScreen())

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)

	want := folderGlyph + " agents  not write-denied to the agent"
	if !strings.Contains(screen, want) || strings.Index(screen, "agents") > strings.Index(screen, "notes") {
		t.Fatalf("got screen:\n%s\nwant agents before notes, as %q", screen, want)
	}
	if cmd != nil || !m.overview.open || m.footerText != "agents is refused: C:/dev/agents/.git has no deny" {
		t.Fatalf("got command %v, open %t, footer %q, want the reason with the overview still open", cmd,
			m.overview.open, m.footerText)
	}
}

func TestPullInTheOverview(t *testing.T) {
	t.Run("theRowShowsPullingUntilThePulledRowReplacesIt", theRowShowsPullingUntilThePulledRowReplacesIt)
	t.Run("aRowWithNoUpstreamIsNotPulled", aRowWithNoUpstreamIsNotPulled)
	t.Run("aRowWhoseUpstreamWasDeletedIsExplained", aRowWhoseUpstreamWasDeletedIsExplained)
	t.Run("aFinishedRowIsSwitchedThoughItTracksNoUpstream", aFinishedRowIsSwitchedThoughItTracksNoUpstream)
}

func theRowShowsPullingUntilThePulledRowReplacesIt(t *testing.T) {
	m := overviewOfTwo(t)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = asModel(t, next)
	pulling := ansi.Strip(m.renderScreen())
	pulled := overviewRow{
		listed: m.overview.rows[0].listed,
		status: gitrepo.Status{Branch: "main", Upstream: "origin/main", HasCommits: true},
	}

	m = update(t, m, overviewPulledMsg{row: pulled})

	if cmd == nil || !strings.Contains(pulling, "pulling...") {
		t.Fatalf("got command %v and screen %q, want a pull running and shown", cmd, pulling)
	}
	if m.overview.rows[0].status.Behind != 0 || len(m.overview.pulling) != 0 || m.footerText != "pulled first" {
		t.Fatalf("got row %+v, pulling %v, footer %q, want the pulled row in place", m.overview.rows[0],
			m.overview.pulling, m.footerText)
	}
}

func aRowWithNoUpstreamIsNotPulled(t *testing.T) {
	m := update(t, overviewOfTwo(t), tea.KeyPressMsg{Code: 'j', Text: "j"})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = asModel(t, next)

	if cmd != nil || len(m.overview.pulling) != 0 || !m.footerIsError {
		t.Fatalf("got command %v, pulling %v, footer %q, want a refusal", cmd, m.overview.pulling, m.footerText)
	}
}

func aRowWhoseUpstreamWasDeletedIsExplained(t *testing.T) {
	m := update(t, overviewOfTwo(t), overviewLoadedMsg{rows: []overviewRow{{
		listed: ListedRepository{Name: "first", Repository: gitrepo.Repository{Dir: "C:/dev/first"}},
		status: gitrepo.Status{Branch: "fix", Upstream: "origin/fix", UpstreamGone: true, HasCommits: true},
	}}, problems: []string{}})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = asModel(t, next)

	if cmd != nil || m.footerText != "first tracks origin/fix, which was deleted from the remote, so there is nothing to pull" {
		t.Fatalf("got command %v, footer %q, want the deleted upstream named", cmd, m.footerText)
	}
}

func aFinishedRowIsSwitchedThoughItTracksNoUpstream(t *testing.T) {
	m := update(t, overviewOfTwo(t), overviewLoadedMsg{rows: []overviewRow{{
		listed:        ListedRepository{Name: "first", Repository: gitrepo.Repository{Dir: "C:/dev/first"}},
		status:        gitrepo.Status{Branch: "fix", HasCommits: true, Files: []gitrepo.FileStatus{}},
		defaultBranch: "origin/main",
		finished:      true,
	}}, problems: []string{}})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = asModel(t, next)

	if cmd == nil || !m.overview.pulling["C:/dev/first"] || m.footerIsError {
		t.Fatalf("got command %v, pulling %v, footer %q, want the switch running", cmd, m.overview.pulling, m.footerText)
	}
}

func TestMayBeFinished(t *testing.T) {
	changed := []gitrepo.FileStatus{{Path: "a.go", IndexStatus: ' ', WorktreeStatus: 'M'}}
	for name, test := range map[string]struct {
		status gitrepo.Status
		want   bool
	}{
		"aBranchWhoseUpstreamWasDeleted": {
			status: gitrepo.Status{Branch: "fix", Upstream: "origin/fix", UpstreamGone: true}, want: true,
		},
		"aBranchThatNeverHadAnUpstream":     {status: gitrepo.Status{Branch: "fix"}, want: true},
		"aBranchWhoseUpstreamStillExists":   {status: gitrepo.Status{Branch: "fix", Upstream: "origin/fix"}},
		"aBranchWithChanges":                {status: gitrepo.Status{Branch: "fix", Files: changed}},
		"theDefaultBranchWithItsRemoteGone": {status: gitrepo.Status{Branch: "main", Upstream: "origin/main", UpstreamGone: true}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := mayBeFinished(test.status, "origin/main"); got != test.want {
				t.Fatalf("got %t, want %t", got, test.want)
			}
		})
	}
}

// first is 2 behind origin/main, and org/elsewhere tracks no upstream.
func overviewOfTwo(t *testing.T) Model {
	t.Helper()
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	return update(t, m, overviewLoadedMsg{rows: []overviewRow{
		{
			listed: ListedRepository{Name: "first", Repository: gitrepo.Repository{Dir: "C:/dev/first"}},
			status: gitrepo.Status{Branch: "main", Upstream: "origin/main", Behind: 2, HasCommits: true},
		},
		{listed: ListedRepository{Name: "org/elsewhere", Repository: gitrepo.Repository{Dir: "C:/dev/org/elsewhere"}}},
	}, problems: []string{}})
}

func TestWholeFile(t *testing.T) {
	t.Run("dropsAFileDiffLoadedWithTheOtherContext", dropsAFileDiffLoadedWithTheOtherContext)
	t.Run("reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext", reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext)
}

func dropsAFileDiffLoadedWithTheOtherContext(t *testing.T) {
	m := update(t, New(listNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, focusOn("C:/dev/repo"))
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Files: twoFiles()}})
	next, reload := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = asModel(t, next)

	m = update(t, m, diffLoadedMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{{Header: "@@ -5 +5 @@"}}}})
	stale := m.fileDiff
	m = update(t, m, diffLoadedMsg{dir: "C:/dev/repo", path: "a.go", contextLines: wholeFileContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{{Header: "@@ -1,9 +1,9 @@"}}}})

	if reload == nil || len(stale.Hunks) != 0 || len(m.fileDiff.Hunks) != 1 || m.fileDiff.Hunks[0].Header != "@@ -1,9 +1,9 @@" {
		t.Fatalf("got reload %v, stale %v, shown %v, want only the whole file diff shown", reload, stale, m.fileDiff)
	}
}

func reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext(t *testing.T) {
	commit := gitrepo.Commit{Hash: strings.Repeat("1", 40), Message: "Add tree"}
	m := logOf(t, []gitrepo.Commit{commit})
	m = update(t, m, commitDiffLoadedMsg{dir: "C:/dev/repo", hash: commit.Hash, contextLines: hunkContext,
		sections: []diff.FileSection{{Path: "a.go"}}})
	loadedAtFirst := m.loadSelectedCommitDiff()

	m = update(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	wholeFile := m.loadSelectedCommitDiff()
	m = update(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})

	if loadedAtFirst != nil || wholeFile == nil || m.loadSelectedCommitDiff() != nil {
		t.Fatalf("got loads %v, %v, %v, want only the whole file one", loadedAtFirst, wholeFile, m.loadSelectedCommitDiff())
	}
}

func TestDiscardOpensOnTheSelectedFile(t *testing.T) {
	m := update(t, New(listNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, focusOn("C:/dev/repo"))
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Files: twoFiles()}})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})

	popup := ansi.Strip(m.renderDiscardPopup())
	if m.popup != discardPopup || !strings.Contains(popup, "discard b.go") || !strings.Contains(popup, "back to HEAD") {
		t.Fatalf("got popup %d:\n%s", m.popup, popup)
	}
}

func listNothing(context.Context) (Listing, error) {
	return Listing{Repositories: []ListedRepository{}, Refusals: []RefusedRepository{}}, nil
}

func focusOn(dir string) RepositoriesFocused {
	return RepositoriesFocused{Repositories: []gitrepo.Repository{{Dir: dir}}, Refusals: []string{}}
}

func TestFullscreenDiffCoversTheListsSoAClickThereSelectsNothing(t *testing.T) {
	m := update(t, New(listNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, focusOn("C:/dev/repo"))
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Files: twoFiles()}})

	m = update(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = update(t, m, tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})

	if m.diffInnerWidth() != 118 || m.selected != 0 {
		t.Fatalf("got diff width %d, selected file %d, want 118 and file 0", m.diffInnerWidth(), m.selected)
	}
}

func TestADivergedBranchOffersAForcePush(t *testing.T) {
	m := pressPush(t, gitrepo.Status{Branch: "fix", Upstream: "origin/fix", Ahead: 2, Behind: 1})

	if m.popup != forcePushPopup || !strings.Contains(m.renderForcePushPopup(), "replaces 1 commit on origin/fix with 2 commits") {
		t.Fatalf("got popup %d:\n%s", m.popup, m.renderForcePushPopup())
	}
}

func pressPush(t *testing.T, status gitrepo.Status) Model {
	t.Helper()
	m := update(t, New(listNothing), focusOn("C:/dev/repo"))
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: status})
	return update(t, m, tea.KeyPressMsg{Code: 'P', Text: "P"})
}

func TestRightClickingAnOlderPushedCommitOffersToAmendItWithAForcePushWarning(t *testing.T) {
	m := update(t, New(listNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, focusOn("C:/dev/repo"))
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "fix", Upstream: "origin/fix",
		Ahead: 1, Files: twoFiles(), HasCommits: true}})
	m = update(t, m, logLoadedMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Parents: []string{strings.Repeat("2", 40)}},
		{Hash: strings.Repeat("2", 40), ShortHash: "2222222", Summary: "Fix wrap"},
	}})
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyTab})

	m = update(t, m, tea.MouseClickMsg{X: 3, Y: 4, Button: tea.MouseRight})

	popup := m.renderAmendPopup()
	if m.popup != amendPopup || m.amendTarget.Hash != strings.Repeat("2", 40) || !strings.Contains(popup, "with 1 file staged") ||
		!strings.Contains(popup, "rewrites every commit above it") || !strings.Contains(popup, "the next push is a force push") {
		t.Fatalf("got popup %d on %q:\n%s", m.popup, m.amendTarget.Hash, popup)
	}
}

func TestLog(t *testing.T) {
	t.Run("selectionFollowsTheCommitWhenNewOnesArriveAbove", selectionFollowsTheCommitWhenNewOnesArriveAbove)
	t.Run("expandingACommitListsItsFilesUnderIt", expandingACommitListsItsFilesUnderIt)
	t.Run("searchKeepsOnlyMatchingMessagesIgnoringCase", searchKeepsOnlyMatchingMessagesIgnoringCase)
	t.Run("anUppercaseLetterMakesTheSearchCaseSensitive", anUppercaseLetterMakesTheSearchCaseSensitive)
	t.Run("searchNeedsEveryWordInAnyOrder", searchNeedsEveryWordInAnyOrder)
	t.Run("searchLooksAtTheHashAuthorAndRefs", searchLooksAtTheHashAuthorAndRefs)
	t.Run("clickingTheFoldedLogOpensIt", clickingTheFoldedLogOpensIt)
	t.Run("nearingTheEndOfAFullPageLoadsTheNextOne", nearingTheEndOfAFullPageLoadsTheNextOne)
	t.Run("aShortLogIsTheWholeHistory", aShortLogIsTheWholeHistory)
	t.Run("aReloadReadWithTheOldLimitIsDropped", aReloadReadWithTheOldLimitIsDropped)
	t.Run("searchLoadsTheWholeHistory", searchLoadsTheWholeHistory)
}

func clickingTheFoldedLogOpensIt(t *testing.T) {
	m := update(t, New(listNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, focusOn("C:/dev/repo"))
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Files: twoFiles(), HasCommits: true}})
	m = update(t, m, logLoadedMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40)}, {Hash: strings.Repeat("2", 40)},
	}})

	m = update(t, m, tea.MouseClickMsg{X: 3, Y: 28, Button: tea.MouseLeft})
	m = update(t, m, tea.MouseClickMsg{X: 3, Y: 4, Button: tea.MouseLeft})

	if m.view != viewLog || m.selectedCommit != 1 {
		t.Fatalf("got view %d, selected commit %d, want the log open on commit 1", m.view, m.selectedCommit)
	}
}

func expandingACommitListsItsFilesUnderIt(t *testing.T) {
	first := gitrepo.Commit{Hash: strings.Repeat("1", 40), Message: "Add tree"}
	second := gitrepo.Commit{Hash: strings.Repeat("2", 40), Message: "Fix wrap"}
	m := logOf(t, []gitrepo.Commit{first, second})
	m = update(t, m, commitDiffLoadedMsg{dir: "C:/dev/repo", hash: first.Hash, contextLines: hunkContext, sections: []diff.FileSection{
		{Path: "a.go", FileDiff: diff.FileDiff{Change: diff.FileAdded}},
		{Path: "b.go", FileDiff: diff.FileDiff{Change: diff.FileModified}},
	}})

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	want := []logRow{{commit: 0, file: commitRow}, {commit: 0, file: 0}, {commit: 0, file: 1}, {commit: 1, file: commitRow}}
	if !reflect.DeepEqual(m.visibleLogRows(), want) || m.selectedFile != 1 || m.commitPaneTitle() != " b.go" {
		t.Fatalf("got rows %v, selected file %d, title %q", m.visibleLogRows(), m.selectedFile, m.commitPaneTitle())
	}
}

func searchKeepsOnlyMatchingMessagesIgnoringCase(t *testing.T) {
	m := logOf(t, []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Message: "Add tree"},
		{Hash: strings.Repeat("2", 40), Message: "Fix WRAP of long lines"},
	})

	m = typeSearch(t, m, "wr.p")

	if !reflect.DeepEqual(m.visibleLogRows(), []logRow{{commit: 1, file: commitRow}}) || m.selectedCommit != 1 {
		t.Fatalf("got rows %v, selected commit %d", m.visibleLogRows(), m.selectedCommit)
	}
}

func anUppercaseLetterMakesTheSearchCaseSensitive(t *testing.T) {
	m := logOf(t, []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Message: "fix tree"},
		{Hash: strings.Repeat("2", 40), Message: "Fix wrap"},
	})

	m = typeSearch(t, m, "Fix")

	if !reflect.DeepEqual(m.visibleLogRows(), []logRow{{commit: 1, file: commitRow}}) {
		t.Fatalf("got rows %v", m.visibleLogRows())
	}
}

func searchNeedsEveryWordInAnyOrder(t *testing.T) {
	m := logOf(t, []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Message: "Fix wrap"},
		{Hash: strings.Repeat("2", 40), Message: "Fix the log paging"},
		{Hash: strings.Repeat("3", 40), Message: "log tree"},
	})

	m = typeSearch(t, m, "log fix")

	if !reflect.DeepEqual(m.visibleLogRows(), []logRow{{commit: 1, file: commitRow}}) {
		t.Fatalf("got rows %v", m.visibleLogRows())
	}
}

func searchLooksAtTheHashAuthorAndRefs(t *testing.T) {
	commits := []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Author: "Ada", Message: "Add tree"},
		{Hash: strings.Repeat("2", 40), Refs: []string{"tag: v1"}, Message: "Add tree"},
		{Hash: strings.Repeat("3", 40), Message: "Add tree"},
	}
	for want, pattern := range []string{"ada", "v1", "3333"} {
		m := typeSearch(t, logOf(t, commits), pattern)

		if !reflect.DeepEqual(m.visibleLogRows(), []logRow{{commit: want, file: commitRow}}) {
			t.Fatalf("searching %q got rows %v, want commit %d", pattern, m.visibleLogRows(), want)
		}
	}
}

func logOf(t *testing.T, commits []gitrepo.Commit) Model {
	t.Helper()
	m := update(t, New(listNothing), focusOn("C:/dev/repo"))
	m = update(t, m, logLoadedMsg{dir: "C:/dev/repo", limit: logPageSize, commits: commits})
	return update(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
}

func typeSearch(t *testing.T, m Model, pattern string) Model {
	t.Helper()
	m = update(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, character := range pattern {
		m = update(t, m, tea.KeyPressMsg{Code: character, Text: string(character)})
	}
	return update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
}

func selectionFollowsTheCommitWhenNewOnesArriveAbove(t *testing.T) {
	first := gitrepo.Commit{Hash: strings.Repeat("1", 40)}
	second := gitrepo.Commit{Hash: strings.Repeat("2", 40)}
	added := gitrepo.Commit{Hash: strings.Repeat("3", 40)}
	m := update(t, logOf(t, []gitrepo.Commit{first, second}), tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, logLoadedMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{added, first, second}})

	if m.selectedCommitHash() != second.Hash {
		t.Fatalf("got selected commit %q, want %q", m.selectedCommitHash(), second.Hash)
	}
}

func dropsFilesLoadedForThePreviousRepository(t *testing.T) {
	m := update(t, New(listNothing), focusOn("C:/dev/old"))
	m = update(t, m, focusOn("C:/dev/new"))

	m = update(t, m, statusLoadedMsg{dir: "C:/dev/old", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})

	if len(m.status.Files) != 0 || m.status.Branch != "" {
		t.Fatalf("got files %v on branch %q from the previous repository", m.status.Files, m.status.Branch)
	}
}

func rejectionClearsTheReviewAndThePopup(t *testing.T) {
	m := update(t, New(listNothing), focusOn("C:/dev/repo"))
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})
	m = update(t, m, tea.KeyPressMsg{Code: 'c', Text: "c"})

	m = update(t, m, FocusRejected{Reason: "outside\nthe dev directory"})

	if m.repository.Dir != "" || len(m.status.Files) != 0 || m.popup != noPopup || m.rejection != "outside | the dev directory" {
		t.Fatalf("got repository %q, %d files, popup %d, rejection %q", m.repository.Dir, len(m.status.Files), m.popup,
			m.rejection)
	}
}

func refocusingTheSameRepositoryKeepsTheSelection(t *testing.T) {
	m := update(t, New(listNothing), focusOn("C:/dev/repo"))
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, focusOn("C:/dev/repo"))

	if m.selected != 1 || len(m.status.Files) != 2 {
		t.Fatalf("got selection %d of %d files, want 1 of 2", m.selected, len(m.status.Files))
	}
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return asModel(t, next)
}

func asModel(t *testing.T, next tea.Model) Model {
	t.Helper()
	model, isModel := next.(Model)
	if !isModel {
		t.Fatalf("got %T, want Model", next)
	}
	return model
}

func twoFiles() []gitrepo.FileStatus {
	return []gitrepo.FileStatus{
		{Path: "a.go", IndexStatus: ' ', WorktreeStatus: 'M'},
		{Path: "b.go", IndexStatus: 'M', WorktreeStatus: ' '},
	}
}

func nearingTheEndOfAFullPageLoadsTheNextOne(t *testing.T) {
	m := update(t, logOf(t, numberedCommits(logPageSize)), tea.WindowSizeMsg{Width: 120, Height: 30})

	next, _ := selectLogRow(t, m, logPageSize-1)

	if next.logLimit != 2*logPageSize {
		t.Fatalf("got limit %d, want %d", next.logLimit, 2*logPageSize)
	}
}

func aShortLogIsTheWholeHistory(t *testing.T) {
	m := update(t, logOf(t, numberedCommits(3)), tea.WindowSizeMsg{Width: 120, Height: 30})

	next, _ := selectLogRow(t, m, 2)

	if next.logLimit != logPageSize {
		t.Fatalf("got limit %d, want %d", next.logLimit, logPageSize)
	}
}

func aReloadReadWithTheOldLimitIsDropped(t *testing.T) {
	m := update(t, logOf(t, numberedCommits(logPageSize)), tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = selectLogRow(t, m, logPageSize-1)

	m = update(t, m, logLoadedMsg{dir: "C:/dev/repo", limit: logPageSize, commits: numberedCommits(2)})

	if len(m.commits) != logPageSize || m.selectedCommit != logPageSize-1 {
		t.Fatalf("got %d commits, selected %d, want the page kept", len(m.commits), m.selectedCommit)
	}
}

func searchLoadsTheWholeHistory(t *testing.T) {
	m := logOf(t, numberedCommits(logPageSize))

	m = typeSearch(t, m, "commit")

	if m.logLimit != wholeHistory {
		t.Fatalf("got limit %d, want %d", m.logLimit, wholeHistory)
	}
}

func numberedCommits(count int) []gitrepo.Commit {
	commits := []gitrepo.Commit{}
	for index := range count {
		commits = append(commits, gitrepo.Commit{Hash: fmt.Sprintf("%040d", index), Message: fmt.Sprintf("commit %d", index)})
	}
	return commits
}

func selectLogRow(t *testing.T, m Model, index int) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.selectLogRow(index)
	return asModel(t, next), cmd
}
