package ui

import "charm.land/lipgloss/v2"

var (
	appStyle          = lipgloss.NewStyle().Padding(1, 2)
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00ADD8"))
	actionButtonStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	helpStyle         = lipgloss.NewStyle().MarginTop(1).Foreground(lipgloss.Color("241"))
)
