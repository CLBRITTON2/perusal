package ui

import (
	"cmp"
	"context"
	"maps"
	"path/filepath"
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
	cells(query subsequenceWords) []string
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

// pendingRow is a repository whose git calls are still running.
type pendingRow struct {
	focus.FoundRepository
}

type overviewMsg struct {
	rows []overviewRow
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

type overviewRowFailureMsg struct {
	dir string
	err error
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
	row, err := loadOverviewRow(context.Background(), r.found)
	if err != nil {
		return syncFailureMsg{dir: dir, err: err}
	}
	return syncedMsg{row: row, notice: "pulled " + r.found.RelativePath}
}

type rebaseRequest struct {
	found focus.FoundRepository
}

func (r rebaseRequest) rebase() tea.Msg {
	dir := r.found.Repository.Dir
	base, err := r.found.Repository.RebaseOntoDefault(context.Background())
	if err != nil {
		return syncFailureMsg{dir: dir, err: err}
	}
	row, err := loadOverviewRow(context.Background(), r.found)
	if err != nil {
		return syncFailureMsg{dir: dir, err: err}
	}
	return syncedMsg{row: row, notice: "rebased " + r.found.RelativePath + " onto " + base.Ref}
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
	results := make([]repositoryRow, len(r.found))
	errs := make([]error, len(r.found))
	var group sync.WaitGroup
	for index, repository := range r.found {
		group.Go(func() {
			results[index], errs[index] = fetchRow(repository)
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
	return fetchedAllMsg{dirs: dirsOf(r.found), rows: rows, failures: failures}
}

func fetchRow(found focus.FoundRepository) (repositoryRow, error) {
	if err := found.Repository.Fetch(context.Background()); err != nil {
		return repositoryRow{}, err
	}
	return loadOverviewRow(context.Background(), found)
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
	row, err := loadOverviewRow(context.Background(), r.found)
	if err != nil {
		return overviewRowFailureMsg{dir: r.found.Repository.Dir, err: err}
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
	rows := []overviewRow{}
	for _, repository := range found.Repositories {
		rows = append(rows, pendingRow{FoundRepository: repository})
	}
	for _, refused := range found.Refusals {
		rows = append(rows, refusedRow{FoundRefusal: refused})
	}
	slices.SortFunc(rows, func(a overviewRow, b overviewRow) int {
		return strings.Compare(a.relativePath(), b.relativePath())
	})
	return overviewMsg{rows: rows}
}

func ReadOwnerWorkTree(ctx context.Context, found focus.FoundRepository) (focus.OwnerWorkTree, error) {
	row, err := loadOverviewRow(ctx, found)
	if err != nil {
		return focus.OwnerWorkTree{}, err
	}
	subject := ""
	mergedInto := ""
	if row.status.HasCommits {
		if subject, err = found.Repository.HeadSubject(ctx); err != nil {
			return focus.OwnerWorkTree{}, err
		}
		onOwnBranch := row.status.Branch != "" && row.status.Branch != row.defaultBranch.Name
		if onOwnBranch && row.defaultBranch.Ref != "" {
			defaultHoldsHead, err := found.Repository.IsAncestor(ctx, "HEAD", row.defaultBranch.Ref)
			if err != nil {
				return focus.OwnerWorkTree{}, err
			}
			if !defaultHoldsHead {
				merged, err := found.Repository.MergedInto(ctx, "HEAD", row.defaultBranch.Ref)
				if err != nil {
					return focus.OwnerWorkTree{}, err
				}
				if merged {
					mergedInto = row.defaultBranch.Ref
				}
			}
		}
	}
	upstream := ""
	if !row.status.UpstreamGone {
		upstream = row.status.Upstream
	}
	changedFiles := []string{}
	// The cap keeps a work tree full of build output short.
	for _, file := range row.status.Files[:min(len(row.status.Files), 50)] {
		changedFiles = append(changedFiles, file.Path)
	}
	return focus.OwnerWorkTree{
		Branch:           row.status.Branch,
		Head:             row.status.Head,
		HeadSubject:      subject,
		ChangedFiles:     changedFiles,
		ChangedFileCount: len(row.status.Files),
		Upstream:         upstream,
		Ahead:            row.status.Ahead,
		Behind:           row.status.Behind,
		BehindDefault:    row.commitsBehindDefault,
		MergedInto:       mergedInto,
	}, nil
}

func loadOverviewRow(ctx context.Context, found focus.FoundRepository) (repositoryRow, error) {
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

func (r pendingRow) relativePath() string {
	return r.RelativePath
}

func (m Model) openOverview() (tea.Model, tea.Cmd) {
	m.mode = modeOverview
	m.overview = selectableList[overviewRow]{rows: []overviewRow{}}
	m.overviewRows = []overviewRow{}
	m.overviewQuery.Reset()
	m.overviewQuery.Blur()
	return m, overviewRequest{findRepositories: m.findRepositories}.load
}

// Lists the focused repositories first and keeps the selected repository on a refresh, or else selects the one under
// review, so enter right away keeps it. Each pending row loads on its own, so the table shows before the slowest
// repository is done.
func (m Model) withOverviewLoaded(msg overviewMsg) (tea.Model, tea.Cmd) {
	rows := slices.Clone(msg.rows)
	slices.SortStableFunc(rows, func(a overviewRow, b overviewRow) int {
		return cmp.Compare(m.focusRank(a), m.focusRank(b))
	})
	selectedRow, _ := m.overview.selectedRow()
	selectedRepository, isRepository := rowRepository(selectedRow)
	var selectedDir string
	if isRepository {
		selectedDir = selectedRepository.Dir
	} else {
		selectedDir = m.repository.Dir
	}
	m.overviewRows = rows
	matching := m.matchingOverviewRows()
	loadedOverview := selectableList[overviewRow]{loaded: true, rows: matching, selected: 0, offset: 0}
	m.overview = loadedOverview.withSelected(slices.IndexFunc(matching, isRowAt(selectedDir)))
	loads := []tea.Cmd{}
	for _, row := range rows {
		pending, isPending := row.(pendingRow)
		if isPending {
			loads = append(loads, overviewRowRequest{found: pending.FoundRepository}.load)
		}
	}
	return m.withListScrolled(), tea.Batch(loads...)
}

// A row git fails on while pending leaves the table, and a reloaded one keeps its last load.
func (m Model) withOverviewRowFailed(msg overviewRowFailureMsg) Model {
	index := slices.IndexFunc(m.overviewRows, isRowAt(msg.dir))
	if index < 0 {
		return m.withFooterError(msg.err.Error())
	}
	if _, isPending := m.overviewRows[index].(pendingRow); !isPending {
		return m.withFooterError(msg.err.Error())
	}
	rows := slices.Delete(slices.Clone(m.overviewRows), index, index+1)
	return m.withOverviewRows(rows).withFooterError("skipped " + msg.err.Error())
}

// withOverviewRows lists the rows overviewQuery matches and keeps the selected row, or else the selected index.
func (m Model) withOverviewRows(rows []overviewRow) Model {
	selectedRow, hasRows := m.overview.selectedRow()
	m.overviewRows = rows
	m.overview.rows = m.matchingOverviewRows()
	selected := slices.IndexFunc(m.overview.rows, func(row overviewRow) bool {
		return hasRows && row.relativePath() == selectedRow.relativePath()
	})
	if selected < 0 {
		selected = m.overview.selected
	}
	m.overview = m.overview.withSelected(selected).scrolled(m.overviewTableInnerHeight())
	return m
}

// matchingOverviewRows matches the query on the relative path, and on a loaded row's branch too.
func (m Model) matchingOverviewRows() []overviewRow {
	query := m.overviewQueryWords()
	matching := []overviewRow{}
	for _, row := range m.overviewRows {
		fields := []string{row.relativePath()}
		if loaded, isLoaded := row.(repositoryRow); isLoaded {
			fields = append(fields, loaded.status.Branch)
		}
		if query.matchEvery(fields) {
			matching = append(matching, row)
		}
	}
	return matching
}

func (m Model) overviewQueryWords() subsequenceWords {
	return strings.Fields(m.overviewQuery.Value())
}

func (m Model) typingOverviewQuery() bool {
	return m.mode == modeOverview && m.overviewQuery.Focused()
}

func (m Model) withOverviewQueryCleared() Model {
	m.overviewQuery.Reset()
	return m.withOverviewRows(m.overviewRows)
}

func rowRepository(row overviewRow) (gitrepo.Repository, bool) {
	switch typedRow := row.(type) {
	case repositoryRow:
		return typedRow.Repository, true
	case pendingRow:
		return typedRow.Repository, true
	}
	return gitrepo.Repository{}, false
}

func isRowAt(dir string) func(overviewRow) bool {
	return func(row overviewRow) bool {
		repository, isRepository := rowRepository(row)
		return isRepository && repository.Dir == dir
	}
}

func (m Model) isFocused(row overviewRow) bool {
	repository, isRepository := rowRepository(row)
	return isRepository && slices.Contains(m.focused, repository)
}

func (m Model) focusRank(row overviewRow) int {
	if m.isFocused(row) {
		return 0
	}
	return 1
}

func (m Model) updateOverview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footer = footer{}
	if m.overviewQuery.Focused() {
		return m.updateOverviewQuery(msg)
	}
	switch {
	case key.Matches(msg, quitKey):
		return m, tea.Quit
	case key.Matches(msg, clearSearchKey) && len(m.overviewQueryWords()) > 0:
		return m.withOverviewQueryCleared(), nil
	case key.Matches(msg, closeOverviewKey):
		m.mode = modeBrowsing
		return m, nil
	case key.Matches(msg, searchKey):
		m.overviewQuery.CursorEnd()
		return m, m.overviewQuery.Focus()
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
	case key.Matches(msg, rebaseKey):
		return m.withSelectedRepository(m.rebaseRepositoryRow)
	case key.Matches(msg, pushKey):
		return m.withSelectedRepository(m.pushRepositoryRow)
	case key.Matches(msg, cherryPickKey):
		return m.withSelectedRepository(m.cherryPickRepositoryRow)
	case key.Matches(msg, deleteWorkTreeKey):
		return m.withSelectedRepository(m.deleteWorkTreeRow)
	case key.Matches(msg, refreshKey):
		return m, overviewRequest{findRepositories: m.findRepositories}.load
	}
	return m, nil
}

// updateOverviewQuery leaves only the arrows to move the selection, so j and k type letters.
func (m Model) updateOverviewQuery(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, openRepositoryKey):
		m.overviewQuery.Blur()
		return m.withSelectedRepository(m.openRepositoryRow)
	case key.Matches(msg, keepFilterKey):
		m.overviewQuery.Blur()
		return m, nil
	case key.Matches(msg, clearSearchKey),
		key.Matches(msg, closeEmptySearchKey) && m.overviewQuery.Value() == "":
		m.overviewQuery.Blur()
		return m.withOverviewQueryCleared(), nil
	case key.Matches(msg, filterDownKey):
		m.overview = m.overview.withSelected(m.overview.selected + 1).scrolled(m.overviewTableInnerHeight())
		return m, nil
	case key.Matches(msg, filterUpKey):
		m.overview = m.overview.withSelected(m.overview.selected - 1).scrolled(m.overviewTableInnerHeight())
		return m, nil
	}
	var inputCmd tea.Cmd
	m.overviewQuery, inputCmd = m.overviewQuery.Update(msg)
	return m.withOverviewRows(m.overviewRows), inputCmd
}

type cherryPickRequest struct {
	mainClone repositoryRow
	branch    string
}

func (r cherryPickRequest) cherryPick() tea.Msg {
	picked, err := r.mainClone.Repository.CherryPickBranch(context.Background(), r.branch)
	return popupResult(r.mainClone.Repository.Dir, err, "cherry-picked "+pluralize(picked, "commit")+" from "+
		r.branch+" onto "+r.mainClone.defaultBranch.Name+" in "+r.mainClone.RelativePath)
}

func (m Model) cherryPickRepositoryRow(row repositoryRow) (tea.Model, tea.Cmd) {
	switch {
	case m.syncing[row.Repository.Dir] != "":
		return m, nil
	case row.status.Branch == "":
		return m.withFooterError(row.RelativePath + " has a detached HEAD, so there is no branch to cherry-pick"), nil
	case len(row.status.Files) > 0:
		return m.withFooterError(row.RelativePath + " has uncommitted changes, and a cherry-pick takes only commits, " +
			"so commit them first"), nil
	case !isLinkedWorkTree(row):
		return m.withFooterError(row.RelativePath + " is a main clone, cherry-pick from a work tree made by " +
			"git worktree add"), nil
	}
	mainClone, isLoaded := m.mainCloneRow(row)
	if !isLoaded {
		return m.withFooterError(row.RelativePath + "'s main clone is not loaded in the overview"), nil
	}
	if mainClone.defaultBranch.Name == "" {
		return m.withFooterError(mainClone.RelativePath + " has no origin/HEAD, so its default branch is unknown, " +
			"git remote set-head origin --auto sets it"), nil
	}
	defaultName := sanitizeText(mainClone.defaultBranch.Name)
	return m.withConfirmation(mainClone.Repository.Dir, confirmation{
		lines: []string{
			titleStyle.Render("cherry-pick " + headName(row.status.Branch) + " onto " + defaultName),
			textStyle.Render("its commits " + defaultName + " lacks go onto " + defaultName + " in " +
				sanitizeText(mainClone.RelativePath) + ", ready to push from there"),
		},
		confirmKey:  confirmPickKey,
		run:         cherryPickRequest{mainClone: mainClone, branch: row.status.Branch}.cherryPick,
		runningText: "cherry-picking...",
	}), nil
}

// isLinkedWorkTree reads a work tree made by git worktree add from its git directory, which sits under its main
// clone's worktrees folder, so no git runs.
func isLinkedWorkTree(row repositoryRow) bool {
	adminDir := row.Repository.GitDir
	return adminDir != "" && filepath.Base(filepath.Dir(adminDir)) == "worktrees"
}

func (m Model) mainCloneRow(row repositoryRow) (repositoryRow, bool) {
	commonDir := filepath.Dir(filepath.Dir(row.Repository.GitDir))
	mainIndex := slices.IndexFunc(m.overviewRows, func(candidate overviewRow) bool {
		mainClone, isLoaded := candidate.(repositoryRow)
		return isLoaded && mainClone.Repository.GitDir == commonDir
	})
	if mainIndex < 0 {
		return repositoryRow{}, false
	}
	return m.overviewRows[mainIndex].(repositoryRow), true
}

// deleteWorkTreeRow runs the work tree list's delete check through the work tree's main clone.
func (m Model) deleteWorkTreeRow(row repositoryRow) (tea.Model, tea.Cmd) {
	switch {
	case m.syncing[row.Repository.Dir] != "":
		return m, nil
	case !isLinkedWorkTree(row):
		return m.withFooterError(row.RelativePath + " is the main work tree, which perusal never deletes"), nil
	case row.Repository.Dir == m.repository.Dir:
		return m.withFooterError(row.RelativePath + " is under review, so it cannot be deleted"), nil
	case row.status.Branch == "":
		return m.withFooterError(row.RelativePath + " has a detached HEAD, so it cannot be deleted"), nil
	}
	mainClone, isLoaded := m.mainCloneRow(row)
	if !isLoaded {
		return m.withFooterError(row.RelativePath + "'s main clone is not loaded in the overview"), nil
	}
	workTree := workTreeRow{workTree: gitrepo.WorkTree{Path: row.Repository.Dir, Branch: row.status.Branch},
		repository: row.Repository, changedCount: len(row.status.Files), refusal: nil}
	return m, workTreeDeleteRequest{repository: mainClone.Repository, row: workTree, folder: row.RelativePath,
		ownerClone: mainClone.Repository.Dir, agentHost: m.agentHost, agentWorkTree: gitrepo.AgentWorkTree{},
		screen: modeOverview, reviewedDir: m.repository.Dir}.check
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

// rebaseRepositoryRow refuses uncommitted changes, since a rebase that stops on a conflict would mix them in.
func (m Model) rebaseRepositoryRow(row repositoryRow) (tea.Model, tea.Cmd) {
	switch {
	case m.syncing[row.Repository.Dir] != "":
		return m, nil
	case row.status.Branch == "":
		return m.withFooterError(row.RelativePath + " has a detached HEAD, so there is no branch to rebase"), nil
	case len(row.status.Files) > 0:
		return m.withFooterError(row.RelativePath + " has uncommitted changes, commit or stash them to rebase"), nil
	}
	return m.withSyncing([]string{row.Repository.Dir}, "rebasing..."), rebaseRequest{found: row.FoundRepository}.rebase
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
	for _, row := range m.overviewRows {
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

func (m Model) reloadOverview() tea.Cmd {
	if m.mode != modeOverview {
		return nil
	}
	return overviewRequest{findRepositories: m.findRepositories}.load
}

func (m Model) reloadOverviewRow(dir string) tea.Cmd {
	index := slices.IndexFunc(m.overviewRows, isRowAt(dir))
	if m.mode != modeOverview || index < 0 {
		return nil
	}
	row, isRepository := m.overviewRows[index].(repositoryRow)
	if !isRepository {
		return nil
	}
	return overviewRowRequest{found: row.FoundRepository}.load
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
	rows := slices.Clone(m.overviewRows)
	index := slices.IndexFunc(rows, isRowAt(row.Repository.Dir))
	if index >= 0 {
		rows[index] = row
	}
	return m.withOverviewRows(rows)
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
	var title, emptyText string
	if len(m.overviewQueryWords()) > 0 {
		title = "repositories /" + sanitizeText(m.overviewQuery.Value())
		emptyText = "no repositories match"
	} else {
		title = "repositories"
		emptyText = "no repositories found"
	}
	table := renderListPane(title, m.overview, m.overviewCells, emptyText, m.width-2, m.overviewTableInnerHeight())
	return lipgloss.JoinVertical(lipgloss.Left, table, m.renderOverviewDetails())
}

func (m Model) overviewTableInnerHeight() int {
	return max(m.columnHeight()-m.columnHeight()/2-2, 0)
}

func (m Model) acceptsOverviewMouse() bool {
	return m.mode == modeOverview && m.popup.kind == noPopup
}

// The table's first row sits under the header and the table's top border.
func (m Model) clickOverviewRow(mouse tea.Mouse) Model {
	row := mouse.Y - headerHeight - 1
	index := m.overview.offset + row
	if row < 0 || row >= m.overviewTableInnerHeight() || index >= len(m.overview.rows) {
		return m
	}
	m.overview = m.overview.withSelected(index).scrolled(m.overviewTableInnerHeight())
	return m
}

// The wheel moves the selection wherever it turns, since the details below follow the selected row.
func (m Model) scrollOverview(mouse tea.Mouse) Model {
	switch mouse.Button {
	case tea.MouseWheelDown:
		m.overview = m.overview.withSelected(m.overview.selected + 1).scrolled(m.overviewTableInnerHeight())
	case tea.MouseWheelUp:
		m.overview = m.overview.withSelected(m.overview.selected - 1).scrolled(m.overviewTableInnerHeight())
	}
	return m
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
	cells := append([]string{focusMark}, row.cells(m.overviewQueryWords())...)
	foundRow, isRepository := row.(repositoryRow)
	if isRepository && m.syncing[foundRow.Repository.Dir] != "" {
		cells = append(cells, mutedStyle.Render(m.syncing[foundRow.Repository.Dir]))
	}
	return cells
}

func (r repositoryRow) cells(query subsequenceWords) []string {
	var branch string
	if r.status.Branch == "" {
		branch = branchStyle.Render(branchGlyph + " " + headName(r.status.Branch))
	} else {
		branch = branchStyle.Render(branchGlyph+" ") + query.highlight(r.status.Branch, branchStyle)
	}
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
		repositoryStyle.Render(folderGlyph+" ") + query.highlight(r.RelativePath, repositoryStyle),
		branch,
		renderChangedCount(len(r.status.Files)),
		unpushed,
		behindDefault,
	}
}

func (r refusedRow) cells(query subsequenceWords) []string {
	return []string{
		mutedStyle.Render(folderGlyph+" ") + query.highlight(r.RelativePath, mutedStyle),
		mutedStyle.Render(sanitizeText(r.Summary())),
	}
}

func (r pendingRow) cells(query subsequenceWords) []string {
	return []string{
		repositoryStyle.Render(folderGlyph+" ") + query.highlight(r.RelativePath, repositoryStyle),
		mutedStyle.Render("loading..."),
	}
}

func renderChangedCount(count int) string {
	if count == 0 {
		return mutedStyle.Render("clean")
	}
	return warningStyle.Render(strconv.Itoa(count) + " changed")
}
