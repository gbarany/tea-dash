package context

import (
	"image/color"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/ui/icons"
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

	// BorderFocused and BorderBlurred are border-color-only styles (just a
	// foreground color, no shape/side decoration): the framed shell draws its
	// own box-art border characters and colors them with these, since panel
	// borders embed tab titles inline (a plain lipgloss.Style.Border() box
	// can't do that).
	BorderFocused lipgloss.Style
	BorderBlurred lipgloss.Style
	// PanelTitle styles panel/section titles embedded in a border line.
	PanelTitle lipgloss.Style
	// StatusBar styles the bottom status-bar row's base text.
	StatusBar lipgloss.Style
	// StatusToastSuccess/Error/Info style the status bar's transient
	// left-segment toast text by outcome.
	StatusToastSuccess lipgloss.Style
	StatusToastError   lipgloss.Style
	StatusToastInfo    lipgloss.Style
	// StateColors maps a row/CI/notification state to its display color,
	// following gh-style conventions (open/success green, merged purple,
	// closed/failure red, running amber, neutral faint). Overridable via
	// theme.colors.state.
	StateColors map[icons.State]lipgloss.Style
}

const (
	defaultPrimary      = "#00ADD8"
	defaultDim          = "240"
	defaultHelp         = "241"
	defaultError        = "196"
	defaultSelectedText = "229"
	defaultSelectedBg   = "57"

	// gh-convention state color defaults.
	defaultStateOpen    = "#2da44e"
	defaultStateDraft   = "#6e7781"
	defaultStateMerged  = "#8250df"
	defaultStateClosed  = "#cf222e"
	defaultStateSuccess = "#2da44e"
	defaultStateFailure = "#cf222e"
	defaultStateRunning = "#d4a72c"
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
		config.ThemeStateColors{},
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
	return stylesFromColors(primary, dim, help, warning, selectedText, selectedBg, colors.State)
}

func stylesFromColors(primary, dim, help, warning, selectedText, selectedBg color.Color, state config.ThemeStateColors) Styles {
	tbl := table.DefaultStyles()
	tbl.Header = tbl.Header.Bold(true).Foreground(primary).BorderBottom(true)
	tbl.Selected = tbl.Selected.Bold(true).Foreground(selectedText).Background(selectedBg)

	openColor := colorOrDefault(state.Open, lipgloss.Color(defaultStateOpen))
	draftColor := colorOrDefault(state.Draft, lipgloss.Color(defaultStateDraft))
	mergedColor := colorOrDefault(state.Merged, lipgloss.Color(defaultStateMerged))
	closedColor := colorOrDefault(state.Closed, lipgloss.Color(defaultStateClosed))
	successColor := colorOrDefault(state.Success, lipgloss.Color(defaultStateSuccess))
	failureColor := colorOrDefault(state.Failure, lipgloss.Color(defaultStateFailure))
	runningColor := colorOrDefault(state.Running, lipgloss.Color(defaultStateRunning))
	// Neutral's default is the same faint/dim color as the rest of the UI
	// (overridable independently via theme.colors.state.neutral).
	neutralColor := colorOrDefault(state.Neutral, dim)

	stateColors := map[icons.State]lipgloss.Style{
		icons.Open:    lipgloss.NewStyle().Foreground(openColor),
		icons.Draft:   lipgloss.NewStyle().Foreground(draftColor),
		icons.Merged:  lipgloss.NewStyle().Foreground(mergedColor),
		icons.Closed:  lipgloss.NewStyle().Foreground(closedColor),
		icons.Success: lipgloss.NewStyle().Foreground(successColor),
		icons.Failure: lipgloss.NewStyle().Foreground(failureColor),
		icons.Running: lipgloss.NewStyle().Foreground(runningColor),
		icons.Neutral: lipgloss.NewStyle().Foreground(neutralColor),
	}

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

		BorderFocused:      lipgloss.NewStyle().Foreground(primary),
		BorderBlurred:      lipgloss.NewStyle().Foreground(dim),
		PanelTitle:         lipgloss.NewStyle().Bold(true).Foreground(primary),
		StatusBar:          lipgloss.NewStyle().Foreground(dim),
		StatusToastSuccess: lipgloss.NewStyle().Bold(true).Foreground(successColor),
		StatusToastError:   lipgloss.NewStyle().Bold(true).Foreground(failureColor),
		StatusToastInfo:    lipgloss.NewStyle().Foreground(primary),
		StateColors:        stateColors,
	}
}

func colorOrDefault(value string, fallback color.Color) color.Color {
	if value == "" {
		return fallback
	}
	return lipgloss.Color(value)
}
