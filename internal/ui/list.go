package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type selectableList[T any] struct {
	loaded   bool
	rows     []T
	selected int
	offset   int
}

func (l selectableList[T]) withSelected(index int) selectableList[T] {
	if len(l.rows) == 0 {
		return l
	}
	l.selected = min(max(index, 0), len(l.rows)-1)
	return l
}

func (l selectableList[T]) selectedRow() (T, bool) {
	if len(l.rows) == 0 {
		var none T
		return none, false
	}
	return l.rows[l.selected], true
}

func (l selectableList[T]) scrolled(height int) selectableList[T] {
	l.offset = scrolledOffset(l.offset, l.selected, height)
	return l
}

func positionLabel(position int, count int) string {
	if count == 0 {
		return ""
	}
	return strconv.Itoa(position) + " of " + strconv.Itoa(count)
}

func renderListPane[T any](title string, list selectableList[T], cellsOf func(T) []string, emptyText string,
	innerWidth int, innerHeight int) string {
	lines := listLines(list, cellsOf, emptyText, innerWidth, innerHeight)
	return renderPane(title, "", positionLabel(list.selected+1, len(list.rows)), lines, innerWidth, innerHeight)
}

func listLines[T any](list selectableList[T], cellsOf func(T) []string, emptyText string, innerWidth int,
	height int) []string {
	switch {
	case !list.loaded:
		return []string{mutedStyle.Render("loading...")}
	case len(list.rows) == 0:
		return []string{mutedStyle.Render(emptyText)}
	}
	cells := [][]string{}
	widths := []int{}
	for _, row := range list.rows {
		rowCells := cellsOf(row)
		for column, cell := range rowCells {
			if column == len(widths) {
				widths = append(widths, 0)
			}
			widths[column] = max(widths[column], ansi.StringWidth(cell))
		}
		cells = append(cells, rowCells)
	}
	aligned := []string{}
	for _, row := range cells {
		padded := []string{}
		for column, cell := range row {
			padded = append(padded, cell+strings.Repeat(" ", widths[column]-ansi.StringWidth(cell)))
		}
		aligned = append(aligned, strings.Join(padded, "  "))
	}
	window := listWindow{offset: list.offset, selected: list.selected, height: height}
	return window.render(len(aligned), innerWidth, func(index int) string { return aligned[index] })
}

type listWindow struct {
	offset   int
	selected int
	height   int
}

func (w listWindow) render(count int, width int, row func(index int) string) []string {
	lines := []string{}
	for index := w.offset; index < min(w.offset+w.height, count); index++ {
		// The selected row drops its own colors, since each styled part would end the row's background early.
		if index == w.selected {
			lines = append(lines, selectionBarStyle.Render("▌")+selectedRowStyle.Render(fitToWidth(ansi.Strip(row(index)),
				width-1)))
		} else {
			lines = append(lines, " "+fitToWidth(row(index), width-1))
		}
	}
	return lines
}
