package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
}

func TestTheOverviewListsTheFocusedRepositoriesFirstAndMarksThem(t *testing.T) {
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})

	m = update(t, m, overviewLoadedMsg{rows: []overviewRow{
		repositoryRow{FoundRepository: listedAt("elsewhere", "C:/dev/elsewhere")},
		repositoryRow{FoundRepository: listedAt("first", "C:/dev/first")},
		repositoryRow{FoundRepository: listedAt("second", "C:/dev/second")},
	}, failures: []string{}})

	lines := listLines(m.overview, m.overviewCells, "no repositories found", 118, m.paneInnerHeight())
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
		Refusals:     []error{errors.New("C:/dev/notes: no .git")},
	})

	if m.repository.Dir != "C:/dev/first" || !m.footer.isError || !strings.Contains(m.footer.text, "C:/dev/notes: no .git") {
		t.Fatalf("got %s, footer %q, want C:/dev/first and the refusal", m.repository.Dir, m.footer.text)
	}
}

func focusOnSeveral(t *testing.T, uncommitted map[string]bool) Model {
	t.Helper()
	focused := []gitrepo.Repository{{Dir: "C:/dev/cwd"}, {Dir: "C:/dev/first"}, {Dir: "C:/dev/second"}}
	m := update(t, New(listNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, RepositoriesFocused{Repositories: focused, Refusals: []error{}})
	return update(t, m, uncommittedLoadedMsg{focused: focused, uncommitted: uncommitted})
}

func TestEnterInTheOverviewReviewsTheSelectedRepositoryAndKeepsTheFocus(t *testing.T) {
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = update(t, m, overviewLoadedMsg{rows: []overviewRow{
		repositoryRow{FoundRepository: listedAt("first", "C:/dev/first")},
		repositoryRow{FoundRepository: listedAt("org/elsewhere", "C:/dev/org/elsewhere")},
	}, failures: []string{}})
	screen := ansi.Strip(m.renderScreen())

	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if !strings.Contains(screen, "org/elsewhere") || !strings.Contains(screen, "1 of 2") {
		t.Fatalf("got screen %q, want both rows with first selected", screen)
	}
	if m.mode == modeOverview || m.repository.Dir != "C:/dev/org/elsewhere" || len(m.focused) != 3 {
		t.Fatalf("got mode %d, %s, %d focused, want the overview closed on org/elsewhere with 3 focused",
			m.mode, m.repository.Dir, len(m.focused))
	}
}

func TestARefusedRepositoryIsAMutedRowWithItsSummaryAndEnterShowsTheReason(t *testing.T) {
	list := func(context.Context) (focus.Found, error) {
		return focus.Found{Repositories: []focus.FoundRepository{}, Refusals: []focus.FoundRefusal{
			{RelativePath: "notes", Err: fmt.Errorf("list C:/dev/notes: %w", fs.ErrPermission)},
			{RelativePath: "agents", Err: fmt.Errorf("%w: C:/dev/agents/.git has no deny", focus.ErrGitDirUnprotected)},
		}}, nil
	}
	m := update(t, New(list), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = update(t, m, overviewRequest{listRepositories: list}.load())
	screen := ansi.Strip(m.renderScreen())

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)

	want := folderGlyph + " agents  not write-denied to the agent"
	if !strings.Contains(screen, want) || strings.Index(screen, "agents") > strings.Index(screen, "notes") {
		t.Fatalf("got screen:\n%s\nwant agents before notes, as %q", screen, want)
	}
	wantFooter := "agents is refused: " + focus.ErrGitDirUnprotected.Error() + ": C:/dev/agents/.git has no deny"
	if cmd != nil || m.mode != modeOverview || m.footer.text != wantFooter {
		t.Fatalf("got command %v, mode %d, footer %q, want the reason with the overview still open", cmd,
			m.mode, m.footer.text)
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
	pulled := repositoryRow{
		FoundRepository: listedAt("first", "C:/dev/first"),
		status: gitrepo.Status{
			Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main"}, HasCommits: true,
		},
	}

	m = update(t, m, overviewPulledMsg{row: pulled})

	if cmd == nil || !strings.Contains(pulling, "pulling...") {
		t.Fatalf("got command %v and screen %q, want a pull running and shown", cmd, pulling)
	}
	if !reflect.DeepEqual(m.overview.rows[0], pulled) || len(m.pulling) != 0 || m.footer.text != "pulled first" {
		t.Fatalf("got row %+v, pulling %v, footer %q, want the pulled row in place", m.overview.rows[0],
			m.pulling, m.footer.text)
	}
}

func aRowWithNoUpstreamIsNotPulled(t *testing.T) {
	m := update(t, overviewOfTwo(t), tea.KeyPressMsg{Code: 'j', Text: "j"})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = asModel(t, next)

	if cmd != nil || len(m.pulling) != 0 || !m.footer.isError {
		t.Fatalf("got command %v, pulling %v, footer %q, want a refusal", cmd, m.pulling, m.footer.text)
	}
}

func aRowWhoseUpstreamWasDeletedIsExplained(t *testing.T) {
	m := update(t, overviewOfTwo(t), overviewLoadedMsg{rows: []overviewRow{repositoryRow{
		FoundRepository: listedAt("first", "C:/dev/first"),
		status: gitrepo.Status{
			Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix", UpstreamGone: true}, HasCommits: true,
		},
	}}, failures: []string{}})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = asModel(t, next)

	if cmd != nil || m.footer.text != "first tracks origin/fix, which was deleted from the remote, so there is nothing to pull" {
		t.Fatalf("got command %v, footer %q, want the deleted upstream named", cmd, m.footer.text)
	}
}

func aFinishedRowIsSwitchedThoughItTracksNoUpstream(t *testing.T) {
	m := update(t, overviewOfTwo(t), overviewLoadedMsg{rows: []overviewRow{repositoryRow{
		FoundRepository: listedAt("first", "C:/dev/first"),
		status:          gitrepo.Status{Branch: "fix", HasCommits: true, Files: []gitrepo.FileStatus{}},
		defaultBranch:   "origin/main",
		finished:        true,
	}}, failures: []string{}})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = asModel(t, next)

	if cmd == nil || !m.pulling["C:/dev/first"] || m.footer.isError {
		t.Fatalf("got command %v, pulling %v, footer %q, want the switch running", cmd, m.pulling, m.footer.text)
	}
}

func TestMayBeFinished(t *testing.T) {
	changed := []gitrepo.FileStatus{{Path: "a.go", IndexStatus: ' ', WorktreeStatus: 'M'}}
	for name, test := range map[string]struct {
		status gitrepo.Status
		want   bool
	}{
		"aBranchWhoseUpstreamWasDeleted": {
			status: gitrepo.Status{Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix", UpstreamGone: true}},
			want:   true,
		},
		"aBranchThatNeverHadAnUpstream": {status: gitrepo.Status{Branch: "fix"}, want: true},
		"aBranchWhoseUpstreamStillExists": {
			status: gitrepo.Status{Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix"}},
		},
		"aBranchWithChanges": {status: gitrepo.Status{Branch: "fix", Files: changed}},
		"theDefaultBranchWithItsRemoteGone": {
			status: gitrepo.Status{Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main", UpstreamGone: true}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := mayBeFinished(test.status, "origin/main"); got != test.want {
				t.Fatalf("got %t, want %t", got, test.want)
			}
		})
	}
}

func overviewOfTwo(t *testing.T) Model {
	t.Helper()
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	return update(t, m, overviewLoadedMsg{rows: []overviewRow{
		repositoryRow{
			FoundRepository: listedAt("first", "C:/dev/first"),
			status: gitrepo.Status{
				Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main", Behind: 2}, HasCommits: true,
			},
		},
		repositoryRow{FoundRepository: listedAt("org/elsewhere", "C:/dev/org/elsewhere")},
	}, failures: []string{}})
}

func listedAt(relativePath string, dir string) focus.FoundRepository {
	return focus.FoundRepository{RelativePath: relativePath, Repository: gitrepo.Repository{Dir: dir}}
}

func TestWholeFile(t *testing.T) {
	t.Run("dropsAFileDiffLoadedWithTheOtherContext", dropsAFileDiffLoadedWithTheOtherContext)
	t.Run("reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext", reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext)
	t.Run("keepsTheTopLineOnScreenBothWays", keepsTheTopLineOnScreenBothWays)
}

func keepsTheTopLineOnScreenBothWays(t *testing.T) {
	hunks := diff.FileDiff{Hunks: []diff.Hunk{contextHunk(10, 24), contextHunk(40, 54), contextHunk(70, 84)}}
	wholeFile := diff.FileDiff{Hunks: []diff.Hunk{contextHunk(1, 100)}}
	m := reviewing(t, gitrepo.Status{Files: twoFiles()})
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 16})
	m = update(t, m, diffLoadedMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext, fileDiff: hunks})
	m.diffView.SetYOffset(20)

	m = update(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = update(t, m, diffLoadedMsg{dir: "C:/dev/repo", path: "a.go", contextLines: wholeFileContext, fileDiff: wholeFile})
	wholeFileOffset := m.diffView.YOffset()
	m = update(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = update(t, m, diffLoadedMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext, fileDiff: hunks})

	if wholeFileOffset != 43 || m.diffView.YOffset() != 20 {
		t.Fatalf("got offsets %d then %d, want line 43 on top at 43 then back at 20", wholeFileOffset, m.diffView.YOffset())
	}
}

func contextHunk(first int, last int) diff.Hunk {
	hunk := diff.Hunk{Header: fmt.Sprintf("@@ -%d +%d @@", first, first)}
	for number := first; number <= last; number++ {
		line := diff.Line{Kind: diff.LineContext, Number: number, Text: "line"}
		hunk.Rows = append(hunk.Rows, diff.Row{Old: line, New: line})
	}
	return hunk
}

func dropsAFileDiffLoadedWithTheOtherContext(t *testing.T) {
	m := reviewing(t, gitrepo.Status{Files: twoFiles()})
	next, reload := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = asModel(t, next)

	m = update(t, m, diffLoadedMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{{Header: "@@ -5 +5 @@"}}}})
	afterStaleLoad := m.fileDiff
	m = update(t, m, diffLoadedMsg{dir: "C:/dev/repo", path: "a.go", contextLines: wholeFileContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{{Header: "@@ -1,9 +1,9 @@"}}}})

	if reload == nil || len(afterStaleLoad.Hunks) != 0 || len(m.fileDiff.Hunks) != 1 || m.fileDiff.Hunks[0].Header != "@@ -1,9 +1,9 @@" {
		t.Fatalf("got reload %v, stale %v, shown %v, want only the whole file diff shown", reload, afterStaleLoad, m.fileDiff)
	}
}

func reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext(t *testing.T) {
	commit := gitrepo.Commit{Hash: strings.Repeat("1", 40), Message: "Add tree"}
	m := logOf(t, []gitrepo.Commit{commit})
	m = update(t, m, commitDiffLoadedMsg{dir: "C:/dev/repo", hash: commit.Hash, contextLines: hunkContext,
		sections: []diff.FileSection{{Path: "a.go"}}})
	hunkContextLoad := m.loadSelectedCommitDiff()

	m = update(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	wholeFileLoad := m.loadSelectedCommitDiff()
	m = update(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})

	if hunkContextLoad != nil || wholeFileLoad == nil || m.loadSelectedCommitDiff() != nil {
		t.Fatalf("got loads %v, %v, %v, want only the whole file one", hunkContextLoad, wholeFileLoad, m.loadSelectedCommitDiff())
	}
}

func TestDiscardOpensOnTheSelectedFile(t *testing.T) {
	m := reviewing(t, gitrepo.Status{Files: twoFiles()})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})

	popup := ansi.Strip(m.renderConfirmPopup())
	if m.popup.kind != confirmPopup || !strings.Contains(popup, "discard b.go") ||
		!strings.Contains(popup, "back to HEAD") {
		t.Fatalf("got popup %d:\n%s", m.popup.kind, popup)
	}
}

func listNothing(context.Context) (focus.Found, error) {
	return focus.Found{Repositories: []focus.FoundRepository{}, Refusals: []focus.FoundRefusal{}}, nil
}

func focusOn(dir string) RepositoriesFocused {
	return RepositoriesFocused{Repositories: []gitrepo.Repository{{Dir: dir}}, Refusals: []error{}}
}

func reviewing(t *testing.T, status gitrepo.Status) Model {
	t.Helper()
	m := update(t, New(listNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, focusOn("C:/dev/repo"))
	return update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: status})
}

func TestFullscreenDiffCoversTheListsSoAClickThereSelectsNothing(t *testing.T) {
	m := reviewing(t, gitrepo.Status{Files: twoFiles()})

	m = update(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = update(t, m, tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})

	if m.diffInnerWidth() != 118 || m.selectedChange != 0 {
		t.Fatalf("got diff width %d, selected file %d, want 118 and file 0", m.diffInnerWidth(), m.selectedChange)
	}
}

func TestADivergedBranchOffersAForcePush(t *testing.T) {
	m := pressPush(t, gitrepo.Status{
		Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix", Ahead: 2, Behind: 1},
	})

	popup := ansi.Strip(m.renderConfirmPopup())
	if m.popup.kind != confirmPopup || !strings.Contains(popup, "replaces 1 commit on origin/fix with 2 commits") {
		t.Fatalf("got popup %d:\n%s", m.popup.kind, popup)
	}
}

func pressPush(t *testing.T, status gitrepo.Status) Model {
	t.Helper()
	return update(t, reviewing(t, status), tea.KeyPressMsg{Code: 'P', Text: "P"})
}

func TestRightClickingAnOlderPushedCommitOffersToAmendItWithAForcePushWarning(t *testing.T) {
	m := reviewing(t, gitrepo.Status{
		Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix", Ahead: 1}, Files: twoFiles(), HasCommits: true,
	})
	m = update(t, m, logLoadedMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Parents: []string{strings.Repeat("2", 40)}},
		{Hash: strings.Repeat("2", 40), ShortHash: "2222222", Summary: "Fix wrap"},
	}})
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyTab})

	m = update(t, m, tea.MouseClickMsg{X: 3, Y: 4, Button: tea.MouseRight})

	popup := ansi.Strip(m.renderConfirmPopup())
	if m.popup.kind != confirmPopup || !strings.Contains(popup, "amend 2222222") ||
		!strings.Contains(popup, "with 1 file staged") || !strings.Contains(popup, "rewrites every commit above it") ||
		!strings.Contains(popup, "the next push is a force push") {
		t.Fatalf("got popup %d:\n%s", m.popup.kind, popup)
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
	m := reviewing(t, gitrepo.Status{Files: twoFiles(), HasCommits: true})
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
	if !reflect.DeepEqual(m.visibleLogRows(), want) || m.selectedCommitFile != 1 || m.commitPaneTitle() != " b.go" {
		t.Fatalf("got rows %v, selected file %d, title %q", m.visibleLogRows(), m.selectedCommitFile, m.commitPaneTitle())
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

	if m.repository.Dir != "" || len(m.status.Files) != 0 || m.popup.kind != noPopup || m.rejection != "outside | the dev directory" {
		t.Fatalf("got repository %q, %d files, popup %d, rejection %q", m.repository.Dir, len(m.status.Files), m.popup.kind,
			m.rejection)
	}
}

func refocusingTheSameRepositoryKeepsTheSelection(t *testing.T) {
	m := update(t, New(listNothing), focusOn("C:/dev/repo"))
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, focusOn("C:/dev/repo"))

	if m.selectedChange != 1 || len(m.status.Files) != 2 {
		t.Fatalf("got selection %d of %d files, want 1 of 2", m.selectedChange, len(m.status.Files))
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
