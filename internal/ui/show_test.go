package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

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
	show := NewShow("dir/file.go", showNote, showFileDiff)
	sized, _ := show.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	sizedShow := sized.(ShowModel)
	toggled, _ := sizedShow.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	toggledShow := toggled.(ShowModel)

	collapsedView := sizedShow.View()
	fullView := toggledShow.View()
	collapsed := ansi.Strip(collapsedView.Content)
	full := ansi.Strip(fullView.Content)

	require.NotContains(t, collapsed, "why b goes")
	require.Contains(t, collapsed, "SUMMARY")
	require.Contains(t, full, "why b goes")
}

func TestShowViewWithAWrappedFullNoteFillsTheScreenExactly(t *testing.T) {
	note := "CHANGE CODE : b becomes B\nSUMMARY     : Apply?\n\n" + strings.Repeat("word ", 20)
	show := NewShow("dir/file.go", note, showFileDiff)
	sized, _ := show.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	sizedShow := sized.(ShowModel)
	updated, _ := sizedShow.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	updatedShow := updated.(ShowModel)

	view := updatedShow.View()
	rows := strings.Split(view.Content, "\n")

	require.Len(t, rows, 20)
	for _, row := range rows {
		width := ansi.StringWidth(row)
		plainRow := ansi.Strip(row)
		require.LessOrEqual(t, width, 40, plainRow)
	}
}
