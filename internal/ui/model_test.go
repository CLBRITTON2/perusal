package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestFocusSwitch(t *testing.T) {
	t.Run("dropsFilesLoadedForThePreviousRepository", dropsFilesLoadedForThePreviousRepository)
	t.Run("rejectionClearsTheReviewAndThePopup", rejectionClearsTheReviewAndThePopup)
	t.Run("refocusingTheSameRepositoryKeepsTheSelection", refocusingTheSameRepositoryKeepsTheSelection)
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
	m := update(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/repo"}})
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
	m = update(t, m, commitDiffLoadedMsg{dir: "C:/dev/repo", hash: first.Hash, sections: []diff.FileSection{
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
	m := update(t, New(), RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/repo"}})
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
	m := update(t, New(), RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/old"}})
	m = update(t, m, RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/new"}})

	m = update(t, m, statusLoadedMsg{dir: "C:/dev/old", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})

	if len(m.status.Files) != 0 || m.status.Branch != "" {
		t.Fatalf("got files %v on branch %q from the previous repository", m.status.Files, m.status.Branch)
	}
}

func rejectionClearsTheReviewAndThePopup(t *testing.T) {
	m := update(t, New(), RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/repo"}})
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})
	m = update(t, m, tea.KeyPressMsg{Code: 'c', Text: "c"})

	m = update(t, m, FocusRejected{Reason: "outside\nthe dev directory"})

	if m.repository.Dir != "" || len(m.status.Files) != 0 || m.committing || m.rejection != "outside | the dev directory" {
		t.Fatalf("got repository %q, %d files, committing %v, rejection %q", m.repository.Dir, len(m.status.Files), m.committing,
			m.rejection)
	}
}

func refocusingTheSameRepositoryKeepsTheSelection(t *testing.T) {
	m := update(t, New(), RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/repo"}})
	m = update(t, m, statusLoadedMsg{dir: "C:/dev/repo", status: gitrepo.Status{Branch: "master", Files: twoFiles()}})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/repo"}})

	if m.selected != 1 || len(m.status.Files) != 2 {
		t.Fatalf("got selection %d of %d files, want 1 of 2", m.selected, len(m.status.Files))
	}
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	model, isModel := next.(Model)
	if !isModel {
		t.Fatalf("Update returned %T, want Model", next)
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
	model, isModel := next.(Model)
	if !isModel {
		t.Fatalf("selectLogRow returned %T, want Model", next)
	}
	return model, cmd
}
