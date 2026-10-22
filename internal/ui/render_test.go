package ui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

	got := strippedLines(renderUnified(fileDiff, 6)[1:])

	want := []string{"1 1 a ", "2   b ", "3   c ", "  2 B ", "  3 C ", "  4 d ", "4 5 e "}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestRenderFileDiff(t *testing.T) {
	t.Run("showsANewFileAcrossTheWholeWidth", showsANewFileAcrossTheWholeWidth)
}

func TestTheChangedPartKeepsItsHighlightPastATabAndAWrap(t *testing.T) {
	line := diff.Line{Kind: diff.LineAdded, Number: 1, Text: "\tkeep CHANGED tail", Changed: diff.Span{Start: 6, End: 13}}

	lines := wrapSide(line, 1, 12)

	got := strippedLines(lines)
	want := []string{"1     keep  ", "  CHANGED   ", "  tail      "}
	if !slices.Equal(got, want) || !strings.Contains(lines[1], addedChangedStyle.Render("CHANGED")+addedTextStyle.Render("   ")) {
		t.Fatalf("got\n%q\nwant\n%q with only CHANGED highlighted", got, want)
	}
}

func TestATokenSpanningTheChangedPartKeepsBothItsColorAndTheChangedBackground(t *testing.T) {
	line := diff.Line{Kind: diff.LineAdded, Number: 1, Text: "\tret value", Changed: diff.Span{Start: 4, End: 10},
		Tokens: []diff.Token{{Span: diff.Span{Start: 1, End: 7}, Color: "#31748f"}}}

	lines := wrapSide(line, 1, 40)

	want := addedTextStyle.Foreground(lipgloss.Color("#31748f")).Render("ret") +
		addedChangedStyle.Foreground(lipgloss.Color("#31748f")).Render(" va") +
		addedChangedStyle.Render("lue")
	if len(lines) != 1 || !strings.Contains(lines[0], want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestBorderLine(t *testing.T) {
	t.Run("cutsALongTitleToKeepTheLineCounts", cutsALongTitleToKeepTheLineCounts)
}

func cutsALongTitleToKeepTheLineCounts(t *testing.T) {
	counts := renderLineCounts(diff.LineCounts{Added: 12, Removed: 3})

	got := strippedLines([]string{
		borderLine("╭", titleStyle, "internal/ui/model.go", counts, "╮", 22),
		borderLine("╭", titleStyle, "a.go", counts, "╮", 22),
		borderLine("╭", titleStyle, "a.go", counts, "╮", 13),
	})

	want := []string{"╭─ internal… ─ +12 -3 ─╮", "╭─ a.go ────── +12 -3 ─╮", "╭─ a.go ──────╮"}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func showsANewFileAcrossTheWholeWidth(t *testing.T) {
	fileDiff := diff.FileDiff{Change: diff.FileAdded, Hunks: []diff.Hunk{{
		Header: "@@ -0,0 +1,2 @@",
		Rows: []diff.Row{
			{Old: diff.Line{Kind: diff.LineBlank}, New: diff.Line{Kind: diff.LineAdded, Number: 1, Text: "package x"}},
			{Old: diff.Line{Kind: diff.LineBlank}, New: diff.Line{Kind: diff.LineAdded, Number: 2, Text: ""}},
		},
	}}}

	lines := renderFileDiff(fileDiff, renderSideBySide, 21)

	got := strippedLines(lines[1:])
	want := []string{"1 package x          ", "2                    "}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func wrapsALongLineAndPadsTheOtherSide(t *testing.T) {
	fileDiff := diff.FileDiff{Hunks: []diff.Hunk{{
		Header: "@@ -7 +7 @@",
		Rows: []diff.Row{
			{Old: diff.Line{Kind: diff.LineRemoved, Number: 7, Text: "old"},
				New: diff.Line{Kind: diff.LineAdded, Number: 7, Text: "abcd efgh ijklmnop"}},
		},
	}}}

	lines := renderSideBySide(fileDiff, 21)

	got := strippedLines(lines[1:])
	want := []string{"7 old     │7 abcd    ", "          │  efgh    ", "          │  ijklmnop"}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func replacesEscapeSequenceStart(t *testing.T) {
	got := sanitizeText("a\x1b]0;title\x07b\x9bc")

	if got != "a�]0;title�b�c" {
		t.Fatalf("got %q", got)
	}
}

func replacesBidiOverride(t *testing.T) {
	got := sanitizeText("if admin‮ {")

	if got != "if admin� {" {
		t.Fatalf("got %q", got)
	}
}

func everyRowFillsTheWidthExactly(t *testing.T) {
	fileDiff := diff.FileDiff{Hunks: []diff.Hunk{{
		Header: "@@ -9,2 +9,2 @@",
		Rows: []diff.Row{
			{Old: diff.Line{Kind: diff.LineRemoved, Number: 9, Text: strings.Repeat("long ", 40)},
				New: diff.Line{Kind: diff.LineAdded, Number: 9, Text: "\t世界 wide"}},
			{Old: diff.Line{Kind: diff.LineRemoved, Number: 10, Text: "x"}, New: diff.Line{Kind: diff.LineBlank}},
			{Old: diff.Line{Kind: diff.LineContext, Number: 11, Text: ""}, New: diff.Line{Kind: diff.LineContext, Number: 11, Text: ""}},
		},
	}}}

	for _, width := range []int{40, 41, 120} {
		for index, line := range renderSideBySide(fileDiff, width) {
			if got := ansi.StringWidth(line); got != width {
				t.Errorf("width %d, row %d: %d cells: %q", width, index, got, ansi.Strip(line))
			}
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
