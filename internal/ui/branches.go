package ui

import (
	"context"
	"errors"
	"path"
	"slices"
	"sync"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// mergedIntoDefault stays false for the default branch itself and for a branch checked out in another work tree, whose
// uncommitted changes the check cannot see.
type branchRow struct {
	branch            gitrepo.Branch
	mergedIntoDefault bool
}

// defaultBranch is origin's, as DefaultBranch names it, and empty when origin/HEAD is unset.
type branchList struct {
	selectableList[branchRow]
	defaultBranch string
}

type branchesMsg struct {
	dir           string
	rows          []branchRow
	defaultBranch string
}

type branchesFailureMsg struct {
	dir string
	err error
}

type branchSwitchMsg struct {
	dir    string
	branch string
}

func (r repositoryRequest) loadBranches() tea.Msg {
	ctx := context.Background()
	dir := r.repository.Dir
	branches, err := r.repository.Branches(ctx)
	if err != nil {
		return branchesFailureMsg{dir: dir, err: err}
	}
	defaultBranch, err := r.repository.DefaultBranch(ctx)
	if err != nil {
		return branchesFailureMsg{dir: dir, err: err}
	}
	rows := make([]branchRow, len(branches))
	errs := make([]error, len(branches))
	var group sync.WaitGroup
	for index, branch := range branches {
		rows[index] = branchRow{branch: branch}
		if defaultBranch == "" || branch.Name == localBranch(defaultBranch) || branch.OtherWorkTree != "" {
			continue
		}
		group.Go(func() {
			rows[index].mergedIntoDefault, errs[index] = r.repository.MergedInto(ctx, "refs/heads/"+branch.Name, defaultBranch)
		})
	}
	group.Wait()
	if err := errors.Join(errs...); err != nil {
		return branchesFailureMsg{dir: dir, err: err}
	}
	return branchesMsg{dir: dir, rows: rows, defaultBranch: defaultBranch}
}

type branchRequest struct {
	repository gitrepo.Repository
	branch     string
}

func (r branchRequest) switchTo() tea.Msg {
	if err := r.repository.Switch(context.Background(), r.branch); err != nil {
		return gitFailureMsg{err: err}
	}
	return branchSwitchMsg{dir: r.repository.Dir, branch: r.branch}
}

func (r branchRequest) delete() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.DeleteBranch(context.Background(), r.branch), "deleted "+r.branch)
}

func (m Model) openBranches() (tea.Model, tea.Cmd) {
	if m.repository.Dir == "" {
		return m, nil
	}
	m.mode = modeBranches
	m.branches = branchList{selectableList: selectableList[branchRow]{rows: []branchRow{}}}
	return m, m.reloadBranches()
}

func (m Model) reloadBranches() tea.Cmd {
	if m.mode != modeBranches {
		return nil
	}
	return repositoryRequest{repository: m.repository}.loadBranches
}

// The first load selects the current branch, and a reload after a delete keeps the selection where it was.
func (m Model) withBranchesLoaded(msg branchesMsg) Model {
	var selected int
	if m.branches.loaded {
		selected = m.branches.selected
	} else {
		selected = slices.IndexFunc(msg.rows, func(row branchRow) bool { return row.branch.Current })
	}
	loadedBranches := selectableList[branchRow]{loaded: true, rows: msg.rows, selected: 0, offset: m.branches.offset}
	m.branches = branchList{selectableList: loadedBranches.withSelected(selected), defaultBranch: msg.defaultBranch}
	return m.withListScrolled()
}

func (m Model) updateBranches(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footer = footer{}
	switch {
	case key.Matches(msg, quitKey):
		return m, tea.Quit
	case key.Matches(msg, closeBranchesKey):
		m.mode = modeBrowsing
		return m, nil
	case key.Matches(msg, downKey):
		m.branches.selectableList = m.branches.withSelected(m.branches.selected + 1).scrolled(m.paneInnerHeight())
		return m, nil
	case key.Matches(msg, upKey):
		m.branches.selectableList = m.branches.withSelected(m.branches.selected - 1).scrolled(m.paneInnerHeight())
		return m, nil
	case key.Matches(msg, switchBranchKey):
		return m.switchToSelectedBranch()
	case key.Matches(msg, deleteBranchKey):
		return m.openDeleteBranchPopup(), nil
	}
	return m, nil
}

// git switch carries uncommitted changes along when they do not conflict, which would mix them into another branch.
func (m Model) switchToSelectedBranch() (tea.Model, tea.Cmd) {
	row, hasRows := m.branches.selectedRow()
	if !hasRows {
		return m, nil
	}
	branch := row.branch
	switch {
	case branch.Current:
		m.mode = modeBrowsing
		return m, nil
	case branch.OtherWorkTree != "":
		openRepository := m.openRepository
		return m, func() tea.Msg {
			repository, err := openRepository(context.Background(), branch.OtherWorkTree)
			if err != nil {
				return gitFailureMsg{err: err}
			}
			return workTreeOpenedMsg{repository: repository}
		}
	case len(m.status.Files) > 0:
		return m.withFooterError("the work tree has uncommitted changes, so perusal does not switch branches"), nil
	}
	return m, branchRequest{repository: m.repository, branch: branch.Name}.switchTo
}

func (m Model) openDeleteBranchPopup() Model {
	row, hasRows := m.branches.selectedRow()
	if !hasRows {
		return m
	}
	name := row.branch.Name
	switch {
	case row.branch.Current:
		return m.withFooterError(name + " is checked out, so it cannot be deleted")
	case row.branch.OtherWorkTree != "":
		return m.withFooterError(name + " is checked out in " + row.branch.OtherWorkTree + ", so it cannot be deleted")
	case m.branches.defaultBranch == "":
		return m.withFooterError("origin's default branch is unknown, so perusal cannot rule out that " + name +
			" is it")
	case row.branch.Name == localBranch(m.branches.defaultBranch):
		return m.withFooterError(name + " is the default branch, which perusal never deletes")
	}
	return m.withConfirmation(m.repository.Dir, confirmation{
		lines: []string{
			titleStyle.Render("delete " + sanitizeText(row.branch.Name)),
			renderMergedDeleteTarget(row, m.branches.defaultBranch),
		},
		confirmKey:  confirmDeleteKey,
		run:         branchRequest{repository: m.repository, branch: row.branch.Name}.delete,
		runningText: "deleting...",
	})
}

func renderMergedDeleteTarget(row branchRow, defaultBranch string) string {
	if row.mergedIntoDefault {
		return textStyle.Render(sanitizeText(defaultBranch) + " has all its changes")
	}
	return warningStyle.Render(sanitizeText(defaultBranch) + " lacks some of its changes")
}

func (m Model) branchCells(row branchRow) []string {
	return []string{
		renderCurrentMark(row.branch.Current),
		branchStyle.Render(branchGlyph + " " + sanitizeText(row.branch.Name)),
		renderUpstream(row.branch),
		renderAheadBehind(row.branch.Tracking),
		renderBranchState(m.shownBranchRow(row), m.branches.defaultBranch),
	}
}

// MergedInto sees only commits, so the current branch with uncommitted work is never shown merged.
func (m Model) shownBranchRow(row branchRow) branchRow {
	if !row.branch.Current || len(m.status.Files) == 0 {
		return row
	}
	return branchRow{branch: row.branch, mergedIntoDefault: false}
}

func renderCurrentMark(current bool) string {
	if !current {
		return ""
	}
	return markStyle.Render("*")
}

func renderUpstream(branch gitrepo.Branch) string {
	if branch.UpstreamGone {
		return warningStyle.Render(sanitizeText(branch.Upstream) + " deleted")
	}
	return mutedStyle.Render(sanitizeText(branch.Upstream))
}

func renderBranchState(row branchRow, defaultBranch string) string {
	switch {
	case row.branch.OtherWorkTree != "":
		return mutedStyle.Render("in work tree " + sanitizeText(path.Base(row.branch.OtherWorkTree)))
	case row.mergedIntoDefault:
		return successStyle.Render("merged") + mutedStyle.Render(" into "+sanitizeText(defaultBranch))
	}
	return ""
}
