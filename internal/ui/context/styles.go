package context

import (
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

// Styles holds the precomputed lipgloss styles shared across components.
// M1a hardcodes the same colors the app used before the refactor.
type Styles struct {
	Table        table.Styles
	Spinner      lipgloss.Style
	DimText      lipgloss.Style
	ErrorText    lipgloss.Style
	Tab          lipgloss.Style
	ActiveTab    lipgloss.Style
	TabSeparator lipgloss.Style
}

// DefaultStyles returns the built-in styles (no theme parsing in M1a).
func DefaultStyles() Styles {
	tbl := table.DefaultStyles()
	tbl.Header = tbl.Header.Bold(true).Foreground(lipgloss.Color("#00ADD8")).BorderBottom(true)
	tbl.Selected = tbl.Selected.Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	return Styles{
		Table:        tbl,
		Spinner:      lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8")),
		DimText:      lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		ErrorText:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		Tab:          lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1),
		ActiveTab:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00ADD8")).Padding(0, 1),
		TabSeparator: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
}
