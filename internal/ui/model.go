package ui

import (
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

const (
	headerHeight      = 1
	footerHeight      = 1
	minimumListWidth  = 20
	listPercent       = 25
	wheelScrollLines  = 3
	maximumPopupWidth = 72
	descriptionHeight = 5
	maximumErrorLines = 12
	branchGlyph       = ""
	folderGlyph       = ""
	appName           = "perusal"
)

const commitRow = -1

type viewKind int

const (
	viewChanges viewKind = iota
	viewLog
)

type popupKind int

const (
	noPopup popupKind = iota
	commitPopup
	pushPopup
)

type Model struct {
	repository     gitrepo.Repository
	rejection      string
	view           viewKind
	diffZoomed     bool
	status         gitrepo.Status
	selected       int
	listOffset     int
	diffPath       string
	fileDiff       diff.FileDiff
	commits        []gitrepo.Commit
	graph          []graphRow
	logLimit       int
	logLoadedAt    time.Time
	selectedCommit int
	selectedFile   int
	// logOffset counts rows of the log tree, commit and file rows alike.
	logOffset          int
	commitFiles        map[string][]diff.FileSection
	expanded           map[string]bool
	logFilter          searchTerms
	searching          bool
	searchInput        textinput.Model
	searchError        string
	diffView           viewport.Model
	help               help.Model
	summaryInput       textinput.Model
	descriptionBox     textarea.Model
	descriptionFocused bool
	popup              popupKind
	popupRunning       bool
	popupError         string
	footerText         string
	footerIsError      bool
	width              int
	height             int
}

func New() Model {
	summaryInput := textinput.New()
	summaryInput.Placeholder = "Summary (required)"
	summaryInput.Prompt = ""
	descriptionBox := textarea.New()
	descriptionBox.Placeholder = "Description"
	descriptionBox.ShowLineNumbers = false
	descriptionBox.Prompt = ""
	searchInput := textinput.New()
	searchInput.Prompt = "/"
	helpView := help.New()
	helpView.ShortSeparator = "  "
	helpView.Styles.ShortKey = helpKeyStyle
	helpView.Styles.ShortDesc = helpDescStyle
	helpView.Styles.ShortSeparator = helpSepStyle
	helpView.Styles.Ellipsis = helpSepStyle
	return Model{
		status:         gitrepo.Status{Files: []gitrepo.FileStatus{}},
		commits:        []gitrepo.Commit{},
		graph:          []graphRow{},
		logLimit:       logPageSize,
		selectedFile:   commitRow,
		commitFiles:    map[string][]diff.FileSection{},
		expanded:       map[string]bool{},
		searchInput:    searchInput,
		diffView:       viewport.New(),
		help:           helpView,
		summaryInput:   summaryInput,
		descriptionBox: descriptionBox,
	}
}

func (m Model) Init() tea.Cmd {
	return nextPoll()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.withLayout(), nil
	case RepositoryFocused:
		if msg.Repository.Dir == m.repository.Dir && m.rejection == "" {
			return m, nil
		}
		m = m.withRepository(msg.Repository, "")
		return m, repositoryRequest{repository: m.repository}.loadStatus
	case FocusRejected:
		return m.withRepository(gitrepo.Repository{}, sanitizeSingleLine(msg.Reason)), nil
	case pollTickMsg:
		if m.repository.Dir == "" {
			return m, nextPoll()
		}
		// The next tick waits for this status, so a slow git never stacks up polls.
		return m, tea.Sequence(repositoryRequest{repository: m.repository}.loadStatus, nextPoll())
	case statusLoadedMsg:
		if msg.dir != m.repository.Dir {
			return m, nil
		}
		return m.withStatus(msg.status)
	case diffLoadedMsg:
		if msg.dir != m.repository.Dir || msg.path != m.selectedPath() {
			return m, nil
		}
		if msg.path != m.diffPath && m.view == viewChanges {
			m.diffView.SetYOffset(0)
		}
		m.diffPath = msg.path
		m.fileDiff = msg.fileDiff
		return m.withDiffContent(), nil
	case logLoadedMsg:
		if msg.dir != m.repository.Dir || msg.limit != m.logLimit {
			return m, nil
		}
		return m.withCommits(msg.commits, msg.loadedAt)
	case commitDiffLoadedMsg:
		if msg.dir != m.repository.Dir {
			return m, nil
		}
		m.commitFiles = maps.Clone(m.commitFiles)
		m.commitFiles[msg.hash] = msg.sections
		return m.withDiffContent().withListScrolled(), nil
	case indexChangedMsg:
		if msg.dir != m.repository.Dir {
			return m, nil
		}
		return m, repositoryRequest{repository: m.repository}.loadStatus
	case committedMsg:
		if msg.dir != m.repository.Dir {
			return m.withFooterNotice("committed in " + msg.dir + ": " + msg.summary), nil
		}
		m = m.withPopupClosed().withFooterNotice(msg.summary)
		m.summaryInput.Reset()
		m.descriptionBox.Reset()
		return m, repositoryRequest{repository: m.repository}.loadStatus
	case pushedMsg:
		if msg.dir != m.repository.Dir {
			return m.withFooterNotice("pushed " + msg.dir + " to " + msg.upstream), nil
		}
		return m.withPopupClosed().withFooterNotice("pushed to " + msg.upstream),
			repositoryRequest{repository: m.repository}.loadStatus
	case popupFailedMsg:
		if msg.dir != m.repository.Dir || m.popup == noPopup {
			return m.withFooterError(msg.err.Error()), nil
		}
		m.popupRunning = false
		m.popupError = sanitizeLines(msg.err.Error())
		return m, nil
	case gitFailedMsg:
		return m.withFooterError(msg.err.Error()), nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, forceQuitKey):
			return m, tea.Quit
		case m.popup == commitPopup:
			return m.updateCommitBox(msg)
		case m.popup == pushPopup:
			return m.updatePushPopup(msg)
		case m.searching:
			return m.updateSearch(msg)
		}
		return m.updateBrowsing(msg)
	case tea.MouseClickMsg:
		if m.popup != noPopup || msg.Button != tea.MouseLeft {
			return m, nil
		}
		return m.clickList(msg.Mouse())
	case tea.MouseWheelMsg:
		if m.popup != noPopup {
			return m, nil
		}
		return m.scrollWheel(msg.Mouse())
	}
	if m.popup == commitPopup {
		return m.updateFocusedField(msg)
	}
	if m.searching {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) withStatus(status gitrepo.Status) (tea.Model, tea.Cmd) {
	m.status = status
	m.selected = min(m.selected, max(len(m.status.Files)-1, 0))
	if !status.HasCommits {
		m.commits = []gitrepo.Commit{}
		m.graph = []graphRow{}
		m.selectedFile = commitRow
		return m.withLayout(), m.loadSelectedDiff()
	}
	return m.withLayout(), tea.Batch(m.loadSelectedDiff(), logRequest{repository: m.repository, limit: m.logLimit}.load)
}

func (m Model) updateBrowsing(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footerText = ""
	switch {
	case key.Matches(msg, quitKey):
		return m, tea.Quit
	case key.Matches(msg, downKey):
		return m.moveSelection(1)
	case key.Matches(msg, upKey):
		return m.moveSelection(-1)
	case key.Matches(msg, scrollDownKey):
		m.diffView.HalfPageDown()
		return m, nil
	case key.Matches(msg, scrollUpKey):
		m.diffView.HalfPageUp()
		return m, nil
	case key.Matches(msg, switchViewKey):
		return m.switchView()
	case key.Matches(msg, zoomKey):
		m.diffZoomed = !m.diffZoomed
		return m.withLayout(), nil
	case key.Matches(msg, expandKey) && m.view == viewLog:
		return m.toggleExpanded()
	case key.Matches(msg, searchKey) && m.view == viewLog:
		m.searching = true
		m.searchInput.CursorEnd()
		return m, m.searchInput.Focus()
	case key.Matches(msg, clearSearchKey) && m.view == viewLog && m.logFilter != nil:
		return m.clearSearch()
	case key.Matches(msg, stageKey):
		if m.view != viewChanges || len(m.status.Files) == 0 {
			return m, nil
		}
		request := fileRequest{repository: m.repository, file: m.status.Files[m.selected]}
		if request.file.HasUnstagedChanges() {
			return m, request.stage
		}
		return m, request.unstage
	case key.Matches(msg, stageAllKey):
		if m.view != viewChanges || len(m.status.Files) == 0 {
			return m, nil
		}
		request := repositoryRequest{repository: m.repository}
		if slices.ContainsFunc(m.status.Files, gitrepo.FileStatus.HasUnstagedChanges) {
			return m, request.stageAll
		}
		return m, request.unstageAll
	case key.Matches(msg, commitKey):
		if m.repository.Dir == "" {
			return m, nil
		}
		m.popup = commitPopup
		m.descriptionFocused = false
		return m, m.summaryInput.Focus()
	case key.Matches(msg, pushKey):
		return m.openPushPopup()
	}
	return m, nil
}

func (m Model) switchView() (tea.Model, tea.Cmd) {
	if m.view == viewChanges {
		m.view = viewLog
	} else {
		m.view = viewChanges
	}
	m.diffView.SetYOffset(0)
	m = m.withLayout()
	return m, m.loadSelectedCommitDiff()
}

func (m Model) withRepository(repository gitrepo.Repository, rejection string) Model {
	next := New()
	next.repository = repository
	next.rejection = rejection
	next.view = m.view
	next.diffZoomed = m.diffZoomed
	next.width = m.width
	next.height = m.height
	return next.withLayout()
}

func (m Model) withFooterNotice(text string) Model {
	m.footerText = sanitizeSingleLine(text)
	m.footerIsError = false
	return m
}

func (m Model) withFooterError(text string) Model {
	m.footerText = sanitizeSingleLine(text)
	m.footerIsError = true
	return m
}

func (m Model) updateCommitBox(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.popupRunning {
		return m, nil
	}
	switch {
	case key.Matches(msg, cancelKey):
		return m.withPopupClosed(), nil
	case key.Matches(msg, nextFieldKey):
		return m.toggleCommitField()
	case key.Matches(msg, submitKey):
		return m.startCommit()
	case key.Matches(msg, enterKey) && !m.descriptionFocused:
		return m.startCommit()
	}
	return m.updateFocusedField(msg)
}

func (m Model) updateFocusedField(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.descriptionFocused {
		m.descriptionBox, cmd = m.descriptionBox.Update(msg)
	} else {
		m.summaryInput, cmd = m.summaryInput.Update(msg)
	}
	return m, cmd
}

func (m Model) toggleCommitField() (tea.Model, tea.Cmd) {
	m.descriptionFocused = !m.descriptionFocused
	if m.descriptionFocused {
		m.summaryInput.Blur()
		return m, m.descriptionBox.Focus()
	}
	m.descriptionBox.Blur()
	return m, m.summaryInput.Focus()
}

func (m Model) startCommit() (tea.Model, tea.Cmd) {
	summary := strings.TrimSpace(m.summaryInput.Value())
	if summary == "" {
		m.popupError = "the summary is required"
		return m, nil
	}
	m.popupRunning = true
	m.popupError = ""
	message := joinCommitMessage(summary, m.descriptionBox.Value())
	return m, commitRequest{repository: m.repository, message: message}.commit
}

func joinCommitMessage(summary string, description string) string {
	trimmedDescription := strings.TrimSpace(description)
	if trimmedDescription == "" {
		return summary
	}
	return summary + "\n\n" + trimmedDescription
}

func (m Model) withPopupClosed() Model {
	m.popup = noPopup
	m.popupRunning = false
	m.popupError = ""
	m.summaryInput.Blur()
	m.descriptionBox.Blur()
	return m
}

func (m Model) openPushPopup() (tea.Model, tea.Cmd) {
	switch {
	case m.repository.Dir == "":
		return m, nil
	case m.status.Upstream == "":
		return m.withFooterError("the branch tracks no upstream, so there is nowhere to push"), nil
	case m.status.Ahead == 0:
		return m.withFooterError("nothing to push"), nil
	}
	m.popup = pushPopup
	return m, nil
}

func (m Model) updatePushPopup(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.popupRunning {
		return m, nil
	}
	switch {
	case key.Matches(msg, cancelKey):
		return m.withPopupClosed(), nil
	case key.Matches(msg, confirmPushKey):
		m.popupRunning = true
		m.popupError = ""
		return m, pushRequest{repository: m.repository, upstream: m.status.Upstream}.push
	}
	return m, nil
}

func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	if m.view == viewLog {
		return m.selectLogRow(m.selectedLogRow(m.visibleLogRows()) + delta)
	}
	return m.selectFile(m.selected + delta)
}

func (m Model) selectFile(index int) (tea.Model, tea.Cmd) {
	if len(m.status.Files) == 0 {
		return m, nil
	}
	m.selected = min(max(index, 0), len(m.status.Files)-1)
	return m.withListScrolled(), m.loadSelectedDiff()
}

func (m Model) selectLogRow(index int) (tea.Model, tea.Cmd) {
	rows := m.visibleLogRows()
	if len(rows) == 0 {
		return m, nil
	}
	row := rows[min(max(index, 0), len(rows)-1)]
	if row.commit != m.selectedCommit || row.file != m.selectedFile {
		m.diffView.SetYOffset(0)
	}
	m.selectedCommit = row.commit
	m.selectedFile = row.file
	m, loadMore := m.withListScrolled().withDiffContent().withMoreLogNearEnd()
	return m, tea.Batch(m.loadSelectedCommitDiff(), loadMore)
}

func (m Model) withMoreLogNearEnd() (Model, tea.Cmd) {
	if len(m.commits) < m.logLimit || m.selectedCommit < len(m.commits)-m.listInnerHeight() {
		return m, nil
	}
	m.logLimit = len(m.commits) + logPageSize
	return m, logRequest{repository: m.repository, limit: m.logLimit}.load
}

func (m Model) withCommits(commits []gitrepo.Commit, loadedAt time.Time) (tea.Model, tea.Cmd) {
	hash := m.selectedCommitHash()
	index := slices.IndexFunc(commits, func(commit gitrepo.Commit) bool { return commit.Hash == hash })
	if index < 0 {
		m.selectedFile = commitRow
	}
	if !sameTopology(commits, m.commits) {
		m.graph = commitGraph(commits)
	}
	m.commits = commits
	m.logLoadedAt = loadedAt
	m.selectedCommit = max(index, 0)
	m = m.withLayout()
	return m, m.loadSelectedCommitDiff()
}

func (m Model) toggleExpanded() (tea.Model, tea.Cmd) {
	hash := m.selectedCommitHash()
	if hash == "" {
		return m, nil
	}
	m.expanded = maps.Clone(m.expanded)
	if m.expanded[hash] {
		delete(m.expanded, hash)
	} else {
		m.expanded[hash] = true
	}
	if m.selectedFile != commitRow {
		m.selectedFile = commitRow
		m.diffView.SetYOffset(0)
	}
	m = m.withListScrolled().withDiffContent()
	return m, m.loadSelectedCommitDiff()
}

func (m Model) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keepSearchKey):
		return m.withSearchClosed(), nil
	case key.Matches(msg, clearSearchKey):
		return m.withSearchClosed().clearSearch()
	}
	var inputCmd tea.Cmd
	m.searchInput, inputCmd = m.searchInput.Update(msg)
	filter, err := compileSearch(m.searchInput.Value())
	if err != nil {
		m.searchError = sanitizeSingleLine(err.Error())
		return m, inputCmd
	}
	m.searchError = ""
	next, loadCmd := m.withLogFilter(filter)
	return next, tea.Batch(inputCmd, loadCmd)
}

func (m Model) withSearchClosed() Model {
	m.searching = false
	m.searchError = ""
	m.searchInput.Blur()
	return m
}

func (m Model) clearSearch() (tea.Model, tea.Cmd) {
	m.searchInput.Reset()
	return m.withLogFilter(nil)
}

func (m Model) withLogFilter(filter searchTerms) (tea.Model, tea.Cmd) {
	m.logFilter = filter
	rows := m.visibleLogRows()
	if m.selectedLogRow(rows) < 0 && len(rows) > 0 {
		m.selectedCommit = rows[0].commit
		m.selectedFile = rows[0].file
		m.diffView.SetYOffset(0)
	}
	m = m.withListScrolled().withDiffContent()
	if filter == nil || len(m.commits) < m.logLimit {
		return m, m.loadSelectedCommitDiff()
	}
	m.logLimit = wholeHistory
	return m, tea.Batch(m.loadSelectedCommitDiff(), logRequest{repository: m.repository, limit: m.logLimit}.load)
}

// file indexes the commit's commitFiles, or is commitRow for the commit itself.
type logRow struct {
	commit int
	file   int
}

func (m Model) visibleLogRows() []logRow {
	rows := []logRow{}
	for index, commit := range m.commits {
		if !m.logFilter.matches(commit) {
			continue
		}
		rows = append(rows, logRow{commit: index, file: commitRow})
		if !m.expanded[commit.Hash] {
			continue
		}
		for file := range m.commitFiles[commit.Hash] {
			rows = append(rows, logRow{commit: index, file: file})
		}
	}
	return rows
}

// -1 when the filter hides the selection.
func (m Model) selectedLogRow(rows []logRow) int {
	for index, row := range rows {
		if row.commit == m.selectedCommit && row.file == m.selectedFile {
			return index
		}
	}
	return -1
}

func (m Model) clickList(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.X >= m.listOuterWidth() {
		return m, nil
	}
	if mouse.Y == m.foldedPaneY() {
		return m.switchView()
	}
	row := mouse.Y - m.listFirstRowY()
	if row < 0 || row >= m.listInnerHeight() {
		return m, nil
	}
	if m.view == viewLog {
		if m.logOffset+row >= len(m.visibleLogRows()) {
			return m, nil
		}
		return m.selectLogRow(m.logOffset + row)
	}
	if m.listOffset+row >= len(m.status.Files) {
		return m, nil
	}
	return m.selectFile(m.listOffset + row)
}

func (m Model) scrollWheel(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	overList := mouse.X < m.listOuterWidth()
	switch {
	case overList && mouse.Button == tea.MouseWheelDown:
		return m.moveSelection(1)
	case overList && mouse.Button == tea.MouseWheelUp:
		return m.moveSelection(-1)
	case mouse.Button == tea.MouseWheelDown:
		m.diffView.ScrollDown(wheelScrollLines)
	case mouse.Button == tea.MouseWheelUp:
		m.diffView.ScrollUp(wheelScrollLines)
	}
	return m, nil
}

func (m Model) loadSelectedDiff() tea.Cmd {
	if len(m.status.Files) == 0 {
		return nil
	}
	return fileRequest{repository: m.repository, file: m.status.Files[m.selected]}.loadDiff
}

func (m Model) loadSelectedCommitDiff() tea.Cmd {
	hash := m.selectedCommitHash()
	_, loaded := m.commitFiles[hash]
	if m.view != viewLog || hash == "" || loaded {
		return nil
	}
	return commitDiffRequest{repository: m.repository, hash: hash}.load
}

func (m Model) selectedPath() string {
	if len(m.status.Files) == 0 {
		return ""
	}
	return m.status.Files[m.selected].Path
}

func (m Model) selectedCommitHash() string {
	if len(m.commits) == 0 {
		return ""
	}
	return m.commits[m.selectedCommit].Hash
}

func (m Model) withLayout() Model {
	m.diffView.SetWidth(m.diffInnerWidth())
	m.diffView.SetHeight(m.paneInnerHeight())
	m.help.SetWidth(m.width)
	// textinput draws its end-of-text cursor one cell past its width
	m.summaryInput.SetWidth(max(m.fieldInnerWidth()-1, 0))
	m.searchInput.SetWidth(max(m.width/2, 0))
	m.descriptionBox.SetWidth(m.fieldInnerWidth())
	m.descriptionBox.SetHeight(descriptionHeight)
	return m.withDiffContent().withListScrolled()
}

func (m Model) withDiffContent() Model {
	m.diffView.SetContentLines(m.diffLines())
	return m
}

func (m Model) diffLines() []string {
	width := m.diffInnerWidth()
	sections, loaded := m.commitFiles[m.selectedCommitHash()]
	switch {
	case m.view == viewLog && loaded && m.selectedFile != commitRow:
		return renderFileDiff(sections[m.selectedFile].FileDiff, width)
	case m.view == viewLog && loaded:
		return renderCommit(m.commits[m.selectedCommit], sections, width)
	case m.view == viewChanges && m.diffPath != "":
		return renderFileDiff(m.fileDiff, width)
	default:
		return []string{}
	}
}

func (m Model) withListScrolled() Model {
	m.logOffset = scrolledOffset(m.logOffset, max(m.selectedLogRow(m.visibleLogRows()), 0), m.listInnerHeight())
	m.listOffset = scrolledOffset(m.listOffset, m.selected, m.listInnerHeight())
	return m
}

func scrolledOffset(offset int, selected int, visibleRows int) int {
	switch {
	case selected < offset:
		return selected
	case visibleRows > 0 && selected >= offset+visibleRows:
		return selected - visibleRows + 1
	default:
		return offset
	}
}

func (m Model) listOuterWidth() int {
	if m.diffZoomed {
		return 0
	}
	return max(m.width*listPercent/100, minimumListWidth)
}

func (m Model) logTitle() string {
	if m.logFilter != nil {
		return "log /" + sanitizeText(m.searchInput.Value())
	}
	return "log"
}

func (m Model) diffInnerWidth() int {
	return max(m.width-m.listOuterWidth()-2, 0)
}

func (m Model) columnHeight() int {
	return max(m.height-headerHeight-footerHeight, 0)
}

func (m Model) paneInnerHeight() int {
	return max(m.columnHeight()-2, 0)
}

// Leaves out the open pane's two borders and the folded pane's line.
func (m Model) listInnerHeight() int {
	return max(m.columnHeight()-3, 0)
}

// A folded changes pane sits above the log and pushes it down a line.
func (m Model) listFirstRowY() int {
	if m.view == viewLog {
		return headerHeight + 2
	}
	return headerHeight + 1
}

func (m Model) foldedPaneY() int {
	if m.view == viewLog {
		return headerHeight
	}
	return headerHeight + m.columnHeight() - 1
}

func (m Model) popupInnerWidth() int {
	return max(min(m.width-4, maximumPopupWidth)-4, 0)
}

func (m Model) fieldInnerWidth() int {
	return max(m.popupInnerWidth()-2, 0)
}

func (m Model) View() tea.View {
	var view tea.View
	switch {
	case m.width == 0:
		view = tea.NewView("")
	case m.popup == commitPopup:
		view = tea.NewView(m.overlayPopup(m.renderScreen(), m.renderCommitPopup()))
	case m.popup == pushPopup:
		view = tea.NewView(m.overlayPopup(m.renderScreen(), m.renderPushPopup()))
	default:
		view = tea.NewView(m.renderScreen())
	}
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = m.windowTitle()
	return view
}

func (m Model) windowTitle() string {
	if m.repository.Dir == "" {
		return appName
	}
	return appName + ": " + sanitizeText(filepath.Base(m.repository.Dir))
}

func (m Model) renderScreen() string {
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), m.renderColumns(), m.renderFooter())
}

func (m Model) renderColumns() string {
	if m.diffZoomed {
		return m.renderDiffPane()
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.renderLists(), m.renderDiffPane())
}

func (m Model) branchLabel() string {
	if m.status.Branch == "" {
		return "detached HEAD"
	}
	return sanitizeText(m.status.Branch)
}

func (m Model) renderHeader() string {
	switch {
	case m.rejection != "":
		return fitToWidth(" "+errorStyle.Render("refused the focused pane's directory"), m.width)
	case m.repository.Dir == "":
		return fitToWidth(" "+mutedStyle.Render("waiting for a repository"), m.width)
	}
	left := " " + repositoryStyle.Render(folderGlyph+" "+sanitizeText(filepath.Base(m.repository.Dir))) + "  " +
		branchStyle.Render(branchGlyph+" "+m.branchLabel()) + m.renderAheadBehind()
	return fitToWidth(left, m.width)
}

func (m Model) renderAheadBehind() string {
	counts := []string{}
	if m.status.Ahead > 0 {
		counts = append(counts, " "+aheadStyle.Render("↑"+strconv.Itoa(m.status.Ahead)))
	}
	if m.status.Behind > 0 {
		counts = append(counts, " "+behindStyle.Render("↓"+strconv.Itoa(m.status.Behind)))
	}
	return strings.Join(counts, "")
}

func (m Model) renderLists() string {
	innerWidth := m.listOuterWidth() - 2
	if m.view == viewLog {
		return lipgloss.JoinVertical(lipgloss.Left,
			renderFoldedPane("changes", m.changesLabel(), innerWidth),
			renderPane(m.logTitle(), "", m.logLabel(), m.renderLogRows(innerWidth), innerWidth, m.listInnerHeight()))
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		renderPane("changes", "", m.changesLabel(), m.fileRows(innerWidth), innerWidth, m.listInnerHeight()),
		renderFoldedPane(m.logTitle(), m.logLabel(), innerWidth))
}

func (m Model) fileRows(innerWidth int) []string {
	if len(m.status.Files) == 0 && m.repository.Dir != "" {
		return []string{mutedStyle.Render("working tree clean")}
	}
	rows := []string{}
	end := min(m.listOffset+m.listInnerHeight(), len(m.status.Files))
	for index := m.listOffset; index < end; index++ {
		rows = append(rows, listRow(renderFileRow(m.status.Files[index]), index == m.selected, innerWidth))
	}
	return rows
}

func (m Model) renderLogRows(innerWidth int) []string {
	rows := m.visibleLogRows()
	switch {
	case m.repository.Dir != "" && !m.status.HasCommits:
		return []string{mutedStyle.Render("no commits yet")}
	case len(rows) == 0 && m.logFilter != nil:
		return []string{mutedStyle.Render("no commit matches")}
	}
	selected := m.selectedLogRow(rows)
	graph := m.logGraph()
	line := lineThrough(m.graph, m.selectedCommit)
	rendered := []string{}
	end := min(m.logOffset+m.listInnerHeight(), len(rows))
	for index := m.logOffset; index < end; index++ {
		rendered = append(rendered, listRow(m.renderLogRow(rows[index], graph[rows[index].commit], line, innerWidth-1),
			index == selected, innerWidth))
	}
	return rendered
}

// Blank while a search hides commits, since the lines between the shown ones would be wrong.
func (m Model) logGraph() []graphRow {
	if m.logFilter != nil {
		return make([]graphRow, len(m.commits))
	}
	return m.graph
}

// Hashes and parents are all commitGraph reads.
func sameTopology(first []gitrepo.Commit, second []gitrepo.Commit) bool {
	return slices.EqualFunc(first, second, func(a gitrepo.Commit, b gitrepo.Commit) bool {
		return a.Hash == b.Hash && slices.Equal(a.Parents, b.Parents)
	})
}

func (m Model) renderLogRow(row logRow, graph graphRow, line []bool, width int) string {
	commit := m.commits[row.commit]
	if row.file == commitRow {
		prefix := renderGraphCells(graph.commit, line) + expandMarker(m.expanded[commit.Hash])
		return prefix + renderCommitRow(commit, m.logFilter, m.logLoadedAt, width-ansi.StringWidth(prefix))
	}
	files := m.commitFiles[commit.Hash]
	below := renderGraphCells(graph.below, line)
	return below + renderTreeFileRow(files[row.file], row.file == len(files)-1, width-ansi.StringWidth(below))
}

// The selected row drops its own colors, since each styled part would end the row's background early.
func listRow(row string, selected bool, width int) string {
	if selected {
		return selectionBarStyle.Render("▌") + selectedRowStyle.Render(fitToWidth(ansi.Strip(row), width-1))
	}
	return " " + fitToWidth(row, width-1)
}

// A partly staged file counts in both.
func (m Model) changesLabel() string {
	if len(m.status.Files) == 0 {
		return ""
	}
	staged := 0
	unstaged := 0
	for _, file := range m.status.Files {
		if file.HasStagedChanges() {
			staged++
		}
		if file.HasUnstagedChanges() {
			unstaged++
		}
	}
	return strconv.Itoa(staged) + " staged · " + strconv.Itoa(unstaged) + " unstaged"
}

func (m Model) logLabel() string {
	matching := 0
	position := 0
	for index, commit := range m.commits {
		if !m.logFilter.matches(commit) {
			continue
		}
		matching++
		if index == m.selectedCommit {
			position = matching
		}
	}
	if matching == 0 {
		return ""
	}
	return strconv.Itoa(position) + " of " + strconv.Itoa(matching)
}

func renderFileRow(file gitrepo.FileStatus) string {
	path := sanitizeText(file.Path)
	if file.IsUntracked() {
		return untrackedStyle.Render("??") + " " + path
	}
	return stagedMarkStyle.Render(string(file.IndexStatus)) + unstagedMarkStyle.Render(string(file.WorktreeStatus)) +
		" " + path
}

func renderCommitRow(commit gitrepo.Commit, search searchTerms, now time.Time, width int) string {
	age := compactAge(now.Sub(commit.AuthoredAt))
	summaryWidth := max(width-len(age)-1, 0)
	summary := search.highlight(commit.ShortHash, hashStyle) + " "
	if len(commit.Refs) > 0 {
		refs := search.highlight(strings.Join(commit.Refs, ", "), refStyle)
		summary += refStyle.Render("(") + refs + refStyle.Render(")") + " "
	}
	summary += search.highlight(commit.Summary, textStyle)
	return fitToWidth(fitToWidth(summary, summaryWidth)+" "+mutedStyle.Render(age), width)
}

func (m Model) renderDiffPane() string {
	width := m.diffInnerWidth()
	height := m.paneInnerHeight()
	switch {
	case m.rejection != "":
		reason := errorStyle.Render(lipgloss.Wrap(m.rejection, width, ""))
		return renderPane("refused", "", "", strings.Split(reason, "\n"), width, height)
	case m.view == viewLog && len(m.commits) > 0:
		return renderPane(m.commitPaneTitle(), m.commitLineCounts(), m.scrollLabel(),
			strings.Split(m.diffView.View(), "\n"), width, height)
	case m.view == viewChanges && len(m.status.Files) > 0:
		return renderPane(sanitizeText(m.diffPath), m.fileLineCounts(), m.scrollLabel(),
			strings.Split(m.diffView.View(), "\n"), width, height)
	default:
		return renderPane("diff", "", "", []string{}, width, height)
	}
}

func (m Model) commitPaneTitle() string {
	commit := m.commits[m.selectedCommit]
	sections, loaded := m.commitFiles[commit.Hash]
	if loaded && m.selectedFile != commitRow {
		return sanitizeText(commit.ShortHash + " " + sections[m.selectedFile].Path)
	}
	return sanitizeText(commit.ShortHash + " " + commit.Summary)
}

func (m Model) fileLineCounts() string {
	if m.diffPath == "" {
		return ""
	}
	return renderLineCounts(diff.CountLines(m.fileDiff))
}

// The selected file's counts, or the whole commit's on its own row.
func (m Model) commitLineCounts() string {
	sections, loaded := m.commitFiles[m.commits[m.selectedCommit].Hash]
	if !loaded {
		return ""
	}
	if m.selectedFile != commitRow {
		return renderLineCounts(diff.CountLines(sections[m.selectedFile].FileDiff))
	}
	total := diff.LineCounts{}
	for _, section := range sections {
		counts := diff.CountLines(section.FileDiff)
		total.Added += counts.Added
		total.Removed += counts.Removed
	}
	return renderLineCounts(total)
}

func (m Model) scrollLabel() string {
	if m.diffView.TotalLineCount() <= m.diffView.Height() {
		return ""
	}
	return strconv.Itoa(int(m.diffView.ScrollPercent()*100)) + "%"
}

func (m Model) renderFooter() string {
	switch {
	case m.footerText != "" && m.footerIsError:
		return fitToWidth(errorStyle.Render("✗ "+m.footerText), m.width)
	case m.footerText != "":
		return fitToWidth(successStyle.Render("✓ "+m.footerText), m.width)
	case m.searching && m.searchError != "":
		return fitToWidth(" "+m.searchInput.View()+"  "+errorStyle.Render(m.searchError), m.width)
	case m.searching:
		return spaceBetween(" "+m.searchInput.View(), m.help.ShortHelpView(searchHelp)+" ", m.width)
	case m.view == viewLog:
		return m.renderHelpLine(logHelp)
	default:
		return m.renderHelpLine(changesHelp)
	}
}

func (m Model) renderHelpLine(bindings []key.Binding) string {
	return spaceBetween(" "+m.help.ShortHelpView(bindings), mutedStyle.Render(appName)+" ", m.width)
}

func (m Model) overlayPopup(screen string, popup string) string {
	x := max((m.width-lipgloss.Width(popup))/2, 0)
	y := max((m.height-lipgloss.Height(popup))/2, 0)
	return lipgloss.NewCompositor(lipgloss.NewLayer(screen), lipgloss.NewLayer(popup).X(x).Y(y).Z(1)).Render()
}

func (m Model) renderCommitPopup() string {
	return m.renderPopup([]string{
		titleStyle.Render("commit to " + m.branchLabel()),
		renderField(m.summaryInput.View(), !m.descriptionFocused, m.fieldInnerWidth(), 1),
		renderField(m.descriptionBox.View(), m.descriptionFocused, m.fieldInnerWidth(), descriptionHeight),
	}, "committing...", commitHelp)
}

func (m Model) renderPushPopup() string {
	return m.renderPopup([]string{
		titleStyle.Render("push " + m.branchLabel() + " to " + sanitizeText(m.status.Upstream)),
		textStyle.Render(commitCount(m.status.Ahead)),
	}, "pushing...", pushHelp)
}

func (m Model) renderPopup(lines []string, runningText string, bindings []key.Binding) string {
	width := m.popupInnerWidth()
	var status string
	switch {
	case m.popupRunning:
		status = mutedStyle.Render(runningText)
	case m.popupError != "":
		wrapped := strings.Split(lipgloss.Wrap(m.popupError, width, ""), "\n")
		status = errorStyle.Render(strings.Join(wrapped[:min(len(wrapped), maximumErrorLines)], "\n"))
	default:
		status = m.help.ShortHelpView(bindings)
	}
	return popupStyle.Width(width + 4).Render(strings.Join(lines, "\n") + "\n\n" + status)
}

func commitCount(count int) string {
	if count == 1 {
		return "1 commit"
	}
	return strconv.Itoa(count) + " commits"
}

func renderField(view string, focused bool, innerWidth int, innerHeight int) string {
	body := strings.Join(fitLines(strings.Split(view, "\n"), innerWidth, innerHeight), "\n")
	if focused {
		return focusedFieldStyle.Render(body)
	}
	return fieldStyle.Render(body)
}
