package ui

import (
	"charm.land/lipgloss/v2"
)

// Rosé Pine main, https://rosepinetheme.com/palette. Nothing sets a screen background, so terminal opacity shows.
var (
	colorOverlay      = lipgloss.Color("#26233a")
	colorMuted        = lipgloss.Color("#6e6a86")
	colorSubtle       = lipgloss.Color("#908caa")
	colorText         = lipgloss.Color("#e0def4")
	colorLove         = lipgloss.Color("#eb6f92")
	colorGold         = lipgloss.Color("#f6c177")
	colorRose         = lipgloss.Color("#ebbcba")
	colorFoam         = lipgloss.Color("#9ccfd8")
	colorIris         = lipgloss.Color("#c4a7e7")
	colorHighlightMed = lipgloss.Color("#403d52")
	// love and foam, each blended 15% over base
	colorRemovedBackground = lipgloss.Color("#382434")
	colorAddedBackground   = lipgloss.Color("#2d333f")
	// love and foam, each blended 35% over base, for the changed part of a line
	colorRemovedChanged = lipgloss.Color("#62364a")
	colorAddedChanged   = lipgloss.Color("#475763")
)

// dim is line's color blended 45% over base, for lines off the selected commit's path.
type laneStyle struct {
	line lipgloss.Style
	dim  lipgloss.Style
}

var (
	borderStyle         = lipgloss.NewStyle().Foreground(colorMuted)
	popupStyle          = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorRose).Padding(0, 1)
	fieldStyle          = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorHighlightMed)
	focusedFieldStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorIris)
	titleStyle          = lipgloss.NewStyle().Foreground(colorIris).Bold(true)
	unfocusedTitleStyle = lipgloss.NewStyle().Foreground(colorSubtle)
	paneLabelStyle      = lipgloss.NewStyle().Foreground(colorSubtle)
	mutedStyle          = lipgloss.NewStyle().Foreground(colorMuted)
	textStyle           = lipgloss.NewStyle().Foreground(colorText)
	repositoryStyle     = lipgloss.NewStyle().Foreground(colorRose).Bold(true)
	branchStyle         = lipgloss.NewStyle().Foreground(colorFoam)
	aheadStyle          = lipgloss.NewStyle().Foreground(colorGold)
	markStyle           = lipgloss.NewStyle().Foreground(colorGold)
	behindStyle         = lipgloss.NewStyle().Foreground(colorLove)
	errorStyle          = lipgloss.NewStyle().Foreground(colorLove)
	warningStyle        = lipgloss.NewStyle().Foreground(colorGold)
	successStyle        = lipgloss.NewStyle().Foreground(colorFoam)
	selectedRowStyle    = lipgloss.NewStyle().Foreground(colorText).Background(colorHighlightMed)
	selectionBarStyle   = lipgloss.NewStyle().Foreground(colorIris).Background(colorHighlightMed)
	stagedMarkStyle     = lipgloss.NewStyle().Foreground(colorFoam).Bold(true)
	unstagedMarkStyle   = lipgloss.NewStyle().Foreground(colorLove).Bold(true)
	untrackedStyle      = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	hashStyle           = lipgloss.NewStyle().Foreground(colorGold)
	refStyle            = lipgloss.NewStyle().Foreground(colorFoam).Bold(true)
	searchMatchStyle    = lipgloss.NewStyle().Foreground(colorRose).Bold(true).Underline(true)
	laneStyles          = []laneStyle{
		{line: lipgloss.NewStyle().Foreground(colorIris), dim: lipgloss.NewStyle().Foreground(lipgloss.Color("#66587c"))},
		{line: lipgloss.NewStyle().Foreground(colorFoam), dim: lipgloss.NewStyle().Foreground(lipgloss.Color("#546a75"))},
		{line: lipgloss.NewStyle().Foreground(colorGold), dim: lipgloss.NewStyle().Foreground(lipgloss.Color("#7c6449"))},
		{line: lipgloss.NewStyle().Foreground(colorRose), dim: lipgloss.NewStyle().Foreground(lipgloss.Color("#786168"))},
		{line: lipgloss.NewStyle().Foreground(colorLove), dim: lipgloss.NewStyle().Foreground(lipgloss.Color("#783f56"))},
	}
	authorStyle         = lipgloss.NewStyle().Foreground(colorFoam)
	fileHeaderStyle     = lipgloss.NewStyle().Foreground(colorRose).Background(colorOverlay).Bold(true)
	hunkHeaderStyle     = lipgloss.NewStyle().Foreground(colorIris).Background(colorOverlay)
	separatorStyle      = lipgloss.NewStyle().Foreground(colorHighlightMed)
	contextTextStyle    = lipgloss.NewStyle().Foreground(colorSubtle)
	removedTextStyle    = lipgloss.NewStyle().Foreground(colorText).Background(colorRemovedBackground)
	addedTextStyle      = lipgloss.NewStyle().Foreground(colorText).Background(colorAddedBackground)
	removedChangedStyle = lipgloss.NewStyle().Foreground(colorText).Background(colorRemovedChanged)
	addedChangedStyle   = lipgloss.NewStyle().Foreground(colorText).Background(colorAddedChanged)
	removedGutterStyle  = lipgloss.NewStyle().Foreground(colorLove).Background(colorRemovedBackground)
	addedGutterStyle    = lipgloss.NewStyle().Foreground(colorFoam).Background(colorAddedBackground)
	addedMarkStyle      = lipgloss.NewStyle().Foreground(colorFoam).Bold(true)
	deletedMarkStyle    = lipgloss.NewStyle().Foreground(colorLove).Bold(true)
	renamedMarkStyle    = lipgloss.NewStyle().Foreground(colorIris).Bold(true)
	modifiedMarkStyle   = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	helpKeyStyle        = lipgloss.NewStyle().Foreground(colorRose).Bold(true)
	helpDescStyle       = lipgloss.NewStyle().Foreground(colorSubtle)
	helpSepStyle        = lipgloss.NewStyle().Foreground(colorHighlightMed)
)
