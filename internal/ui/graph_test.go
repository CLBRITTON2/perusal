package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

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
	commits := mergedBranch()
	rows := commitGraph(commits)

	got := []string{}
	for _, row := range rows {
		commitCells := renderGraphCells(row.commit, nil)
		belowCells := renderGraphCells(row.below, nil)
		plainCommitCells := ansi.Strip(commitCells)
		plainBelowCells := ansi.Strip(belowCells)
		got = append(got, plainCommitCells+"|"+plainBelowCells)
	}

	want := []string{"○─╮ |│ │ ", "│ ● |│ │ ", "● │ |│ │ ", "●─╯ |    "}
	require.Equal(t, want, got)
}

func aConnectorTakesTheColorOfTheLaneItJoins(t *testing.T) {
	commits := mergedBranch()
	rows := commitGraph(commits)

	mergeLanes := []int32{}
	for _, cell := range rows[0].commit[:3] {
		mergeLanes = append(mergeLanes, cell.lane)
	}
	rootLanes := []int32{}
	for _, cell := range rows[3].commit[:3] {
		rootLanes = append(rootLanes, cell.lane)
	}

	require.Equal(t, []int32{0, 1, 1}, mergeLanes)
	require.Equal(t, []int32{0, 1, 1}, rootLanes)
}

func theLineRunsUpTheSameLaneAndDownFirstParents(t *testing.T) {
	commits := mergedBranch()
	rows := commitGraph(commits)

	fromSide := pathThrough(rows, 1)
	fromMainline := pathThrough(rows, 2)

	require.Equal(t, []bool{false, true, false, true}, fromSide)
	require.Equal(t, []bool{true, false, true, true}, fromMainline)
}

func onlyTheSelectedLineKeepsItsFullColor(t *testing.T) {
	commits := mergedBranch()
	rows := commitGraph(commits)
	selectedLine := pathThrough(rows, 1)

	got := renderGraphCells(rows[3].commit, selectedLine)

	want := laneStyles[0].line.Render("●") + laneStyles[1].line.Render("─╯") + laneStyles[1].dim.Render(" ")
	require.Equal(t, want, got)
}
