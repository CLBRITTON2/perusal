package ui

import (
	"regexp"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/diff"
)

// A note line such as "CHANGE DRY  : text" or "QUESTION : text", the shape the slop-review skill writes.
var noteFieldPattern = regexp.MustCompile(`^(CHANGE|SUMMARY|QUESTION)(\s+\w+)?(\s*:)(.*)$`)

// ShowModel is perusal show: one diff under a note, read only, with nothing to select.
type ShowModel struct {
	label    string
	note     string
	fileDiff diff.FileDiff
	unified  bool
	fullNote bool
	diffView viewport.Model
	help     help.Model
	width    int
	height   int
}

func NewShow(label string, note string, fileDiff diff.FileDiff) ShowModel {
	return ShowModel{
		label:    label,
		note:     note,
		fileDiff: fileDiff,
		unified:  true,
		diffView: viewport.New(),
		help:     newHelpView(),
	}
}

func (m ShowModel) Init() tea.Cmd {
	return nil
}

func (m ShowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		previous := m.diffLines()
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(m.width)
		m.diffView.SetWidth(m.diffInnerWidth())
		m.diffView.SetHeight(m.diffInnerHeight())
		return m.withDiffKeptInPlace(previous), nil
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			m.diffView.ScrollDown(wheelScrollLines)
		case tea.MouseWheelUp:
			m.diffView.ScrollUp(wheelScrollLines)
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m ShowModel) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, closeShowKey), key.Matches(msg, forceQuitKey):
		return m, tea.Quit
	case key.Matches(msg, lineDownKey):
		m.diffView.ScrollDown(1)
	case key.Matches(msg, lineUpKey):
		m.diffView.ScrollUp(1)
	case key.Matches(msg, scrollDownKey):
		m.diffView.HalfPageDown()
	case key.Matches(msg, scrollUpKey):
		m.diffView.HalfPageUp()
	case key.Matches(msg, nextChangeKey):
		m.diffView.SetYOffset(nextChangeOffset(m.diffLines(), m.diffView.YOffset(), maxOffset(m.diffView)))
	case key.Matches(msg, previousChangeKey):
		m.diffView.SetYOffset(previousChangeOffset(m.diffLines(), m.diffView.YOffset(), maxOffset(m.diffView)))
	case key.Matches(msg, unifiedKey):
		previous := m.diffLines()
		m.unified = !m.unified
		return m.withDiffKeptInPlace(previous), nil
	case key.Matches(msg, fullNoteKey):
		previous := m.diffLines()
		m.fullNote = !m.fullNote
		m.diffView.SetHeight(m.diffInnerHeight())
		return m.withDiffKeptInPlace(previous), nil
	}
	return m, nil
}

func (m ShowModel) withDiffKeptInPlace(previous []renderedLine) ShowModel {
	offset := m.diffView.YOffset()
	lines := m.diffLines()
	m.diffView.SetContentLines(lineTexts(lines))
	m.diffView.SetYOffset(keptOffset(previous, lines, offset))
	return m
}

func (m ShowModel) diffLines() []renderedLine {
	var layout diffLayout
	if m.unified {
		layout = renderUnified
	} else {
		layout = renderSideBySide
	}
	return renderFileDiff(m.fileDiff, layout, m.diffInnerWidth())
}

func (m ShowModel) diffInnerWidth() int {
	return max(m.width-2, 0)
}

// Leaves out the note, the pane's two borders and the footer.
func (m ShowModel) diffInnerHeight() int {
	return max(m.height-len(m.noteRows())-2-footerHeight, 0)
}

// noteRows wraps the note to the screen, followed by a blank row when there is a note. Until fullNote is on, only the
// CHANGE, SUMMARY and QUESTION lines show, so the diff keeps the screen.
func (m ShowModel) noteRows() []string {
	note := sanitizeLines(m.note)
	if note == "" {
		return []string{}
	}
	rows := []string{}
	for _, line := range strings.Split(note, "\n") {
		var styled string
		if field := noteFieldPattern.FindStringSubmatch(line); field != nil {
			styled = noteLabelStyle.Render(field[1]+field[2]) + mutedStyle.Render(field[3]) + noteValueStyle.Render(field[4])
		} else if m.fullNote {
			styled = textStyle.Render(line)
		} else {
			continue
		}
		for _, wrapped := range strings.Split(ansi.Wrap(styled, max(m.width-2, 1), ""), "\n") {
			rows = append(rows, " "+wrapped)
		}
	}
	return append(rows, "")
}

func (m ShowModel) View() tea.View {
	var view tea.View
	if m.width == 0 {
		view = tea.NewView("")
	} else {
		pane := renderPane(sanitizeText(m.label), renderLineCounts(diff.CountLines(m.fileDiff)), scrollLabel(m.diffView),
			strings.Split(m.diffView.View(), "\n"), m.diffInnerWidth(), m.diffInnerHeight())
		footer := spaceBetween(" "+m.help.ShortHelpView(showHelp), mutedStyle.Render(appName)+" ", m.width)
		view = tea.NewView(strings.Join(append(m.noteRows(), pane, footer), "\n"))
	}
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = appName + ": " + sanitizeText(m.label)
	return view
}
