package ui

import (
	"path/filepath"
	"strconv"
	"strings"

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
	wheelScrollLines     = 3
	maximumPopupWidth    = 72
	descriptionHeight    = 5
	maximumErrorLines    = 12
	keyHelp              = "j/k file  space stage  c commit  ctrl+d/u scroll  r refresh  q quit"
	commitKeyHelp        = "tab switch field  ctrl+enter commit  esc cancel"
	detachedBranchLabel  = "detached HEAD"
	emptySummaryMessage  = "the summary is required"
	commitRunningMessage = "committing..."
	waitingMessage       = "waiting for a repository"
	refusedMessage       = "refused the focused pane's directory"
)

// listFirstRowY is the screen row of the first file in the list: below the header, the top border and the title.
const listFirstRowY = headerHeight + 2

type Model struct {
	repository gitrepo.Repository
	// rejection says why no repository is on screen, empty while one is or before the first arrives.
	rejection      string
	branch         string
	files          []gitrepo.FileStatus
	selected       int
	listOffset     int
	diffPath       string
	fileDiff       diff.FileDiff
	diffView       viewport.Model
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
	return Model{
		files:          []gitrepo.FileStatus{},
		diffView:       viewport.New(),
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
		return m, filesRequest{repository: m.repository}.load
	case FocusRejected:
		return m.withRepository(gitrepo.Repository{}, sanitizeText(singleLine(msg.Reason))), nil
	case filesLoadedMsg:
		if msg.dir != m.repository.Dir {
			return m, nil
		}
		m.branch = msg.branch
		m.files = msg.files
		m.selected = min(m.selected, max(len(m.files)-1, 0))
		return m.withLayout(), m.loadSelectedDiff()
	case diffLoadedMsg:
		if msg.dir != m.repository.Dir || msg.path != m.selectedPath() {
			return m, nil
		}
		if msg.path != m.diffPath {
			m.diffView.SetYOffset(0)
		}
		m.diffPath = msg.path
		m.fileDiff = msg.fileDiff
		m.diffView.SetContentLines(renderSideBySide(m.fileDiff, m.diffView.Width()))
		return m, nil
	case indexChangedMsg:
		if msg.dir != m.repository.Dir {
			return m, nil
		}
		return m, filesRequest{repository: m.repository}.load
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
		return m, filesRequest{repository: m.repository}.load
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
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.committing {
			return m.updateCommitBox(msg)
		}
		return m.updateBrowsing(msg)
	case tea.MouseClickMsg:
		if m.committing || msg.Button != tea.MouseLeft {
			return m, nil
		}
		return m.clickFileList(msg.Mouse())
	case tea.MouseWheelMsg:
		if m.committing {
			return m, nil
		}
		return m.scrollWheel(msg.Mouse())
	}
	if m.committing {
		return m.updateFocusedField(msg)
	}
	return m, nil
}

func (m Model) updateBrowsing(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footerText = ""
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "j", "down":
		return m.selectFile(m.selected + 1)
	case "k", "up":
		return m.selectFile(m.selected - 1)
	case "ctrl+d", "pgdown":
		m.diffView.HalfPageDown()
		return m, nil
	case "ctrl+u", "pgup":
		m.diffView.HalfPageUp()
		return m, nil
	case "space":
		if len(m.files) == 0 {
			return m, nil
		}
		request := fileRequest{repository: m.repository, file: m.files[m.selected]}
		if request.file.HasUnstagedChanges() {
			return m, request.stage
		}
		return m, request.unstage
	case "c":
		if m.repository.Dir == "" {
			return m, nil
		}
		m.committing = true
		m.descriptionFocused = false
		return m, m.summaryInput.Focus()
	case "r":
		if m.repository.Dir == "" {
			return m, nil
		}
		return m, filesRequest{repository: m.repository}.load
	}
	return m, nil
}

// withRepository shows repository, or rejection when no repository is allowed, and drops everything shown for the
// previous one, including a commit message being written for it.
func (m Model) withRepository(repository gitrepo.Repository, rejection string) Model {
	m = m.withCommitPopupClosed()
	m.summaryInput.Reset()
	m.descriptionBox.Reset()
	m.repository = repository
	m.rejection = rejection
	m.branch = ""
	m.files = []gitrepo.FileStatus{}
	m.selected = 0
	m.listOffset = 0
	m.diffPath = ""
	m.fileDiff = diff.FileDiff{}
	m.diffView.SetYOffset(0)
	m.footerText = ""
	m.footerIsError = false
	return m.withLayout()
}

func (m Model) updateCommitBox(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.commitRunning {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		return m.withCommitPopupClosed(), nil
	case "tab", "shift+tab":
		return m.toggleCommitField()
	case "ctrl+enter":
		return m.startCommit()
	case "enter":
		if !m.descriptionFocused {
			return m.startCommit()
		}
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

func (m Model) selectFile(index int) (tea.Model, tea.Cmd) {
	if len(m.files) == 0 {
		return m, nil
	}
	m.selected = min(max(index, 0), len(m.files)-1)
	return m.withListScrolled(), m.loadSelectedDiff()
}

func (m Model) clickFileList(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	row := mouse.Y - listFirstRowY
	if mouse.X >= m.listOuterWidth() || row < 0 || row >= m.paneInnerHeight() {
		return m, nil
	}
	index := m.listOffset + row
	if index >= len(m.files) {
		return m, nil
	}
	return m.selectFile(index)
}

// scrollWheel moves the file selection over the list and scrolls the diff over the diff pane.
func (m Model) scrollWheel(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	overList := mouse.X < m.listOuterWidth()
	switch {
	case overList && mouse.Button == tea.MouseWheelDown:
		return m.selectFile(m.selected + 1)
	case overList && mouse.Button == tea.MouseWheelUp:
		return m.selectFile(m.selected - 1)
	case mouse.Button == tea.MouseWheelDown:
		m.diffView.ScrollDown(wheelScrollLines)
	case mouse.Button == tea.MouseWheelUp:
		m.diffView.ScrollUp(wheelScrollLines)
	}
	return m, nil
}

func (m Model) loadSelectedDiff() tea.Cmd {
	if len(m.files) == 0 {
		return nil
	}
	return fileRequest{repository: m.repository, file: m.files[m.selected]}.loadDiff
}

func (m Model) selectedPath() string {
	if len(m.files) == 0 {
		return ""
	}
	return m.files[m.selected].Path
}

func (m Model) withLayout() Model {
	m.diffView.SetWidth(m.diffInnerWidth())
	m.diffView.SetHeight(m.paneInnerHeight())
	m.diffView.SetContentLines(renderSideBySide(m.fileDiff, m.diffInnerWidth()))
	// textinput draws its end-of-text cursor one cell past its width
	m.summaryInput.SetWidth(max(m.fieldInnerWidth()-1, 0))
	m.descriptionBox.SetWidth(m.fieldInnerWidth())
	m.descriptionBox.SetHeight(descriptionHeight)
	return m.withListScrolled()
}

// withListScrolled moves the list window so the selected file is visible.
func (m Model) withListScrolled() Model {
	visibleRows := m.paneInnerHeight()
	if m.selected < m.listOffset {
		m.listOffset = m.selected
	} else if visibleRows > 0 && m.selected >= m.listOffset+visibleRows {
		m.listOffset = m.selected - visibleRows + 1
	}
	return m
}

// listOuterWidth fits the longest file row, within a minimum width and a share of the screen.
func (m Model) listOuterWidth() int {
	longestRow := lipgloss.Width(m.listTitle())
	for _, file := range m.files {
		longestRow = max(longestRow, ansi.StringWidth(renderFileRow(file)))
	}
	return min(max(longestRow+2, minimumListWidth), m.width*maximumListPercent/100)
}

func (m Model) listTitle() string {
	return "changes " + strconv.Itoa(len(m.files))
}

func (m Model) diffInnerWidth() int {
	return max(m.width-m.listOuterWidth()-2, 0)
}

// paneInnerHeight is the rows left for content inside a pane, after its borders and title.
func (m Model) paneInnerHeight() int {
	return max(m.height-headerHeight-footerHeight-3, 0)
}

func (m Model) popupInnerWidth() int {
	return max(min(m.width-4, maximumPopupWidth)-4, 0)
}

// fieldInnerWidth is the text width inside the border of a commit popup field.
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
		lipgloss.JoinHorizontal(lipgloss.Top, m.renderFileList(), m.renderDiffPane()),
		m.renderFooter(),
	)
}

func (m Model) branchLabel() string {
	if m.branch == "" {
		return detachedBranchLabel
	}
	return sanitizeText(m.branch)
}

func (m Model) renderHeader() string {
	appName := appNameStyle.Render("perusal") + " "
	switch {
	case m.rejection != "":
		return fitToWidth(appName+errorStyle.Render(refusedMessage), m.width)
	case m.repository.Dir == "":
		return fitToWidth(appName+mutedStyle.Render(waitingMessage), m.width)
	}
	header := appName + repositoryStyle.Render(sanitizeText(filepath.Base(m.repository.Dir))) +
		"  " + branchStyle.Render(" "+m.branchLabel())
	return fitToWidth(header, m.width)
}

func (m Model) renderFileList() string {
	innerWidth := m.listOuterWidth() - 2
	rows := []string{}
	end := min(m.listOffset+m.paneInnerHeight(), len(m.files))
	for index := m.listOffset; index < end; index++ {
		row := fitToWidth(renderFileRow(m.files[index]), innerWidth)
		if index == m.selected {
			rows = append(rows, selectedRowStyle.Render(row))
		} else {
			rows = append(rows, row)
		}
	}
	if len(m.files) == 0 && m.repository.Dir != "" {
		rows = append(rows, mutedStyle.Render("working tree clean"))
	}
	return renderPane(m.listTitle(), rows, innerWidth, m.paneInnerHeight())
}

// renderFileRow shows the index and worktree codes of git status, then the path.
func renderFileRow(file gitrepo.FileStatus) string {
	path := sanitizeText(file.Path)
	if file.IsUntracked() {
		return untrackedStyle.Render("??") + " " + path
	}
	return stagedMarkStyle.Render(string(file.IndexStatus)) + unstagedMarkStyle.Render(string(file.WorktreeStatus)) +
		" " + path
}

func (m Model) renderDiffPane() string {
	if m.rejection != "" {
		reason := errorStyle.Render(lipgloss.Wrap(m.rejection, m.diffInnerWidth(), ""))
		return renderPane("refused", strings.Split(reason, "\n"), m.diffInnerWidth(), m.paneInnerHeight())
	}
	if len(m.files) == 0 {
		return renderPane("diff", []string{}, m.diffInnerWidth(), m.paneInnerHeight())
	}
	body := strings.Split(m.diffView.View(), "\n")
	return renderPane(sanitizeText(m.diffPath), body, m.diffInnerWidth(), m.paneInnerHeight())
}

func (m Model) renderFooter() string {
	if m.footerText == "" {
		return fitToWidth(mutedStyle.Render(keyHelp), m.width)
	}
	if m.footerIsError {
		return fitToWidth(errorStyle.Render(m.footerText), m.width)
	}
	return fitToWidth(successStyle.Render(m.footerText), m.width)
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
		lines = append(lines, mutedStyle.Render(commitKeyHelp))
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
