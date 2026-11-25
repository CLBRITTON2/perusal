package highlight

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/stretchr/testify/require"
)

func TestFileDiff(t *testing.T) {
	t.Run("aCommentOpenedInContextColorsTheAddedLineBelowIt", aCommentOpenedInContextColorsTheAddedLineBelowIt)
	t.Run("theRepositorysOwnFilesLexWithoutDrift", theRepositorysOwnFilesLexWithoutDrift)
}

func aCommentOpenedInContextColorsTheAddedLineBelowIt(t *testing.T) {
	fileDiff := diff.FileDiff{Hunks: []diff.Hunk{{
		Header: "@@ -1,2 +1,2 @@",
		Rows: []diff.Row{
			{Old: diff.Line{Kind: diff.LineContext, Number: 1, Text: "/* open"},
				New: diff.Line{Kind: diff.LineContext, Number: 1, Text: "/* open"}},
			{Old: diff.Line{Kind: diff.LineRemoved, Number: 2, Text: "x := 1"},
				New: diff.Line{Kind: diff.LineAdded, Number: 2, Text: "still a comment */"}},
		},
	}}}

	highlighted, err := FileDiff("a.go", fileDiff)
	require.NoError(t, err)

	added := highlighted.Hunks[0].Rows[1].New
	removed := highlighted.Hunks[0].Rows[1].Old
	want := diff.Token{Span: diff.Span{Start: 0, End: len(added.Text)}, Color: "#6e6a86"}
	require.Equal(t, []diff.Token{want}, added.Tokens)
	require.NotEmpty(t, removed.Tokens)
	require.NotEqual(t, want.Color, removed.Tokens[0].Color)
}

func theRepositorysOwnFilesLexWithoutDrift(t *testing.T) {
	paths := []string{"../../go.mod", "../../Makefile", "../../README.md", "highlight.go", "../ui/render.go"}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		text := strings.TrimSuffix(string(content), "\n")
		lines := strings.Split(text, "\n")
		rows := []diff.Row{}
		for number, line := range lines {
			// The CR catches a lexer that normalizes line endings, which would drift the offsets from Text.
			rows = append(rows, diff.Row{Old: diff.Line{Kind: diff.LineBlank},
				New: diff.Line{Kind: diff.LineAdded, Number: number + 1, Text: line + "\r"}})
		}
		fileDiff := diff.FileDiff{Change: diff.FileAdded, Hunks: []diff.Hunk{{Header: "@@ -0,0 +1 @@", Rows: rows}}}
		name := filepath.Base(path)

		highlighted, err := FileDiff(name, fileDiff)
		require.NoError(t, err, path)

		colored := slices.ContainsFunc(highlighted.Hunks[0].Rows, func(row diff.Row) bool {
			return len(row.New.Tokens) > 0
		})
		require.True(t, colored, path)
	}
}
