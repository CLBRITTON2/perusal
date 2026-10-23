package ui

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

type RepositoryFinder func(context.Context) (focus.Found, error)

const overviewCommitLimit = 50

type overviewRow interface {
	relativePath() string
	cells() []string
}

// commitsBehindDefault counts the commits HEAD lacks from defaultBranch, and stays 0 when defaultBranch is the
// upstream, whose count the status already holds. commitsOnNoRemote is counted only for an unpublished branch.
// finished is a branch p leaves for the default branch. outgoing holds the commits P sends and incoming those p
// brings, newest first and at most overviewCommitLimit of each.
type repositoryRow struct {
	focus.FoundRepository
	status               gitrepo.Status
	defaultBranch        string
	commitsBehindDefault int
	commitsOnNoRemote    int
	finished             bool
	outgoing             []gitrepo.Commit
	incoming             []gitrepo.Commit
	loadedAt             time.Time
}

type refusedRow struct {
	focus.FoundRefusal
}

// failures are the repositories git failed on, which get no row.
type overviewMsg struct {
	rows     []overviewRow
	failures []string
}

type overviewFailureMsg struct {
	err error
}

type pullMsg struct {
	row repositoryRow
}

type overviewRowMsg struct {
	row repositoryRow
}

type syncFailureMsg struct {
	dir string
	err error
}

type pullRequest struct {
	found focus.FoundRepository
}

func (r pullRequest) pull() tea.Msg {
	dir := r.found.Repository.Dir
	if err := r.found.Repository.Pull(context.Background()); err != nil {
		return syncFailureMsg{dir: dir, err: err}
	}
	row, err := loadOverviewRow(r.found)
	if err != nil {
		return syncFailureMsg{dir: dir, err: err}
	}
	return pullMsg{row: row}
}

type overviewRowRequest struct {
	found focus.FoundRepository
}

func (r overviewRowRequest) load() tea.Msg {
	row, err := loadOverviewRow(r.found)
	if err != nil {
		return gitFailureMsg{err: err}
	}
	return overviewRowMsg{row: row}
}

type switchRequest struct {
	found  focus.FoundRepository
	branch string
}

func (r switchRequest) switchAndPull() tea.Msg {
	if err := r.found.Repository.Switch(context.Background(), r.branch); err != nil {
		return syncFailureMsg{dir: r.found.Repository.Dir, err: err}
	}
	return pullRequest{found: r.found}.pull()
}

type overviewRequest struct {
	findRepositories RepositoryFinder
}

func (r overviewRequest) load() tea.Msg {
	found, err := r.findRepositories(context.Background())
	if err != nil {
		return overviewFailureMsg{err: err}
	}
	results := make([]repositoryRow, len(found.Repositories))
	errs := make([]error, len(found.Repositories))
	var group sync.WaitGroup
	for index, repository := range found.Repositories {
		group.Go(func() {
			results[index], errs[index] = loadOverviewRow(repository)
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
	for _, refused := range found.Refusals {
		rows = append(rows, refusedRow{FoundRefusal: refused})
	}
	slices.SortFunc(rows, func(a overviewRow, b overviewRow) int {
		return strings.Compare(a.relativePath(), b.relativePath())
	})
	return overviewMsg{rows: rows, failures: failures}
}

func loadOverviewRow(found focus.FoundRepository) (repositoryRow, error) {
	ctx := context.Background()
	status, err := found.Repository.Status(ctx)
	if err != nil {
		return repositoryRow{}, err
	}
	defaultBranch, err := found.Repository.DefaultBranch(ctx)
	if err != nil {
		return repositoryRow{}, err
	}
	row := repositoryRow{FoundRepository: found, status: status, defaultBranch: defaultBranch, loadedAt: time.Now()}
	if !status.HasCommits {
		return row, nil
	}
	outgoing, err := loadOutgoing(ctx, found.Repository, status)
	if err != nil {
		return repositoryRow{}, err
	}
	row.outgoing = outgoing
	incoming, err := loadIncoming(ctx, found.Repository, status)
	if err != nil {
		return repositoryRow{}, err
	}
	row.incoming = incoming
	if status.Branch != "" && !status.IsPublished() {
		onNoRemote, err := found.Repository.CommitsOnNoRemote(ctx)
		if err != nil {
			return repositoryRow{}, err
		}
		row.commitsOnNoRemote = onNoRemote
	}
	if defaultBranch == "" {
		return row, nil
	}
	if mayBeFinished(status, defaultBranch) {
		merged, err := found.Repository.MergedInto(ctx, "HEAD", defaultBranch)
		if err != nil {
			return repositoryRow{}, err
		}
		row.finished = merged
	}
	if defaultBranch == status.Upstream {
		return row, nil
	}
	behind, err := found.Repository.CommitsBehind(ctx, defaultBranch)
	if err != nil {
		return repositoryRow{}, err
	}
	row.commitsBehindDefault = behind
	return row, nil
}

// The commits P sends: those past the upstream, or for an unpublished branch those no remote holds.
func loadOutgoing(ctx context.Context, repository gitrepo.Repository, status gitrepo.Status) ([]gitrepo.Commit, error) {
	switch {
	case status.Branch == "":
		return []gitrepo.Commit{}, nil
	case status.IsPublished():
		return repository.LogAhead(ctx, status.Upstream, overviewCommitLimit)
	}
	return repository.LogOnNoRemote(ctx, overviewCommitLimit)
}

func loadIncoming(ctx context.Context, repository gitrepo.Repository, status gitrepo.Status) ([]gitrepo.Commit, error) {
	if status.Upstream == "" || status.UpstreamGone {
		return []gitrepo.Commit{}, nil
	}
	return repository.LogBehind(ctx, status.Upstream, overviewCommitLimit)
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
	return m, overviewRequest{findRepositories: m.findRepositories}.load
}

// Lists the focused repositories first and selects the one under review, so enter right away keeps it.
func (m Model) withOverviewLoaded(msg overviewMsg) Model {
	rows := slices.Clone(msg.rows)
	slices.SortStableFunc(rows, func(a overviewRow, b overviewRow) int {
		return cmp.Compare(m.focusRank(a), m.focusRank(b))
	})
	loadedOverview := selectableList[overviewRow]{loaded: true, rows: rows, selected: 0, offset: 0}
	m.overview = loadedOverview.withSelected(slices.IndexFunc(rows, isRowOf(m.repository)))
	m = m.withListScrolled()
	if len(msg.failures) == 0 {
		return m
	}
	return m.withFooterError("skipped " + strings.Join(msg.failures, ", "))
}

func isRowOf(repository gitrepo.Repository) func(overviewRow) bool {
	return func(row overviewRow) bool {
		foundRow, isRepository := row.(repositoryRow)
		return isRepository && foundRow.Repository == repository
	}
}

func (m Model) isFocused(row overviewRow) bool {
	foundRow, isRepository := row.(repositoryRow)
	return isRepository && slices.Contains(m.focused, foundRow.Repository)
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
		m.overview = m.overview.withSelected(m.overview.selected + 1).scrolled(m.overviewTableInnerHeight())
		return m, nil
	case key.Matches(msg, upKey):
		m.overview = m.overview.withSelected(m.overview.selected - 1).scrolled(m.overviewTableInnerHeight())
		return m, nil
	case key.Matches(msg, openRepositoryKey):
		return m.withSelectedRepository(m.openRepositoryRow)
	case key.Matches(msg, pullKey):
		return m.withSelectedRepository(m.pullRepositoryRow)
	case key.Matches(msg, pushKey):
		return m.withSelectedRepository(m.pushRepositoryRow)
	}
	return m, nil
}

// withSelectedRepository acts on the selected row when it is a repository and explains the refusal when it is not.
func (m Model) withSelectedRepository(act func(repositoryRow) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	selected, hasRows := m.overview.selectedRow()
	if !hasRows {
		return m, nil
	}
	switch row := selected.(type) {
	case refusedRow:
		return m.withFooterError(row.RelativePath + " is refused: " + row.Err.Error()), nil
	case repositoryRow:
		return act(row)
	}
	return m, nil
}

func (m Model) pullRepositoryRow(row repositoryRow) (tea.Model, tea.Cmd) {
	var pull tea.Cmd
	switch {
	case m.pulling[row.Repository.Dir]:
		return m, nil
	case row.finished:
		pull = switchRequest{found: row.FoundRepository, branch: localBranch(row.defaultBranch)}.switchAndPull
	case row.status.Upstream == "":
		return m.withFooterError(row.RelativePath + " tracks no upstream, so there is nothing to pull"), nil
	case row.status.UpstreamGone:
		return m.withFooterError(row.RelativePath + " tracks " + row.status.Upstream +
			", which was deleted from the remote, so there is nothing to pull"), nil
	default:
		pull = pullRequest{found: row.FoundRepository}.pull
	}
	pulling := maps.Clone(m.pulling)
	pulling[row.Repository.Dir] = true
	m.pulling = pulling
	return m, pull
}

func (m Model) pushRepositoryRow(row repositoryRow) (tea.Model, tea.Cmd) {
	return m.openPushPopup(row.Repository, row.status)
}

func (m Model) reloadOverviewRow(dir string) tea.Cmd {
	index := slices.IndexFunc(m.overview.rows, isRowOf(gitrepo.Repository{Dir: dir}))
	if m.mode != modeOverview || index < 0 {
		return nil
	}
	return overviewRowRequest{found: m.overview.rows[index].(repositoryRow).FoundRepository}.load
}

// A clean branch other than the default one, with no upstream left to pull, is finished once it is merged.
func mayBeFinished(status gitrepo.Status, defaultBranch string) bool {
	return len(status.Files) == 0 && (status.Upstream == "" || status.UpstreamGone) &&
		status.Branch != "" && status.Branch != localBranch(defaultBranch)
}

func localBranch(defaultBranch string) string {
	return strings.TrimPrefix(defaultBranch, "origin/")
}

func (m Model) withOverviewPulled(msg pullMsg) (tea.Model, tea.Cmd) {
	repository := msg.row.Repository
	m.pulling = withoutDir(m.pulling, repository.Dir)
	m = m.withOverviewRow(msg.row).withFooterNotice("pulled " + msg.row.RelativePath)
	if repository != m.repository {
		return m, nil
	}
	return m, m.reloadStatus()
}

func (m Model) withOverviewRow(row repositoryRow) Model {
	rows := slices.Clone(m.overview.rows)
	index := slices.IndexFunc(rows, isRowOf(row.Repository))
	if index >= 0 {
		rows[index] = row
	}
	m.overview.rows = rows
	return m
}

func withoutDir(set map[string]bool, dir string) map[string]bool {
	remaining := maps.Clone(set)
	delete(remaining, dir)
	return remaining
}

func (m Model) openRepositoryRow(row repositoryRow) (tea.Model, tea.Cmd) {
	m.mode = modeBrowsing
	if row.Repository == m.repository {
		return m, nil
	}
	return m.withFocusedRepository(row.Repository)
}

// The table takes the top half and the selected repository's changes, outgoing and incoming commits the rest.
func (m Model) renderOverview() string {
	table := renderListPane("repositories", m.overview, m.overviewCells, "no repositories found", m.width-2,
		m.overviewTableInnerHeight())
	return lipgloss.JoinVertical(lipgloss.Left, table, m.renderOverviewDetails())
}

func (m Model) overviewTableInnerHeight() int {
	return max(m.columnHeight()-m.columnHeight()/2-2, 0)
}

func (m Model) renderOverviewDetails() string {
	height := max(m.columnHeight()/2-2, 0)
	selected, _ := m.overview.selectedRow()
	row, isRepository := selected.(repositoryRow)
	if !isRepository {
		return renderPane("", "", "", []string{}, m.width-2, height)
	}
	width := m.width / 3
	lastWidth := m.width - 2*width
	return lipgloss.JoinHorizontal(lipgloss.Top,
		renderPane("changes", "", "", renderChangeRows(row.status.Files), width-2, height),
		renderPane("outgoing", "", "", renderCommitRows(row.outgoing, row.loadedAt, "nothing to push", width-2),
			width-2, height),
		renderPane("incoming", "", "", renderCommitRows(row.incoming, row.loadedAt, "nothing to pull", lastWidth-2),
			lastWidth-2, height))
}

func renderChangeRows(files []gitrepo.FileStatus) []string {
	if len(files) == 0 {
		return []string{mutedStyle.Render("clean")}
	}
	lines := []string{}
	for _, file := range files {
		lines = append(lines, renderFileRow(file))
	}
	return lines
}

func renderCommitRows(commits []gitrepo.Commit, loadedAt time.Time, emptyText string, width int) []string {
	if len(commits) == 0 {
		return []string{mutedStyle.Render(emptyText)}
	}
	lines := []string{}
	for _, commit := range commits {
		lines = append(lines, renderCommitRow(commit, searchTerms{}, loadedAt, width))
	}
	return lines
}

func (m Model) overviewCells(row overviewRow) []string {
	cells := append([]string{m.renderFocusMark(row)}, row.cells()...)
	foundRow, isRepository := row.(repositoryRow)
	if isRepository && m.pulling[foundRow.Repository.Dir] {
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
		renderUnpushed(r),
		renderBehindDefault(r),
	}
}

func renderUnpushed(row repositoryRow) string {
	if row.commitsOnNoRemote == 0 {
		return renderAheadBehind(row.status.Tracking)
	}
	return aheadStyle.Render("↑"+strconv.Itoa(row.commitsOnNoRemote)) + mutedStyle.Render(" unpublished")
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
	if row.commitsBehindDefault == 0 {
		return ""
	}
	return behindStyle.Render("↓"+strconv.Itoa(row.commitsBehindDefault)) +
		mutedStyle.Render(" "+sanitizeText(row.defaultBranch))
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
