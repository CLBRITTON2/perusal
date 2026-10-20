package ui

import (
	"slices"
	"strings"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

const noRow = -1

// graphEdge is the line from child down to one of its parents.
type graphEdge struct {
	child       string
	parent      string
	firstParent bool
}

// A cell is one terminal column, on the selected line when its childRow and parentRow both are. A commit's own
// symbol holds its row in both, and a cell off any first-parent line holds noRow. int32 keeps a cell at 16 bytes, since the ziti graph holds 760,000 of them.
type graphCell struct {
	glyph     rune
	lane      int32
	childRow  int32
	parentRow int32
}

// commit and below are the same width, two cells per lane. lineChild and lineParent are the rows the commit's
// first-parent line continues to, or noRow.
type graphRow struct {
	commit     []graphCell
	below      []graphCell
	lineChild  int32
	lineParent int32
}

func commitGraph(commits []gitrepo.Commit) []graphRow {
	rowOf := map[string]int32{}
	for index, commit := range commits {
		rowOf[commit.Hash] = int32(index)
	}
	rows := []graphRow{}
	lanes := []graphEdge{}
	for _, commit := range commits {
		row, next := graphStep(lanes, commit, rowOf)
		rows = append(rows, row)
		lanes = next
	}
	return rows
}

// Each lane holds the edge it leads down along.
func graphStep(above []graphEdge, commit gitrepo.Commit, rowOf map[string]int32) (graphRow, []graphEdge) {
	column := commitLane(above, commit.Hash)
	below := slices.Clone(above)
	for len(below) <= column {
		below = append(below, graphEdge{})
	}
	// Each connector is a lane joined to the commit by a horizontal line, which draws that lane's edge.
	connectors := map[int]graphEdge{}
	ending := map[int]bool{}
	for lane, edge := range above {
		if lane != column && edge.parent == commit.Hash {
			ending[lane] = true
			connectors[lane] = edge
			below[lane] = graphEdge{}
		}
	}
	var lineChild int32 = noRow
	if column < len(above) && above[column].parent == commit.Hash && above[column].firstParent {
		lineChild = rowOf[above[column].child]
	}
	var lineParent int32 = noRow
	if len(commit.Parents) == 0 {
		below[column] = graphEdge{}
	} else {
		below[column] = graphEdge{child: commit.Hash, parent: commit.Parents[0], firstParent: true}
		if row, loaded := rowOf[commit.Parents[0]]; loaded {
			lineParent = row
		}
	}
	opening := map[int]bool{}
	joining := map[int]bool{}
	for _, parent := range commit.Parents[min(1, len(commit.Parents)):] {
		edge := graphEdge{child: commit.Hash, parent: parent, firstParent: false}
		existing := slices.IndexFunc(below, func(lane graphEdge) bool { return lane.parent == parent })
		if existing >= 0 {
			joining[existing] = true
			connectors[existing] = edge
			continue
		}
		lane := freeLane(below, above)
		for len(below) <= lane {
			below = append(below, graphEdge{})
		}
		below[lane] = edge
		opening[lane] = true
		connectors[lane] = edge
	}

	first := column
	last := column
	for lane := range connectors {
		first = min(first, lane)
		last = max(last, lane)
	}
	var commitGlyph rune
	if len(commit.Parents) > 1 {
		commitGlyph = '○'
	} else {
		commitGlyph = '●'
	}
	width := max(len(above), len(below))
	commitCells := []graphCell{}
	belowCells := []graphCell{}
	for lane := range width {
		passing := lane < len(above) && above[lane].parent != "" && lane < len(below) && below[lane].parent != ""
		crossed := first < lane && lane < last
		var symbol graphCell
		switch {
		case lane == column:
			row := rowOf[commit.Hash]
			symbol = graphCell{glyph: commitGlyph, lane: int32(lane), childRow: row, parentRow: row}
		case ending[lane] && lane > column:
			symbol = edgeCell('╯', lane, connectors[lane], rowOf)
		case ending[lane]:
			symbol = edgeCell('╰', lane, connectors[lane], rowOf)
		case opening[lane] && lane > column:
			symbol = edgeCell('╮', lane, connectors[lane], rowOf)
		case opening[lane]:
			symbol = edgeCell('╭', lane, connectors[lane], rowOf)
		case joining[lane] && lane > column:
			symbol = edgeCell('┤', lane, connectors[lane], rowOf)
		case joining[lane]:
			symbol = edgeCell('├', lane, connectors[lane], rowOf)
		case passing && crossed:
			symbol = edgeCell('┼', lane, above[lane], rowOf)
		case passing:
			symbol = edgeCell('│', lane, above[lane], rowOf)
		case crossed:
			connector := nearestConnector(connectors, column, lane)
			symbol = edgeCell('─', connector, connectors[connector], rowOf)
		default:
			symbol = blankCell(lane)
		}
		var filler graphCell
		if first <= lane && lane < last {
			connector := nearestConnector(connectors, column, gapSide(lane, column))
			filler = edgeCell('─', connector, connectors[connector], rowOf)
		} else {
			filler = blankCell(lane)
		}
		commitCells = append(commitCells, symbol, filler)
		if lane < len(below) && below[lane].parent != "" {
			belowCells = append(belowCells, edgeCell('│', lane, below[lane], rowOf), blankCell(lane))
		} else {
			belowCells = append(belowCells, blankCell(lane), blankCell(lane))
		}
	}
	for len(below) > 0 && below[len(below)-1].parent == "" {
		below = below[:len(below)-1]
	}
	return graphRow{commit: commitCells, below: belowCells, lineChild: lineChild, lineParent: lineParent}, below
}

func edgeCell(glyph rune, lane int, edge graphEdge, rowOf map[string]int32) graphCell {
	cell := blankCell(lane)
	cell.glyph = glyph
	if !edge.firstParent {
		return cell
	}
	if parentRow, loaded := rowOf[edge.parent]; loaded {
		cell.childRow = rowOf[edge.child]
		cell.parentRow = parentRow
	}
	return cell
}

func blankCell(lane int) graphCell {
	return graphCell{glyph: ' ', lane: int32(lane), childRow: noRow, parentRow: noRow}
}

// The gap right of lane belongs to the side of the commit it lies on: lane+1 right of it, lane left of it.
func gapSide(lane int, column int) int {
	if lane >= column {
		return lane + 1
	}
	return lane
}

// A horizontal cell takes the color of the first connector at or beyond it, going outward from the commit.
func nearestConnector(connectors map[int]graphEdge, column int, from int) int {
	nearest := noRow
	for lane := range connectors {
		outward := (from > column && lane >= from) || (from < column && lane <= from)
		if !outward {
			continue
		}
		if nearest == noRow || absInt(lane-from) < absInt(nearest-from) {
			nearest = lane
		}
	}
	return nearest
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// Also empty above, so a lane that just ended is not reused on the same row.
func freeLane(lanes []graphEdge, above []graphEdge) int {
	for lane := range lanes {
		if lanes[lane].parent == "" && (lane >= len(above) || above[lane].parent == "") {
			return lane
		}
	}
	return len(lanes)
}

func commitLane(above []graphEdge, hash string) int {
	lane := slices.IndexFunc(above, func(edge graphEdge) bool { return edge.parent == hash })
	if lane >= 0 {
		return lane
	}
	return freeLane(above, above)
}

// lineThrough marks the rows on the selected commit's first-parent line: its ancestors down to the oldest loaded
// one, and the commits above whose first parent continues straight down into it.
func lineThrough(rows []graphRow, selected int) []bool {
	line := make([]bool, len(rows))
	if selected < 0 || selected >= len(rows) {
		return line
	}
	for row := int32(selected); row != noRow; row = rows[row].lineParent {
		line[row] = true
	}
	for row := rows[selected].lineChild; row != noRow; row = rows[row].lineChild {
		line[row] = true
	}
	return line
}

func (c graphCell) onLine(line []bool) bool {
	return onRow(line, c.childRow) && onRow(line, c.parentRow)
}

func onRow(line []bool, row int32) bool {
	return row != noRow && int(row) < len(line) && line[row]
}

// Runs of cells drawn alike share one style, to keep the escape codes per row down.
func renderGraphCells(cells []graphCell, line []bool) string {
	var rendered strings.Builder
	start := 0
	for start < len(cells) {
		highlighted := cells[start].onLine(line)
		lane := cells[start].lane % int32(len(laneStyles))
		end := start
		var glyphs strings.Builder
		for end < len(cells) && cells[end].onLine(line) == highlighted && cells[end].lane%int32(len(laneStyles)) == lane {
			glyphs.WriteRune(cells[end].glyph)
			end++
		}
		if highlighted {
			rendered.WriteString(laneStyles[lane].Render(glyphs.String()))
		} else {
			rendered.WriteString(dimLaneStyles[lane].Render(glyphs.String()))
		}
		start = end
	}
	return rendered.String()
}
