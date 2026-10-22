package ui

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

type Lister func(context.Context) (focus.Found, error)

// behindDefault counts the commits HEAD lacks from defaultBranch, and stays 0 when defaultBranch is the upstream, whose
// count the status already holds. finished is a branch p leaves for the default branch.
type overviewRow struct {
	listed        focus.FoundRepository
	refused       focus.FoundRefusal
	status        gitrepo.Status
	defaultBranch string
	behindDefault int
	finished      bool
}

// pulling holds the Dir of each repository a pull is running in, and outlives closing the overview.
type overview struct {
	open     bool
	loaded   bool
	rows     []overviewRow
	selected int
	offset   int
	pulling  map[string]bool
}

// failures are the repositories git failed on, which get no row.
type overviewLoadedMsg struct {
	rows     []overviewRow
	failures []string
}

type overviewFailedMsg struct {
	err error
}

type overviewPulledMsg struct {
	row overviewRow
}

type overviewSyncFailedMsg struct {
	dir string
	err error
}

type pullRequest struct {
	listed focus.FoundRepository
}

func (r pullRequest) pull() tea.Msg {
	dir := r.listed.Repository.Dir
	if err := r.listed.Repository.Pull(context.Background()); err != nil {
		return overviewSyncFailedMsg{dir: dir, err: err}
	}
	row, err := loadOverviewRow(r.listed)
	if err != nil {
		return overviewSyncFailedMsg{dir: dir, err: err}
	}
	return overviewPulledMsg{row: row}
}

type switchRequest struct {
	listed focus.FoundRepository
	branch string
}

func (r switchRequest) switchAndPull() tea.Msg {
	if err := r.listed.Repository.Switch(context.Background(), r.branch); err != nil {
		return overviewSyncFailedMsg{dir: r.listed.Repository.Dir, err: err}
	}
	return pullRequest{listed: r.listed}.pull()
}

type overviewRequest struct {
	listRepositories Lister
}

func (r overviewRequest) load() tea.Msg {
	listing, err := r.listRepositories(context.Background())
	if err != nil {
		return overviewFailedMsg{err: err}
	}
	results := make([]overviewRow, len(listing.Repositories))
	errs := make([]error, len(listing.Repositories))
	var group sync.WaitGroup
	for index, listed := range listing.Repositories {
		group.Go(func() {
			results[index], errs[index] = loadOverviewRow(listed)
		})
	}
	group.Wait()
	rows := []overviewRow{}
	failures := []string{}
	for index, row := range results {
		if errs[index] != nil {
			failures = append(failures, errs[index].Error())
		} else {
			rows = append(rows, row)
		}
	}
	for _, refused := range listing.Refusals {
		rows = append(rows, overviewRow{listed: focus.FoundRepository{RelativePath: refused.RelativePath}, refused: refused})
	}
	slices.SortFunc(rows, func(a overviewRow, b overviewRow) int {
		return strings.Compare(a.listed.RelativePath, b.listed.RelativePath)
	})
	return overviewLoadedMsg{rows: rows, failures: failures}
}

func loadOverviewRow(listed focus.FoundRepository) (overviewRow, error) {
	ctx := context.Background()
	status, err := listed.Repository.Status(ctx)
	if err != nil {
		return overviewRow{}, err
	}
	defaultBranch, err := listed.Repository.DefaultBranch(ctx)
	if err != nil {
		return overviewRow{}, err
	}
	row := overviewRow{listed: listed, status: status, defaultBranch: defaultBranch}
	if defaultBranch == "" || !status.HasCommits {
		return row, nil
	}
	if mayBeFinished(status, defaultBranch) {
		merged, err := listed.Repository.MergedInto(ctx, "HEAD", defaultBranch)
		if err != nil {
			return overviewRow{}, err
		}
		row.finished = merged
	}
	if defaultBranch == status.Upstream {
		return row, nil
	}
	behind, err := listed.Repository.CommitsBehind(ctx, defaultBranch)
	if err != nil {
		return overviewRow{}, err
	}
	row.behindDefault = behind
	return row, nil
}

func (r overviewRow) isRefused() bool {
	return r.refused.Err != nil
}

func (m Model) openOverview() (tea.Model, tea.Cmd) {
	m.overview = overview{open: true, rows: []overviewRow{}, pulling: m.overview.pulling}
	return m, overviewRequest{listRepositories: m.listRepositories}.load
}

// Lists the focused repositories first and selects the one under review, so enter right away keeps it.
func (m Model) withOverviewLoaded(msg overviewLoadedMsg) Model {
	rows := slices.Clone(msg.rows)
	slices.SortStableFunc(rows, func(a overviewRow, b overviewRow) int {
		return cmp.Compare(m.focusRank(a), m.focusRank(b))
	})
	m.overview.loaded = true
	m.overview.rows = rows
	m.overview.selected = max(slices.IndexFunc(rows, func(row overviewRow) bool {
		return !row.isRefused() && row.listed.Repository == m.repository
	}), 0)
	m.overview.offset = 0
	m = m.withListScrolled()
	if len(msg.failures) == 0 {
		return m
	}
	return m.withFooterError("skipped " + strings.Join(msg.failures, ", "))
}

func (m Model) focusRank(row overviewRow) int {
	if slices.Contains(m.focused, row.listed.Repository) {
		return 0
	}
	return 1
}

func (m Model) updateOverview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footerText = ""
	switch {
	case key.Matches(msg, quitKey):
		return m, tea.Quit
	case key.Matches(msg, closeOverviewKey):
		m.overview.open = false
		return m, nil
	case key.Matches(msg, downKey):
		return m.selectOverviewRow(m.overview.selected + 1), nil
	case key.Matches(msg, upKey):
		return m.selectOverviewRow(m.overview.selected - 1), nil
	case key.Matches(msg, openRepositoryKey):
		return m.openOverviewRow()
	case key.Matches(msg, pullKey):
		return m.pullOverviewRow()
	}
	return m, nil
}

func (m Model) pullOverviewRow() (tea.Model, tea.Cmd) {
	if len(m.overview.rows) == 0 {
		return m, nil
	}
	row := m.overview.rows[m.overview.selected]
	listed := row.listed
	var pull tea.Cmd
	switch {
	case row.isRefused():
		return m.withRefusalShown(row), nil
	case m.overview.pulling[listed.Repository.Dir]:
		return m, nil
	case row.finished:
		pull = switchRequest{listed: listed, branch: localBranch(row.defaultBranch)}.switchAndPull
	case row.status.Upstream == "":
		return m.withFooterError(listed.RelativePath + " tracks no upstream, so there is nothing to pull"), nil
	case row.status.UpstreamGone:
		return m.withFooterError(listed.RelativePath + " tracks " + sanitizeSingleLine(row.status.Upstream) +
			", which was deleted from the remote, so there is nothing to pull"), nil
	default:
		pull = pullRequest{listed: listed}.pull
	}
	pulling := maps.Clone(m.overview.pulling)
	pulling[listed.Repository.Dir] = true
	m.overview.pulling = pulling
	return m, pull
}

// A clean branch other than the default one, with no upstream left to pull, is finished once it is merged.
func mayBeFinished(status gitrepo.Status, defaultBranch string) bool {
	return len(status.Files) == 0 && (status.Upstream == "" || status.UpstreamGone) &&
		status.Branch != localBranch(defaultBranch)
}

func localBranch(defaultBranch string) string {
	return strings.TrimPrefix(defaultBranch, "origin/")
}

func (m Model) withOverviewPulled(msg overviewPulledMsg) (tea.Model, tea.Cmd) {
	repository := msg.row.listed.Repository
	m.overview.pulling = withoutDir(m.overview.pulling, repository.Dir)
	rows := slices.Clone(m.overview.rows)
	index := slices.IndexFunc(rows, func(row overviewRow) bool { return row.listed.Repository == repository })
	if index >= 0 {
		rows[index] = msg.row
	}
	m.overview.rows = rows
	m = m.withFooterNotice("pulled " + msg.row.listed.RelativePath)
	if repository != m.repository {
		return m, nil
	}
	return m, repositoryRequest{repository: m.repository}.loadStatus
}

func withoutDir(set map[string]bool, dir string) map[string]bool {
	without := maps.Clone(set)
	delete(without, dir)
	return without
}

func (m Model) selectOverviewRow(index int) Model {
	if len(m.overview.rows) == 0 {
		return m
	}
	m.overview.selected = min(max(index, 0), len(m.overview.rows)-1)
	return m.withListScrolled()
}

func (m Model) openOverviewRow() (tea.Model, tea.Cmd) {
	if len(m.overview.rows) == 0 {
		return m, nil
	}
	row := m.overview.rows[m.overview.selected]
	if row.isRefused() {
		return m.withRefusalShown(row), nil
	}
	repository := row.listed.Repository
	m.overview.open = false
	if repository == m.repository {
		return m, nil
	}
	return m.withFocusedRepository(repository)
}

func (m Model) withRefusalShown(row overviewRow) Model {
	return m.withFooterError(row.listed.RelativePath + " is refused: " + row.refused.Err.Error())
}

func (m Model) renderOverviewPane() string {
	innerWidth := m.width - 2
	return renderPane("repositories", "", m.overviewLabel(), m.overviewLines(innerWidth), innerWidth, m.paneInnerHeight())
}

func (m Model) overviewLabel() string {
	if len(m.overview.rows) == 0 {
		return ""
	}
	return strconv.Itoa(m.overview.selected+1) + " of " + strconv.Itoa(len(m.overview.rows))
}

func (m Model) overviewLines(innerWidth int) []string {
	switch {
	case !m.overview.loaded:
		return []string{mutedStyle.Render("loading...")}
	case len(m.overview.rows) == 0:
		return []string{mutedStyle.Render("no repositories found")}
	}
	cells := [][]string{}
	for _, row := range m.overview.rows {
		rowCells := append([]string{m.renderFocusMark(row)}, renderOverviewCells(row)...)
		if m.overview.pulling[row.listed.Repository.Dir] {
			rowCells = append(rowCells, mutedStyle.Render("pulling..."))
		}
		cells = append(cells, rowCells)
	}
	aligned := alignColumns(cells)
	lines := []string{}
	end := min(m.overview.offset+m.paneInnerHeight(), len(aligned))
	for index := m.overview.offset; index < end; index++ {
		lines = append(lines, listRow(aligned[index], index == m.overview.selected, innerWidth))
	}
	return lines
}

func (m Model) renderFocusMark(row overviewRow) string {
	if !slices.Contains(m.focused, row.listed.Repository) {
		return ""
	}
	return markStyle.Render("●")
}

func renderOverviewCells(row overviewRow) []string {
	if row.isRefused() {
		return []string{
			mutedStyle.Render(folderGlyph + " " + sanitizeText(row.listed.RelativePath)),
			mutedStyle.Render(sanitizeText(row.refused.Summary())),
		}
	}
	return []string{
		repositoryStyle.Render(folderGlyph + " " + sanitizeText(row.listed.RelativePath)),
		branchStyle.Render(branchGlyph + " " + headName(row.status)),
		renderChangedCount(len(row.status.Files)),
		renderAheadBehind(row.status.Tracking),
		renderBehindDefault(row),
	}
}

func renderChangedCount(count int) string {
	if count == 0 {
		return mutedStyle.Render("clean")
	}
	return warningStyle.Render(strconv.Itoa(count) + " changed")
}

func renderBehindDefault(row overviewRow) string {
	if row.behindDefault == 0 {
		return ""
	}
	return behindStyle.Render("↓"+strconv.Itoa(row.behindDefault)) + mutedStyle.Render(" "+sanitizeText(row.defaultBranch))
}

func alignColumns(rows [][]string) []string {
	widths := []int{}
	for _, row := range rows {
		for column, cell := range row {
			if column == len(widths) {
				widths = append(widths, 0)
			}
			widths[column] = max(widths[column], ansi.StringWidth(cell))
		}
	}
	lines := []string{}
	for _, row := range rows {
		padded := []string{}
		for column, cell := range row {
			padded = append(padded, cell+strings.Repeat(" ", widths[column]-ansi.StringWidth(cell)))
		}
		lines = append(lines, strings.Join(padded, "  "))
	}
	return lines
}
