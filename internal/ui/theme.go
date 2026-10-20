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
)

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
	behindStyle         = lipgloss.NewStyle().Foreground(colorLove)
	errorStyle          = lipgloss.NewStyle().Foreground(colorLove)
	successStyle        = lipgloss.NewStyle().Foreground(colorFoam)
	selectedRowStyle    = lipgloss.NewStyle().Foreground(colorText).Background(colorHighlightMed)
	selectionBarStyle   = lipgloss.NewStyle().Foreground(colorIris).Background(colorHighlightMed)
	stagedMarkStyle     = lipgloss.NewStyle().Foreground(colorFoam).Bold(true)
	unstagedMarkStyle   = lipgloss.NewStyle().Foreground(colorLove).Bold(true)
	untrackedStyle      = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	hashStyle           = lipgloss.NewStyle().Foreground(colorGold)
	refStyle            = lipgloss.NewStyle().Foreground(colorFoam).Bold(true)
	searchMatchStyle    = lipgloss.NewStyle().Foreground(colorRose).Bold(true).Underline(true)
	laneStyles          = []lipgloss.Style{
		lipgloss.NewStyle().Foreground(colorIris),
		lipgloss.NewStyle().Foreground(colorFoam),
		lipgloss.NewStyle().Foreground(colorGold),
		lipgloss.NewStyle().Foreground(colorRose),
		lipgloss.NewStyle().Foreground(colorLove),
	}
	// laneStyles' colors, each blended 45% over base, for lines off the selected commit's line
	dimLaneStyles = []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#66587c")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#546a75")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#7c6449")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#786168")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#783f56")),
	}
	authorStyle       = lipgloss.NewStyle().Foreground(colorFoam)
	fileHeaderStyle   = lipgloss.NewStyle().Foreground(colorRose).Background(colorOverlay).Bold(true)
	hunkHeaderStyle   = lipgloss.NewStyle().Foreground(colorIris).Background(colorOverlay)
	separatorStyle    = lipgloss.NewStyle().Foreground(colorHighlightMed)
	contextTextStyle  = lipgloss.NewStyle().Foreground(colorSubtle)
	removedTextStyle  = lipgloss.NewStyle().Foreground(colorText).Background(colorRemovedBackground)
	addedTextStyle    = lipgloss.NewStyle().Foreground(colorText).Background(colorAddedBackground)
	removedGutter     = lipgloss.NewStyle().Foreground(colorLove).Background(colorRemovedBackground)
	addedGutter       = lipgloss.NewStyle().Foreground(colorFoam).Background(colorAddedBackground)
	addedMarkStyle    = lipgloss.NewStyle().Foreground(colorFoam).Bold(true)
	deletedMarkStyle  = lipgloss.NewStyle().Foreground(colorLove).Bold(true)
	renamedMarkStyle  = lipgloss.NewStyle().Foreground(colorIris).Bold(true)
	modifiedMarkStyle = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	helpKeyStyle      = lipgloss.NewStyle().Foreground(colorRose).Bold(true)
	helpDescStyle     = lipgloss.NewStyle().Foreground(colorSubtle)
	helpSepStyle      = lipgloss.NewStyle().Foreground(colorHighlightMed)
)
