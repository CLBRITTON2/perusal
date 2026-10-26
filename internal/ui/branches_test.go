package ui

import (
	"slices"
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
		"refusesABranchCheckedOutInAnotherWorkTree": {
			moves: 4, defaultBranch: "origin/main",
			want: "side is checked out in C:/dev/repo/.claude/worktrees/side, so it cannot be deleted",
		},
		"refusesARemoteBranch": {
			moves: 5, defaultBranch: "origin/main", want: "origin/fix-x is a remote branch, which perusal never deletes",
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := selectRow(t, branchesOf(t, gitrepo.Status{}, test.defaultBranch), test.moves)

			m = update(t, m, tea.KeyPressMsg{Code: 'D', Text: "D"})

			if m.popup.kind != noPopup || !m.footer.isError || m.footer.text != test.want {
				t.Fatalf("got popup %d, footer %q, want the refusal %q", m.popup.kind, m.footer.text, test.want)
			}
		})
	}
}

func aMergedBranchIsDeletedAfterAConfirmAndTheListReloadsInPlace(t *testing.T) {
	m := selectRow(t, branchesOf(t, gitrepo.Status{}, "origin/main"), 2)
	screen := ansi.Strip(m.renderScreen())
	m = update(t, m, tea.KeyPressMsg{Code: 'D', Text: "D"})
	popup := ansi.Strip(m.renderConfirmPopup())
	next, deleting := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)
	next, reloading := m.Update(popupSuccessMsg{dir: "C:/dev/repo", notice: "deleted done"})
	m = asModel(t, next)
	rows := sixBranches()
	m = update(t, m, branchesMsg{
		dir: "C:/dev/repo", rows: []branchRow{rows[0], rows[1], rows[3]}, defaultBranch: "origin/main"})

	if !strings.Contains(screen, "origin/done deleted") || !strings.Contains(screen, "merged into origin/main") ||
		!strings.Contains(screen, "in work tree side") {
		t.Fatalf("got screen:\n%s\nwant done marked merged with its upstream deleted and side's work tree", screen)
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
	m := selectRow(t, branchesOf(t, gitrepo.Status{}, "origin/main"), 3)
	m = update(t, m, tea.KeyPressMsg{Code: 'D', Text: "D"})
	popup := ansi.Strip(m.renderConfirmPopup())

	_, deleting := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !strings.Contains(popup, "delete wip") || !strings.Contains(popup, "origin/main lacks some of its changes") ||
		deleting == nil {
		t.Fatalf("got popup:\n%s\ndelete %v, want a warning and then the delete", popup, deleting)
	}
}

func TestTheCurrentBranchShowsMergedOnlyWithNoUncommittedChanges(t *testing.T) {
	for name, test := range map[string]struct {
		files      []gitrepo.FileStatus
		wantMerged bool
	}{
		"clean":                  {files: []gitrepo.FileStatus{}, wantMerged: true},
		"withUncommittedChanges": {files: twoFiles(), wantMerged: false},
	} {
		t.Run(name, func(t *testing.T) {
			m := reviewingRepository(t, gitrepo.Status{Branch: "fix", Files: test.files})
			m = update(t, m, tea.KeyPressMsg{Code: 'b', Text: "b"})
			m = update(t, m, branchesMsg{
				dir:           "C:/dev/repo",
				rows:          []branchRow{{branch: gitrepo.Branch{Name: "fix", Current: true}, mergedIntoDefault: true}},
				defaultBranch: "origin/main",
			})

			screen := ansi.Strip(m.renderScreen())

			if strings.Contains(screen, "merged into origin/main") != test.wantMerged {
				t.Fatalf("got screen:\n%s\nwant merged shown: %v", screen, test.wantMerged)
			}
		})
	}
}

func TestSwitchBranch(t *testing.T) {
	t.Run("isRefusedWithUncommittedChanges", isRefusedWithUncommittedChanges)
	t.Run("closesTheListAndReloadsTheStatus", closesTheListAndReloadsTheStatus)
	t.Run("toARemoteBranchCreatesItsLocalBranch", toARemoteBranchCreatesItsLocalBranch)
}

func isRefusedWithUncommittedChanges(t *testing.T) {
	for name, moves := range map[string]int{"local": 3, "remote": 5} {
		t.Run(name, func(t *testing.T) {
			m := selectRow(t, branchesOf(t, gitrepo.Status{Branch: "fix", Files: twoFiles()}, "origin/main"), moves)

			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = asModel(t, next)

			if cmd != nil || !m.footer.isError || m.mode != modeBranches {
				t.Fatalf("got command %v, footer %q, want a refusal with the list still open", cmd, m.footer.text)
			}
		})
	}
}

func toARemoteBranchCreatesItsLocalBranch(t *testing.T) {
	m := selectRow(t, branchesOf(t, gitrepo.Status{Branch: "fix"}, "origin/main"), 5)
	screen := ansi.Strip(m.renderScreen())

	_, switching := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !strings.Contains(screen, "origin/fix-x") || switching == nil {
		t.Fatalf("got screen:\n%s\nswitch %v, want origin/fix-x listed and switched to", screen, switching)
	}
}

func TestFetchInTheBranchListReloadsIt(t *testing.T) {
	m := branchesOf(t, gitrepo.Status{Branch: "fix"}, "origin/main")

	next, fetching := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = asModel(t, next)
	fetchingText := m.footer.text
	next, reloading := m.Update(branchesFetchedMsg{dir: "C:/dev/repo"})
	m = asModel(t, next)

	if fetching == nil || fetchingText != "fetching..." || reloading == nil || m.mode != modeBranches ||
		m.footer.text != "fetched" {
		t.Fatalf("got fetch %v (%q), reload %v, mode %d, footer %q, want a fetch then a reload of the open list",
			fetching, fetchingText, reloading, m.mode, m.footer.text)
	}
}

func closesTheListAndReloadsTheStatus(t *testing.T) {
	m := selectRow(t, branchesOf(t, gitrepo.Status{Branch: "fix"}, "origin/main"), 3)

	next, switching := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)
	next, reloading := m.Update(branchSwitchMsg{dir: "C:/dev/repo", branch: "wip"})
	m = asModel(t, next)

	if switching == nil || reloading == nil || m.mode == modeBranches || m.footer.text != "switched to wip" {
		t.Fatalf("got switch %v, reload %v, mode %d, footer %q, want the list closed after the switch", switching,
			reloading, m.mode, m.footer.text)
	}
}

func TestRemoteOnlyBranchesDropsEachOneALocalBranchShadows(t *testing.T) {
	local := []gitrepo.Branch{{Name: "main"}, {Name: "fix/y"}}
	remote := []gitrepo.RemoteBranch{
		{Ref: "origin/main", Name: "main"},
		{Ref: "origin/fix/x", Name: "fix/x"},
		{Ref: "team/fork/fix/y", Name: "fix/y"},
		{Ref: "team/fork/main-old", Name: "main-old"},
	}

	got := remoteOnlyBranches(local, remote)

	want := []gitrepo.RemoteBranch{remote[1], remote[3]}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func branchesOf(t *testing.T, status gitrepo.Status, defaultBranch string) Model {
	t.Helper()
	m := reviewingRepository(t, status)
	m = update(t, m, tea.KeyPressMsg{Code: 'b', Text: "b"})
	return update(t, m, branchesMsg{dir: "C:/dev/repo", rows: sixBranches(), defaultBranch: defaultBranch})
}

func selectRow(t *testing.T, m Model, moves int) Model {
	t.Helper()
	for range moves {
		m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	return m
}

func sixBranches() []branchRow {
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
		{branch: gitrepo.Branch{Name: "side", OtherWorkTree: "C:/dev/repo/.claude/worktrees/side"}},
		{remoteBranch: gitrepo.RemoteBranch{Ref: "origin/fix-x", Name: "fix-x"}},
	}
}
