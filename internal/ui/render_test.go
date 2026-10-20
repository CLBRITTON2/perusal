package ui

import (
	"strings"
	"testing"

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

func TestRenderFileDiff(t *testing.T) {
	t.Run("showsANewFileAcrossTheWholeWidth", showsANewFileAcrossTheWholeWidth)
}

func TestBorderLine(t *testing.T) {
	t.Run("cutsALongTitleToKeepTheLineCounts", cutsALongTitleToKeepTheLineCounts)
}

func cutsALongTitleToKeepTheLineCounts(t *testing.T) {
	counts := renderLineCounts(diff.LineCounts{Added: 12, Removed: 3})

	got := []string{
		ansi.Strip(borderLine("╭", titleStyle, "internal/ui/model.go", counts, "╮", 22)),
		ansi.Strip(borderLine("╭", titleStyle, "a.go", counts, "╮", 22)),
		ansi.Strip(borderLine("╭", titleStyle, "a.go", counts, "╮", 13)),
	}

	want := []string{"╭─ internal… ─ +12 -3 ─╮", "╭─ a.go ────── +12 -3 ─╮", "╭─ a.go ──────╮"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func showsANewFileAcrossTheWholeWidth(t *testing.T) {
	fileDiff := diff.FileDiff{Change: diff.FileAdded, Hunks: []diff.Hunk{{
		Header: "@@ -0,0 +1,2 @@",
		Rows: []diff.Row{
			{Left: diff.Line{Kind: diff.LineBlank}, Right: diff.Line{Kind: diff.LineAdded, Number: 1, Text: "package x"}},
			{Left: diff.Line{Kind: diff.LineBlank}, Right: diff.Line{Kind: diff.LineAdded, Number: 2, Text: ""}},
		},
	}}}

	lines := renderFileDiff(fileDiff, 21)

	got := []string{}
	for _, line := range lines[1:] {
		got = append(got, ansi.Strip(line))
	}
	want := []string{"1 package x          ", "2                    "}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func wrapsALongLineAndPadsTheOtherSide(t *testing.T) {
	fileDiff := diff.FileDiff{Hunks: []diff.Hunk{{
		Header: "@@ -7 +7 @@",
		Rows: []diff.Row{
			{Left: diff.Line{Kind: diff.LineRemoved, Number: 7, Text: "old"},
				Right: diff.Line{Kind: diff.LineAdded, Number: 7, Text: "abcdefghijklmnopqrst"}},
		},
	}}}

	lines := renderSideBySide(fileDiff, 21)

	got := []string{}
	for _, line := range lines[1:] {
		got = append(got, ansi.Strip(line))
	}
	want := []string{"7 old     │7 abcdefgh", "          │  ijklmnop", "          │  qrst    "}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
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
			{Left: diff.Line{Kind: diff.LineRemoved, Number: 9, Text: strings.Repeat("long ", 40)},
				Right: diff.Line{Kind: diff.LineAdded, Number: 9, Text: "\t世界 wide"}},
			{Left: diff.Line{Kind: diff.LineRemoved, Number: 10, Text: "x"}, Right: diff.Line{Kind: diff.LineBlank}},
			{Left: diff.Line{Kind: diff.LineContext, Number: 11, Text: ""}, Right: diff.Line{Kind: diff.LineContext, Number: 11, Text: ""}},
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
