package ui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/diff"
)

const tabWidth = 4

const columnSeparator = "│"

// renderSideBySide lays out fileDiff as rows exactly width cells wide: old file left, new file right.
func renderSideBySide(fileDiff diff.FileDiff, width int) []string {
	if fileDiff.Binary {
		return []string{mutedStyle.Render("binary file, no text diff")}
	}
	if len(fileDiff.Hunks) == 0 {
		return []string{mutedStyle.Render("no changes against HEAD")}
	}
	numberWidth := len(strconv.Itoa(maxLineNumber(fileDiff)))
	leftWidth := (width - 1) / 2
	rightWidth := width - 1 - leftWidth
	separator := separatorStyle.Render(columnSeparator)
	lines := []string{}
	for _, hunk := range fileDiff.Hunks {
		lines = append(lines, hunkHeaderStyle.Render(fitToWidth(sanitizeText(hunk.Header), width)))
		for _, row := range hunk.Rows {
			lines = append(lines, renderSide(row.Left, numberWidth, leftWidth)+separator+
				renderSide(row.Right, numberWidth, rightWidth))
		}
	}
	return lines
}

// renderSide renders one half of a row: a right-aligned line number, a space, then the text cut or padded to width.
func renderSide(line diff.Line, numberWidth int, width int) string {
	gutterWidth := numberWidth + 1
	if width <= gutterWidth {
		return strings.Repeat(" ", max(width, 0))
	}
	text := fitToWidth(sanitizeText(line.Text), width-gutterWidth)
	switch line.Kind {
	case diff.LineRemoved:
		return removedGutter.Render(fmt.Sprintf("%*d ", numberWidth, line.Number)) + removedTextStyle.Render(text)
	case diff.LineAdded:
		return addedGutter.Render(fmt.Sprintf("%*d ", numberWidth, line.Number)) + addedTextStyle.Render(text)
	case diff.LineContext:
		return mutedStyle.Render(fmt.Sprintf("%*d ", numberWidth, line.Number)) + contextTextStyle.Render(text)
	default:
		return strings.Repeat(" ", width)
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

// fitToWidth cuts text, which may hold ANSI styling, to width cells with an ellipsis, or pads it with spaces.
func fitToWidth(text string, width int) string {
	truncated := ansi.Truncate(text, width, "…")
	return truncated + strings.Repeat(" ", max(width-ansi.StringWidth(truncated), 0))
}

// fitLines returns exactly height lines, each fitted to width.
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

// renderPane draws a bordered box whose first inner line is title.
func renderPane(title string, body []string, innerWidth int, innerHeight int) string {
	lines := append([]string{titleStyle.Render(title)}, body...)
	return paneStyle.Render(strings.Join(fitLines(lines, innerWidth, innerHeight+1), "\n"))
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

// singleLine joins multi-line git output into one line for the footer.
func singleLine(text string) string {
	lines := strings.FieldsFunc(text, isLineBreak)
	return strings.Join(lines, " | ")
}

func isLineBreak(character rune) bool {
	return character == '\n' || character == '\r'
}
