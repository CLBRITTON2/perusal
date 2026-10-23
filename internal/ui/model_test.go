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
	t.Run("aFocusWithADifferentFirstRepositoryPicksAgain", aFocusWithADifferentFirstRepositoryPicksAgain)
}

func TestTheOverviewListsTheFocusedRepositoriesFirstAndMarksThem(t *testing.T) {
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})

	m = update(t, m, overviewMsg{rows: []overviewRow{
		repositoryRow{FoundRepository: foundAt("elsewhere", "C:/dev/elsewhere")},
		repositoryRow{FoundRepository: foundAt("first", "C:/dev/first")},
		repositoryRow{FoundRepository: foundAt("second", "C:/dev/second")},
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

	m = update(t, m, focus.Focus{
		Repositories: []gitrepo.Repository{{Dir: "C:/dev/cwd"}, {Dir: "C:/dev/other"}, {Dir: "C:/dev/first"}},
		Refusals:     []error{errors.New("C:/dev/notes: no .git")},
	})

	if m.repository.Dir != "C:/dev/first" || !m.footer.isError || !strings.Contains(m.footer.text, "C:/dev/notes: no .git") {
		t.Fatalf("got %s, footer %q, want C:/dev/first and the refusal", m.repository.Dir, m.footer.text)
	}
}

func aFocusWithADifferentFirstRepositoryPicksAgain(t *testing.T) {
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	focused := []gitrepo.Repository{{Dir: "C:/dev/other"}, {Dir: "C:/dev/first"}}

	m = update(t, m, focus.Focus{Repositories: focused, Refusals: []error{}})
	uncommitted := map[string]bool{"C:/dev/other": true, "C:/dev/first": true}
	m = update(t, m, uncommittedMsg{focused: focused, uncommitted: uncommitted})

	if m.repository.Dir != "C:/dev/other" {
		t.Fatalf("got %s, want C:/dev/other", m.repository.Dir)
	}
}

func focusOnSeveral(t *testing.T, uncommitted map[string]bool) Model {
	t.Helper()
	focused := []gitrepo.Repository{{Dir: "C:/dev/cwd"}, {Dir: "C:/dev/first"}, {Dir: "C:/dev/second"}}
	m := update(t, New(findNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, focus.Focus{Repositories: focused, Refusals: []error{}})
	return update(t, m, uncommittedMsg{focused: focused, uncommitted: uncommitted})
}

func TestEnterInTheOverviewReviewsTheSelectedRepositoryAndKeepsTheFocus(t *testing.T) {
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = update(t, m, overviewMsg{rows: []overviewRow{
		repositoryRow{FoundRepository: foundAt("first", "C:/dev/first")},
		repositoryRow{FoundRepository: foundAt("org/elsewhere", "C:/dev/org/elsewhere")},
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
	m = update(t, m, overviewRequest{findRepositories: list}.load())
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
	m := overviewWithTwoRepositories(t)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = asModel(t, next)
	pulling := ansi.Strip(m.renderScreen())
	pulled := repositoryRow{
		FoundRepository: foundAt("first", "C:/dev/first"),
		status: gitrepo.Status{
			Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main"}, HasCommits: true,
		},
	}

	m = update(t, m, pullMsg{row: pulled})

	if cmd == nil || !strings.Contains(pulling, "pulling...") {
		t.Fatalf("got command %v and screen %q, want a pull running and shown", cmd, pulling)
	}
	if !reflect.DeepEqual(m.overview.rows[0], pulled) || len(m.pulling) != 0 || m.footer.text != "pulled first" {
		t.Fatalf("got row %+v, pulling %v, footer %q, want the pulled row in place", m.overview.rows[0],
			m.pulling, m.footer.text)
	}
}

func aRowWithNoUpstreamIsNotPulled(t *testing.T) {
	m := update(t, overviewWithTwoRepositories(t), tea.KeyPressMsg{Code: 'j', Text: "j"})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = asModel(t, next)

	if cmd != nil || len(m.pulling) != 0 || !m.footer.isError {
		t.Fatalf("got command %v, pulling %v, footer %q, want a refusal", cmd, m.pulling, m.footer.text)
	}
}

func aRowWhoseUpstreamWasDeletedIsExplained(t *testing.T) {
	m := update(t, overviewWithTwoRepositories(t), overviewMsg{rows: []overviewRow{repositoryRow{
		FoundRepository: foundAt("first", "C:/dev/first"),
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
	m := update(t, overviewWithTwoRepositories(t), overviewMsg{rows: []overviewRow{repositoryRow{
		FoundRepository: foundAt("first", "C:/dev/first"),
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

func TestPushInTheOverview(t *testing.T) {
	t.Run("aRepositoryNotUnderReviewClosesItsPopupOnItsResult", aRepositoryNotUnderReviewClosesItsPopupOnItsResult)
	t.Run("aBranchWithNoUpstreamIsPublished", aBranchWithNoUpstreamIsPublished)
}

func aRepositoryNotUnderReviewClosesItsPopupOnItsResult(t *testing.T) {
	m := update(t, overviewWithTwoRepositories(t), overviewMsg{rows: []overviewRow{repositoryRow{
		FoundRepository: foundAt("second", "C:/dev/second"),
		status: gitrepo.Status{
			Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main", Ahead: 1}, HasCommits: true,
		},
	}}, failures: []string{}})
	m = update(t, m, tea.KeyPressMsg{Code: 'P', Text: "P"})
	popup := ansi.Strip(m.renderConfirmPopup())

	next, cmd := m.Update(popupSuccessMsg{dir: "C:/dev/second", notice: "pushed to origin/main"})
	m = asModel(t, next)

	if !strings.Contains(popup, "push main to origin/main") || m.repository.Dir == "C:/dev/second" {
		t.Fatalf("got popup:\n%s\nfor repository %q, want a push of second", popup, m.repository.Dir)
	}
	if m.popup.kind != noPopup || m.footer.text != "pushed to origin/main" || cmd == nil {
		t.Fatalf("got popup %d, footer %q, command %v, want it closed and the row reloading", m.popup.kind,
			m.footer.text, cmd)
	}
}

func aBranchWithNoUpstreamIsPublished(t *testing.T) {
	m := update(t, overviewWithTwoRepositories(t), overviewMsg{rows: []overviewRow{repositoryRow{
		FoundRepository:   foundAt("second", "C:/dev/second"),
		status:            gitrepo.Status{Branch: "feature", HasCommits: true},
		commitsOnNoRemote: 2,
	}}, failures: []string{}})
	screen := ansi.Strip(m.renderScreen())

	m = update(t, m, tea.KeyPressMsg{Code: 'P', Text: "P"})

	popup := ansi.Strip(m.renderConfirmPopup())
	if !strings.Contains(screen, "↑2 unpublished") || !strings.Contains(popup, "publish feature to origin") {
		t.Fatalf("got screen:\n%s\npopup:\n%s\nwant the unpushed commits shown and offered", screen, popup)
	}
}

func TestTheOverviewDetailsTheSelectedRepository(t *testing.T) {
	m := update(t, overviewWithTwoRepositories(t), overviewMsg{rows: []overviewRow{
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
	}, failures: []string{}})
	first := ansi.Strip(m.renderScreen())

	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	for _, want := range []string{"changes", " M edited.go", "outgoing", "aaa1111 Add the local change", "incoming",
		"bbb2222 Add the remote change"} {
		if !strings.Contains(first, want) {
			t.Fatalf("got screen:\n%s\nwant %q", first, want)
		}
	}
	if second := ansi.Strip(m.renderScreen()); strings.Contains(second, "edited.go") ||
		strings.Contains(second, "aaa1111") {
		t.Fatalf("got screen:\n%s\nwant the details of second only", second)
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
		"aDetachedHead":      {status: gitrepo.Status{}},
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

func overviewWithTwoRepositories(t *testing.T) Model {
	t.Helper()
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	return update(t, m, overviewMsg{rows: []overviewRow{
		repositoryRow{
			FoundRepository: foundAt("first", "C:/dev/first"),
			status: gitrepo.Status{
				Branch: "main", Tracking: gitrepo.Tracking{Upstream: "origin/main", Behind: 2}, HasCommits: true,
			},
		},
		repositoryRow{FoundRepository: foundAt("org/elsewhere", "C:/dev/org/elsewhere")},
	}, failures: []string{}})
}

func foundAt(relativePath string, dir string) focus.FoundRepository {
	return focus.FoundRepository{RelativePath: relativePath, Repository: gitrepo.Repository{Dir: dir}}
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
	m := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 16})
	m = update(t, m, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext, fileDiff: hunks})
	m.diffView.SetYOffset(20)

	m = update(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = update(t, m, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: wholeFileContext, fileDiff: wholeFile})
	wholeFileOffset := m.diffView.YOffset()
	m = update(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = update(t, m, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext, fileDiff: hunks})

	if wholeFileOffset != 43 || m.diffView.YOffset() != 20 {
		t.Fatalf("got offsets %d then %d, want line 43 on top at 43 then back at 20", wholeFileOffset, m.diffView.YOffset())
	}
}

func TestARewrapKeepsTheTopDiffLineOnScreen(t *testing.T) {
	longLines := diff.FileDiff{Hunks: []diff.Hunk{contextHunk(1, 60, strings.Repeat("word ", 30))}}
	m := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 16})
	m = update(t, m, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext, fileDiff: longLines})
	m.diffView.SetYOffset(firstReaching(m.diffLines(), sourceLine{section: 0, oldNumber: 0, newNumber: 20}))
	m = update(t, m, tea.KeyPressMsg{Code: 'u', Text: "u"})
	afterUnified := topLineNumber(m)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 16})
	afterResize := topLineNumber(m)
	m = update(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})

	if afterUnified != 20 || afterResize != 20 || topLineNumber(m) != 20 {
		t.Fatalf("got top lines %d, %d, %d, want line 20 throughout", afterUnified, afterResize, topLineNumber(m))
	}
}

func TestNAndShiftNJumpBetweenChangesAndWrapAround(t *testing.T) {
	hunk := contextHunk(1, 100, "line")
	for _, number := range []int{30, 95, 96} {
		hunk.Rows[number-1] = diff.Row{Old: diff.Line{Kind: diff.LineRemoved, Number: number, Text: "old"},
			New: diff.Line{Kind: diff.LineAdded, Number: number, Text: "new"}}
	}
	m := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 16})
	m = update(t, m, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{hunk}}})

	got := []int{}
	for _, press := range []tea.KeyPressMsg{{Code: 'n', Text: "n"}, {Code: 'n', Text: "n"}, {Code: 'n', Text: "n"},
		{Code: 'N', Text: "N"}, {Code: 'N', Text: "N"}} {
		m = update(t, m, press)
		got = append(got, topLineNumber(m))
	}

	if !reflect.DeepEqual(got, []int{27, 89, 27, 89, 27}) {
		t.Fatalf("got top lines %v, want 27, 89, 27, 89, 27", got)
	}
}

func TestTheFooterFitsAndQuestionMarkListsEveryKey(t *testing.T) {
	m := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	footer := ansi.Strip(m.renderFooter())

	m = update(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	keyList := ansi.Strip(m.renderKeyListPopup())
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})

	if strings.Contains(footer, "…") || !strings.Contains(footer, "? all keys") {
		t.Fatalf("got footer %q, want it whole with ? all keys", footer)
	}
	for _, want := range []string{"discard", "next change", "unified", "overview", "quit"} {
		if !strings.Contains(keyList, want) {
			t.Fatalf("got key list:\n%s\nwant the key list to hold %q", keyList, want)
		}
	}
	if m.popup.kind != noPopup {
		t.Fatalf("got popup %d after esc, want none", m.popup.kind)
	}
}

func topLineNumber(m Model) int {
	return m.diffLines()[m.diffView.YOffset()].source.newNumber
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
	m := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	next, reload := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = asModel(t, next)

	m = update(t, m, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: hunkContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{{Header: "@@ -5 +5 @@"}}}})
	afterStaleLoad := m.fileDiff
	m = update(t, m, diffMsg{dir: "C:/dev/repo", path: "a.go", contextLines: wholeFileContext,
		fileDiff: diff.FileDiff{Hunks: []diff.Hunk{{Header: "@@ -1,9 +1,9 @@"}}}})

	if reload == nil || len(afterStaleLoad.Hunks) != 0 || len(m.fileDiff.Hunks) != 1 || m.fileDiff.Hunks[0].Header != "@@ -1,9 +1,9 @@" {
		t.Fatalf("got reload %v, stale %v, shown %v, want only the whole file diff shown", reload, afterStaleLoad, m.fileDiff)
	}
}

func reloadsACommitOnlyWhileItWasLoadedWithTheOtherContext(t *testing.T) {
	commit := gitrepo.Commit{Hash: strings.Repeat("1", 40), Message: "Add tree"}
	m := logOf(t, []gitrepo.Commit{commit})
	m = update(t, m, commitDiffMsg{dir: "C:/dev/repo", hash: commit.Hash, contextLines: hunkContext,
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
	m := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})

	popup := ansi.Strip(m.renderConfirmPopup())
	if m.popup.kind != confirmPopup || !strings.Contains(popup, "discard b.go") ||
		!strings.Contains(popup, "back to HEAD") {
		t.Fatalf("got popup %d:\n%s", m.popup.kind, popup)
	}
}

func findNothing(context.Context) (focus.Found, error) {
	return focus.Found{Repositories: []focus.FoundRepository{}, Refusals: []focus.FoundRefusal{}}, nil
}

func focusOnOne(dir string) focus.Focus {
	return focus.Focus{Repositories: []gitrepo.Repository{{Dir: dir}}, Refusals: []error{}}
}

func reviewingRepository(t *testing.T, status gitrepo.Status) Model {
	t.Helper()
	m := update(t, New(findNothing), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, focusOnOne("C:/dev/repo"))
	return update(t, m, statusMsg{dir: "C:/dev/repo", status: status})
}

func TestFullscreenDiffCoversTheListsSoAClickThereSelectsNothing(t *testing.T) {
	m := reviewingRepository(t, gitrepo.Status{Files: twoFiles()})

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

func TestABranchTrackingAnotherNameIsPublishedNotPushedThere(t *testing.T) {
	m := pressPush(t, gitrepo.Status{
		Branch: "enrollment-links", Tracking: gitrepo.Tracking{Upstream: "origin/main", Ahead: 1}, HasCommits: true,
	})

	popup := ansi.Strip(m.renderConfirmPopup())
	if m.popup.kind != confirmPopup || !strings.Contains(popup, "publish enrollment-links to origin") ||
		strings.Contains(popup, "origin/main") {
		t.Fatalf("got popup %d:\n%s\nwant a publish that never names origin/main", m.popup.kind, popup)
	}
}

func pressPush(t *testing.T, status gitrepo.Status) Model {
	t.Helper()
	return update(t, reviewingRepository(t, status), tea.KeyPressMsg{Code: 'P', Text: "P"})
}

func TestRightClickingAnOlderPushedCommitOffersToAmendItWithAForcePushWarning(t *testing.T) {
	m := reviewingRepository(t, gitrepo.Status{
		Branch: "fix", Tracking: gitrepo.Tracking{Upstream: "origin/fix", Ahead: 1}, Files: twoFiles(), HasCommits: true,
	})
	m = update(t, m, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{
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

func TestRewordFillsInTheSelectedMessageAndEscBringsBackTheCommitDraft(t *testing.T) {
	m := reviewingRepository(t, gitrepo.Status{Branch: "fix", HasCommits: true})
	m = update(t, m, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40), Parents: []string{strings.Repeat("2", 40)}},
		{Hash: strings.Repeat("2", 40), ShortHash: "2222222", Summary: "Fix wrap", Message: "Fix wrap\n\nKeep words whole"},
	}})
	m = update(t, m, tea.KeyPressMsg{Code: 'c', Text: "c"})
	for _, letter := range "WIP" {
		m = update(t, m, tea.KeyPressMsg{Code: letter, Text: string(letter)})
	}
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	popup := ansi.Strip(m.renderRewordPopup())
	summary, description := m.summaryInput.Value(), m.descriptionBox.Value()
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})

	if summary != "Fix wrap" || description != "Keep words whole" || !strings.Contains(popup, "reword 2222222") ||
		!strings.Contains(popup, "rewrites every commit above it") {
		t.Fatalf("got summary %q, description %q, popup:\n%s", summary, description, popup)
	}
	if m.popup.kind != noPopup || m.summaryInput.Value() != "WIP" || m.descriptionBox.Value() != "" {
		t.Fatalf("got popup %d, summary %q, description %q after esc, want the WIP draft back", m.popup.kind,
			m.summaryInput.Value(), m.descriptionBox.Value())
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
	t.Run("clickingTheFoldedChangesDuringASearchClosesIt", clickingTheFoldedChangesDuringASearchClosesIt)
	t.Run("nearingTheEndOfAFullPageLoadsTheNextOne", nearingTheEndOfAFullPageLoadsTheNextOne)
	t.Run("aShortLogIsTheWholeHistory", aShortLogIsTheWholeHistory)
	t.Run("aReloadReadWithTheOldLimitIsDropped", aReloadReadWithTheOldLimitIsDropped)
	t.Run("searchLoadsTheWholeHistory", searchLoadsTheWholeHistory)
}

func clickingTheFoldedLogOpensIt(t *testing.T) {
	m := reviewingRepository(t, gitrepo.Status{Files: twoFiles(), HasCommits: true})
	m = update(t, m, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{
		{Hash: strings.Repeat("1", 40)}, {Hash: strings.Repeat("2", 40)},
	}})

	m = update(t, m, tea.MouseClickMsg{X: 3, Y: 28, Button: tea.MouseLeft})
	m = update(t, m, tea.MouseClickMsg{X: 3, Y: 4, Button: tea.MouseLeft})

	if m.view != viewLog || m.selectedCommit != 1 {
		t.Fatalf("got view %d, selected commit %d, want the log open on commit 1", m.view, m.selectedCommit)
	}
}

func clickingTheFoldedChangesDuringASearchClosesIt(t *testing.T) {
	m := logOf(t, []gitrepo.Commit{{Hash: strings.Repeat("1", 40)}})
	m = update(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})

	m = update(t, m, tea.MouseClickMsg{X: 3, Y: m.foldedPaneY(), Button: tea.MouseLeft})

	if m.view != viewChanges || m.mode != modeBrowsing {
		t.Fatalf("got view %d, mode %d, want the changes view browsing", m.view, m.mode)
	}
}

func expandingACommitListsItsFilesUnderIt(t *testing.T) {
	first := gitrepo.Commit{Hash: strings.Repeat("1", 40), Message: "Add tree"}
	second := gitrepo.Commit{Hash: strings.Repeat("2", 40), Message: "Fix wrap"}
	m := logOf(t, []gitrepo.Commit{first, second})
	m = update(t, m, commitDiffMsg{
		dir: "C:/dev/repo", hash: first.Hash, contextLines: hunkContext, sections: []diff.FileSection{
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
	m := update(t, New(findNothing), focusOnOne("C:/dev/repo"))
	m = update(t, m, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: commits})
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

	m = update(t, m, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: []gitrepo.Commit{added, first, second}})

	if m.selectedCommitHash() != second.Hash {
		t.Fatalf("got selected commit %q, want %q", m.selectedCommitHash(), second.Hash)
	}
}

func dropsFilesLoadedForThePreviousRepository(t *testing.T) {
	m := update(t, New(findNothing), focusOnOne("C:/dev/old"))
	m = update(t, m, focusOnOne("C:/dev/new"))

	m = update(t, m, statusMsg{dir: "C:/dev/old", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})

	if len(m.status.Files) != 0 || m.status.Branch != "" {
		t.Fatalf("got files %v on branch %q from the previous repository", m.status.Files, m.status.Branch)
	}
}

func rejectionClearsTheReviewAndThePopup(t *testing.T) {
	m := update(t, New(findNothing), focusOnOne("C:/dev/repo"))
	m = update(t, m, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})
	m = update(t, m, tea.KeyPressMsg{Code: 'c', Text: "c"})

	m = update(t, m, FocusRejection{Reason: "outside\nthe dev directory"})

	if m.repository.Dir != "" || len(m.status.Files) != 0 || m.popup.kind != noPopup || m.rejection != "outside | the dev directory" {
		t.Fatalf("got repository %q, %d files, popup %d, rejection %q", m.repository.Dir, len(m.status.Files), m.popup.kind,
			m.rejection)
	}
}

func refocusingTheSameRepositoryKeepsTheSelection(t *testing.T) {
	m := update(t, New(findNothing), focusOnOne("C:/dev/repo"))
	m = update(t, m, statusMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, focusOnOne("C:/dev/repo"))

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

	m = update(t, m, logMsg{dir: "C:/dev/repo", limit: logPageSize, commits: numberedCommits(2)})

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
