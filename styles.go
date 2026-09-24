package main

import "github.com/charmbracelet/lipgloss"

// Palette colors (matching "vague" theme).
var (
	accent  = lipgloss.Color("6e94b2")
	red     = lipgloss.Color("d8647e")
	fg      = lipgloss.Color("cdcdcd")
	fgMid   = lipgloss.Color("878787")
	fgDim   = lipgloss.Color("606079")
	grey    = lipgloss.Color("8a8a8a")
	bgRound = lipgloss.Color("252530")
)

// Shared lipgloss styles, initialized once at startup to avoid
// recreating styles on every render frame.
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("000000")).
			Background(accent).
			Padding(0, 1).
			Bold(true)

	promptBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(bgRound).
			Padding(1, 2).
			Width(46)

	promptTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(accent)

	errorStyle = lipgloss.NewStyle().Foreground(red)

	statusStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(accent)

	accentStyle = lipgloss.NewStyle().Foreground(accent)

	dimStyle = lipgloss.NewStyle().Foreground(fgDim)

	barStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(fgDim)

	greyHintStyle = lipgloss.NewStyle().Foreground(grey)

	footerHintStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(grey)

	emptyStyle = lipgloss.NewStyle().Foreground(fgMid)

	headerStyle = lipgloss.NewStyle().Padding(0, 1)

	searchingPrefixStyle = lipgloss.NewStyle().Foreground(fg)
	searchingQueryStyle  = lipgloss.NewStyle().Bold(true).Foreground(fg)

	previewTitleStyle         = lipgloss.NewStyle().Bold(true).Foreground(accent)
	previewChannelStyle       = lipgloss.NewStyle().Foreground(fgMid)
	previewChannelActiveStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("000000")).
					Background(accent).
					Bold(true).
					Padding(0, 1)

	listActiveRowStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	listRowStyle       = lipgloss.NewStyle().Foreground(fg)

	paneBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(bgRound).
			Padding(0, 1)
)
