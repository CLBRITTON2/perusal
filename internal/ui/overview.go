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
	defaultBranch        gitrepo.RemoteBranch
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

// notice is the footer text for the pull or fetch that loaded row.
type syncedMsg struct {
	row    repositoryRow
	notice string
}

// dirs are every repository the fetch ran in, failed or not.
type fetchedAllMsg struct {
	dirs     []string
	rows     []repositoryRow
	failures []string
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
	return syncedMsg{row: row, notice: "pulled " + r.found.RelativePath}
}

type fetchRequest struct {
	found focus.FoundRepository
}

func (r fetchRequest) fetch() tea.Msg {
	row, err := fetchRow(r.found)
	if err != nil {
		return syncFailureMsg{dir: r.found.Repository.Dir, err: err}
	}
	return syncedMsg{row: row, notice: "fetched " + r.found.RelativePath}
}

type fetchAllRequest struct {
	found []focus.FoundRepository
}

func (r fetchAllRequest) fetchAll() tea.Msg {
	rows, failures := loadRows(r.found, fetchRow)
	return fetchedAllMsg{dirs: dirsOf(r.found), rows: rows, failures: failures}
}

func fetchRow(found focus.FoundRepository) (repositoryRow, error) {
	if err := found.Repository.Fetch(context.Background()); err != nil {
		return repositoryRow{}, err
	}
	return loadOverviewRow(found)
}

func dirsOf(found []focus.FoundRepository) []string {
	dirs := []string{}
	for _, repository := range found {
		dirs = append(dirs, repository.Repository.Dir)
	}
	return dirs
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
	loaded, failures := loadRows(found.Repositories, loadOverviewRow)
	rows := []overviewRow{}
	for _, row := range loaded {
		rows = append(rows, row)
	}
	for _, refused := range found.Refusals {
		rows = append(rows, refusedRow{FoundRefusal: refused})
	}
	slices.SortFunc(rows, func(a overviewRow, b overviewRow) int {
		return strings.Compare(a.relativePath(), b.relativePath())
	})
	return overviewMsg{rows: rows, failures: failures}
}

// loadRows runs load in every repository at once and returns the rows it loaded and the errors of the rest.
func loadRows(found []focus.FoundRepository, load func(focus.FoundRepository) (repositoryRow, error)) ([]repositoryRow,
	[]string) {
	results := make([]repositoryRow, len(found))
	errs := make([]error, len(found))
	var group sync.WaitGroup
	for index, repository := range found {
		group.Go(func() {
			results[index], errs[index] = load(repository)
		})
	}
	group.Wait()
	rows := []repositoryRow{}
	failures := []string{}
	for index, row := range results {
		if errs[index] != nil {
			failures = append(failures, errs[index].Error())
		} else {
			rows = append(rows, row)
		}
	}
	return rows, failures
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
	// The commits P sends: those past the upstream, or for an unpublished branch those no remote holds.
	switch {
	case status.Branch == "":
		row.outgoing = []gitrepo.Commit{}
	case status.IsPublished():
		row.outgoing, err = found.Repository.LogAhead(ctx, status.Upstream, overviewCommitLimit)
	default:
		row.outgoing, err = found.Repository.LogOnNoRemote(ctx, overviewCommitLimit)
	}
	if err != nil {
		return repositoryRow{}, err
	}
	if status.Upstream == "" || status.UpstreamGone {
		row.incoming = []gitrepo.Commit{}
	} else {
		row.incoming, err = found.Repository.LogBehind(ctx, status.Upstream, overviewCommitLimit)
	}
	if err != nil {
		return repositoryRow{}, err
	}
	if status.Branch != "" && !status.IsPublished() {
		onNoRemote, err := found.Repository.CommitsOnNoRemote(ctx)
		if err != nil {
			return repositoryRow{}, err
		}
		row.commitsOnNoRemote = onNoRemote
	}
	if defaultBranch.Ref == "" {
		return row, nil
	}
	if mayBeFinished(status, defaultBranch) {
		merged, err := found.Repository.MergedInto(ctx, "HEAD", defaultBranch.Ref)
		if err != nil {
			return repositoryRow{}, err
		}
		row.finished = merged
	}
	if defaultBranch.Ref == status.Upstream {
		return row, nil
	}
	behind, err := found.Repository.CommitsBehind(ctx, defaultBranch.Ref)
	if err != nil {
		return repositoryRow{}, err
	}
	row.commitsBehindDefault = behind
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
	return m, overviewRequest{findRepositories: m.findRepositories}.load
}

// Lists the focused repositories first and selects the one under review, so enter right away keeps it.
func (m Model) withOverviewLoaded(msg overviewMsg) Model {
	rows := slices.Clone(msg.rows)
	slices.SortStableFunc(rows, func(a overviewRow, b overviewRow) int {
		return cmp.Compare(m.focusRank(a), m.focusRank(b))
	})
	loadedOverview := selectableList[overviewRow]{loaded: true, rows: rows, selected: 0, offset: 0}
	m.overview = loadedOverview.withSelected(slices.IndexFunc(rows, isRowAt(m.repository.Dir)))
	m = m.withListScrolled()
	if len(msg.failures) == 0 {
		return m
	}
	return m.withFooterError("skipped " + strings.Join(msg.failures, ", "))
}

func isRowAt(dir string) func(overviewRow) bool {
	return func(row overviewRow) bool {
		foundRow, isRepository := row.(repositoryRow)
		return isRepository && foundRow.Repository.Dir == dir
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
	case key.Matches(msg, fetchKey):
		return m.withSelectedRepository(m.fetchRepositoryRow)
	case key.Matches(msg, fetchAllKey):
		return m.fetchAll()
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
	case m.syncing[row.Repository.Dir] != "":
		return m, nil
	case row.finished:
		pull = switchRequest{found: row.FoundRepository, branch: row.defaultBranch.Name}.switchAndPull
	case row.status.Upstream == "":
		return m.withFooterError(row.RelativePath + " tracks no upstream, so there is nothing to pull"), nil
	case row.status.UpstreamGone:
		return m.withFooterError(row.RelativePath + " tracks " + row.status.Upstream +
			", which was deleted from the remote, so there is nothing to pull"), nil
	default:
		pull = pullRequest{found: row.FoundRepository}.pull
	}
	return m.withSyncing([]string{row.Repository.Dir}, "pulling..."), pull
}

func (m Model) fetchRepositoryRow(row repositoryRow) (tea.Model, tea.Cmd) {
	if m.syncing[row.Repository.Dir] != "" {
		return m, nil
	}
	return m.withSyncing([]string{row.Repository.Dir}, "fetching..."), fetchRequest{found: row.FoundRepository}.fetch
}

// Skips the repositories a pull or fetch is already running in.
func (m Model) fetchAll() (tea.Model, tea.Cmd) {
	found := []focus.FoundRepository{}
	for _, row := range m.overview.rows {
		foundRow, isRepository := row.(repositoryRow)
		if isRepository && m.syncing[foundRow.Repository.Dir] == "" {
			found = append(found, foundRow.FoundRepository)
		}
	}
	if len(found) == 0 {
		return m, nil
	}
	return m.withSyncing(dirsOf(found), "fetching..."), fetchAllRequest{found: found}.fetchAll
}

func (m Model) withSyncing(dirs []string, text string) Model {
	syncing := maps.Clone(m.syncing)
	for _, dir := range dirs {
		syncing[dir] = text
	}
	m.syncing = syncing
	return m
}

func (m Model) pushRepositoryRow(row repositoryRow) (tea.Model, tea.Cmd) {
	return m.openPushPopup(row.Repository, row.status)
}

func (m Model) reloadOverviewRow(dir string) tea.Cmd {
	index := slices.IndexFunc(m.overview.rows, isRowAt(dir))
	if m.mode != modeOverview || index < 0 {
		return nil
	}
	return overviewRowRequest{found: m.overview.rows[index].(repositoryRow).FoundRepository}.load
}

// A clean branch other than the default one, with no upstream left to pull, is finished once it is merged.
func mayBeFinished(status gitrepo.Status, defaultBranch gitrepo.RemoteBranch) bool {
	return len(status.Files) == 0 && (status.Upstream == "" || status.UpstreamGone) &&
		status.Branch != "" && status.Branch != defaultBranch.Name
}

func (m Model) withOverviewSynced(msg syncedMsg) (tea.Model, tea.Cmd) {
	repository := msg.row.Repository
	m.syncing = withoutDirs(m.syncing, []string{repository.Dir})
	m = m.withOverviewRow(msg.row).withFooterNotice(msg.notice)
	if repository != m.repository {
		return m, nil
	}
	return m, m.reloadStatus()
}

func (m Model) withOverviewFetchedAll(msg fetchedAllMsg) (tea.Model, tea.Cmd) {
	m.syncing = withoutDirs(m.syncing, msg.dirs)
	for _, row := range msg.rows {
		m = m.withOverviewRow(row)
	}
	if len(msg.failures) > 0 {
		m = m.withFooterError("fetch failed: " + strings.Join(msg.failures, ", "))
	} else {
		m = m.withFooterNotice("fetched every repository")
	}
	if !slices.Contains(msg.dirs, m.repository.Dir) {
		return m, nil
	}
	return m, m.reloadStatus()
}

func (m Model) withOverviewRow(row repositoryRow) Model {
	rows := slices.Clone(m.overview.rows)
	index := slices.IndexFunc(rows, isRowAt(row.Repository.Dir))
	if index >= 0 {
		rows[index] = row
	}
	m.overview.rows = rows
	return m
}

func withoutDirs(set map[string]string, dirs []string) map[string]string {
	remaining := maps.Clone(set)
	for _, dir := range dirs {
		delete(remaining, dir)
	}
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
	var changeRows []string
	if len(row.status.Files) == 0 {
		changeRows = []string{mutedStyle.Render("clean")}
	} else {
		changeRows = []string{}
		for _, file := range row.status.Files {
			changeRows = append(changeRows, renderFileRow(file))
		}
	}
	width := m.width / 3
	lastWidth := m.width - 2*width
	return lipgloss.JoinHorizontal(lipgloss.Top,
		renderPane("changes", "", "", changeRows, width-2, height),
		renderPane("outgoing", "", "", renderCommitRows(row.outgoing, row.loadedAt, "nothing to push", width-2),
			width-2, height),
		renderPane("incoming", "", "", renderCommitRows(row.incoming, row.loadedAt, "nothing to pull", lastWidth-2),
			lastWidth-2, height))
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
	var focusMark string
	if m.isFocused(row) {
		focusMark = markStyle.Render("●")
	}
	cells := append([]string{focusMark}, row.cells()...)
	foundRow, isRepository := row.(repositoryRow)
	if isRepository && m.syncing[foundRow.Repository.Dir] != "" {
		cells = append(cells, mutedStyle.Render(m.syncing[foundRow.Repository.Dir]))
	}
	return cells
}

func (r repositoryRow) cells() []string {
	var unpushed string
	if r.commitsOnNoRemote == 0 {
		unpushed = renderAheadBehind(r.status.Tracking)
	} else {
		unpushed = aheadStyle.Render("↑"+strconv.Itoa(r.commitsOnNoRemote)) + mutedStyle.Render(" unpublished")
	}
	var behindDefault string
	if r.commitsBehindDefault > 0 {
		behindDefault = behindStyle.Render("↓"+strconv.Itoa(r.commitsBehindDefault)) +
			mutedStyle.Render(" "+sanitizeText(r.defaultBranch.Ref))
	}
	return []string{
		repositoryStyle.Render(folderGlyph + " " + sanitizeText(r.RelativePath)),
		branchStyle.Render(branchGlyph + " " + headName(r.status.Branch)),
		renderChangedCount(len(r.status.Files)),
		unpushed,
		behindDefault,
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
