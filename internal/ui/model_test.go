package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestJoinCommitMessage(t *testing.T) {
	t.Run("putsTheDescriptionBelowABlankLine", putsTheDescriptionBelowABlankLine)
	t.Run("leavesASummaryWithNoDescriptionAlone", leavesASummaryWithNoDescriptionAlone)
}

func putsTheDescriptionBelowABlankLine(t *testing.T) {
	got := joinCommitMessage("Fix list width", "\nFirst line\nsecond line\n\n")

	if got != "Fix list width\n\nFirst line\nsecond line" {
		t.Fatalf("got %q", got)
	}
}

func leavesASummaryWithNoDescriptionAlone(t *testing.T) {
	got := joinCommitMessage("Fix list width", " \n ")

	if got != "Fix list width" {
		t.Fatalf("got %q", got)
	}
}

func TestFocusSwitch(t *testing.T) {
	t.Run("dropsFilesLoadedForThePreviousRepository", dropsFilesLoadedForThePreviousRepository)
	t.Run("rejectionClearsTheReviewAndThePopup", rejectionClearsTheReviewAndThePopup)
	t.Run("refocusingTheSameRepositoryKeepsTheSelection", refocusingTheSameRepositoryKeepsTheSelection)
}

func dropsFilesLoadedForThePreviousRepository(t *testing.T) {
	m := update(t, New(), RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/old"}})
	m = update(t, m, RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/new"}})

	m = update(t, m, filesLoadedMsg{dir: "C:/dev/old", branch: "master", files: twoFiles()})

	if len(m.files) != 0 || m.branch != "" {
		t.Fatalf("got files %v on branch %q from the previous repository", m.files, m.branch)
	}
}

func rejectionClearsTheReviewAndThePopup(t *testing.T) {
	m := update(t, New(), RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/repo"}})
	m = update(t, m, filesLoadedMsg{dir: "C:/dev/repo", branch: "master", files: twoFiles()})
	m = update(t, m, tea.KeyPressMsg{Code: 'c', Text: "c"})

	m = update(t, m, FocusRejected{Reason: "outside\nthe dev directory"})

	if m.repository.Dir != "" || len(m.files) != 0 || m.committing || m.rejection != "outside | the dev directory" {
		t.Fatalf("got repository %q, %d files, committing %v, rejection %q", m.repository.Dir, len(m.files), m.committing,
			m.rejection)
	}
}

func refocusingTheSameRepositoryKeepsTheSelection(t *testing.T) {
	m := update(t, New(), RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/repo"}})
	m = update(t, m, filesLoadedMsg{dir: "C:/dev/repo", branch: "master", files: twoFiles()})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})

	m = update(t, m, RepositoryFocused{Repository: gitrepo.Repository{Dir: "C:/dev/repo"}})

	if m.selected != 1 || len(m.files) != 2 {
		t.Fatalf("got selection %d of %d files, want 1 of 2", m.selected, len(m.files))
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
