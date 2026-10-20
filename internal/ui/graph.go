package ui

import (
	"slices"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// commit and below are the same width, two cells per lane.
type graphRow struct {
	commit string
	below  string
}

// Each lane holds the hash of the commit it leads down to.
func commitGraph(commits []gitrepo.Commit) []graphRow {
	rows := []graphRow{}
	lanes := []string{}
	for _, commit := range commits {
		row, next := graphStep(lanes, commit)
		rows = append(rows, row)
		lanes = next
	}
	return rows
}

func graphStep(above []string, commit gitrepo.Commit) (graphRow, []string) {
	column := commitLane(above, commit.Hash)
	below := slices.Clone(above)
	for len(below) <= column {
		below = append(below, "")
	}
	ending := map[int]bool{}
	for lane, hash := range above {
		if lane != column && hash == commit.Hash {
			ending[lane] = true
			below[lane] = ""
		}
	}
	if len(commit.Parents) == 0 {
		below[column] = ""
	} else {
		below[column] = commit.Parents[0]
	}
	opening := map[int]bool{}
	joining := map[int]bool{}
	for _, parent := range commit.Parents[min(1, len(commit.Parents)):] {
		if slices.Contains(below, parent) {
			joining[slices.Index(below, parent)] = true
			continue
		}
		lane := freeLane(below, above)
		for len(below) <= lane {
			below = append(below, "")
		}
		below[lane] = parent
		opening[lane] = true
	}

	first := column
	last := column
	for _, lanes := range []map[int]bool{ending, opening, joining} {
		for lane := range lanes {
			first = min(first, lane)
			last = max(last, lane)
		}
	}
	width := max(len(above), len(below))
	commitCells := ""
	belowCells := ""
	for lane := range width {
		passing := lane < len(above) && above[lane] != "" && lane < len(below) && below[lane] != ""
		crossed := first < lane && lane < last
		var symbol string
		switch {
		case lane == column:
			symbol = "●"
		case ending[lane] && lane > column:
			symbol = "╯"
		case ending[lane]:
			symbol = "╰"
		case opening[lane] && lane > column:
			symbol = "╮"
		case opening[lane]:
			symbol = "╭"
		case joining[lane] && lane > column:
			symbol = "┤"
		case joining[lane]:
			symbol = "├"
		case passing && crossed:
			symbol = "┼"
		case passing:
			symbol = "│"
		case crossed:
			symbol = "─"
		default:
			symbol = " "
		}
		var filler string
		if first <= lane && lane < last {
			filler = "─"
		} else {
			filler = " "
		}
		var continued string
		if lane < len(below) && below[lane] != "" {
			continued = "│ "
		} else {
			continued = "  "
		}
		style := laneStyles[lane%len(laneStyles)]
		commitCells += style.Render(symbol + filler)
		belowCells += style.Render(continued)
	}
	for len(below) > 0 && below[len(below)-1] == "" {
		below = below[:len(below)-1]
	}
	return graphRow{commit: commitCells, below: belowCells}, below
}

// Also empty above, so a lane that just ended is not reused on the same row.
func freeLane(lanes []string, above []string) int {
	for lane := range lanes {
		if lanes[lane] == "" && (lane >= len(above) || above[lane] == "") {
			return lane
		}
	}
	return len(lanes)
}

func commitLane(above []string, hash string) int {
	if slices.Contains(above, hash) {
		return slices.Index(above, hash)
	}
	return freeLane(above, above)
}
