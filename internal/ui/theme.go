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
	paneStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorHighlightMed)
	popupStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorRose).Padding(0, 1)
	fieldStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorHighlightMed)
	focusedFieldStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorIris)
	titleStyle        = lipgloss.NewStyle().Foreground(colorIris).Bold(true)
	mutedStyle        = lipgloss.NewStyle().Foreground(colorMuted)
	appNameStyle      = lipgloss.NewStyle().Foreground(colorBase).Background(colorRose).Bold(true).Padding(0, 1)
	repositoryStyle   = lipgloss.NewStyle().Foreground(colorIris).Bold(true)
	branchStyle       = lipgloss.NewStyle().Foreground(colorFoam)
	errorStyle        = lipgloss.NewStyle().Foreground(colorLove)
	successStyle      = lipgloss.NewStyle().Foreground(colorFoam)
	selectedRowStyle  = lipgloss.NewStyle().Background(colorHighlightMed)
	stagedMarkStyle   = lipgloss.NewStyle().Foreground(colorFoam).Bold(true)
	unstagedMarkStyle = lipgloss.NewStyle().Foreground(colorLove).Bold(true)
	untrackedStyle    = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	hunkHeaderStyle   = lipgloss.NewStyle().Foreground(colorIris).Background(colorOverlay)
	separatorStyle    = lipgloss.NewStyle().Foreground(colorHighlightMed)
	contextTextStyle  = lipgloss.NewStyle().Foreground(colorSubtle)
	removedTextStyle  = lipgloss.NewStyle().Foreground(colorText).Background(colorRemovedBackground)
	addedTextStyle    = lipgloss.NewStyle().Foreground(colorText).Background(colorAddedBackground)
	removedGutter     = lipgloss.NewStyle().Foreground(colorLove).Background(colorRemovedBackground)
	addedGutter       = lipgloss.NewStyle().Foreground(colorFoam).Background(colorAddedBackground)
)
