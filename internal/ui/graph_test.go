package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

var (
	rootHash     = strings.Repeat("1", 40)
	mainlineHash = strings.Repeat("2", 40)
	sideHash     = strings.Repeat("3", 40)
	mergeHash    = strings.Repeat("4", 40)
)

func TestCommitGraph(t *testing.T) {
	t.Run("drawsABranchMergedBackIn", drawsABranchMergedBackIn)
	t.Run("aConnectorTakesTheColorOfTheLaneItJoins", aConnectorTakesTheColorOfTheLaneItJoins)
	t.Run("theLineRunsUpTheSameLaneAndDownFirstParents", theLineRunsUpTheSameLaneAndDownFirstParents)
	t.Run("onlyTheSelectedLineKeepsItsFullColor", onlyTheSelectedLineKeepsItsFullColor)
}

func mergedBranch() []gitrepo.Commit {
	return []gitrepo.Commit{
		{Hash: mergeHash, Parents: []string{mainlineHash, sideHash}},
		{Hash: sideHash, Parents: []string{rootHash}},
		{Hash: mainlineHash, Parents: []string{rootHash}},
		{Hash: rootHash},
	}
}

func drawsABranchMergedBackIn(t *testing.T) {
	got := []string{}
	for _, row := range commitGraph(mergedBranch()) {
		got = append(got, ansi.Strip(renderGraphCells(row.commit, nil))+"|"+ansi.Strip(renderGraphCells(row.below, nil)))
	}

	want := []string{"○─╮ |│ │ ", "│ ● |│ │ ", "● │ |│ │ ", "●─╯ |    "}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func aConnectorTakesTheColorOfTheLaneItJoins(t *testing.T) {
	rows := commitGraph(mergedBranch())

	got := [][]int32{}
	for _, row := range []graphRow{rows[0], rows[3]} {
		lanes := []int32{}
		for _, cell := range row.commit[:3] {
			lanes = append(lanes, cell.lane)
		}
		got = append(got, lanes)
	}

	if !slices.EqualFunc(got, [][]int32{{0, 1, 1}, {0, 1, 1}}, slices.Equal) {
		t.Fatalf("got lanes %v for the merge and root rows' first three cells", got)
	}
}

func theLineRunsUpTheSameLaneAndDownFirstParents(t *testing.T) {
	rows := commitGraph(mergedBranch())

	fromSide := pathThrough(rows, 1)
	fromMainline := pathThrough(rows, 2)

	if !slices.Equal(fromSide, []bool{false, true, false, true}) ||
		!slices.Equal(fromMainline, []bool{true, false, true, true}) {
		t.Fatalf("got line %v from the side commit, %v from the mainline commit", fromSide, fromMainline)
	}
}

func onlyTheSelectedLineKeepsItsFullColor(t *testing.T) {
	rows := commitGraph(mergedBranch())

	got := renderGraphCells(rows[3].commit, pathThrough(rows, 1))

	want := laneStyles[0].Render("●") + laneStyles[1].Render("─╯") + dimLaneStyles[1].Render(" ")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
