package ui

import (
	"fmt"
	"slices"
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

// diffLayout renders a diff that has both an old and a new side.
type diffLayout func(fileDiff diff.FileDiff, width int) []string

func renderFileDiff(fileDiff diff.FileDiff, layout diffLayout, width int) []string {
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
		return layout(fileDiff, width)
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

// renderUnified lists each run of changes as its removed lines above its added lines, numbering a line by its own
// side, as GitHub's unified view does.
func renderUnified(fileDiff diff.FileDiff, width int) []string {
	numberWidth := len(strconv.Itoa(maxLineNumber(fileDiff)))
	lines := []string{}
	for _, hunk := range fileDiff.Hunks {
		lines = append(lines, hunkHeaderStyle.Render(fitToWidth(sanitizeText(hunk.Header), width)))
		added := []string{}
		for _, row := range hunk.Rows {
			if row.Left.Kind == diff.LineContext {
				lines = append(append(lines, added...), wrapLine(row.Right, unifiedGutter(row.Left, row.Right, numberWidth), width)...)
				added = []string{}
			}
			if row.Left.Kind == diff.LineRemoved {
				lines = append(lines, wrapLine(row.Left, unifiedGutter(row.Left, diff.Line{Kind: diff.LineBlank}, numberWidth), width)...)
			}
			if row.Right.Kind == diff.LineAdded {
				added = append(added, wrapLine(row.Right, unifiedGutter(diff.Line{Kind: diff.LineBlank}, row.Right, numberWidth), width)...)
			}
		}
		lines = append(lines, added...)
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

func unifiedGutter(before diff.Line, after diff.Line, numberWidth int) string {
	return numberOrBlank(before, numberWidth) + " " + numberOrBlank(after, numberWidth) + " "
}

func numberOrBlank(line diff.Line, numberWidth int) string {
	if line.Kind == diff.LineBlank {
		return strings.Repeat(" ", numberWidth)
	}
	return fmt.Sprintf("%*d", numberWidth, line.Number)
}

func wrapSide(line diff.Line, numberWidth int, width int) []string {
	return wrapLine(line, fmt.Sprintf("%*d ", numberWidth, line.Number), width)
}

// wrapLine renders line under width, with the gutter text on its first row and blank on the rest.
func wrapLine(line diff.Line, gutterText string, width int) []string {
	gutterWidth := len(gutterText)
	if width <= gutterWidth || line.Kind == diff.LineBlank {
		return []string{strings.Repeat(" ", max(width, 0))}
	}
	textWidth := width - gutterWidth
	gutter, text := lineStyles(line.Kind)
	runs := lineRuns(line)
	var joined strings.Builder
	for _, piece := range runs {
		joined.WriteString(piece.text)
	}
	rendered := []string{}
	for index, row := range wrapRows(joined.String(), textWidth) {
		var rowGutter string
		if index == 0 {
			rowGutter = gutterText
		} else {
			rowGutter = strings.Repeat(" ", gutterWidth)
		}
		rendered = append(rendered, gutter.Render(rowGutter)+renderSegment(runs, row.Start, row.End, text,
			changedTextStyle(line.Kind), textWidth))
	}
	return rendered
}

// wrapRows cuts text into rows of width cells, breaking before the last run of spaces that follows other text in the
// row and leaving those spaces out. A row with no such run is cut at the width.
func wrapRows(text string, width int) []diff.Span {
	if text == "" {
		return []diff.Span{{Start: 0, End: 0}}
	}
	rows := []diff.Span{}
	start := 0
	for start < len(text) {
		row, next := wrapRow(text, start, width)
		rows = append(rows, row)
		start = next
	}
	return rows
}

// wrapRow returns the row starting at start and where the next row starts. It always takes at least one grapheme
// cluster, so a cluster wider than the row still moves on.
func wrapRow(text string, start int, width int) (diff.Span, int) {
	used := 0
	breakAt := start
	afterText := false
	position := start
	for position < len(text) {
		cluster, clusterWidth := ansi.FirstGraphemeCluster(text[position:], ansi.GraphemeWidth)
		space := cluster == " "
		if space && afterText {
			breakAt = position
		}
		if used+clusterWidth > width {
			switch {
			case breakAt > start:
				return diff.Span{Start: start, End: breakAt}, skipSpaces(text, breakAt)
			case position == start:
				return diff.Span{Start: start, End: position + len(cluster)}, position + len(cluster)
			default:
				return diff.Span{Start: start, End: position}, position
			}
		}
		used += clusterWidth
		afterText = !space
		position += len(cluster)
	}
	return diff.Span{Start: start, End: len(text)}, len(text)
}

func skipSpaces(text string, position int) int {
	return len(text) - len(strings.TrimLeft(text[position:], " "))
}

type run struct {
	text       string
	color      string
	emphasized bool
}

// lineRuns cuts the line wherever a token or the Changed span starts or ends, and sanitizes each piece. sanitizeText
// maps each rune on its own, so the pieces add up to the sanitized line.
func lineRuns(line diff.Line) []run {
	cuts := []int{0, len(line.Text), line.Changed.Start, line.Changed.End}
	for _, token := range line.Tokens {
		cuts = append(cuts, token.Span.Start, token.Span.End)
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	runs := []run{}
	for index := 1; index < len(cuts); index++ {
		start := cuts[index-1]
		runs = append(runs, run{
			text:       sanitizeText(line.Text[start:cuts[index]]),
			color:      tokenColor(line.Tokens, start),
			emphasized: start >= line.Changed.Start && start < line.Changed.End,
		})
	}
	return runs
}

func tokenColor(tokens []diff.Token, position int) string {
	for _, token := range tokens {
		if position >= token.Span.Start && position < token.Span.End {
			return token.Color
		}
	}
	return ""
}

// start and end are a byte range of the runs joined, the segment being rendered.
func renderSegment(runs []run, start int, end int, plain lipgloss.Style, emphasized lipgloss.Style, width int) string {
	var rendered strings.Builder
	position := 0
	for _, piece := range runs {
		from := max(start-position, 0)
		to := min(end-position, len(piece.text))
		if from < to {
			rendered.WriteString(runStyle(piece, plain, emphasized).Render(piece.text[from:to]))
		}
		position += len(piece.text)
	}
	padding := strings.Repeat(" ", max(width-ansi.StringWidth(rendered.String()), 0))
	return rendered.String() + plain.Render(padding)
}

func runStyle(piece run, plain lipgloss.Style, emphasized lipgloss.Style) lipgloss.Style {
	var base lipgloss.Style
	if piece.emphasized {
		base = emphasized
	} else {
		base = plain
	}
	if piece.color == "" {
		return base
	}
	return base.Foreground(lipgloss.Color(piece.color))
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

func changedTextStyle(kind diff.LineKind) lipgloss.Style {
	switch kind {
	case diff.LineRemoved:
		return removedChangedStyle
	case diff.LineAdded:
		return addedChangedStyle
	default:
		return lipgloss.NewStyle()
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

func renderCommit(commit gitrepo.Commit, sections []diff.FileSection, layout diffLayout, width int) []string {
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
		lines = append(lines, renderFileDiff(section.FileDiff, layout, width)...)
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
