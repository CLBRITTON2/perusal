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

// Letters match git log --name-status.
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

func spaceBetween(left string, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fitToWidth(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// topRight is pre-styled and sits at the right end of the top border, kept whole while the title is cut.
func renderPane(title string, topRight string, label string, body []string, innerWidth int, innerHeight int) string {
	lines := []string{borderLine("╭", titleStyle, title, topRight, "╮", innerWidth)}
	side := borderStyle.Render("│")
	for _, line := range fitLines(body, innerWidth, innerHeight) {
		lines = append(lines, side+line+side)
	}
	lines = append(lines, borderLine("╰", paneLabelStyle, label, "", "╯", innerWidth))
	return strings.Join(lines, "\n")
}

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

func borderLine(leftCorner string, style lipgloss.Style, text string, rightText string, rightCorner string,
	innerWidth int) string {
	if text == "" || innerWidth < 4 {
		return borderStyle.Render(leftCorner + strings.Repeat("─", max(innerWidth, 0)) + rightCorner)
	}
	var right string
	var reserved int
	if rightText != "" && ansi.StringWidth(rightText)+8 <= innerWidth {
		right = " " + rightText + borderStyle.Render(" ─"+rightCorner)
		reserved = ansi.StringWidth(rightText) + 3
	} else {
		right = borderStyle.Render(rightCorner)
		reserved = 0
	}
	fitted := ansi.Truncate(text, innerWidth-4-reserved, "…")
	fill := innerWidth - 3 - reserved - ansi.StringWidth(fitted)
	return borderStyle.Render(leftCorner+"─ ") + style.Render(fitted) + borderStyle.Render(" "+
		strings.Repeat("─", fill)) + right
}

func renderLineCounts(counts diff.LineCounts) string {
	return addedMarkStyle.Render("+"+strconv.Itoa(counts.Added)) + " " +
		deletedMarkStyle.Render("-"+strconv.Itoa(counts.Removed))
}

// Text can come from the agent: a control character could start an escape, a bidi control could disguise code.
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

func sanitizeLines(text string) string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n"), "\n")
	sanitized := []string{}
	for _, line := range lines {
		sanitized = append(sanitized, sanitizeText(line))
	}
	return strings.Join(sanitized, "\n")
}

func sanitizeSingleLine(text string) string {
	lines := strings.FieldsFunc(text, func(character rune) bool { return character == '\n' || character == '\r' })
	return sanitizeText(strings.Join(lines, " | "))
}
