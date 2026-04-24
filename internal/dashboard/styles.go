// Package dashboard implements the Bubble Tea interactive TUI for Cerebro metrics.
package dashboard

import "charm.land/lipgloss/v2"

// Color palette.
var (
	colorPrimary   = lipgloss.Color("#7D56F4")
	colorSecondary = lipgloss.Color("#874BFD")
	colorMuted     = lipgloss.Color("#626262")
	colorHighlight = lipgloss.Color("#FAFAFA")
	colorWarning   = lipgloss.Color("#FF5F56")
)

// Styles.
var (
	// Tab bar styles.
	tabStyle = lipgloss.NewStyle().
			Padding(0, 2)

	activeTabStyle = tabStyle.
			Bold(true).
			Foreground(colorHighlight).
			Background(colorPrimary)

	inactiveTabStyle = tabStyle.
				Foreground(colorMuted)

	// Header/title.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, 1)

	// Section headers within panels.
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary)

	// Metric labels.
	labelStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(14)

	// Metric values.
	valueStyle = lipgloss.NewStyle().
			Bold(true)

	// Warning flags (zero thinking, blind edits).
	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning).
			Bold(true)

	// Help text at bottom.
	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	// Main content border.
	contentStyle = lipgloss.NewStyle().
			Padding(1, 2)
)
