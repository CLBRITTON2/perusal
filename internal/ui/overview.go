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

type overviewRow interface {
	relativePath() string
	cells() []string
}

// behindDefault counts the commits HEAD lacks from defaultBranch, and stays 0 when defaultBranch is the upstream, whose
// count the status already holds. finished is a branch p leaves for the default branch.
type repositoryRow struct {
	focus.FoundRepository
	status        gitrepo.Status
	defaultBranch string
	behindDefault int
	finished      bool
}

type refusedRow struct {
	focus.FoundRefusal
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
	row repositoryRow
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
	results := make([]repositoryRow, len(listing.Repositories))
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
		rows = append(rows, refusedRow{FoundRefusal: refused})
	}
	slices.SortFunc(rows, func(a overviewRow, b overviewRow) int {
		return strings.Compare(a.relativePath(), b.relativePath())
	})
	return overviewLoadedMsg{rows: rows, failures: failures}
}

func loadOverviewRow(listed focus.FoundRepository) (repositoryRow, error) {
	ctx := context.Background()
	status, err := listed.Repository.Status(ctx)
	if err != nil {
		return repositoryRow{}, err
	}
	defaultBranch, err := listed.Repository.DefaultBranch(ctx)
	if err != nil {
		return repositoryRow{}, err
	}
	row := repositoryRow{FoundRepository: listed, status: status, defaultBranch: defaultBranch}
	if defaultBranch == "" || !status.HasCommits {
		return row, nil
	}
	if mayBeFinished(status, defaultBranch) {
		merged, err := listed.Repository.MergedInto(ctx, "HEAD", defaultBranch)
		if err != nil {
			return repositoryRow{}, err
		}
		row.finished = merged
	}
	if defaultBranch == status.Upstream {
		return row, nil
	}
	behind, err := listed.Repository.CommitsBehind(ctx, defaultBranch)
	if err != nil {
		return repositoryRow{}, err
	}
	row.behindDefault = behind
	return row, nil
}

func (r repositoryRow) relativePath() string {
	return r.RelativePath
}

func (r refusedRow) relativePath() string {
	return r.RelativePath
}

func (m Model) openOverview() (tea.Model, tea.Cmd) {
	m.mode = modeOverview
	m.overview = selectableList[overviewRow]{rows: []overviewRow{}}
	return m, overviewRequest{listRepositories: m.listRepositories}.load
}

// Lists the focused repositories first and selects the one under review, so enter right away keeps it.
func (m Model) withOverviewLoaded(msg overviewLoadedMsg) Model {
	rows := slices.Clone(msg.rows)
	slices.SortStableFunc(rows, func(a overviewRow, b overviewRow) int {
		return cmp.Compare(m.focusRank(a), m.focusRank(b))
	})
	loaded := selectableList[overviewRow]{loaded: true, rows: rows, selected: 0, offset: 0}
	m.overview = loaded.withSelected(slices.IndexFunc(rows, isRowOf(m.repository)))
	m = m.withListScrolled()
	if len(msg.failures) == 0 {
		return m
	}
	return m.withFooterError("skipped " + strings.Join(msg.failures, ", "))
}

func isRowOf(repository gitrepo.Repository) func(overviewRow) bool {
	return func(row overviewRow) bool {
		listed, isRepository := row.(repositoryRow)
		return isRepository && listed.Repository == repository
	}
}

func (m Model) isFocused(row overviewRow) bool {
	listed, isRepository := row.(repositoryRow)
	return isRepository && slices.Contains(m.focused, listed.Repository)
}

func (m Model) focusRank(row overviewRow) int {
	if m.isFocused(row) {
		return 0
	}
	return 1
}

func (m Model) updateOverview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footer = footer{}
	switch {
	case key.Matches(msg, quitKey):
		return m, tea.Quit
	case key.Matches(msg, closeOverviewKey):
		m.mode = modeBrowsing
		return m, nil
	case key.Matches(msg, downKey):
		m.overview = m.overview.withSelected(m.overview.selected + 1).scrolled(m.paneInnerHeight())
		return m, nil
	case key.Matches(msg, upKey):
		m.overview = m.overview.withSelected(m.overview.selected - 1).scrolled(m.paneInnerHeight())
		return m, nil
	case key.Matches(msg, openRepositoryKey):
		return m.openOverviewRow()
	case key.Matches(msg, pullKey):
		return m.pullOverviewRow()
	}
	return m, nil
}

func (m Model) pullOverviewRow() (tea.Model, tea.Cmd) {
	selected, hasRows := m.overview.selectedRow()
	if !hasRows {
		return m, nil
	}
	switch row := selected.(type) {
	case refusedRow:
		return m.withRefusalShown(row), nil
	case repositoryRow:
		return m.pullRepositoryRow(row)
	}
	return m, nil
}

func (m Model) pullRepositoryRow(row repositoryRow) (tea.Model, tea.Cmd) {
	var pull tea.Cmd
	switch {
	case m.pulling[row.Repository.Dir]:
		return m, nil
	case row.finished:
		pull = switchRequest{listed: row.FoundRepository, branch: localBranch(row.defaultBranch)}.switchAndPull
	case row.status.Upstream == "":
		return m.withFooterError(row.RelativePath + " tracks no upstream, so there is nothing to pull"), nil
	case row.status.UpstreamGone:
		return m.withFooterError(row.RelativePath + " tracks " + sanitizeSingleLine(row.status.Upstream) +
			", which was deleted from the remote, so there is nothing to pull"), nil
	default:
		pull = pullRequest{listed: row.FoundRepository}.pull
	}
	pulling := maps.Clone(m.pulling)
	pulling[row.Repository.Dir] = true
	m.pulling = pulling
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
	repository := msg.row.Repository
	m.pulling = withoutDir(m.pulling, repository.Dir)
	rows := slices.Clone(m.overview.rows)
	index := slices.IndexFunc(rows, isRowOf(repository))
	if index >= 0 {
		rows[index] = msg.row
	}
	m.overview.rows = rows
	m = m.withFooterNotice("pulled " + msg.row.RelativePath)
	if repository != m.repository {
		return m, nil
	}
	return m, m.reloadStatus()
}

func withoutDir(set map[string]bool, dir string) map[string]bool {
	without := maps.Clone(set)
	delete(without, dir)
	return without
}

func (m Model) openOverviewRow() (tea.Model, tea.Cmd) {
	selected, hasRows := m.overview.selectedRow()
	if !hasRows {
		return m, nil
	}
	switch row := selected.(type) {
	case refusedRow:
		return m.withRefusalShown(row), nil
	case repositoryRow:
		m.mode = modeBrowsing
		if row.Repository == m.repository {
			return m, nil
		}
		return m.withFocusedRepository(row.Repository)
	}
	return m, nil
}

func (m Model) withRefusalShown(row refusedRow) Model {
	return m.withFooterError(row.RelativePath + " is refused: " + row.Err.Error())
}

func (m Model) overviewCells(row overviewRow) []string {
	cells := append([]string{m.renderFocusMark(row)}, row.cells()...)
	listed, isRepository := row.(repositoryRow)
	if isRepository && m.pulling[listed.Repository.Dir] {
		cells = append(cells, mutedStyle.Render("pulling..."))
	}
	return cells
}

func (m Model) renderFocusMark(row overviewRow) string {
	if !m.isFocused(row) {
		return ""
	}
	return markStyle.Render("●")
}

func (r repositoryRow) cells() []string {
	return []string{
		repositoryStyle.Render(folderGlyph + " " + sanitizeText(r.RelativePath)),
		branchStyle.Render(branchGlyph + " " + headName(r.status)),
		renderChangedCount(len(r.status.Files)),
		renderAheadBehind(r.status.Tracking),
		renderBehindDefault(r),
	}
}

func (r refusedRow) cells() []string {
	return []string{
		mutedStyle.Render(folderGlyph + " " + sanitizeText(r.RelativePath)),
		mutedStyle.Render(sanitizeText(r.Summary())),
	}
}

func renderChangedCount(count int) string {
	if count == 0 {
		return mutedStyle.Render("clean")
	}
	return warningStyle.Render(strconv.Itoa(count) + " changed")
}

func renderBehindDefault(row repositoryRow) string {
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
