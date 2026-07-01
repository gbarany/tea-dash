package ui

import "github.com/charmbracelet/lipgloss"

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00ADD8")) // gitea / go cyan

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			Underline(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	dividerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	bodyStyle = lipgloss.NewStyle().
			MarginTop(1).
			Foreground(lipgloss.Color("252"))

	helpStyle = lipgloss.NewStyle().
			MarginTop(1).
			Foreground(lipgloss.Color("241"))
)
