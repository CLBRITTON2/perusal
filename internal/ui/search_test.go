package ui

import (
	"testing"
)

func TestSearchHighlight(t *testing.T) {
	t.Run("mergesOverlappingMatchesAndSanitizesEachRun", mergesOverlappingMatchesAndSanitizesEachRun)
	t.Run("marksTheFirstFitOfEachSubsequenceCharacter", marksTheFirstFitOfEachSubsequenceCharacter)
}

func marksTheFirstFitOfEachSubsequenceCharacter(t *testing.T) {
	got := subsequenceWords{"dwfix"}.highlight("worktrees/desktop-edge-win/fix-proxy", textStyle)

	want := textStyle.Render("worktrees/") + searchMatchStyle.Render("d") + textStyle.Render("esktop-edge-") +
		searchMatchStyle.Render("w") + textStyle.Render("in/") + searchMatchStyle.Render("fix") +
		textStyle.Render("-proxy")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func mergesOverlappingMatchesAndSanitizesEachRun(t *testing.T) {
	search, err := compileSearch("wr ra")
	if err != nil {
		t.Fatal(err)
	}

	got := search.highlight("\x1b[31mwrap", textStyle)

	want := textStyle.Render("�[31m") + searchMatchStyle.Render("wra") + textStyle.Render("p")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
