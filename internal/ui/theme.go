package ui

import (
	"charm.land/lipgloss/v2"
)

// Rosé Pine main variant, https://rosepinetheme.com/palette. Nothing sets a background for the whole screen, so the
// terminal's own background and opacity show through.
var (
	colorBase         = lipgloss.Color("#191724")
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
	// love and foam blended 15% over base
	colorRemovedBackground = lipgloss.Color("#382434")
	colorAddedBackground   = lipgloss.Color("#2d333f")
)

var (
	borderStyle       = lipgloss.NewStyle().Foreground(colorHighlightMed)
	popupStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorRose).Padding(0, 1)
	fieldStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorHighlightMed)
	focusedFieldStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorIris)
	titleStyle        = lipgloss.NewStyle().Foreground(colorIris).Bold(true)
	paneLabelStyle    = lipgloss.NewStyle().Foreground(colorSubtle)
	mutedStyle        = lipgloss.NewStyle().Foreground(colorMuted)
	textStyle         = lipgloss.NewStyle().Foreground(colorText)
	appNameStyle      = lipgloss.NewStyle().Foreground(colorBase).Background(colorRose).Bold(true).Padding(0, 1)
	repositoryStyle   = lipgloss.NewStyle().Foreground(colorIris).Bold(true)
	branchStyle       = lipgloss.NewStyle().Foreground(colorFoam)
	aheadStyle        = lipgloss.NewStyle().Foreground(colorGold)
	behindStyle       = lipgloss.NewStyle().Foreground(colorLove)
	activeTabStyle    = lipgloss.NewStyle().Foreground(colorBase).Background(colorIris).Bold(true).Padding(0, 1)
	inactiveTabStyle  = lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)
	errorStyle        = lipgloss.NewStyle().Foreground(colorLove)
	successStyle      = lipgloss.NewStyle().Foreground(colorFoam)
	selectedRowStyle  = lipgloss.NewStyle().Foreground(colorText).Background(colorHighlightMed)
	selectionBarStyle = lipgloss.NewStyle().Foreground(colorIris).Background(colorHighlightMed)
	stagedMarkStyle   = lipgloss.NewStyle().Foreground(colorFoam).Bold(true)
	unstagedMarkStyle = lipgloss.NewStyle().Foreground(colorLove).Bold(true)
	untrackedStyle    = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	hashStyle         = lipgloss.NewStyle().Foreground(colorGold)
	refStyle          = lipgloss.NewStyle().Foreground(colorFoam).Bold(true)
	laneStyles        = []lipgloss.Style{
		lipgloss.NewStyle().Foreground(colorIris),
		lipgloss.NewStyle().Foreground(colorFoam),
		lipgloss.NewStyle().Foreground(colorGold),
		lipgloss.NewStyle().Foreground(colorRose),
		lipgloss.NewStyle().Foreground(colorLove),
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
