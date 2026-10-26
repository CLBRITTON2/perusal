package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

const tabWidth = 4

const columnSeparator = "│"

// diffLayout renders the rows of one hunk of a diff that has both an old and a new side.
type diffLayout func(hunk diff.Hunk, numberWidth int, width int) []renderedLine

type renderedLine struct {
	text    string
	source  sourceLine
	changed bool
}

// sourceLine holds the file line numbers a rendered line shows, zero for a side it does not show, and the index of
// the commit file section it belongs to.
type sourceLine struct {
	section   int
	oldNumber int
	newNumber int
}

// reaches reports whether the line is the anchor or comes after it, comparing new line numbers when the anchor has one.
func (line renderedLine) reaches(anchor sourceLine) bool {
	switch {
	case line.source.section != anchor.section:
		return line.source.section > anchor.section
	case anchor.newNumber > 0:
		return line.source.newNumber >= anchor.newNumber
	default:
		return line.source.oldNumber >= anchor.oldNumber
	}
}

// keptOffset returns the offset into next that shows the first numbered line at or below offset in previous on the
// same screen row, or the first line after it when next leaves it out.
func keptOffset(previous []renderedLine, next []renderedLine, offset int) int {
	anchorIndex := slices.IndexFunc(previous[min(offset, len(previous)):], func(line renderedLine) bool {
		return line.source.oldNumber > 0 || line.source.newNumber > 0
	})
	if anchorIndex < 0 {
		return offset
	}
	anchor := previous[offset+anchorIndex].source
	return offset + firstReaching(next, anchor) - firstReaching(previous, anchor)
}

func firstReaching(lines []renderedLine, anchor sourceLine) int {
	index := slices.IndexFunc(lines, func(line renderedLine) bool { return line.reaches(anchor) })
	if index < 0 {
		return len(lines)
	}
	return index
}

// nextChangeOffset wraps from the last change to the first.
func nextChangeOffset(lines []renderedLine, offset int, maxOffset int) int {
	offsets := changeOffsets(lines, maxOffset)
	if len(offsets) == 0 {
		return offset
	}
	for _, changeOffset := range offsets {
		if changeOffset > offset {
			return changeOffset
		}
	}
	return offsets[0]
}

// previousChangeOffset wraps from the first change to the last.
func previousChangeOffset(lines []renderedLine, offset int, maxOffset int) int {
	offsets := changeOffsets(lines, maxOffset)
	if len(offsets) == 0 {
		return offset
	}
	for index := len(offsets) - 1; index >= 0; index-- {
		if offsets[index] < offset {
			return offsets[index]
		}
	}
	return offsets[len(offsets)-1]
}

// changeOffsets returns the offset that shows each change hunkContext rows below the top, or as near as the offset
// range allows, each once.
func changeOffsets(lines []renderedLine, maxOffset int) []int {
	offsets := []int{}
	for index, line := range lines {
		if line.changed && (index == 0 || !lines[index-1].changed) {
			offsets = append(offsets, min(max(index-hunkContext, 0), maxOffset))
		}
	}
	return slices.Compact(offsets)
}

func lineTexts(lines []renderedLine) []string {
	texts := []string{}
	for _, line := range lines {
		texts = append(texts, line.text)
	}
	return texts
}

func linesWithSource(texts []string, source sourceLine, changed bool) []renderedLine {
	lines := []renderedLine{}
	for _, text := range texts {
		lines = append(lines, renderedLine{text: text, source: source, changed: changed})
	}
	return lines
}

func renderFileDiff(fileDiff diff.FileDiff, layout diffLayout, width int) []renderedLine {
	switch {
	case fileDiff.Binary:
		return []renderedLine{{text: mutedStyle.Render("binary file, no text diff")}}
	case len(fileDiff.Hunks) == 0:
		return []renderedLine{{text: mutedStyle.Render("no text changes")}}
	}
	var hunkLayout diffLayout
	if fileDiff.Change == diff.FileAdded || fileDiff.Change == diff.FileDeleted {
		hunkLayout = renderOneSide
	} else {
		hunkLayout = layout
	}
	highestNumber := 0
	for _, hunk := range fileDiff.Hunks {
		for _, row := range hunk.Rows {
			highestNumber = max(highestNumber, row.Old.Number, row.New.Number)
		}
	}
	numberWidth := len(strconv.Itoa(highestNumber))
	lines := []renderedLine{}
	for _, hunk := range fileDiff.Hunks {
		header := renderedLine{text: hunkHeaderStyle.Render(fitToWidth(sanitizeText(hunk.Header), width))}
		lines = append(append(lines, header), hunkLayout(hunk, numberWidth, width)...)
	}
	return lines
}

func renderSideBySide(hunk diff.Hunk, numberWidth int, width int) []renderedLine {
	leftWidth := (width - 1) / 2
	rightWidth := width - 1 - leftWidth
	separator := separatorStyle.Render(columnSeparator)
	lines := []renderedLine{}
	for _, row := range hunk.Rows {
		left := wrapSide(row.Old, numberWidth, leftWidth)
		right := wrapSide(row.New, numberWidth, rightWidth)
		for index := 0; index < max(len(left), len(right)); index++ {
			lines = append(lines, renderedLine{
				text: segmentOrFiller(left, index, row.Old.Kind, leftWidth) + separator +
					segmentOrFiller(right, index, row.New.Kind, rightWidth),
				source:  sourceLine{oldNumber: row.Old.Number, newNumber: row.New.Number},
				changed: row.Old.Kind != diff.LineContext,
			})
		}
	}
	return lines
}

// renderUnified lists each run of changes as its removed lines above its added lines, numbering a line by its own
// side, as GitHub's unified view does.
func renderUnified(hunk diff.Hunk, numberWidth int, width int) []renderedLine {
	blank := diff.Line{Kind: diff.LineBlank}
	lines := []renderedLine{}
	added := []renderedLine{}
	for _, row := range hunk.Rows {
		if row.Old.Kind == diff.LineContext {
			lines = append(append(lines, added...), renderUnifiedLine(row.New, row.Old, row.New, numberWidth, width)...)
			added = []renderedLine{}
		}
		if row.Old.Kind == diff.LineRemoved {
			lines = append(lines, renderUnifiedLine(row.Old, row.Old, blank, numberWidth, width)...)
		}
		if row.New.Kind == diff.LineAdded {
			added = append(added, renderUnifiedLine(row.New, blank, row.New, numberWidth, width)...)
		}
	}
	return append(lines, added...)
}

func renderUnifiedLine(shown diff.Line, oldSide diff.Line, newSide diff.Line, numberWidth int, width int) []renderedLine {
	gutterText := numberOrBlank(oldSide, numberWidth) + " " + numberOrBlank(newSide, numberWidth) + " "
	source := sourceLine{oldNumber: oldSide.Number, newNumber: newSide.Number}
	return linesWithSource(wrapLine(shown, gutterText, width), source, shown.Kind != diff.LineContext)
}

// renderOneSide lays out an added or deleted file, whose rows each have one side.
func renderOneSide(hunk diff.Hunk, numberWidth int, width int) []renderedLine {
	lines := []renderedLine{}
	for _, row := range hunk.Rows {
		var shown diff.Line
		if row.New.Kind == diff.LineBlank {
			shown = row.Old
		} else {
			shown = row.New
		}
		lines = append(lines, linesWithSource(wrapSide(shown, numberWidth, width),
			sourceLine{oldNumber: row.Old.Number, newNumber: row.New.Number}, shown.Kind != diff.LineContext)...)
	}
	return lines
}

func numberOrBlank(line diff.Line, numberWidth int) string {
	if line.Kind == diff.LineBlank {
		return strings.Repeat(" ", numberWidth)
	}
	return fmt.Sprintf("%*d", numberWidth, line.Number)
}

func wrapSide(line diff.Line, numberWidth int, width int) []string {
	return wrapLine(line, numberOrBlank(line, numberWidth)+" ", width)
}

func wrapLine(line diff.Line, gutterText string, width int) []string {
	gutterWidth := len(gutterText)
	if width <= gutterWidth || line.Kind == diff.LineBlank {
		return []string{strings.Repeat(" ", max(width, 0))}
	}
	textWidth := width - gutterWidth
	style := styleOf(line.Kind)
	runs := lineRuns(line)
	var joined strings.Builder
	for _, run := range runs {
		joined.WriteString(run.text)
	}
	rendered := []string{}
	for index, segment := range wrapRows(joined.String(), textWidth) {
		var rowGutter string
		if index == 0 {
			rowGutter = gutterText
		} else {
			rowGutter = strings.Repeat(" ", gutterWidth)
		}
		rendered = append(rendered, style.gutter.Render(rowGutter)+renderSegment(runs, segment, style.text, style.changed,
			textWidth))
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
	usedWidth := 0
	breakAt := start
	afterText := false
	position := start
	for position < len(text) {
		cluster, clusterWidth := ansi.FirstGraphemeCluster(text[position:], ansi.GraphemeWidth)
		space := cluster == " "
		if space && afterText {
			breakAt = position
		}
		if usedWidth+clusterWidth > width {
			switch {
			case breakAt > start:
				return diff.Span{Start: start, End: breakAt}, len(text) - len(strings.TrimLeft(text[breakAt:], " "))
			case position == start:
				return diff.Span{Start: start, End: position + len(cluster)}, position + len(cluster)
			default:
				return diff.Span{Start: start, End: position}, position
			}
		}
		usedWidth += clusterWidth
		afterText = !space
		position += len(cluster)
	}
	return diff.Span{Start: start, End: len(text)}, len(text)
}

type styledRun struct {
	text       string
	color      string
	emphasized bool
}

// lineRuns cuts the line wherever a token or the Changed span starts or ends, and sanitizes each piece. sanitizeText
// maps each rune on its own, so the pieces add up to the sanitized line.
func lineRuns(line diff.Line) []styledRun {
	runBounds := []int{0, len(line.Text), line.Changed.Start, line.Changed.End}
	for _, token := range line.Tokens {
		runBounds = append(runBounds, token.Span.Start, token.Span.End)
	}
	slices.Sort(runBounds)
	runBounds = slices.Compact(runBounds)
	runs := []styledRun{}
	for index := 1; index < len(runBounds); index++ {
		start := runBounds[index-1]
		token := slices.IndexFunc(line.Tokens, func(token diff.Token) bool {
			return start >= token.Span.Start && start < token.Span.End
		})
		var color string
		if token >= 0 {
			color = line.Tokens[token].Color
		}
		runs = append(runs, styledRun{
			text:       sanitizeText(line.Text[start:runBounds[index]]),
			color:      color,
			emphasized: start >= line.Changed.Start && start < line.Changed.End,
		})
	}
	return runs
}

// segment indexes the runs joined, not the line's Text.
func renderSegment(runs []styledRun, segment diff.Span, plain lipgloss.Style, emphasized lipgloss.Style, width int) string {
	var rendered strings.Builder
	position := 0
	for _, run := range runs {
		from := max(segment.Start-position, 0)
		to := min(segment.End-position, len(run.text))
		if from < to {
			var base lipgloss.Style
			if run.emphasized {
				base = emphasized
			} else {
				base = plain
			}
			var style lipgloss.Style
			if run.color == "" {
				style = base
			} else {
				style = base.Foreground(lipgloss.Color(run.color))
			}
			rendered.WriteString(style.Render(run.text[from:to]))
		}
		position += len(run.text)
	}
	padding := strings.Repeat(" ", max(width-ansi.StringWidth(rendered.String()), 0))
	return rendered.String() + plain.Render(padding)
}

func segmentOrFiller(segments []string, index int, kind diff.LineKind, width int) string {
	if index < len(segments) {
		return segments[index]
	}
	return styleOf(kind).text.Render(strings.Repeat(" ", width))
}

type lineStyle struct {
	gutter  lipgloss.Style
	text    lipgloss.Style
	changed lipgloss.Style
}

func styleOf(kind diff.LineKind) lineStyle {
	switch kind {
	case diff.LineRemoved:
		return lineStyle{gutter: removedGutterStyle, text: removedTextStyle, changed: removedChangedStyle}
	case diff.LineAdded:
		return lineStyle{gutter: addedGutterStyle, text: addedTextStyle, changed: addedChangedStyle}
	case diff.LineContext:
		return lineStyle{gutter: mutedStyle, text: contextTextStyle}
	default:
		return lineStyle{}
	}
}

func renderCommit(commit gitrepo.Commit, sections []diff.FileSection, layout diffLayout, width int) []renderedLine {
	texts := []string{
		hashStyle.Render(fitToWidth(commit.Hash, width)),
		fitToWidth(authorStyle.Render(sanitizeText(commit.Author))+mutedStyle.Render(" · "+
			commit.AuthoredAt.Format("2006-01-02 15:04")), width),
		"",
	}
	for _, line := range strings.Split(sanitizeLines(commit.Message), "\n") {
		for _, wrapped := range strings.Split(ansi.Wrap(line, width, ""), "\n") {
			texts = append(texts, textStyle.Render(fitToWidth(wrapped, width)))
		}
	}
	if len(sections) == 0 {
		texts = append(texts, "", mutedStyle.Render("no file changes"))
	}
	lines := linesWithSource(texts, sourceLine{}, false)
	for index, section := range sections {
		header := []string{"", fileHeaderStyle.Render(fitToWidth(" "+sanitizeText(section.Path), width))}
		sectionLines := append(linesWithSource(header, sourceLine{}, false),
			renderFileDiff(section.FileDiff, layout, width)...)
		for _, line := range sectionLines {
			line.source.section = index
			lines = append(lines, line)
		}
	}
	return lines
}

func renderTreeFileRow(section diff.FileSection, last bool, width int) string {
	var treeGlyph string
	if last {
		treeGlyph = "└ "
	} else {
		treeGlyph = "├ "
	}
	var changeMark string
	// Letters match git log --name-status.
	switch section.FileDiff.Change {
	case diff.FileAdded:
		changeMark = addedMarkStyle.Render("A")
	case diff.FileDeleted:
		changeMark = deletedMarkStyle.Render("D")
	case diff.FileRenamed:
		changeMark = renamedMarkStyle.Render("R")
	default:
		changeMark = modifiedMarkStyle.Render("M")
	}
	return fitToWidth(mutedStyle.Render(treeGlyph)+changeMark+" "+textStyle.Render(sanitizeText(section.Path)), width)
}

func fitToWidth(text string, width int) string {
	truncated := ansi.Truncate(text, width, "…")
	return truncated + strings.Repeat(" ", max(width-ansi.StringWidth(truncated), 0))
}

func fitLines(lines []string, width int, height int) []string {
	fitted := []string{}
	for index := 0; index < height; index++ {
		if index < len(lines) {
			fitted = append(fitted, fitToWidth(lines[index], width))
		} else {
			fitted = append(fitted, strings.Repeat(" ", width))
		}
	}
	return fitted
}

func spaceBetween(left string, right string, width int) string {
	gapWidth := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gapWidth < 1 {
		return fitToWidth(left, width)
	}
	return left + strings.Repeat(" ", gapWidth) + right
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
	fillWidth := max(outerWidth-lipgloss.Width(left)-lipgloss.Width(right), 0)
	return left + borderStyle.Render(strings.Repeat("─", fillWidth)) + right
}

func borderLine(leftCorner string, style lipgloss.Style, title string, topRight string, rightCorner string,
	innerWidth int) string {
	if title == "" || innerWidth < 4 {
		return borderStyle.Render(leftCorner + strings.Repeat("─", max(innerWidth, 0)) + rightCorner)
	}
	var right string
	var reserved int
	if topRight != "" && ansi.StringWidth(topRight)+8 <= innerWidth {
		right = " " + topRight + borderStyle.Render(" ─"+rightCorner)
		reserved = ansi.StringWidth(topRight) + 3
	} else {
		right = borderStyle.Render(rightCorner)
		reserved = 0
	}
	fitted := ansi.Truncate(title, innerWidth-4-reserved, "…")
	fillWidth := innerWidth - 3 - reserved - ansi.StringWidth(fitted)
	return borderStyle.Render(leftCorner+"─ ") + style.Render(fitted) + borderStyle.Render(" "+
		strings.Repeat("─", fillWidth)) + right
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
