package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestDeleteBranch(t *testing.T) {
	t.Run("aMergedBranchIsDeletedAfterAConfirmAndTheListReloadsInPlace",
		aMergedBranchIsDeletedAfterAConfirmAndTheListReloadsInPlace)
	t.Run("anUnmergedBranchIsDeletedAfterAWarning", anUnmergedBranchIsDeletedAfterAWarning)
	for name, test := range map[string]struct {
		moves         int
		defaultBranch string
		want          string
	}{
		"refusesTheCurrentBranch": {moves: 0, defaultBranch: "origin/main", want: "fix is checked out, so it cannot be deleted"},
		"refusesTheDefaultBranch": {
			moves: 1, defaultBranch: "origin/main", want: "main is the default branch, which perusal never deletes",
		},
		"refusesABranchWhenTheDefaultBranchIsUnknown": {
			moves: 2, want: "origin's default branch is unknown, so perusal cannot rule out that done is it",
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := selectBranch(t, branchesOf(t, gitrepo.Status{}, test.defaultBranch), test.moves)

			m = update(t, m, tea.KeyPressMsg{Code: 'D', Text: "D"})

			if m.popup.kind != noPopup || !m.footer.isError || m.footer.text != test.want {
				t.Fatalf("got popup %d, footer %q, want the refusal %q", m.popup.kind, m.footer.text, test.want)
			}
		})
	}
}

func aMergedBranchIsDeletedAfterAConfirmAndTheListReloadsInPlace(t *testing.T) {
	m := selectBranch(t, branchesOf(t, gitrepo.Status{}, "origin/main"), 2)
	screen := ansi.Strip(m.renderScreen())
	m = update(t, m, tea.KeyPressMsg{Code: 'D', Text: "D"})
	popup := ansi.Strip(m.renderConfirmPopup())
	next, deleting := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)
	next, reloading := m.Update(popupSuccessMsg{dir: "C:/dev/repo", notice: "deleted done"})
	m = asModel(t, next)
	rows := fourBranches()
	m = update(t, m, branchesMsg{
		dir: "C:/dev/repo", rows: []branchRow{rows[0], rows[1], rows[3]}, defaultBranch: "origin/main"})

	if !strings.Contains(screen, "origin/done deleted") || !strings.Contains(screen, "merged into origin/main") {
		t.Fatalf("got screen:\n%s\nwant done marked merged with its upstream deleted", screen)
	}
	if !strings.Contains(popup, "delete done") || !strings.Contains(popup, "origin/main has all its changes") {
		t.Fatalf("got popup:\n%s", popup)
	}
	if deleting == nil || reloading == nil || m.popup.kind != noPopup || m.footer.text != "deleted done" ||
		m.branches.rows[m.branches.selected].branch.Name != "wip" {
		t.Fatalf("got delete %v, reload %v, popup %d, footer %q, selected %d, want wip selected after the delete",
			deleting, reloading, m.popup.kind, m.footer.text, m.branches.selected)
	}
}

func anUnmergedBranchIsDeletedAfterAWarning(t *testing.T) {
	m := selectBranch(t, branchesOf(t, gitrepo.Status{}, "origin/main"), 3)
	m = update(t, m, tea.KeyPressMsg{Code: 'D', Text: "D"})
	popup := ansi.Strip(m.renderConfirmPopup())

	_, deleting := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !strings.Contains(popup, "delete wip") || !strings.Contains(popup, "origin/main lacks some of its changes") ||
		deleting == nil {
		t.Fatalf("got popup:\n%s\ndelete %v, want a warning and then the delete", popup, deleting)
	}
}

func TestSwitchBranch(t *testing.T) {
	t.Run("isRefusedWithUncommittedChanges", isRefusedWithUncommittedChanges)
	t.Run("closesTheListAndReloadsTheStatus", closesTheListAndReloadsTheStatus)
}

func isRefusedWithUncommittedChanges(t *testing.T) {
	m := selectBranch(t, branchesOf(t, gitrepo.Status{Branch: "fix", Files: twoFiles()}, "origin/main"), 3)

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)

	if cmd != nil || !m.footer.isError || m.mode != modeBranches {
		t.Fatalf("got command %v, footer %q, want a refusal with the list still open", cmd, m.footer.text)
	}
}

func closesTheListAndReloadsTheStatus(t *testing.T) {
	m := selectBranch(t, branchesOf(t, gitrepo.Status{Branch: "fix"}, "origin/main"), 3)

	next, switching := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)
	next, reloading := m.Update(branchSwitchMsg{dir: "C:/dev/repo", branch: "wip"})
	m = asModel(t, next)

	if switching == nil || reloading == nil || m.mode == modeBranches || m.footer.text != "switched to wip" {
		t.Fatalf("got switch %v, reload %v, mode %d, footer %q, want the list closed after the switch", switching,
			reloading, m.mode, m.footer.text)
	}
}

func branchesOf(t *testing.T, status gitrepo.Status, defaultBranch string) Model {
	t.Helper()
	m := reviewingRepository(t, status)
	m = update(t, m, tea.KeyPressMsg{Code: 'b', Text: "b"})
	return update(t, m, branchesMsg{dir: "C:/dev/repo", rows: fourBranches(), defaultBranch: defaultBranch})
}

func selectBranch(t *testing.T, m Model, moves int) Model {
	t.Helper()
	for range moves {
		m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	return m
}

func fourBranches() []branchRow {
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
	}
}
