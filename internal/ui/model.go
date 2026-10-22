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
	"github.com/CLBRITTON2/perusal/internal/focus"
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

type screenMode int

const (
	modeBrowsing screenMode = iota
	modeSearching
	modeOverview
	modeBranches
)

type popupKind int

const (
	noPopup popupKind = iota
	commitPopup
	confirmPopup
)

// confirmation is a confirm popup's text and the git run its confirmKey starts, both fixed when the popup opens.
type confirmation struct {
	lines       []string
	confirmKey  key.Binding
	run         tea.Cmd
	runningText string
}

type popupState struct {
	kind         popupKind
	confirmation confirmation
	running      bool
	err          string
}

// display holds what survives a repository switch.
type display struct {
	view           viewKind
	fullscreenDiff bool
	wholeFile      bool
	unified        bool
	width          int
	height         int
}

type footer struct {
	text    string
	isError bool
}

type commitDiff struct {
	sections     []diff.FileSection
	contextLines int
}

type Model struct {
	display
	findRepositories RepositoryFinder
	mode             screenMode
	popup            popupState
	footer           footer

	overview selectableList[overviewRow]
	// pulling holds the Dir of each repository an overview pull is running in.
	pulling  map[string]bool
	branches branchList

	repository gitrepo.Repository
	// focused lists the repositories the focus named.
	focused   []gitrepo.Repository
	rejection string

	status           gitrepo.Status
	stagedLineCounts diff.LineCounts
	selectedChange   int
	changesOffset    int
	diffPath         string
	fileDiff         diff.FileDiff

	commits            []gitrepo.Commit
	graph              []graphRow
	logLimit           int
	logLoadedAt        time.Time
	selectedCommit     int
	selectedCommitFile int
	// logOffset counts rows of the log tree, commit and file rows alike.
	logOffset   int
	commitDiffs map[string]commitDiff
	expanded    map[string]bool
	logFilter   searchTerms
	searchInput textinput.Model
	searchError string

	diffView           viewport.Model
	help               help.Model
	summaryInput       textinput.Model
	descriptionBox     textarea.Model
	editingDescription bool
}

func New(findRepositories RepositoryFinder) Model {
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
		findRepositories:   findRepositories,
		overview:           selectableList[overviewRow]{rows: []overviewRow{}},
		pulling:            map[string]bool{},
		branches:           branchList{selectableList: selectableList[branchRow]{rows: []branchRow{}}},
		focused:            []gitrepo.Repository{},
		status:             gitrepo.Status{Files: []gitrepo.FileStatus{}},
		commits:            []gitrepo.Commit{},
		graph:              []graphRow{},
		logLimit:           logPageSize,
		selectedCommitFile: commitRow,
		commitDiffs:        map[string]commitDiff{},
		expanded:           map[string]bool{},
		searchInput:        searchInput,
		diffView:           viewport.New(),
		help:               helpView,
		summaryInput:       summaryInput,
		descriptionBox:     descriptionBox,
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
	case focus.Focus:
		return m.withFocus(msg)
	case FocusRejection:
		return m.withRepository(gitrepo.Repository{}, sanitizeSingleLine(msg.Reason)), nil
	case pollTickMsg:
		if m.repository.Dir == "" {
			return m, tea.Sequence(m.loadUncommitted(), nextPoll())
		}
		// The next tick waits for the status, so a slow git never stacks up polls.
		return m, tea.Sequence(m.reloadStatus(), nextPoll())
	case uncommittedMsg:
		if !slices.Equal(msg.focused, m.focused) {
			return m, nil
		}
		if m.repository.Dir != "" {
			return m, nil
		}
		return m.withFocusedRepository(m.firstToReview(msg.uncommitted))
	case statusMsg:
		if msg.dir != m.repository.Dir {
			return m, nil
		}
		m.stagedLineCounts = msg.stagedLineCounts
		return m.withStatus(msg.status)
	case diffMsg:
		if msg.dir != m.repository.Dir || msg.path != m.selectedPath() || msg.contextLines != m.contextLines() {
			return m, nil
		}
		if msg.path != m.diffPath && m.view == viewChanges {
			m.diffView.SetYOffset(0)
			m.diffPath = msg.path
			m.fileDiff = msg.fileDiff
			return m.withDiffContent(), nil
		}
		previous := m.diffLines()
		m.diffPath = msg.path
		m.fileDiff = msg.fileDiff
		return m.withDiffKeptInPlace(previous), nil
	case logMsg:
		if msg.dir != m.repository.Dir || msg.limit != m.logLimit {
			return m, nil
		}
		return m.withCommits(msg.commits, msg.loadedAt)
	case commitDiffMsg:
		if msg.dir != m.repository.Dir || msg.contextLines != m.contextLines() {
			return m, nil
		}
		previous := m.diffLines()
		m.commitDiffs = maps.Clone(m.commitDiffs)
		m.commitDiffs[msg.hash] = commitDiff{sections: msg.sections, contextLines: msg.contextLines}
		return m.withDiffKeptInPlace(previous).withListScrolled(), nil
	case indexChangeMsg:
		if msg.dir != m.repository.Dir {
			return m, nil
		}
		return m, m.reloadStatus()
	case commitMsg:
		if msg.dir != m.repository.Dir {
			return m.withFooterNotice("committed in " + msg.dir + ": " + msg.summary), nil
		}
		m = m.withPopupClosed().withFooterNotice(msg.summary)
		m.summaryInput.Reset()
		m.descriptionBox.Reset()
		return m, m.reloadStatus()
	case popupSuccessMsg:
		if msg.dir != m.repository.Dir {
			return m.withFooterNotice(msg.notice + " in " + msg.dir), nil
		}
		m = m.withPopupClosed().withFooterNotice(msg.notice)
		return m, tea.Batch(m.reloadStatus(), m.reloadBranches())
	case branchesMsg:
		if msg.dir != m.repository.Dir || m.mode != modeBranches {
			return m, nil
		}
		return m.withBranchesLoaded(msg), nil
	case branchesFailureMsg:
		if msg.dir == m.repository.Dir {
			m = m.withModeClosed(modeBranches)
		}
		return m.withFooterError(msg.err.Error()), nil
	case branchSwitchMsg:
		if msg.dir != m.repository.Dir {
			return m.withFooterNotice("switched " + msg.dir + " to " + msg.branch), nil
		}
		m = m.withModeClosed(modeBranches).withFooterNotice("switched to " + msg.branch)
		return m, m.reloadStatus()
	case pullMsg:
		return m.withOverviewPulled(msg)
	case syncFailureMsg:
		m.pulling = withoutDir(m.pulling, msg.dir)
		return m.withFooterError(msg.err.Error()), nil
	case popupFailureMsg:
		if msg.dir != m.repository.Dir || m.popup.kind == noPopup {
			return m.withFooterError(msg.err.Error()), nil
		}
		m.popup.running = false
		m.popup.err = sanitizeLines(msg.err.Error())
		return m, nil
	case overviewMsg:
		if m.mode != modeOverview {
			return m, nil
		}
		return m.withOverviewLoaded(msg), nil
	case overviewFailureMsg:
		return m.withModeClosed(modeOverview).withFooterError(msg.err.Error()), nil
	case gitFailureMsg:
		return m.withFooterError(msg.err.Error()), nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, forceQuitKey):
			return m, tea.Quit
		case m.popup.kind == commitPopup:
			return m.updateCommitBox(msg)
		case m.popup.kind == confirmPopup:
			return m.updateConfirmPopup(msg)
		case m.mode == modeOverview:
			return m.updateOverview(msg)
		case m.mode == modeBranches:
			return m.updateBranches(msg)
		case m.mode == modeSearching:
			return m.updateSearch(msg)
		}
		return m.updateBrowsing(msg)
	case tea.MouseClickMsg:
		switch {
		case !m.acceptsMouse():
			return m, nil
		case msg.Button == tea.MouseLeft:
			return m.clickList(msg.Mouse())
		case msg.Button == tea.MouseRight:
			return m.rightClickList(msg.Mouse())
		}
		return m, nil
	case tea.MouseWheelMsg:
		if !m.acceptsMouse() {
			return m, nil
		}
		return m.scrollWheel(msg.Mouse())
	}
	if m.popup.kind == commitPopup {
		return m.updateFocusedField(msg)
	}
	if m.mode == modeSearching {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// The lists and the diff take the mouse only while they are on screen with no popup over them.
func (m Model) acceptsMouse() bool {
	return m.popup.kind == noPopup && (m.mode == modeBrowsing || m.mode == modeSearching)
}

// A late result closes only the pane it was for, never one opened since.
func (m Model) withModeClosed(mode screenMode) Model {
	if m.mode == mode {
		m.mode = modeBrowsing
	}
	return m
}

func (m Model) reloadStatus() tea.Cmd {
	return repositoryRequest{repository: m.repository}.loadStatus
}

func (m Model) reloadLog() tea.Cmd {
	return logRequest{repository: m.repository, limit: m.logLimit}.load
}

func (m Model) withStatus(status gitrepo.Status) (tea.Model, tea.Cmd) {
	m.status = status
	m.selectedChange = min(m.selectedChange, max(len(m.status.Files)-1, 0))
	if !status.HasCommits {
		m.commits = []gitrepo.Commit{}
		m.graph = []graphRow{}
		m.selectedCommitFile = commitRow
		return m.withLayout(), m.loadSelectedDiff()
	}
	return m.withLayout(), tea.Batch(m.loadSelectedDiff(), m.reloadLog())
}

func (m Model) updateBrowsing(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footer = footer{}
	onFile := m.view == viewChanges && len(m.status.Files) > 0
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
	case key.Matches(msg, fullscreenKey):
		m.fullscreenDiff = !m.fullscreenDiff
		return m.withLayout(), nil
	case key.Matches(msg, wholeFileKey):
		m.wholeFile = !m.wholeFile
		return m, tea.Batch(m.loadSelectedDiff(), m.loadSelectedCommitDiff())
	case key.Matches(msg, unifiedKey):
		m.unified = !m.unified
		return m.withDiffContent(), nil
	case key.Matches(msg, expandKey) && m.view == viewLog:
		return m.toggleExpanded()
	case key.Matches(msg, searchKey) && m.view == viewLog:
		m.mode = modeSearching
		m.searchInput.CursorEnd()
		return m, m.searchInput.Focus()
	case key.Matches(msg, clearSearchKey) && m.view == viewLog && m.logFilter != nil:
		return m.clearSearch()
	case key.Matches(msg, stageKey) && onFile:
		request := fileRequest{repository: m.repository, file: m.status.Files[m.selectedChange]}
		if request.file.HasUnstagedChanges() {
			return m, request.stage
		}
		return m, request.unstage
	case key.Matches(msg, stageAllKey) && onFile:
		request := repositoryRequest{repository: m.repository}
		if slices.ContainsFunc(m.status.Files, gitrepo.FileStatus.HasUnstagedChanges) {
			return m, request.stageAll
		}
		return m, request.unstageAll
	case key.Matches(msg, discardKey) && onFile:
		file := m.status.Files[m.selectedChange]
		return m.withConfirmation(discardConfirmation(sanitizeText(file.Path), discardEffect(file),
			fileRequest{repository: m.repository, file: file}.discard)), nil
	case key.Matches(msg, discardAllKey) && onFile:
		return m.withConfirmation(discardConfirmation(pluralize(len(m.status.Files), "file"),
			"deletes untracked files and puts the rest back to HEAD",
			repositoryRequest{repository: m.repository}.discardAll)), nil
	case key.Matches(msg, commitKey):
		if m.repository.Dir == "" {
			return m, nil
		}
		m.popup = popupState{kind: commitPopup}
		m.editingDescription = false
		return m, m.summaryInput.Focus()
	case key.Matches(msg, pushKey):
		return m.openPushPopup()
	case key.Matches(msg, amendKey) && m.view == viewLog:
		return m.openAmendPopup(), nil
	case key.Matches(msg, branchesKey):
		return m.openBranches()
	case key.Matches(msg, overviewKey):
		return m.openOverview()
	}
	return m, nil
}

func (m Model) switchView() (tea.Model, tea.Cmd) {
	m = m.withSearchClosed()
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
	next := New(m.findRepositories)
	next.display = m.display
	next.mode = modeAfterSwitch(m.mode)
	next.overview = m.overview
	next.pulling = m.pulling
	next.repository = repository
	next.rejection = rejection
	return next.withLayout()
}

// Only the overview spans repositories, so it alone stays open across a switch.
func modeAfterSwitch(mode screenMode) screenMode {
	if mode == modeOverview {
		return modeOverview
	}
	return modeBrowsing
}

// A focus that still names the repository under review keeps it, so a new edit elsewhere never pulls the review away.
func (m Model) withFocus(msg focus.Focus) (tea.Model, tea.Cmd) {
	if m.rejection == "" && slices.Contains(msg.Repositories, m.repository) {
		m.focused = msg.Repositories
		return m.withRefusals(msg.Refusals), nil
	}
	next := m.withRepository(gitrepo.Repository{}, "")
	next.focused = msg.Repositories
	if len(next.focused) == 1 {
		return next.withRefusals(msg.Refusals).withFocusedRepository(next.focused[0])
	}
	return next.withRefusals(msg.Refusals), next.loadUncommitted()
}

func (m Model) withRefusals(refusals []error) Model {
	if len(refusals) == 0 {
		return m
	}
	reasons := []string{}
	for _, refusal := range refusals {
		reasons = append(reasons, refusal.Error())
	}
	return m.withFooterError("refused " + strings.Join(reasons, ", "))
}

// Keeps the focus and the footer, which may hold refusals of the focus that is still current.
func (m Model) withFocusedRepository(repository gitrepo.Repository) (tea.Model, tea.Cmd) {
	next := m.withRepository(repository, "")
	next.focused = m.focused
	next.footer = m.footer
	return next, next.reloadStatus()
}

func (m Model) loadUncommitted() tea.Cmd {
	if len(m.focused) < 2 {
		return nil
	}
	return focusedRequest{repositories: m.focused}.loadUncommitted
}

// uncommitted maps each Dir to whether it has changes.
func (m Model) firstToReview(uncommitted map[string]bool) gitrepo.Repository {
	for _, repository := range m.focused {
		if uncommitted[repository.Dir] {
			return repository
		}
	}
	return m.focused[0]
}

func (m Model) withFooterNotice(text string) Model {
	m.footer = footer{text: sanitizeSingleLine(text), isError: false}
	return m
}

func (m Model) withFooterError(text string) Model {
	m.footer = footer{text: sanitizeSingleLine(text), isError: true}
	return m
}

func (m Model) updateCommitBox(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.popup.running {
		return m, nil
	}
	switch {
	case key.Matches(msg, cancelKey):
		return m.withPopupClosed(), nil
	case key.Matches(msg, nextFieldKey):
		return m.toggleCommitField()
	case key.Matches(msg, submitKey), key.Matches(msg, enterKey) && !m.editingDescription:
		return m.startCommit()
	}
	return m.updateFocusedField(msg)
}

func (m Model) updateFocusedField(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.editingDescription {
		m.descriptionBox, cmd = m.descriptionBox.Update(msg)
	} else {
		m.summaryInput, cmd = m.summaryInput.Update(msg)
	}
	return m, cmd
}

func (m Model) toggleCommitField() (tea.Model, tea.Cmd) {
	m.editingDescription = !m.editingDescription
	if m.editingDescription {
		m.summaryInput.Blur()
		return m, m.descriptionBox.Focus()
	}
	m.descriptionBox.Blur()
	return m, m.summaryInput.Focus()
}

func (m Model) startCommit() (tea.Model, tea.Cmd) {
	summary := strings.TrimSpace(m.summaryInput.Value())
	if summary == "" {
		m.popup.err = "the summary is required"
		return m, nil
	}
	m.popup.running = true
	m.popup.err = ""
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
	m.popup = popupState{}
	m.summaryInput.Blur()
	m.descriptionBox.Blur()
	return m
}

func (m Model) withConfirmation(c confirmation) Model {
	m.popup = popupState{kind: confirmPopup, confirmation: c}
	return m
}

func (m Model) openPushPopup() (tea.Model, tea.Cmd) {
	request := pushRequest{repository: m.repository, upstream: m.status.Upstream}
	upstream := sanitizeText(m.status.Upstream)
	switch {
	case m.repository.Dir == "":
		return m, nil
	case m.status.Upstream == "":
		return m.withFooterError("the branch tracks no upstream, so there is nowhere to push"), nil
	case m.status.Ahead == 0:
		return m.withFooterError("nothing to push"), nil
	case m.status.Behind == 0:
		return m.withConfirmation(confirmation{
			lines: []string{
				titleStyle.Render("push " + headName(m.status) + " to " + upstream),
				textStyle.Render(pluralize(m.status.Ahead, "commit")),
			},
			confirmKey:  confirmPushKey,
			run:         request.push,
			runningText: "pushing...",
		}), nil
	}
	return m.withConfirmation(confirmation{
		lines: []string{
			titleStyle.Render("force push " + headName(m.status) + " to " + upstream),
			textStyle.Render("replaces " + pluralize(m.status.Behind, "commit") + " on " + upstream + " with " +
				pluralize(m.status.Ahead, "commit")),
		},
		confirmKey:  confirmForceKey,
		run:         request.forcePush,
		runningText: "force pushing...",
	}), nil
}

func (m Model) openAmendPopup() Model {
	if m.repository.Dir == "" || len(m.commits) == 0 {
		return m
	}
	if m.stagedFileCount() == 0 {
		return m.withFooterError("nothing is staged to amend with")
	}
	target := m.commits[m.selectedCommit]
	lines := []string{
		titleStyle.Render("amend " + sanitizeText(target.ShortHash)),
		textStyle.Render(sanitizeText(target.Summary)),
		textStyle.Render("with " + pluralize(m.stagedFileCount(), "file") + " staged"),
	}
	if target.Hash != m.commits[0].Hash {
		lines = append(lines, warningStyle.Render("rewrites every commit above it"))
	}
	// No merge sits above an amendable commit, so the log lists those first and the first Ahead are the unpushed.
	if m.status.Upstream != "" && m.selectedCommit >= m.status.Ahead {
		lines = append(lines, warningStyle.Render("it is on "+sanitizeText(m.status.Upstream)+
			" already, so the next push is a force push"))
	}
	return m.withConfirmation(confirmation{
		lines:       lines,
		confirmKey:  confirmAmendKey,
		run:         amendRequest{repository: m.repository, commit: target}.amend,
		runningText: "amending...",
	})
}

func discardConfirmation(target string, effect string, run tea.Cmd) confirmation {
	return confirmation{
		lines: []string{
			titleStyle.Render("discard " + target),
			textStyle.Render(effect),
			warningStyle.Render("this cannot be undone"),
		},
		confirmKey:  confirmDiscardKey,
		run:         run,
		runningText: "discarding...",
	}
}

func discardEffect(file gitrepo.FileStatus) string {
	if file.IsUntracked() {
		return "deletes it"
	}
	return "puts it back to HEAD, staged and unstaged changes alike"
}

func (m Model) updateConfirmPopup(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.popup.running {
		return m, nil
	}
	switch {
	case key.Matches(msg, cancelKey):
		return m.withPopupClosed(), nil
	case key.Matches(msg, m.popup.confirmation.confirmKey):
		m.popup.running = true
		m.popup.err = ""
		return m, m.popup.confirmation.run
	}
	return m, nil
}

func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	if m.view == viewLog {
		return m.selectLogRow(m.selectedLogRow(m.visibleLogRows()) + delta)
	}
	return m.selectFile(m.selectedChange + delta)
}

func (m Model) selectFile(index int) (tea.Model, tea.Cmd) {
	if len(m.status.Files) == 0 {
		return m, nil
	}
	m.selectedChange = min(max(index, 0), len(m.status.Files)-1)
	return m.withListScrolled(), m.loadSelectedDiff()
}

func (m Model) selectLogRow(index int) (Model, tea.Cmd) {
	rows := m.visibleLogRows()
	if len(rows) == 0 {
		return m, nil
	}
	row := rows[min(max(index, 0), len(rows)-1)]
	if row.commit != m.selectedCommit || row.file != m.selectedCommitFile {
		m.diffView.SetYOffset(0)
	}
	m.selectedCommit = row.commit
	m.selectedCommitFile = row.file
	m, loadMore := m.withListScrolled().withDiffContent().withMoreLogNearEnd()
	return m, tea.Batch(m.loadSelectedCommitDiff(), loadMore)
}

func (m Model) withMoreLogNearEnd() (Model, tea.Cmd) {
	if len(m.commits) < m.logLimit || m.selectedCommit < len(m.commits)-m.listInnerHeight() {
		return m, nil
	}
	m.logLimit = len(m.commits) + logPageSize
	return m, m.reloadLog()
}

func (m Model) withCommits(commits []gitrepo.Commit, loadedAt time.Time) (tea.Model, tea.Cmd) {
	hash := m.selectedCommitHash()
	index := slices.IndexFunc(commits, func(commit gitrepo.Commit) bool { return commit.Hash == hash })
	if index < 0 {
		m.selectedCommitFile = commitRow
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
	if m.selectedCommitFile != commitRow {
		m.selectedCommitFile = commitRow
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
	m.mode = modeBrowsing
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
		m.selectedCommitFile = rows[0].file
		m.diffView.SetYOffset(0)
	}
	m = m.withListScrolled().withDiffContent()
	if filter == nil || len(m.commits) < m.logLimit {
		return m, m.loadSelectedCommitDiff()
	}
	m.logLimit = wholeHistory
	return m, tea.Batch(m.loadSelectedCommitDiff(), m.reloadLog())
}

// file indexes the sections of the commit's commitDiff, or is commitRow for the commit itself.
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
		for file := range m.commitDiffs[commit.Hash].sections {
			rows = append(rows, logRow{commit: index, file: file})
		}
	}
	return rows
}

// -1 when the filter hides the selection.
func (m Model) selectedLogRow(rows []logRow) int {
	for index, row := range rows {
		if row.commit == m.selectedCommit && row.file == m.selectedCommitFile {
			return index
		}
	}
	return -1
}

func (m Model) clickList(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.X < m.listOuterWidth() && mouse.Y == m.foldedPaneY() {
		return m.switchView()
	}
	index, hitRow := m.listIndexAt(mouse)
	switch {
	case !hitRow:
		return m, nil
	case m.view == viewLog:
		return m.selectLogRow(index)
	}
	return m.selectFile(index)
}

func (m Model) rightClickList(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	index, hitRow := m.listIndexAt(mouse)
	if m.view != viewLog || !hitRow {
		return m, nil
	}
	selected, cmd := m.selectLogRow(index)
	return selected.openAmendPopup(), cmd
}

// listIndexAt returns the index of the changed file or log row under the mouse in the open pane.
func (m Model) listIndexAt(mouse tea.Mouse) (int, bool) {
	row := mouse.Y - m.listFirstRowY()
	if mouse.X >= m.listOuterWidth() || row < 0 || row >= m.listInnerHeight() {
		return 0, false
	}
	if m.view == viewLog {
		return m.logOffset + row, m.logOffset+row < len(m.visibleLogRows())
	}
	return m.changesOffset + row, m.changesOffset+row < len(m.status.Files)
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
	return fileDiffRequest{repository: m.repository, file: m.status.Files[m.selectedChange], contextLines: m.contextLines()}.load
}

func (m Model) loadSelectedCommitDiff() tea.Cmd {
	hash := m.selectedCommitHash()
	loadedDiff, found := m.commitDiffs[hash]
	if m.view != viewLog || hash == "" || (found && loadedDiff.contextLines == m.contextLines()) {
		return nil
	}
	return commitDiffRequest{repository: m.repository, hash: hash, contextLines: m.contextLines()}.load
}

func (m Model) contextLines() int {
	if m.wholeFile {
		return wholeFileContext
	}
	return hunkContext
}

func (m Model) selectedPath() string {
	if len(m.status.Files) == 0 {
		return ""
	}
	return m.status.Files[m.selectedChange].Path
}

func (m Model) selectedCommitHash() string {
	if len(m.commits) == 0 {
		return ""
	}
	return m.commits[m.selectedCommit].Hash
}

func (m Model) selectedCommitDiff() ([]diff.FileSection, bool) {
	loadedDiff, found := m.commitDiffs[m.selectedCommitHash()]
	return loadedDiff.sections, found
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
	m.diffView.SetContentLines(lineTexts(m.diffLines()))
	return m
}

func (m Model) withDiffKeptInPlace(previous []renderedLine) Model {
	offset := m.diffView.YOffset()
	lines := m.diffLines()
	m.diffView.SetContentLines(lineTexts(lines))
	m.diffView.SetYOffset(keptOffset(previous, lines, offset))
	return m
}

func (m Model) diffLines() []renderedLine {
	width := m.diffInnerWidth()
	sections, loaded := m.selectedCommitDiff()
	switch {
	case m.view == viewLog && loaded && m.selectedCommitFile != commitRow:
		return renderFileDiff(sections[m.selectedCommitFile].FileDiff, m.changeLayout(), width)
	case m.view == viewLog && loaded:
		return renderCommit(m.commits[m.selectedCommit], sections, m.changeLayout(), width)
	case m.view == viewChanges && m.diffPath != "":
		return renderFileDiff(m.fileDiff, m.changeLayout(), width)
	default:
		return []renderedLine{}
	}
}

func (m Model) changeLayout() diffLayout {
	if m.unified {
		return renderUnified
	}
	return renderSideBySide
}

func (m Model) withListScrolled() Model {
	m.logOffset = scrolledOffset(m.logOffset, max(m.selectedLogRow(m.visibleLogRows()), 0), m.listInnerHeight())
	m.changesOffset = scrolledOffset(m.changesOffset, m.selectedChange, m.listInnerHeight())
	m.overview = m.overview.scrolled(m.paneInnerHeight())
	m.branches.selectableList = m.branches.scrolled(m.paneInnerHeight())
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
	if m.fullscreenDiff {
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
	case m.popup.kind == commitPopup:
		view = tea.NewView(m.overlayPopup(m.renderScreen(), m.renderCommitPopup()))
	case m.popup.kind == confirmPopup:
		view = tea.NewView(m.overlayPopup(m.renderScreen(), m.renderConfirmPopup()))
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
	switch {
	case m.mode == modeOverview:
		return renderListPane("repositories", m.overview, m.overviewCells, "no repositories found", m.width-2,
			m.paneInnerHeight())
	case m.mode == modeBranches:
		return renderListPane("branches", m.branches.selectableList, m.branches.cells, "no branches yet", m.width-2,
			m.paneInnerHeight())
	case m.fullscreenDiff:
		return m.renderDiffPane()
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.renderLists(), m.renderDiffPane())
}

func headName(status gitrepo.Status) string {
	if status.Branch == "" {
		return "detached HEAD"
	}
	return sanitizeText(status.Branch)
}

func (m Model) renderHeader() string {
	switch {
	case m.rejection != "":
		return fitToWidth(" "+errorStyle.Render("refused the focused pane's directory"), m.width)
	case m.repository.Dir == "":
		return fitToWidth(" "+mutedStyle.Render("waiting for a repository"), m.width)
	}
	left := " " + repositoryStyle.Render(folderGlyph+" "+sanitizeText(filepath.Base(m.repository.Dir))) +
		"  " + branchStyle.Render(branchGlyph+" "+headName(m.status)) + " " + renderAheadBehind(m.status.Tracking)
	return fitToWidth(left, m.width)
}

func renderAheadBehind(tracking gitrepo.Tracking) string {
	counts := []string{}
	if tracking.Ahead > 0 {
		counts = append(counts, aheadStyle.Render("↑"+strconv.Itoa(tracking.Ahead)))
	}
	if tracking.Behind > 0 {
		counts = append(counts, behindStyle.Render("↓"+strconv.Itoa(tracking.Behind)))
	}
	return strings.Join(counts, " ")
}

func (m Model) renderLists() string {
	innerWidth := m.listOuterWidth() - 2
	if m.view == viewLog {
		return lipgloss.JoinVertical(lipgloss.Left,
			renderFoldedPane("changes", m.changesLabel(), innerWidth),
			renderPane(m.logTitle(), "", m.logLabel(), m.renderLogRows(innerWidth), innerWidth, m.listInnerHeight()))
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		renderPane("changes", m.changesLineCounts(), m.changesLabel(), m.fileRows(innerWidth), innerWidth, m.listInnerHeight()),
		renderFoldedPane(m.logTitle(), m.logLabel(), innerWidth))
}

func (m Model) fileRows(innerWidth int) []string {
	if len(m.status.Files) == 0 && m.repository.Dir != "" {
		return []string{mutedStyle.Render("working tree clean")}
	}
	window := listWindow{offset: m.changesOffset, selected: m.selectedChange, height: m.listInnerHeight()}
	return window.render(len(m.status.Files), innerWidth, func(index int) string {
		return renderFileRow(m.status.Files[index])
	})
}

func (m Model) renderLogRows(innerWidth int) []string {
	rows := m.visibleLogRows()
	switch {
	case m.repository.Dir != "" && !m.status.HasCommits:
		return []string{mutedStyle.Render("no commits yet")}
	case len(rows) == 0 && m.logFilter != nil:
		return []string{mutedStyle.Render("no commit matches")}
	}
	graph := m.logGraph()
	path := pathThrough(m.graph, m.selectedCommit)
	window := listWindow{offset: m.logOffset, selected: m.selectedLogRow(rows), height: m.listInnerHeight()}
	return window.render(len(rows), innerWidth, func(index int) string {
		return m.renderLogRow(rows[index], graph[rows[index].commit], path, innerWidth-1)
	})
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

func (m Model) renderLogRow(row logRow, graph graphRow, path []bool, width int) string {
	commit := m.commits[row.commit]
	if row.file == commitRow {
		prefix := renderGraphCells(graph.commit, path) + expandMarker(m.expanded[commit.Hash])
		return prefix + renderCommitRow(commit, m.logFilter, m.logLoadedAt, width-ansi.StringWidth(prefix))
	}
	files := m.commitDiffs[commit.Hash].sections
	below := renderGraphCells(graph.below, path)
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
	unstaged := countFiles(m.status.Files, gitrepo.FileStatus.HasUnstagedChanges)
	return strconv.Itoa(m.stagedFileCount()) + " staged · " + strconv.Itoa(unstaged) + " unstaged"
}

func (m Model) changesLineCounts() string {
	if m.stagedFileCount() == 0 {
		return ""
	}
	return renderLineCounts(m.stagedLineCounts)
}

func (m Model) stagedFileCount() int {
	return countFiles(m.status.Files, gitrepo.FileStatus.HasStagedChanges)
}

func countFiles(files []gitrepo.FileStatus, has func(gitrepo.FileStatus) bool) int {
	count := 0
	for _, file := range files {
		if has(file) {
			count++
		}
	}
	return count
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
	return positionLabel(position, matching)
}

func renderFileRow(file gitrepo.FileStatus) string {
	path := sanitizeText(file.Path)
	if file.IsUntracked() {
		return untrackedStyle.Render("??") + " " + path
	}
	return stagedMarkStyle.Render(string(file.IndexStatus)) + unstagedMarkStyle.Render(string(file.WorktreeStatus)) +
		" " + path
}

func renderCommitRow(commit gitrepo.Commit, search searchTerms, loadedAt time.Time, width int) string {
	age := compactAge(loadedAt.Sub(commit.AuthoredAt))
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
	sections, loaded := m.selectedCommitDiff()
	if loaded && m.selectedCommitFile != commitRow {
		return sanitizeText(commit.ShortHash + " " + sections[m.selectedCommitFile].Path)
	}
	return sanitizeText(commit.ShortHash + " " + commit.Summary)
}

func (m Model) fileLineCounts() string {
	if m.diffPath == "" {
		return ""
	}
	return renderLineCounts(diff.CountLines(m.fileDiff))
}

func (m Model) commitLineCounts() string {
	sections, loaded := m.selectedCommitDiff()
	if !loaded {
		return ""
	}
	if m.selectedCommitFile != commitRow {
		return renderLineCounts(diff.CountLines(sections[m.selectedCommitFile].FileDiff))
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
	case m.footer.text != "" && m.footer.isError:
		return fitToWidth(errorStyle.Render("✗ "+m.footer.text), m.width)
	case m.footer.text != "":
		return fitToWidth(successStyle.Render("✓ "+m.footer.text), m.width)
	case m.mode == modeOverview:
		return m.renderHelpLine(overviewHelp)
	case m.mode == modeBranches:
		return m.renderHelpLine(branchesHelp)
	case m.mode == modeSearching && m.searchError != "":
		return fitToWidth(" "+m.searchInput.View()+"  "+errorStyle.Render(m.searchError), m.width)
	case m.mode == modeSearching:
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
		titleStyle.Render("commit to " + headName(m.status)),
		renderField(m.summaryInput.View(), !m.editingDescription, m.fieldInnerWidth(), 1),
		renderField(m.descriptionBox.View(), m.editingDescription, m.fieldInnerWidth(), descriptionHeight),
	}, "committing...", commitHelp)
}

func (m Model) renderConfirmPopup() string {
	confirmation := m.popup.confirmation
	return m.renderPopup(confirmation.lines, confirmation.runningText,
		[]key.Binding{confirmation.confirmKey, cancelKey})
}

func (m Model) renderPopup(lines []string, runningText string, bindings []key.Binding) string {
	width := m.popupInnerWidth()
	var status string
	switch {
	case m.popup.running:
		status = mutedStyle.Render(runningText)
	case m.popup.err != "":
		wrapped := strings.Split(lipgloss.Wrap(m.popup.err, width, ""), "\n")
		status = errorStyle.Render(strings.Join(wrapped[:min(len(wrapped), maximumErrorLines)], "\n"))
	default:
		status = m.help.ShortHelpView(bindings)
	}
	return popupStyle.Width(width + 4).Render(strings.Join(lines, "\n") + "\n\n" + status)
}

func pluralize(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(count) + " " + noun + "s"
}

func renderField(view string, focused bool, innerWidth int, innerHeight int) string {
	body := strings.Join(fitLines(strings.Split(view, "\n"), innerWidth, innerHeight), "\n")
	if focused {
		return focusedFieldStyle.Render(body)
	}
	return fieldStyle.Render(body)
}
