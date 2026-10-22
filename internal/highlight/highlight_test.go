package highlight

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CLBRITTON2/perusal/internal/diff"
)

func TestFileDiff(t *testing.T) {
	t.Run("aCommentOpenedInContextColorsTheAddedLineBelowIt", aCommentOpenedInContextColorsTheAddedLineBelowIt)
	t.Run("theRepositorysOwnFilesLexWithoutDrift", theRepositorysOwnFilesLexWithoutDrift)
}

func aCommentOpenedInContextColorsTheAddedLineBelowIt(t *testing.T) {
	fileDiff := diff.FileDiff{Hunks: []diff.Hunk{{
		Header: "@@ -1,2 +1,2 @@",
		Rows: []diff.Row{
			{Left: diff.Line{Kind: diff.LineContext, Number: 1, Text: "/* open"},
				Right: diff.Line{Kind: diff.LineContext, Number: 1, Text: "/* open"}},
			{Left: diff.Line{Kind: diff.LineRemoved, Number: 2, Text: "x := 1"},
				Right: diff.Line{Kind: diff.LineAdded, Number: 2, Text: "still a comment */"}},
		},
	}}}

	highlighted, err := FileDiff("a.go", fileDiff)
	if err != nil {
		t.Fatal(err)
	}

	added := highlighted.Hunks[0].Rows[1].Right
	removed := highlighted.Hunks[0].Rows[1].Left
	want := diff.Token{Span: diff.Span{Start: 0, End: len(added.Text)}, Color: "#6e6a86"}
	if len(added.Tokens) != 1 || added.Tokens[0] != want || len(removed.Tokens) == 0 ||
		removed.Tokens[0].Color == want.Color {
		t.Fatalf("got added %v, removed %v, want the added line one comment token and the removed line code",
			added.Tokens, removed.Tokens)
	}
}

func theRepositorysOwnFilesLexWithoutDrift(t *testing.T) {
	paths := []string{"../../go.mod", "../../Makefile", "../../README.md", "highlight.go", "../ui/render.go"}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rows := []diff.Row{}
		for number, text := range strings.Split(strings.TrimSuffix(string(content), "\n"), "\n") {
			rows = append(rows, diff.Row{Left: diff.Line{Kind: diff.LineBlank},
				Right: diff.Line{Kind: diff.LineAdded, Number: number + 1, Text: text + "\r"}})
		}
		fileDiff := diff.FileDiff{Change: diff.FileAdded, Hunks: []diff.Hunk{{Header: "@@ -0,0 +1 @@", Rows: rows}}}

		highlighted, err := FileDiff(filepath.Base(path), fileDiff)

		if err != nil || !anyTokens(highlighted) {
			t.Fatalf("%s: got error %v, want every line lexed and some of them colored", path, err)
		}
	}
}

func anyTokens(fileDiff diff.FileDiff) bool {
	for _, row := range fileDiff.Hunks[0].Rows {
		if len(row.Right.Tokens) > 0 {
			return true
		}
	}
	return false
}
