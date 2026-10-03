package main

import "github.com/charmbracelet/lipgloss"

// ytplay's palette. Colours are adaptive so the UI keeps its contrast on both
// dark and light terminals. Blue and violet carry identity and focus, green/red/
// yellow are reserved for state (playing, errors, warnings).
var (
	bgBar  = lipgloss.AdaptiveColor{Light: "#e2e7f3", Dark: "#1b1e2b"}
	bgSel  = lipgloss.AdaptiveColor{Light: "#ccd6f2", Dark: "#2b3149"}
	bgChip = lipgloss.AdaptiveColor{Light: "#dbe1f0", Dark: "#252a3a"}

	accent  = lipgloss.AdaptiveColor{Light: "#3b5b9e", Dark: "#7aa2f7"}
	accent2 = lipgloss.AdaptiveColor{Light: "#6f42a8", Dark: "#bb9af7"}
	green   = lipgloss.AdaptiveColor{Light: "#2f7d4f", Dark: "#9ece6a"}
	red     = lipgloss.AdaptiveColor{Light: "#b3404a", Dark: "#f7768e"}

	fg     = lipgloss.AdaptiveColor{Light: "#20242f", Dark: "#c8d3f5"}
	fgMid  = lipgloss.AdaptiveColor{Light: "#4a5169", Dark: "#9aa5ce"}
	fgDim  = lipgloss.AdaptiveColor{Light: "#7b8299", Dark: "#565f89"}
	border = lipgloss.AdaptiveColor{Light: "#c3cade", Dark: "#343b58"}
)

// Text styles.
var (
	midStyle   = lipgloss.NewStyle().Foreground(fgMid)
	dimStyle   = lipgloss.NewStyle().Foreground(fgDim)
	errorStyle = lipgloss.NewStyle().Foreground(red).Bold(true)
	sepStyle   = lipgloss.NewStyle().Foreground(border)
)

// Full-width chrome bars (header + status line). Segment styles are given the
// bar background at render time by renderBar so the bar reads as one strip.
var (
	barBgStyle = lipgloss.NewStyle().Background(bgBar)

	barBrandStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	barSepStyle     = lipgloss.NewStyle().Foreground(fgDim)
	barCtxStyle     = lipgloss.NewStyle().Foreground(fg)
	barStateStyle   = lipgloss.NewStyle().Foreground(fgMid)
	barAccentStyle  = lipgloss.NewStyle().Foreground(accent2)
	barMpvStyle     = lipgloss.NewStyle().Bold(true).Foreground(green)
	barErrorStyle   = lipgloss.NewStyle().Bold(true).Foreground(red)
	barStatusStyle  = lipgloss.NewStyle().Foreground(green)
	barHintStyle    = lipgloss.NewStyle().Foreground(fgDim)
	barHintKeyStyle = lipgloss.NewStyle().Bold(true).Foreground(fgMid)
)

// Prompt / searching screens.
var (
	wordmarkStyle    = lipgloss.NewStyle().Bold(true).Foreground(accent)
	promptBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 2)
	promptHintStyle  = lipgloss.NewStyle().Foreground(fgDim)
	promptKeyStyle   = lipgloss.NewStyle().Bold(true).Foreground(fgMid)
	searchQueryStyle = lipgloss.NewStyle().Bold(true).Foreground(fg)
	emptyTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(fgMid)
)

// List rows.
var (
	rowSelTitle     = lipgloss.NewStyle().Foreground(fg).Bold(true).Background(bgSel)
	rowSelTitleDim  = lipgloss.NewStyle().Foreground(fgMid).Background(bgSel)
	rowSelMeta      = lipgloss.NewStyle().Foreground(fgMid).Background(bgSel)
	rowSelMarker    = lipgloss.NewStyle().Bold(true).Foreground(accent).Background(bgSel)
	rowSelMarkerDim = lipgloss.NewStyle().Foreground(fgDim).Background(bgSel)
	rowSelBg        = lipgloss.NewStyle().Background(bgSel)

	rowTitle = lipgloss.NewStyle().Foreground(fgMid)
	rowMeta  = lipgloss.NewStyle().Foreground(fgDim)

	rowMarkerPlaying = lipgloss.NewStyle().Bold(true).Foreground(green)
	rowMarkerQueued  = lipgloss.NewStyle().Foreground(accent2)
)

// Preview pane.
var (
	previewTitle     = lipgloss.NewStyle().Bold(true).Foreground(fg)
	previewTitleDim  = lipgloss.NewStyle().Bold(true).Foreground(fgMid)
	previewMetaBar   = lipgloss.NewStyle().Foreground(accent).Bold(true)
	previewMetaBarD  = lipgloss.NewStyle().Foreground(border)
	previewMeta      = lipgloss.NewStyle().Foreground(fgMid)
	previewMetaDim   = lipgloss.NewStyle().Foreground(fgDim)
	previewHintStyle = lipgloss.NewStyle().Foreground(fgDim)
)
