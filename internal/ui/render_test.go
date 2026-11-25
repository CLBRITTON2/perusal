package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/perusal/internal/diff"
)

func TestSanitizeText(t *testing.T) {
	t.Run("replacesEscapeSequenceStart", replacesEscapeSequenceStart)
	t.Run("replacesBidiOverride", replacesBidiOverride)
}

func TestRenderSideBySide(t *testing.T) {
	t.Run("everyRowFillsTheWidthExactly", everyRowFillsTheWidthExactly)
	t.Run("wrapsALongLineAndPadsTheOtherSide", wrapsALongLineAndPadsTheOtherSide)
}

func TestRenderUnifiedListsRemovedLinesAboveAddedOnes(t *testing.T) {
	fileDiff := diff.FileDiff{Hunks: []diff.Hunk{{
		Header: "@@ -1,4 +1,5 @@",
		Rows: []diff.Row{
			{Old: diff.Line{Kind: diff.LineContext, Number: 1, Text: "a"}, New: diff.Line{Kind: diff.LineContext, Number: 1, Text: "a"}},
			{Old: diff.Line{Kind: diff.LineRemoved, Number: 2, Text: "b"}, New: diff.Line{Kind: diff.LineAdded, Number: 2, Text: "B"}},
			{Old: diff.Line{Kind: diff.LineRemoved, Number: 3, Text: "c"}, New: diff.Line{Kind: diff.LineAdded, Number: 3, Text: "C"}},
			{Old: diff.Line{Kind: diff.LineBlank}, New: diff.Line{Kind: diff.LineAdded, Number: 4, Text: "d"}},
			{Old: diff.Line{Kind: diff.LineContext, Number: 4, Text: "e"}, New: diff.Line{Kind: diff.LineContext, Number: 5, Text: "e"}},
		},
	}}}

	rendered := renderFileDiff(fileDiff, renderUnified, 6)
	lines := lineTexts(rendered)

	got := strippedLines(lines[1:])
	want := []string{"1 1 a ", "2   b ", "3   c ", "  2 B ", "  3 C ", "  4 d ", "4 5 e "}
	require.Equal(t, want, got)
}

func TestWrapRowsGivesAClusterWiderThanTheRowARowOfItsOwn(t *testing.T) {
	got := wrapRows("世界", 1)

	want := []diff.Span{{Start: 0, End: 3}, {Start: 3, End: 6}}
	require.Equal(t, want, got)
}

func TestRenderFileDiff(t *testing.T) {
	t.Run("showsANewFileAcrossTheWholeWidth", showsANewFileAcrossTheWholeWidth)
}

func TestTheChangedPartKeepsItsHighlightPastATabAndAWrap(t *testing.T) {
	line := diff.Line{Kind: diff.LineAdded, Number: 1, Text: "\tkeep CHANGED tail", Changed: diff.Span{Start: 6, End: 13}}

	lines := wrapSide(line, 1, 12)

	got := strippedLines(lines)
	want := []string{"1     keep  ", "  CHANGED   ", "  tail      "}
	require.Equal(t, want, got)
	changedRun := addedChangedStyle.Render("CHANGED") + addedTextStyle.Render("   ")
	require.Contains(t, lines[1], changedRun)
}

func TestATokenSpanningTheChangedPartKeepsBothItsColorAndTheChangedBackground(t *testing.T) {
	line := diff.Line{Kind: diff.LineAdded, Number: 1, Text: "\tret value", Changed: diff.Span{Start: 4, End: 10},
		Tokens: []diff.Token{{Span: diff.Span{Start: 1, End: 7}, Color: "#31748f"}}}

	lines := wrapSide(line, 1, 40)

	tokenColor := lipgloss.Color("#31748f")
	want := addedTextStyle.Foreground(tokenColor).Render("ret") +
		addedChangedStyle.Foreground(tokenColor).Render(" va") +
		addedChangedStyle.Render("lue")
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], want)
}

func TestBorderLine(t *testing.T) {
	t.Run("cutsALongTitleToKeepTheLineCounts", cutsALongTitleToKeepTheLineCounts)
}

func cutsALongTitleToKeepTheLineCounts(t *testing.T) {
	counts := renderLineCounts(diff.LineCounts{Added: 12, Removed: 3})
	longTitleLine := borderLine("╭", titleStyle, "internal/ui/model.go", counts, "╮", 22)
	shortTitleLine := borderLine("╭", titleStyle, "a.go", counts, "╮", 22)
	narrowLine := borderLine("╭", titleStyle, "a.go", counts, "╮", 13)

	got := strippedLines([]string{longTitleLine, shortTitleLine, narrowLine})

	want := []string{"╭─ internal… ─ +12 -3 ─╮", "╭─ a.go ────── +12 -3 ─╮", "╭─ a.go ──────╮"}
	require.Equal(t, want, got)
}

func showsANewFileAcrossTheWholeWidth(t *testing.T) {
	fileDiff := diff.FileDiff{Change: diff.FileAdded, Hunks: []diff.Hunk{{
		Header: "@@ -0,0 +1,2 @@",
		Rows: []diff.Row{
			{Old: diff.Line{Kind: diff.LineBlank}, New: diff.Line{Kind: diff.LineAdded, Number: 1, Text: "package x"}},
			{Old: diff.Line{Kind: diff.LineBlank}, New: diff.Line{Kind: diff.LineAdded, Number: 2, Text: ""}},
		},
	}}}

	rendered := renderFileDiff(fileDiff, renderSideBySide, 21)
	lines := lineTexts(rendered)

	got := strippedLines(lines[1:])
	want := []string{"1 package x          ", "2                    "}
	require.Equal(t, want, got)
}

func wrapsALongLineAndPadsTheOtherSide(t *testing.T) {
	fileDiff := diff.FileDiff{Hunks: []diff.Hunk{{
		Header: "@@ -7 +7 @@",
		Rows: []diff.Row{
			{Old: diff.Line{Kind: diff.LineRemoved, Number: 7, Text: "old"},
				New: diff.Line{Kind: diff.LineAdded, Number: 7, Text: "abcd efgh ijklmnop"}},
		},
	}}}

	rendered := renderFileDiff(fileDiff, renderSideBySide, 21)
	lines := lineTexts(rendered)

	got := strippedLines(lines[1:])
	want := []string{"7 old     │7 abcd    ", "          │  efgh    ", "          │  ijklmnop"}
	require.Equal(t, want, got)
}

func replacesEscapeSequenceStart(t *testing.T) {
	got := sanitizeText("a\x1b]0;title\x07b\x9bc")

	require.Equal(t, "a�]0;title�b�c", got)
}

func replacesBidiOverride(t *testing.T) {
	got := sanitizeText("if admin‮ {")

	require.Equal(t, "if admin� {", got)
}

func everyRowFillsTheWidthExactly(t *testing.T) {
	longText := strings.Repeat("long ", 40)
	fileDiff := diff.FileDiff{Hunks: []diff.Hunk{{
		Header: "@@ -9,2 +9,2 @@",
		Rows: []diff.Row{
			{Old: diff.Line{Kind: diff.LineRemoved, Number: 9, Text: longText},
				New: diff.Line{Kind: diff.LineAdded, Number: 9, Text: "\t世界 wide"}},
			{Old: diff.Line{Kind: diff.LineRemoved, Number: 10, Text: "x"}, New: diff.Line{Kind: diff.LineBlank}},
			{Old: diff.Line{Kind: diff.LineContext, Number: 11, Text: ""}, New: diff.Line{Kind: diff.LineContext, Number: 11, Text: ""}},
		},
	}}}

	for _, width := range []int{40, 41, 120} {
		rendered := renderFileDiff(fileDiff, renderSideBySide, width)
		lines := lineTexts(rendered)
		for _, line := range lines {
			lineWidth := ansi.StringWidth(line)
			plainLine := ansi.Strip(line)
			assert.Equal(t, width, lineWidth, plainLine)
		}
	}
}

func strippedLines(lines []string) []string {
	stripped := []string{}
	for _, line := range lines {
		stripped = append(stripped, ansi.Strip(line))
	}
	return stripped
}
