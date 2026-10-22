package ui

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// mergedIntoDefault stays false for the default branch itself.
type branchRow struct {
	branch            gitrepo.Branch
	mergedIntoDefault bool
}

// defaultBranch is origin's, as DefaultBranch names it, and empty when origin/HEAD is unset.
type branchList struct {
	open          bool
	loaded        bool
	rows          []branchRow
	defaultBranch string
	selected      int
	offset        int
}

type branchesLoadedMsg struct {
	dir           string
	rows          []branchRow
	defaultBranch string
}

type branchesFailedMsg struct {
	dir string
	err error
}

type branchSwitchedMsg struct {
	dir    string
	branch string
}

type branchDeletedMsg struct {
	dir    string
	branch string
}

type branchListRequest struct {
	repository gitrepo.Repository
}

func (r branchListRequest) load() tea.Msg {
	ctx := context.Background()
	dir := r.repository.Dir
	branches, err := r.repository.Branches(ctx)
	if err != nil {
		return branchesFailedMsg{dir: dir, err: err}
	}
	defaultBranch, err := r.repository.DefaultBranch(ctx)
	if err != nil {
		return branchesFailedMsg{dir: dir, err: err}
	}
	rows := make([]branchRow, len(branches))
	errs := make([]error, len(branches))
	var group sync.WaitGroup
	for index, branch := range branches {
		rows[index] = branchRow{branch: branch}
		if defaultBranch == "" || branch.Name == localBranch(defaultBranch) {
			continue
		}
		group.Go(func() {
			rows[index].mergedIntoDefault, errs[index] = r.repository.MergedInto(ctx, "refs/heads/"+branch.Name, defaultBranch)
		})
	}
	group.Wait()
	if err := errors.Join(errs...); err != nil {
		return branchesFailedMsg{dir: dir, err: err}
	}
	return branchesLoadedMsg{dir: dir, rows: rows, defaultBranch: defaultBranch}
}

type branchRequest struct {
	repository gitrepo.Repository
	branch     string
}

func (r branchRequest) switchTo() tea.Msg {
	if err := r.repository.Switch(context.Background(), r.branch); err != nil {
		return gitFailedMsg{err: err}
	}
	return branchSwitchedMsg{dir: r.repository.Dir, branch: r.branch}
}

func (r branchRequest) delete() tea.Msg {
	if err := r.repository.DeleteBranch(context.Background(), r.branch); err != nil {
		return popupFailedMsg{dir: r.repository.Dir, err: err}
	}
	return branchDeletedMsg{dir: r.repository.Dir, branch: r.branch}
}

func (m Model) openBranches() (tea.Model, tea.Cmd) {
	if m.repository.Dir == "" {
		return m, nil
	}
	m.branches = branchList{open: true, rows: []branchRow{}}
	return m, branchListRequest{repository: m.repository}.load
}

// The first load selects the current branch, and a reload after a delete keeps the selection where it was.
func (m Model) withBranchesLoaded(msg branchesLoadedMsg) Model {
	if m.branches.loaded {
		m.branches.selected = min(m.branches.selected, max(len(msg.rows)-1, 0))
	} else {
		m.branches.selected = max(slices.IndexFunc(msg.rows, func(row branchRow) bool { return row.branch.Current }), 0)
	}
	m.branches.loaded = true
	m.branches.rows = msg.rows
	m.branches.defaultBranch = msg.defaultBranch
	return m.withListScrolled()
}

func (m Model) updateBranches(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footerText = ""
	switch {
	case key.Matches(msg, quitKey):
		return m, tea.Quit
	case key.Matches(msg, closeBranchesKey):
		m.branches.open = false
		return m, nil
	case key.Matches(msg, downKey):
		return m.selectBranchRow(m.branches.selected + 1), nil
	case key.Matches(msg, upKey):
		return m.selectBranchRow(m.branches.selected - 1), nil
	case key.Matches(msg, switchBranchKey):
		return m.switchToSelectedBranch()
	case key.Matches(msg, deleteBranchKey):
		return m.openDeleteBranchPopup(), nil
	}
	return m, nil
}

func (m Model) selectBranchRow(index int) Model {
	if len(m.branches.rows) == 0 {
		return m
	}
	m.branches.selected = min(max(index, 0), len(m.branches.rows)-1)
	return m.withListScrolled()
}

// git switch carries uncommitted changes along when they do not conflict, which would mix them into another branch.
func (m Model) switchToSelectedBranch() (tea.Model, tea.Cmd) {
	if len(m.branches.rows) == 0 {
		return m, nil
	}
	branch := m.branches.rows[m.branches.selected].branch
	switch {
	case branch.Current:
		m.branches.open = false
		return m, nil
	case len(m.status.Files) > 0:
		return m.withFooterError("the work tree has uncommitted changes, so perusal does not switch branches"), nil
	}
	return m, branchRequest{repository: m.repository, branch: branch.Name}.switchTo
}

func (m Model) openDeleteBranchPopup() Model {
	if len(m.branches.rows) == 0 {
		return m
	}
	row := m.branches.rows[m.branches.selected]
	name := sanitizeSingleLine(row.branch.Name)
	switch {
	case row.branch.Current:
		return m.withFooterError(name + " is checked out, so it cannot be deleted")
	case m.branches.defaultBranch == "":
		return m.withFooterError("origin's default branch is unknown, so perusal cannot rule out that " + name +
			" is it")
	case row.branch.Name == localBranch(m.branches.defaultBranch):
		return m.withFooterError(name + " is the default branch, which perusal never deletes")
	}
	m.popup = deleteBranchPopup
	m.deleteBranchTarget = row
	return m
}

func (m Model) withBranchDeleted(msg branchDeletedMsg) (tea.Model, tea.Cmd) {
	if msg.dir != m.repository.Dir {
		return m.withFooterNotice("deleted " + msg.branch + " in " + msg.dir), nil
	}
	m = m.withPopupClosed().withFooterNotice("deleted " + msg.branch)
	if !m.branches.open {
		return m, nil
	}
	return m, branchListRequest{repository: m.repository}.load
}

func (m Model) renderDeleteBranchPopup() string {
	return m.renderPopup([]string{
		titleStyle.Render("delete " + sanitizeText(m.deleteBranchTarget.branch.Name)),
		m.renderDeleteTargetMerged(),
	}, "deleting...", deleteBranchHelp)
}

func (m Model) renderDeleteTargetMerged() string {
	defaultBranch := sanitizeText(m.branches.defaultBranch)
	if m.deleteBranchTarget.mergedIntoDefault {
		return textStyle.Render(defaultBranch + " has all its changes")
	}
	return warningStyle.Render(defaultBranch + " lacks some of its changes")
}

func (m Model) renderBranchesPane() string {
	innerWidth := m.width - 2
	return renderPane("branches", "", m.branchesLabel(), m.branchLines(innerWidth), innerWidth, m.paneInnerHeight())
}

func (m Model) branchesLabel() string {
	if len(m.branches.rows) == 0 {
		return ""
	}
	return strconv.Itoa(m.branches.selected+1) + " of " + strconv.Itoa(len(m.branches.rows))
}

func (m Model) branchLines(innerWidth int) []string {
	switch {
	case !m.branches.loaded:
		return []string{mutedStyle.Render("loading...")}
	case len(m.branches.rows) == 0:
		return []string{mutedStyle.Render("no branches yet")}
	}
	cells := [][]string{}
	for _, row := range m.branches.rows {
		cells = append(cells, renderBranchCells(row, m.branches.defaultBranch))
	}
	aligned := alignColumns(cells)
	lines := []string{}
	end := min(m.branches.offset+m.paneInnerHeight(), len(aligned))
	for index := m.branches.offset; index < end; index++ {
		lines = append(lines, listRow(aligned[index], index == m.branches.selected, innerWidth))
	}
	return lines
}

func renderBranchCells(row branchRow, defaultBranch string) []string {
	return []string{
		renderCurrentMark(row.branch),
		branchStyle.Render(branchGlyph + " " + sanitizeText(row.branch.Name)),
		renderUpstream(row.branch),
		renderAheadBehind(row.branch.Tracking),
		renderMerged(row, defaultBranch),
	}
}

func renderCurrentMark(branch gitrepo.Branch) string {
	if !branch.Current {
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

func renderMerged(row branchRow, defaultBranch string) string {
	if !row.mergedIntoDefault {
		return ""
	}
	return successStyle.Render("merged") + mutedStyle.Render(" into "+sanitizeText(defaultBranch))
}
