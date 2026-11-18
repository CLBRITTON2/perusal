package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/diff"
)

var showFileDiff = diff.FileDiff{Change: diff.FileModified, Hunks: []diff.Hunk{{
	Header: "@@ -1 +1 @@",
	Rows: []diff.Row{
		{Old: diff.Line{Kind: diff.LineRemoved, Number: 1, Text: "b"}, New: diff.Line{Kind: diff.LineAdded, Number: 1, Text: "B"}},
	},
}}}

const showNote = "CHANGE CODE : b becomes B\nSUMMARY     : Apply?\n\nwhy b goes"

func TestShowViewHidesNoteProseUntilFullNoteKey(t *testing.T) {
	sized, _ := NewShow("dir/file.go", showNote, showFileDiff).Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	toggled, _ := sized.(ShowModel).Update(tea.KeyPressMsg{Code: 'i', Text: "i"})

	collapsed := ansi.Strip(sized.(ShowModel).View().Content)
	full := ansi.Strip(toggled.(ShowModel).View().Content)

	if strings.Contains(collapsed, "why b goes") || !strings.Contains(collapsed, "SUMMARY") {
		t.Fatalf("got collapsed screen:\n%s\nwant SUMMARY and no prose", collapsed)
	}
	if !strings.Contains(full, "why b goes") {
		t.Fatalf("got screen after i:\n%s\nwant the prose", full)
	}
}

func TestShowViewWithAWrappedFullNoteFillsTheScreenExactly(t *testing.T) {
	note := "CHANGE CODE : b becomes B\nSUMMARY     : Apply?\n\n" + strings.Repeat("word ", 20)
	sized, _ := NewShow("dir/file.go", note, showFileDiff).Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	updated, _ := sized.(ShowModel).Update(tea.KeyPressMsg{Code: 'i', Text: "i"})

	rows := strings.Split(updated.(ShowModel).View().Content, "\n")

	if len(rows) != 20 {
		t.Fatalf("got %d rows, want 20:\n%s", len(rows), ansi.Strip(strings.Join(rows, "\n")))
	}
	for index, row := range rows {
		if ansi.StringWidth(row) > 40 {
			t.Fatalf("row %d is %d cells wide, want at most 40: %q", index, ansi.StringWidth(row), ansi.Strip(row))
		}
	}
}
