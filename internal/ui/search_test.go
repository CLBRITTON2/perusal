package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchHighlight(t *testing.T) {
	t.Run("mergesOverlappingMatchesAndSanitizesEachRun", mergesOverlappingMatchesAndSanitizesEachRun)
	t.Run("marksTheFirstFitOfEachSubsequenceCharacter", marksTheFirstFitOfEachSubsequenceCharacter)
}

func marksTheFirstFitOfEachSubsequenceCharacter(t *testing.T) {
	words := subsequenceWords{"dwfix"}
	got := words.highlight("worktrees/desktop-edge-win/fix-proxy", textStyle)

	want := textStyle.Render("worktrees/") + searchMatchStyle.Render("d") + textStyle.Render("esktop-edge-") +
		searchMatchStyle.Render("w") + textStyle.Render("in/") + searchMatchStyle.Render("fix") +
		textStyle.Render("-proxy")
	require.Equal(t, want, got)
}

func mergesOverlappingMatchesAndSanitizesEachRun(t *testing.T) {
	search, err := compileSearch("wr ra")
	require.NoError(t, err)

	got := search.highlight("\x1b[31mwrap", textStyle)

	want := textStyle.Render("�[31m") + searchMatchStyle.Render("wra") + textStyle.Render("p")
	require.Equal(t, want, got)
}
