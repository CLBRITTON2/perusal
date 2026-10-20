package ui

import (
	"testing"
)

func TestJoinCommitMessage(t *testing.T) {
	t.Run("putsTheDescriptionBelowABlankLine", putsTheDescriptionBelowABlankLine)
	t.Run("leavesASummaryWithNoDescriptionAlone", leavesASummaryWithNoDescriptionAlone)
}

func putsTheDescriptionBelowABlankLine(t *testing.T) {
	got := joinCommitMessage("Fix list width", "\nFirst line\nsecond line\n\n")

	if got != "Fix list width\n\nFirst line\nsecond line" {
		t.Fatalf("got %q", got)
	}
}

func leavesASummaryWithNoDescriptionAlone(t *testing.T) {
	got := joinCommitMessage("Fix list width", " \n ")

	if got != "Fix list width" {
		t.Fatalf("got %q", got)
	}
}
