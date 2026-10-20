package ui

import (
	"testing"
)

func TestHighlight(t *testing.T) {
	t.Run("mergesOverlappingMatchesAndSanitizesEachRun", mergesOverlappingMatchesAndSanitizesEachRun)
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
