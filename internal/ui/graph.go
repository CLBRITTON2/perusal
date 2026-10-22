package ui

import (
	"slices"
	"strings"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

const (
	noRow  = -1
	noLane = -1
)

type graphEdge struct {
	child       string
	parent      string
	firstParent bool
}

// A cell is one terminal column, on the selected path when its childRow and parentRow both are. A commit's own
// symbol holds its row in both, and a cell off any first-parent path holds noRow. int32 keeps a cell at 16 bytes,
// since a long history holds close to a million of them.
type graphCell struct {
	glyph     rune
	lane      int32
	childRow  int32
	parentRow int32
}

// commit and below are the same width, two cells per lane. pathChild and pathParent are the rows the commit's
// first-parent path continues to, or noRow.
type graphRow struct {
	commit     []graphCell
	below      []graphCell
	pathChild  int32
	pathParent int32
}

func commitGraph(commits []gitrepo.Commit) []graphRow {
	rowByHash := map[string]int32{}
	for index, commit := range commits {
		rowByHash[commit.Hash] = int32(index)
	}
	rows := []graphRow{}
	lanes := []graphEdge{}
	for _, commit := range commits {
		row, next := graphStep(lanes, commit, rowByHash)
		rows = append(rows, row)
		lanes = next
	}
	return rows
}

// Each lane holds the edge it leads down along.
func graphStep(above []graphEdge, commit gitrepo.Commit, rowByHash map[string]int32) (graphRow, []graphEdge) {
	commitLane := laneOf(above, commit.Hash)
	below := growTo(slices.Clone(above), commitLane)
	// Each connector is a lane joined to the commit by a horizontal line, which draws that lane's edge.
	connectors := map[int]graphEdge{}
	endingLanes := map[int]bool{}
	for lane, edge := range above {
		if lane != commitLane && edge.parent == commit.Hash {
			endingLanes[lane] = true
			connectors[lane] = edge
			below[lane] = graphEdge{}
		}
	}
	var pathChild int32 = noRow
	if commitLane < len(above) && above[commitLane].parent == commit.Hash && above[commitLane].firstParent {
		pathChild = rowByHash[above[commitLane].child]
	}
	var pathParent int32 = noRow
	if len(commit.Parents) == 0 {
		below[commitLane] = graphEdge{}
	} else {
		below[commitLane] = graphEdge{child: commit.Hash, parent: commit.Parents[0], firstParent: true}
		if row, loaded := rowByHash[commit.Parents[0]]; loaded {
			pathParent = row
		}
	}
	openingLanes := map[int]bool{}
	joiningLanes := map[int]bool{}
	for _, parent := range commit.Parents[min(1, len(commit.Parents)):] {
		edge := graphEdge{child: commit.Hash, parent: parent, firstParent: false}
		existing := slices.IndexFunc(below, func(lane graphEdge) bool { return lane.parent == parent })
		if existing >= 0 {
			joiningLanes[existing] = true
			connectors[existing] = edge
			continue
		}
		lane := freeLane(below, above)
		below = growTo(below, lane)
		below[lane] = edge
		openingLanes[lane] = true
		connectors[lane] = edge
	}

	leftmost := commitLane
	rightmost := commitLane
	for lane := range connectors {
		leftmost = min(leftmost, lane)
		rightmost = max(rightmost, lane)
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
		passing := occupied(above, lane) && occupied(below, lane)
		crossed := leftmost < lane && lane < rightmost
		var symbol graphCell
		switch {
		case lane == commitLane:
			row := rowByHash[commit.Hash]
			symbol = graphCell{glyph: commitGlyph, lane: int32(lane), childRow: row, parentRow: row}
		case endingLanes[lane] && lane > commitLane:
			symbol = edgeCell('╯', lane, connectors[lane], rowByHash)
		case endingLanes[lane]:
			symbol = edgeCell('╰', lane, connectors[lane], rowByHash)
		case openingLanes[lane] && lane > commitLane:
			symbol = edgeCell('╮', lane, connectors[lane], rowByHash)
		case openingLanes[lane]:
			symbol = edgeCell('╭', lane, connectors[lane], rowByHash)
		case joiningLanes[lane] && lane > commitLane:
			symbol = edgeCell('┤', lane, connectors[lane], rowByHash)
		case joiningLanes[lane]:
			symbol = edgeCell('├', lane, connectors[lane], rowByHash)
		case passing && crossed:
			symbol = edgeCell('┼', lane, above[lane], rowByHash)
		case passing:
			symbol = edgeCell('│', lane, above[lane], rowByHash)
		case crossed:
			connector := nearestConnector(connectors, commitLane, lane)
			symbol = edgeCell('─', connector, connectors[connector], rowByHash)
		default:
			symbol = blankCell(lane)
		}
		var filler graphCell
		if leftmost <= lane && lane < rightmost {
			connector := nearestConnector(connectors, commitLane, gapSide(lane, commitLane))
			filler = edgeCell('─', connector, connectors[connector], rowByHash)
		} else {
			filler = blankCell(lane)
		}
		commitCells = append(commitCells, symbol, filler)
		if occupied(below, lane) {
			belowCells = append(belowCells, edgeCell('│', lane, below[lane], rowByHash), blankCell(lane))
		} else {
			belowCells = append(belowCells, blankCell(lane), blankCell(lane))
		}
	}
	for len(below) > 0 && !occupied(below, len(below)-1) {
		below = below[:len(below)-1]
	}
	return graphRow{commit: commitCells, below: belowCells, pathChild: pathChild, pathParent: pathParent}, below
}

func growTo(lanes []graphEdge, lane int) []graphEdge {
	return append(lanes, make([]graphEdge, max(lane+1-len(lanes), 0))...)
}

func occupied(lanes []graphEdge, lane int) bool {
	return lane < len(lanes) && lanes[lane].parent != ""
}

func edgeCell(glyph rune, lane int, edge graphEdge, rowByHash map[string]int32) graphCell {
	cell := blankCell(lane)
	cell.glyph = glyph
	if !edge.firstParent {
		return cell
	}
	if parentRow, loaded := rowByHash[edge.parent]; loaded {
		cell.childRow = rowByHash[edge.child]
		cell.parentRow = parentRow
	}
	return cell
}

func blankCell(lane int) graphCell {
	return graphCell{glyph: ' ', lane: int32(lane), childRow: noRow, parentRow: noRow}
}

// The gap right of lane belongs to the side of the commit it lies on: lane+1 right of it, lane left of it.
func gapSide(lane int, commitLane int) int {
	if lane >= commitLane {
		return lane + 1
	}
	return lane
}

// A horizontal cell takes the color of the first connector at or beyond it, going outward from the commit.
func nearestConnector(connectors map[int]graphEdge, commitLane int, from int) int {
	nearest := noLane
	for lane := range connectors {
		outward := (from > commitLane && lane >= from) || (from < commitLane && lane <= from)
		if !outward {
			continue
		}
		if nearest == noLane || absInt(lane-from) < absInt(nearest-from) {
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
		if !occupied(lanes, lane) && !occupied(above, lane) {
			return lane
		}
	}
	return len(lanes)
}

func laneOf(above []graphEdge, hash string) int {
	lane := slices.IndexFunc(above, func(edge graphEdge) bool { return edge.parent == hash })
	if lane >= 0 {
		return lane
	}
	return freeLane(above, above)
}

// pathThrough marks the rows on the selected commit's first-parent path: its ancestors down to the oldest loaded
// one, and the commits above whose first parent continues straight down into it.
func pathThrough(rows []graphRow, selected int) []bool {
	path := make([]bool, len(rows))
	if selected < 0 || selected >= len(rows) {
		return path
	}
	for row := int32(selected); row != noRow; row = rows[row].pathParent {
		path[row] = true
	}
	for row := rows[selected].pathChild; row != noRow; row = rows[row].pathChild {
		path[row] = true
	}
	return path
}

func (c graphCell) onPath(path []bool) bool {
	return onRow(path, c.childRow) && onRow(path, c.parentRow)
}

func (c graphCell) styleIndex() int32 {
	return c.lane % int32(len(laneStyles))
}

func onRow(path []bool, row int32) bool {
	return row != noRow && int(row) < len(path) && path[row]
}

// Runs of cells drawn alike share one style, to keep the escape codes per row down.
func renderGraphCells(cells []graphCell, path []bool) string {
	var rendered strings.Builder
	start := 0
	for start < len(cells) {
		highlighted := cells[start].onPath(path)
		style := cells[start].styleIndex()
		end := start
		var glyphs strings.Builder
		for end < len(cells) && cells[end].onPath(path) == highlighted && cells[end].styleIndex() == style {
			glyphs.WriteRune(cells[end].glyph)
			end++
		}
		if highlighted {
			rendered.WriteString(laneStyles[style].line.Render(glyphs.String()))
		} else {
			rendered.WriteString(laneStyles[style].dim.Render(glyphs.String()))
		}
		start = end
	}
	return rendered.String()
}
