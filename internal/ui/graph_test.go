package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestCommitGraph(t *testing.T) {
	t.Run("drawsABranchMergedBackIn", drawsABranchMergedBackIn)
}

func drawsABranchMergedBackIn(t *testing.T) {
	root := strings.Repeat("1", 40)
	mainline := strings.Repeat("2", 40)
	side := strings.Repeat("3", 40)
	merge := strings.Repeat("4", 40)
	commits := []gitrepo.Commit{
		{Hash: merge, Parents: []string{mainline, side}},
		{Hash: side, Parents: []string{root}},
		{Hash: mainline, Parents: []string{root}},
		{Hash: root},
	}

	got := []string{}
	for _, row := range commitGraph(commits) {
		got = append(got, ansi.Strip(row.commit)+"|"+ansi.Strip(row.below))
	}

	want := []string{"●─╮ |│ │ ", "│ ● |│ │ ", "● │ |│ │ ", "●─╯ |    "}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
