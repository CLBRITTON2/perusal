package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

const tabWidth = 4

const columnSeparator = "│"

// renderFileDiff lays out fileDiff as rows exactly width cells wide. Only a file that existed before and after gets
// the side-by-side split: an added or deleted file has one side, so it takes the whole width.
func renderFileDiff(fileDiff diff.FileDiff, width int) []string {
	switch {
	case fileDiff.Binary:
		return []string{mutedStyle.Render("binary file, no text diff")}
	case len(fileDiff.Hunks) == 0:
		return []string{mutedStyle.Render("no text changes")}
	case fileDiff.Change == diff.FileAdded:
		return renderOneSide(fileDiff, newLine, width)
	case fileDiff.Change == diff.FileDeleted:
		return renderOneSide(fileDiff, oldLine, width)
	default:
		return renderSideBySide(fileDiff, width)
	}
}

// renderSideBySide puts the old file left and the new file right. A line too long for its side wraps onto the rows
// below, and the other side is padded to match.
func renderSideBySide(fileDiff diff.FileDiff, width int) []string {
	numberWidth := len(strconv.Itoa(maxLineNumber(fileDiff)))
	leftWidth := (width - 1) / 2
	rightWidth := width - 1 - leftWidth
	separator := separatorStyle.Render(columnSeparator)
	lines := []string{}
	for _, hunk := range fileDiff.Hunks {
		lines = append(lines, hunkHeaderStyle.Render(fitToWidth(sanitizeText(hunk.Header), width)))
		for _, row := range hunk.Rows {
			left := wrapSide(row.Left, numberWidth, leftWidth)
			right := wrapSide(row.Right, numberWidth, rightWidth)
			for index := 0; index < max(len(left), len(right)); index++ {
				lines = append(lines, segmentOrFiller(left, index, row.Left.Kind, leftWidth)+separator+
					segmentOrFiller(right, index, row.Right.Kind, rightWidth))
			}
		}
	}
	return lines
}

func renderOneSide(fileDiff diff.FileDiff, side func(diff.Row) diff.Line, width int) []string {
	numberWidth := len(strconv.Itoa(maxLineNumber(fileDiff)))
	lines := []string{}
	for _, hunk := range fileDiff.Hunks {
		lines = append(lines, hunkHeaderStyle.Render(fitToWidth(sanitizeText(hunk.Header), width)))
		for _, row := range hunk.Rows {
			if side(row).Kind != diff.LineBlank {
				lines = append(lines, wrapSide(side(row), numberWidth, width)...)
			}
		}
	}
	return lines
}

func oldLine(row diff.Row) diff.Line {
	return row.Left
}

func newLine(row diff.Row) diff.Line {
	return row.Right
}

// wrapSide renders one half of a row, width cells wide per returned line: a right-aligned line number and a space on
// the first line, a blank gutter on the lines the text wraps onto.
func wrapSide(line diff.Line, numberWidth int, width int) []string {
	gutterWidth := numberWidth + 1
	if width <= gutterWidth || line.Kind == diff.LineBlank {
		return []string{strings.Repeat(" ", max(width, 0))}
	}
	textWidth := width - gutterWidth
	gutter, text := lineStyles(line.Kind)
	segments := strings.Split(ansi.Hardwrap(sanitizeText(line.Text), textWidth, true), "\n")
	rendered := []string{}
	for index, segment := range segments {
		var number string
		if index == 0 {
			number = fmt.Sprintf("%*d ", numberWidth, line.Number)
		} else {
			number = strings.Repeat(" ", gutterWidth)
		}
		rendered = append(rendered, gutter.Render(number)+text.Render(fitToWidth(segment, textWidth)))
	}
	return rendered
}

// segmentOrFiller returns the wrapped line at index, or blank cells in the background of kind once the side's text
// has run out.
func segmentOrFiller(segments []string, index int, kind diff.LineKind, width int) string {
	if index < len(segments) {
		return segments[index]
	}
	_, text := lineStyles(kind)
	return text.Render(strings.Repeat(" ", width))
}

func lineStyles(kind diff.LineKind) (lipgloss.Style, lipgloss.Style) {
	switch kind {
	case diff.LineRemoved:
		return removedGutter, removedTextStyle
	case diff.LineAdded:
		return addedGutter, addedTextStyle
	case diff.LineContext:
		return mutedStyle, contextTextStyle
	default:
		return lipgloss.NewStyle(), lipgloss.NewStyle()
	}
}

func maxLineNumber(fileDiff diff.FileDiff) int {
	highest := 0
	for _, hunk := range fileDiff.Hunks {
		for _, row := range hunk.Rows {
			highest = max(highest, row.Left.Number, row.Right.Number)
		}
	}
	return highest
}

func renderCommit(commit gitrepo.Commit, sections []diff.FileSection, width int) []string {
	lines := []string{
		hashStyle.Render(fitToWidth(commit.Hash, width)),
		fitToWidth(authorStyle.Render(sanitizeText(commit.Author))+mutedStyle.Render(" · "+
			commit.AuthoredAt.Format("2006-01-02 15:04")), width),
		"",
	}
	for _, line := range strings.Split(sanitizeLines(commit.Message), "\n") {
		for _, wrapped := range strings.Split(ansi.Wrap(line, width, ""), "\n") {
			lines = append(lines, textStyle.Render(fitToWidth(wrapped, width)))
		}
	}
	if len(sections) == 0 {
		return append(lines, "", mutedStyle.Render("no file changes"))
	}
	for _, section := range sections {
		lines = append(lines, "", fileHeaderStyle.Render(fitToWidth(" "+sanitizeText(section.Path), width)))
		lines = append(lines, renderFileDiff(section.FileDiff, width)...)
	}
	return lines
}

// renderTreeFileRow shows one file of an expanded commit under it, with the letter git log --name-status uses.
func renderTreeFileRow(section diff.FileSection, last bool, width int) string {
	var branch string
	if last {
		branch = "└ "
	} else {
		branch = "├ "
	}
	return fitToWidth(mutedStyle.Render(branch)+changeMark(section.FileDiff.Change)+" "+
		textStyle.Render(sanitizeText(section.Path)), width)
}

func changeMark(change diff.FileChange) string {
	switch change {
	case diff.FileAdded:
		return addedMarkStyle.Render("A")
	case diff.FileDeleted:
		return deletedMarkStyle.Render("D")
	case diff.FileRenamed:
		return renamedMarkStyle.Render("R")
	default:
		return modifiedMarkStyle.Render("M")
	}
}

func expandMarker(expanded bool) string {
	if expanded {
		return mutedStyle.Render("▾ ")
	}
	return mutedStyle.Render("▸ ")
}

// compactAge shortens age to its largest whole unit, such as 5m, 3h, 2d, 6w, 4mo or 2y.
func compactAge(age time.Duration) string {
	const day = 24 * time.Hour
	switch {
	case age < time.Minute:
		return "now"
	case age < time.Hour:
		return strconv.Itoa(int(age/time.Minute)) + "m"
	case age < day:
		return strconv.Itoa(int(age/time.Hour)) + "h"
	case age < 14*day:
		return strconv.Itoa(int(age/day)) + "d"
	case age < 60*day:
		return strconv.Itoa(int(age/(7*day))) + "w"
	case age < 365*day:
		return strconv.Itoa(int(age/(30*day))) + "mo"
	default:
		return strconv.Itoa(int(age/(365*day))) + "y"
	}
}

// fitToWidth cuts text, which may hold ANSI styling, to width cells with an ellipsis, or pads it with spaces.
func fitToWidth(text string, width int) string {
	truncated := ansi.Truncate(text, width, "…")
	return truncated + strings.Repeat(" ", max(width-ansi.StringWidth(truncated), 0))
}

func fitLines(lines []string, width int, height int) []string {
	fitted := []string{}
	for i := 0; i < height; i++ {
		if i < len(lines) {
			fitted = append(fitted, fitToWidth(lines[i], width))
		} else {
			fitted = append(fitted, strings.Repeat(" ", width))
		}
	}
	return fitted
}

// spaceBetween puts left and right at the two ends of a line width cells wide. Left wins when both do not fit.
func spaceBetween(left string, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fitToWidth(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderPane draws a rounded box with title set into its top border and label into its bottom border.
func renderPane(title string, label string, body []string, innerWidth int, innerHeight int) string {
	lines := []string{borderLine("╭", titleStyle, title, "╮", innerWidth)}
	side := borderStyle.Render("│")
	for _, line := range fitLines(body, innerWidth, innerHeight) {
		lines = append(lines, side+line+side)
	}
	lines = append(lines, borderLine("╰", paneLabelStyle, label, "╯", innerWidth))
	return strings.Join(lines, "\n")
}

// renderFoldedPane draws a pane folded to one line as wide as the open box, title at the left and label at the
// right. The label is the first to go when both do not fit.
func renderFoldedPane(title string, label string, innerWidth int) string {
	outerWidth := innerWidth + 2
	fittedTitle := ansi.Truncate(title, max(outerWidth-6, 0), "…")
	left := borderStyle.Render("╶─ ") + unfocusedTitleStyle.Render(fittedTitle) + " "
	labelled := " " + paneLabelStyle.Render(label) + borderStyle.Render(" ─╴")
	var right string
	if label != "" && lipgloss.Width(left)+lipgloss.Width(labelled) < outerWidth {
		right = labelled
	} else {
		right = borderStyle.Render("─╴")
	}
	fill := max(outerWidth-lipgloss.Width(left)-lipgloss.Width(right), 0)
	return left + borderStyle.Render(strings.Repeat("─", fill)) + right
}

// borderLine draws one horizontal border with text after a short lead, such as "╭─ changes ──╮".
func borderLine(leftCorner string, style lipgloss.Style, text string, rightCorner string, innerWidth int) string {
	if text == "" || innerWidth < 4 {
		return borderStyle.Render(leftCorner + strings.Repeat("─", max(innerWidth, 0)) + rightCorner)
	}
	fitted := ansi.Truncate(text, innerWidth-4, "…")
	fill := innerWidth - 3 - ansi.StringWidth(fitted)
	return borderStyle.Render(leftCorner+"─ ") + style.Render(fitted) + borderStyle.Render(" "+
		strings.Repeat("─", fill)+rightCorner)
}

// sanitizeText makes repository content safe to print. File text, paths and git output can come from the agent
// account, so a control character, which could start an escape sequence, becomes U+FFFD, and so does a bidi
// control, which could make the shown code read differently from what gets committed. A tab becomes spaces.
func sanitizeText(text string) string {
	var sanitized strings.Builder
	for _, character := range text {
		switch {
		case character == '\t':
			sanitized.WriteString(strings.Repeat(" ", tabWidth))
		case unicode.IsControl(character), unicode.Is(unicode.Bidi_Control, character):
			sanitized.WriteRune(utf8.RuneError)
		default:
			sanitized.WriteRune(character)
		}
	}
	return sanitized.String()
}

// sanitizeLines sanitizes multi-line text one line at a time, so its line breaks survive.
func sanitizeLines(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	sanitized := []string{}
	for _, line := range lines {
		sanitized = append(sanitized, sanitizeText(line))
	}
	return strings.Join(sanitized, "\n")
}

func singleLine(text string) string {
	lines := strings.FieldsFunc(text, isLineBreak)
	return strings.Join(lines, " | ")
}

func isLineBreak(character rune) bool {
	return character == '\n' || character == '\r'
}
