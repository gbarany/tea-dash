package context

import (
	"image/color"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
)

// Styles holds the precomputed lipgloss styles shared across components.
type Styles struct {
	Table        table.Styles
	Title        lipgloss.Style
	HelpText     lipgloss.Style
	ActionButton lipgloss.Style
	Spinner      lipgloss.Style
	DimText      lipgloss.Style
	ErrorText    lipgloss.Style
	Tab          lipgloss.Style
	ActiveTab    lipgloss.Style
	TabSeparator lipgloss.Style
}

const (
	defaultPrimary      = "#00ADD8"
	defaultDim          = "240"
	defaultHelp         = "241"
	defaultError        = "196"
	defaultSelectedText = "229"
	defaultSelectedBg   = "57"
)

// DefaultStyles returns the built-in styles.
func DefaultStyles() Styles {
	return stylesFromColors(
		lipgloss.Color(defaultPrimary),
		lipgloss.Color(defaultDim),
		lipgloss.Color(defaultHelp),
		lipgloss.Color(defaultError),
		lipgloss.Color(defaultSelectedText),
		lipgloss.Color(defaultSelectedBg),
	)
}

// StylesForConfig returns the default styles with any configured theme colors
// applied. The config shape mirrors gh-dash's theme.colors block, while this
// first pass applies only the colors tea-dash currently renders.
func StylesForConfig(cfg *config.Config) Styles {
	if cfg == nil {
		return DefaultStyles()
	}
	colors := cfg.Theme.Colors
	primary := colorOrDefault(colors.Text.Primary, lipgloss.Color(defaultPrimary))
	dim := colorOrDefault(colors.Text.Faint, lipgloss.Color(defaultDim))
	help := dim
	if colors.Text.Faint == "" {
		help = lipgloss.Color(defaultHelp)
	}
	warning := colorOrDefault(colors.Text.Warning, lipgloss.Color(defaultError))
	selectedText := colorOrDefault(colors.Text.Secondary, lipgloss.Color(defaultSelectedText))
	selectedBg := colorOrDefault(colors.Background.Selected, lipgloss.Color(defaultSelectedBg))
	return stylesFromColors(primary, dim, help, warning, selectedText, selectedBg)
}

func stylesFromColors(primary, dim, help, warning, selectedText, selectedBg color.Color) Styles {
	tbl := table.DefaultStyles()
	tbl.Header = tbl.Header.Bold(true).Foreground(primary).BorderBottom(true)
	tbl.Selected = tbl.Selected.Bold(true).Foreground(selectedText).Background(selectedBg)
	return Styles{
		Table:        tbl,
		Title:        lipgloss.NewStyle().Bold(true).Foreground(primary),
		HelpText:     lipgloss.NewStyle().MarginTop(1).Foreground(help),
		ActionButton: lipgloss.NewStyle().Foreground(primary),
		Spinner:      lipgloss.NewStyle().Foreground(primary),
		DimText:      lipgloss.NewStyle().Foreground(dim),
		ErrorText:    lipgloss.NewStyle().Foreground(warning),
		Tab:          lipgloss.NewStyle().Foreground(dim).Padding(0, 1),
		ActiveTab:    lipgloss.NewStyle().Bold(true).Foreground(primary).Padding(0, 1),
		TabSeparator: lipgloss.NewStyle().Foreground(dim),
	}
}

func colorOrDefault(value string, fallback color.Color) color.Color {
	if value == "" {
		return fallback
	}
	return lipgloss.Color(value)
}
