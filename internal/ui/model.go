package ui

import (
	"maps"
	"path/filepath"
	"regexp"
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
	headerHeight         = 1
	footerHeight         = 1
	minimumListWidth     = 20
	maximumListPercent   = 25
	logListPercent       = 40
	wheelScrollLines     = 3
	maximumPopupWidth    = 72
	descriptionHeight    = 5
	maximumErrorLines    = 12
	branchGlyph          = ""
	detachedBranchLabel  = "detached HEAD"
	emptySummaryMessage  = "the summary is required"
	commitRunningMessage = "committing..."
	waitingMessage       = "waiting for a repository"
	refusedMessage       = "refused the focused pane's directory"
)

// listFirstRowY is the screen row of the first list entry: below the header and the top border.
const listFirstRowY = headerHeight + 1

// commitRow is the selectedFile of a commit row in the log, as opposed to one of its files.
const commitRow = -1

type viewKind int

const (
	viewChanges viewKind = iota
	viewLog
)

type Model struct {
	repository gitrepo.Repository
	// rejection says why no repository is on screen, empty while one is or before the first arrives.
	rejection  string
	view       viewKind
	status     gitrepo.Status
	selected   int
	listOffset int
	diffPath   string
	fileDiff   diff.FileDiff
	commits    []gitrepo.Commit
	// logLoadedAt is when commits was read, the reference for their ages.
	logLoadedAt    time.Time
	selectedCommit int
	selectedFile   int
	// logOffset counts rows of the log tree, commit and file rows alike.
	logOffset int
	// commitFiles holds the parsed diff of every commit loaded so far, by hash, and expanded the commits whose files
	// the log tree lists.
	commitFiles map[string][]diff.FileSection
	expanded    map[string]bool
	// logFilter is nil while the log shows every commit.
	logFilter      *regexp.Regexp
	searching      bool
	searchInput    textinput.Model
	searchError    string
	diffView       viewport.Model
	help           help.Model
	summaryInput   textinput.Model
	descriptionBox textarea.Model
	// committing is true while the commit popup is open, commitRunning while git commit runs.
	committing         bool
	descriptionFocused bool
	commitRunning      bool
	commitError        string
	footerText         string
	footerIsError      bool
	width              int
	height             int
}

// New starts with no repository. A RepositoryFocused message picks the one to review.
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
	return nil
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
		return m.withRepository(gitrepo.Repository{}, sanitizeText(singleLine(msg.Reason))), nil
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
		if msg.dir != m.repository.Dir {
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
			m.footerText = sanitizeText("committed in " + msg.dir + ": " + msg.summary)
			m.footerIsError = false
			return m, nil
		}
		m = m.withCommitPopupClosed()
		m.summaryInput.Reset()
		m.descriptionBox.Reset()
		m.footerText = sanitizeText(msg.summary)
		m.footerIsError = false
		return m, repositoryRequest{repository: m.repository}.loadStatus
	case commitFailedMsg:
		if msg.dir != m.repository.Dir {
			m.footerText = sanitizeText(singleLine(msg.err.Error()))
			m.footerIsError = true
			return m, nil
		}
		m.commitRunning = false
		m.commitError = sanitizeLines(strings.TrimSpace(msg.err.Error()))
		return m, nil
	case gitFailedMsg:
		m.footerText = sanitizeText(singleLine(msg.err.Error()))
		m.footerIsError = true
		return m, nil
	case tea.KeyPressMsg:
		if key.Matches(msg, forceQuitKey) {
			return m, tea.Quit
		}
		if m.committing {
			return m.updateCommitBox(msg)
		}
		if m.searching {
			return m.updateSearch(msg)
		}
		return m.updateBrowsing(msg)
	case tea.MouseClickMsg:
		if m.committing || msg.Button != tea.MouseLeft {
			return m, nil
		}
		return m.clickList(msg.Mouse())
	case tea.MouseWheelMsg:
		if m.committing {
			return m, nil
		}
		return m.scrollWheel(msg.Mouse())
	}
	if m.committing {
		return m.updateFocusedField(msg)
	}
	if m.searching {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// withStatus shows status and loads the diff of the selected file, and the log when the branch has commits.
func (m Model) withStatus(status gitrepo.Status) (tea.Model, tea.Cmd) {
	m.status = status
	m.selected = min(m.selected, max(len(m.status.Files)-1, 0))
	if !status.HasCommits {
		m.commits = []gitrepo.Commit{}
		m.selectedFile = commitRow
		return m.withLayout(), m.loadSelectedDiff()
	}
	return m.withLayout(), tea.Batch(m.loadSelectedDiff(), repositoryRequest{repository: m.repository}.loadLog)
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
	case key.Matches(msg, expandKey) && m.view == viewLog:
		return m.toggleExpanded()
	case key.Matches(msg, searchKey) && m.view == viewLog:
		m.searching = true
		m.searchInput.CursorEnd()
		return m, m.searchInput.Focus()
	case key.Matches(msg, clearSearchKey) && m.view == viewLog && m.logFilter != nil:
		m.searchInput.Reset()
		return m.withLogFilter(nil)
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
		m.committing = true
		m.descriptionFocused = false
		return m, m.summaryInput.Focus()
	case key.Matches(msg, refreshKey):
		if m.repository.Dir == "" {
			return m, nil
		}
		return m, repositoryRequest{repository: m.repository}.loadStatus
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

// withRepository shows repository, or rejection when no repository is allowed, and drops everything shown for the
// previous one, including a commit message being written for it.
func (m Model) withRepository(repository gitrepo.Repository, rejection string) Model {
	m = m.withCommitPopupClosed()
	m.summaryInput.Reset()
	m.descriptionBox.Reset()
	m.repository = repository
	m.rejection = rejection
	m.status = gitrepo.Status{Files: []gitrepo.FileStatus{}}
	m.selected = 0
	m.listOffset = 0
	m.diffPath = ""
	m.fileDiff = diff.FileDiff{}
	m.commits = []gitrepo.Commit{}
	m.selectedCommit = 0
	m.selectedFile = commitRow
	m.logOffset = 0
	m.commitFiles = map[string][]diff.FileSection{}
	m.expanded = map[string]bool{}
	m.logFilter = nil
	m.searching = false
	m.searchInput.Blur()
	m.searchInput.Reset()
	m.searchError = ""
	m.diffView.SetYOffset(0)
	m.footerText = ""
	m.footerIsError = false
	return m.withLayout()
}

func (m Model) updateCommitBox(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.commitRunning {
		return m, nil
	}
	switch {
	case key.Matches(msg, cancelKey):
		return m.withCommitPopupClosed(), nil
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
		m.commitError = emptySummaryMessage
		return m, nil
	}
	m.commitRunning = true
	m.commitError = ""
	message := joinCommitMessage(summary, m.descriptionBox.Value())
	return m, commitRequest{repository: m.repository, message: message}.commit
}

// joinCommitMessage puts the description one blank line below the summary, the shape git log and GitHub expect.
func joinCommitMessage(summary string, description string) string {
	trimmedDescription := strings.TrimSpace(description)
	if trimmedDescription == "" {
		return summary
	}
	return summary + "\n\n" + trimmedDescription
}

func (m Model) withCommitPopupClosed() Model {
	m.committing = false
	m.commitRunning = false
	m.commitError = ""
	m.summaryInput.Blur()
	m.descriptionBox.Blur()
	return m
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

// selectLogRow selects the commit or file at index of the log tree, and loads the commit's diff unless it is loaded.
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
	m = m.withListScrolled().withDiffContent()
	return m, m.loadSelectedCommitDiff()
}

// withCommits shows a newly read log. The selection stays on the same commit when new ones arrive above it.
func (m Model) withCommits(commits []gitrepo.Commit, loadedAt time.Time) (tea.Model, tea.Cmd) {
	index, found := commitIndex(commits, m.selectedCommitHash())
	if !found {
		m.selectedFile = commitRow
	}
	m.commits = commits
	m.logLoadedAt = loadedAt
	m.selectedCommit = index
	m = m.withLayout()
	return m, m.loadSelectedCommitDiff()
}

// toggleExpanded lists or hides the files of the selected commit. Collapsing from one of its files selects the commit.
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

// updateSearch filters the log as the pattern is typed. A pattern that does not compile leaves the last filter on.
func (m Model) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keepSearchKey):
		m.searching = false
		m.searchError = ""
		m.searchInput.Blur()
		return m, nil
	case key.Matches(msg, clearSearchKey):
		m.searching = false
		m.searchError = ""
		m.searchInput.Blur()
		m.searchInput.Reset()
		return m.withLogFilter(nil)
	}
	var inputCmd tea.Cmd
	m.searchInput, inputCmd = m.searchInput.Update(msg)
	filter, err := compileSearch(m.searchInput.Value())
	if err != nil {
		m.searchError = sanitizeText(singleLine(err.Error()))
		return m, inputCmd
	}
	m.searchError = ""
	next, loadCmd := m.withLogFilter(filter)
	return next, tea.Batch(inputCmd, loadCmd)
}

// compileSearch compiles pattern as a Go regular expression, matched case-insensitively unless it holds an uppercase
// letter, the way ripgrep --smart-case does. An empty pattern is no filter.
func compileSearch(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	if strings.ToLower(pattern) == pattern {
		return regexp.Compile("(?i)" + pattern)
	}
	return regexp.Compile(pattern)
}

// withLogFilter shows only the commits whose message filter matches, every commit when it is nil. A selection the
// filter hides moves to the first row left.
func (m Model) withLogFilter(filter *regexp.Regexp) (tea.Model, tea.Cmd) {
	m.logFilter = filter
	rows := m.visibleLogRows()
	if m.selectedLogRow(rows) < 0 && len(rows) > 0 {
		m.selectedCommit = rows[0].commit
		m.selectedFile = rows[0].file
		m.diffView.SetYOffset(0)
	}
	m = m.withListScrolled().withDiffContent()
	return m, m.loadSelectedCommitDiff()
}

// logRow is one row of the log tree: a commit, or one of its files under it when it is expanded. file indexes the
// commit's commitFiles.
type logRow struct {
	commit int
	file   int
}

// visibleLogRows lists the commits the filter lets through, each followed by its files when it is expanded.
func (m Model) visibleLogRows() []logRow {
	rows := []logRow{}
	for index, commit := range m.commits {
		if !m.commitMatches(commit) {
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

// selectedLogRow is the index of the selection in rows, -1 when the filter hides it.
func (m Model) selectedLogRow(rows []logRow) int {
	for index, row := range rows {
		if row.commit == m.selectedCommit && row.file == m.selectedFile {
			return index
		}
	}
	return -1
}

func (m Model) commitMatches(commit gitrepo.Commit) bool {
	return m.logFilter == nil || m.logFilter.MatchString(commit.Message)
}

func (m Model) clickList(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	row := mouse.Y - listFirstRowY
	if mouse.X >= m.listOuterWidth() || row < 0 || row >= m.paneInnerHeight() {
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

// scrollWheel moves the selection over the list and scrolls the diff over the diff pane.
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

// loadSelectedCommitDiff loads the diff of the selected commit while the log is on screen, unless it is loaded.
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

func commitIndex(commits []gitrepo.Commit, hash string) (int, bool) {
	for index, commit := range commits {
		if commit.Hash == hash {
			return index, true
		}
	}
	return 0, false
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
	if m.view == viewLog {
		m.logOffset = scrolledOffset(m.logOffset, max(m.selectedLogRow(m.visibleLogRows()), 0), m.paneInnerHeight())
	} else {
		m.listOffset = scrolledOffset(m.listOffset, m.selected, m.paneInnerHeight())
	}
	return m
}

// scrolledOffset returns the first visible row that keeps selected within visibleRows rows of it.
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

// listOuterWidth fits the longest file row within a minimum width and a share of the screen. The log takes a fixed,
// wider share, since its rows are commit summaries.
func (m Model) listOuterWidth() int {
	if m.view == viewLog {
		return max(m.width*logListPercent/100, minimumListWidth)
	}
	longestRow := lipgloss.Width(m.listTitle()) + 4
	for _, file := range m.status.Files {
		longestRow = max(longestRow, ansi.StringWidth(renderFileRow(file)))
	}
	return min(max(longestRow+3, minimumListWidth), m.width*maximumListPercent/100)
}

func (m Model) listTitle() string {
	switch {
	case m.view == viewLog && m.logFilter != nil:
		return "log /" + sanitizeText(m.searchInput.Value())
	case m.view == viewLog:
		return "log"
	default:
		return "changes"
	}
}

func (m Model) diffInnerWidth() int {
	return max(m.width-m.listOuterWidth()-2, 0)
}

func (m Model) paneInnerHeight() int {
	return max(m.height-headerHeight-footerHeight-2, 0)
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
	case m.committing:
		view = tea.NewView(m.overlayCommitPopup(m.renderScreen()))
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
		return "perusal"
	}
	return "perusal: " + sanitizeText(filepath.Base(m.repository.Dir))
}

func (m Model) renderScreen() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		lipgloss.JoinHorizontal(lipgloss.Top, m.renderList(), m.renderDiffPane()),
		m.renderFooter(),
	)
}

func (m Model) branchLabel() string {
	if m.status.Branch == "" {
		return detachedBranchLabel
	}
	return sanitizeText(m.status.Branch)
}

func (m Model) renderHeader() string {
	appName := appNameStyle.Render("perusal") + " "
	switch {
	case m.rejection != "":
		return fitToWidth(appName+errorStyle.Render(refusedMessage), m.width)
	case m.repository.Dir == "":
		return fitToWidth(appName+mutedStyle.Render(waitingMessage), m.width)
	}
	left := appName + repositoryStyle.Render(sanitizeText(filepath.Base(m.repository.Dir))) + "  " +
		branchStyle.Render(branchGlyph+" "+m.branchLabel()) + m.renderAheadBehind()
	return spaceBetween(left, m.renderTabs(), m.width)
}

// renderAheadBehind shows how far the branch has moved from its upstream, nothing when it tracks none or matches it.
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

func (m Model) renderTabs() string {
	if m.view == viewLog {
		return inactiveTabStyle.Render("changes") + activeTabStyle.Render("log")
	}
	return activeTabStyle.Render("changes") + inactiveTabStyle.Render("log")
}

func (m Model) renderList() string {
	innerWidth := m.listOuterWidth() - 2
	if m.view == viewLog {
		return renderPane(m.listTitle(), m.logLabel(), m.renderLogRows(innerWidth), innerWidth, m.paneInnerHeight())
	}
	return renderPane(m.listTitle(), m.changesLabel(), m.fileRows(innerWidth), innerWidth, m.paneInnerHeight())
}

func (m Model) fileRows(innerWidth int) []string {
	if len(m.status.Files) == 0 && m.repository.Dir != "" {
		return []string{mutedStyle.Render("working tree clean")}
	}
	rows := []string{}
	end := min(m.listOffset+m.paneInnerHeight(), len(m.status.Files))
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
		return []string{mutedStyle.Render("no commit message matches")}
	}
	selected := m.selectedLogRow(rows)
	rendered := []string{}
	end := min(m.logOffset+m.paneInnerHeight(), len(rows))
	for index := m.logOffset; index < end; index++ {
		rendered = append(rendered, listRow(m.renderLogRow(rows[index], innerWidth-1), index == selected, innerWidth))
	}
	return rendered
}

func (m Model) renderLogRow(row logRow, width int) string {
	commit := m.commits[row.commit]
	if row.file == commitRow {
		return expandMarker(m.expanded[commit.Hash]) + renderCommitRow(commit, m.logLoadedAt, width-2)
	}
	files := m.commitFiles[commit.Hash]
	return renderTreeFileRow(files[row.file], row.file == len(files)-1, width)
}

// listRow fits row to width behind a one-cell gutter, which holds a bar on the selected row. The selected row drops
// its own colors, since each styled part would end the row's background early.
func listRow(row string, selected bool, width int) string {
	if selected {
		return selectionBarStyle.Render("▌") + selectedRowStyle.Render(fitToWidth(ansi.Strip(row), width-1))
	}
	return " " + fitToWidth(row, width-1)
}

// changesLabel counts the files with staged and with unstaged changes. A partly staged file counts in both.
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

// logLabel places the selected commit among the commits the filter lets through.
func (m Model) logLabel() string {
	matching := 0
	position := 0
	for index, commit := range m.commits {
		if !m.commitMatches(commit) {
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

func renderCommitRow(commit gitrepo.Commit, now time.Time, width int) string {
	age := compactAge(now.Sub(commit.AuthoredAt))
	summaryWidth := max(width-len(age)-1, 0)
	summary := hashStyle.Render(sanitizeText(commit.ShortHash)) + " " + textStyle.Render(sanitizeText(commit.Summary))
	return fitToWidth(fitToWidth(summary, summaryWidth)+" "+mutedStyle.Render(age), width)
}

func (m Model) renderDiffPane() string {
	width := m.diffInnerWidth()
	height := m.paneInnerHeight()
	switch {
	case m.rejection != "":
		reason := errorStyle.Render(lipgloss.Wrap(m.rejection, width, ""))
		return renderPane("refused", "", strings.Split(reason, "\n"), width, height)
	case m.view == viewLog && len(m.commits) > 0:
		return renderPane(m.commitPaneTitle(), m.scrollLabel(), strings.Split(m.diffView.View(), "\n"), width, height)
	case m.view == viewChanges && len(m.status.Files) > 0:
		return renderPane(sanitizeText(m.diffPath), m.scrollLabel(), strings.Split(m.diffView.View(), "\n"), width,
			height)
	default:
		return renderPane("diff", "", []string{}, width, height)
	}
}

// commitPaneTitle names the selected file of the log tree, or the selected commit when no file is.
func (m Model) commitPaneTitle() string {
	commit := m.commits[m.selectedCommit]
	sections, loaded := m.commitFiles[commit.Hash]
	if loaded && m.selectedFile != commitRow {
		return sanitizeText(commit.ShortHash + " " + sections[m.selectedFile].Path)
	}
	return sanitizeText(commit.ShortHash + " " + commit.Summary)
}

// scrollLabel shows how far down the diff is scrolled, nothing when it fits.
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
		return fitToWidth(" "+m.help.ShortHelpView(logHelp), m.width)
	default:
		return fitToWidth(" "+m.help.ShortHelpView(changesHelp), m.width)
	}
}

func (m Model) overlayCommitPopup(screen string) string {
	popup := m.renderCommitPopup()
	x := max((m.width-lipgloss.Width(popup))/2, 0)
	y := max((m.height-lipgloss.Height(popup))/2, 0)
	return lipgloss.NewCompositor(lipgloss.NewLayer(screen), lipgloss.NewLayer(popup).X(x).Y(y).Z(1)).Render()
}

func (m Model) renderCommitPopup() string {
	width := m.popupInnerWidth()
	lines := []string{
		titleStyle.Render("commit to " + m.branchLabel()),
		renderField(m.summaryInput.View(), !m.descriptionFocused, m.fieldInnerWidth(), 1),
		renderField(m.descriptionBox.View(), m.descriptionFocused, m.fieldInnerWidth(), descriptionHeight),
		"",
	}
	switch {
	case m.commitRunning:
		lines = append(lines, mutedStyle.Render(commitRunningMessage))
	case m.commitError != "":
		wrapped := strings.Split(lipgloss.Wrap(m.commitError, width, ""), "\n")
		lines = append(lines, errorStyle.Render(strings.Join(wrapped[:min(len(wrapped), maximumErrorLines)], "\n")))
	default:
		lines = append(lines, m.help.ShortHelpView(commitHelp))
	}
	return popupStyle.Width(width + 4).Render(strings.Join(lines, "\n"))
}

func renderField(view string, focused bool, innerWidth int, innerHeight int) string {
	body := strings.Join(fitLines(strings.Split(view, "\n"), innerWidth, innerHeight), "\n")
	if focused {
		return focusedFieldStyle.Render(body)
	}
	return fieldStyle.Render(body)
}
