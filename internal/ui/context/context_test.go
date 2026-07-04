package context

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

func TestDefaultStylesNonZero(t *testing.T) {
	s := DefaultStyles()
	// Spinner/DimText/ErrorText must be usable (Render must not panic).
	_ = s.Spinner.Render("x")
	_ = s.DimText.Render("x")
	_ = s.ErrorText.Render("x")
}

func TestStylesForConfigAppliesThemeColors(t *testing.T) {
	s := StylesForConfig(&config.Config{
		Theme: config.Theme{
			Colors: config.ThemeColors{
				Text: config.ThemeTextColors{
					Primary: "#CBE3E7",
					Faint:   "#8A889D",
					Warning: "#F48FB1",
				},
				Background: config.ThemeBackgroundColors{
					Selected: "#3E3859",
				},
			},
		},
	})

	assertColor(t, s.Title.GetForeground(), lipgloss.Color("#CBE3E7"), "title foreground")
	assertColor(t, s.Spinner.GetForeground(), lipgloss.Color("#CBE3E7"), "spinner foreground")
	assertColor(t, s.ActionButton.GetForeground(), lipgloss.Color("#CBE3E7"), "action button foreground")
	assertColor(t, s.Table.Header.GetForeground(), lipgloss.Color("#CBE3E7"), "table header foreground")
	assertColor(t, s.ActiveTab.GetForeground(), lipgloss.Color("#CBE3E7"), "active tab foreground")
	assertColor(t, s.DimText.GetForeground(), lipgloss.Color("#8A889D"), "dim text foreground")
	assertColor(t, s.Tab.GetForeground(), lipgloss.Color("#8A889D"), "tab foreground")
	assertColor(t, s.ErrorText.GetForeground(), lipgloss.Color("#F48FB1"), "error foreground")
	assertColor(t, s.Table.Selected.GetBackground(), lipgloss.Color("#3E3859"), "selected row background")
}

func assertColor(t *testing.T, got, want color.Color, label string) {
	t.Helper()
	gr, gg, gb, ga := got.RGBA()
	wr, wg, wb, wa := want.RGBA()
	if gr != wr || gg != wg || gb != wb || ga != wa {
		t.Fatalf("%s color = rgba(%d,%d,%d,%d), want rgba(%d,%d,%d,%d)", label, gr, gg, gb, ga, wr, wg, wb, wa)
	}
}

func TestDefaultStylesIncludeNewStyleFields(t *testing.T) {
	s := DefaultStyles()

	// Border, panel-title, status-bar, and toast styles must render without
	// panicking and be distinguishable border colors.
	_ = s.BorderFocused.Render("x")
	_ = s.BorderBlurred.Render("x")
	_ = s.PanelTitle.Render("x")
	_ = s.StatusBar.Render("x")
	_ = s.StatusToastSuccess.Render("x")
	_ = s.StatusToastError.Render("x")
	_ = s.StatusToastInfo.Render("x")

	if s.BorderFocused.GetForeground() == s.BorderBlurred.GetForeground() {
		t.Fatal("BorderFocused and BorderBlurred should use different default colors")
	}

	if s.StateColors == nil {
		t.Fatal("DefaultStyles().StateColors should not be nil")
	}
	for _, state := range []icons.State{
		icons.Open, icons.Draft, icons.Merged, icons.Closed,
		icons.Success, icons.Failure, icons.Running, icons.Neutral,
	} {
		style, ok := s.StateColors[state]
		if !ok {
			t.Fatalf("DefaultStyles().StateColors missing entry for state %v", state)
		}
		_ = style.Render("x")
	}

	// gh-convention defaults.
	assertColor(t, s.StateColors[icons.Open].GetForeground(), lipgloss.Color("#2da44e"), "default open state color")
	assertColor(t, s.StateColors[icons.Merged].GetForeground(), lipgloss.Color("#8250df"), "default merged state color")
	assertColor(t, s.StateColors[icons.Closed].GetForeground(), lipgloss.Color("#cf222e"), "default closed state color")
	assertColor(t, s.StateColors[icons.Success].GetForeground(), lipgloss.Color("#2da44e"), "default success state color")
	assertColor(t, s.StateColors[icons.Failure].GetForeground(), lipgloss.Color("#cf222e"), "default failure state color")
	assertColor(t, s.StateColors[icons.Running].GetForeground(), lipgloss.Color("#d4a72c"), "default running state color")
}

func TestStylesForConfigStateColorOverrideLands(t *testing.T) {
	def := DefaultStyles()
	cfg := &config.Config{
		Theme: config.Theme{
			Colors: config.ThemeColors{
				State: config.ThemeStateColors{
					Open: "#123456",
				},
			},
		},
	}
	s := StylesForConfig(cfg)

	assertColor(t, s.StateColors[icons.Open].GetForeground(), lipgloss.Color("#123456"), "configured open state color")

	probeDefault := def.StateColors[icons.Open].Render("state")
	probeConfigured := s.StateColors[icons.Open].Render("state")
	if probeDefault == probeConfigured {
		t.Fatal("configured open state override should render differently from the default")
	}

	// Every other state color keeps its default when unconfigured.
	assertColor(t, s.StateColors[icons.Draft].GetForeground(), def.StateColors[icons.Draft].GetForeground(), "draft state color should stay default")
	assertColor(t, s.StateColors[icons.Merged].GetForeground(), def.StateColors[icons.Merged].GetForeground(), "merged state color should stay default")
	assertColor(t, s.StateColors[icons.Closed].GetForeground(), def.StateColors[icons.Closed].GetForeground(), "closed state color should stay default")
	assertColor(t, s.StateColors[icons.Success].GetForeground(), def.StateColors[icons.Success].GetForeground(), "success state color should stay default")
	assertColor(t, s.StateColors[icons.Failure].GetForeground(), def.StateColors[icons.Failure].GetForeground(), "failure state color should stay default")
	assertColor(t, s.StateColors[icons.Running].GetForeground(), def.StateColors[icons.Running].GetForeground(), "running state color should stay default")
	assertColor(t, s.StateColors[icons.Neutral].GetForeground(), def.StateColors[icons.Neutral].GetForeground(), "neutral state color should stay default")
}

func TestStylesForConfigAllStateColorsOverride(t *testing.T) {
	cfg := &config.Config{
		Theme: config.Theme{
			Colors: config.ThemeColors{
				State: config.ThemeStateColors{
					Open:    "#111111",
					Draft:   "#222222",
					Merged:  "#333333",
					Closed:  "#444444",
					Success: "#555555",
					Failure: "#666666",
					Running: "#777777",
					Neutral: "#888888",
				},
			},
		},
	}
	s := StylesForConfig(cfg)

	assertColor(t, s.StateColors[icons.Open].GetForeground(), lipgloss.Color("#111111"), "open override")
	assertColor(t, s.StateColors[icons.Draft].GetForeground(), lipgloss.Color("#222222"), "draft override")
	assertColor(t, s.StateColors[icons.Merged].GetForeground(), lipgloss.Color("#333333"), "merged override")
	assertColor(t, s.StateColors[icons.Closed].GetForeground(), lipgloss.Color("#444444"), "closed override")
	assertColor(t, s.StateColors[icons.Success].GetForeground(), lipgloss.Color("#555555"), "success override")
	assertColor(t, s.StateColors[icons.Failure].GetForeground(), lipgloss.Color("#666666"), "failure override")
	assertColor(t, s.StateColors[icons.Running].GetForeground(), lipgloss.Color("#777777"), "running override")
	assertColor(t, s.StateColors[icons.Neutral].GetForeground(), lipgloss.Color("#888888"), "neutral override")
}

func TestStylesForConfigEmptyThemeLeavesExistingStylesUnchanged(t *testing.T) {
	def := DefaultStyles()
	s := StylesForConfig(&config.Config{})

	assertColor(t, s.Title.GetForeground(), def.Title.GetForeground(), "title foreground")
	assertColor(t, s.Spinner.GetForeground(), def.Spinner.GetForeground(), "spinner foreground")
	assertColor(t, s.ActionButton.GetForeground(), def.ActionButton.GetForeground(), "action button foreground")
	assertColor(t, s.DimText.GetForeground(), def.DimText.GetForeground(), "dim text foreground")
	assertColor(t, s.ErrorText.GetForeground(), def.ErrorText.GetForeground(), "error text foreground")
	assertColor(t, s.Tab.GetForeground(), def.Tab.GetForeground(), "tab foreground")
	assertColor(t, s.ActiveTab.GetForeground(), def.ActiveTab.GetForeground(), "active tab foreground")
	assertColor(t, s.BorderFocused.GetForeground(), def.BorderFocused.GetForeground(), "border focused foreground")
	assertColor(t, s.BorderBlurred.GetForeground(), def.BorderBlurred.GetForeground(), "border blurred foreground")
	assertColor(t, s.PanelTitle.GetForeground(), def.PanelTitle.GetForeground(), "panel title foreground")
	assertColor(t, s.StatusBar.GetForeground(), def.StatusBar.GetForeground(), "status bar foreground")
}

func TestGetViewSectionsConfig(t *testing.T) {
	ctx := &ProgramContext{}
	secs := ctx.GetViewSectionsConfig()
	if len(secs) != 2 {
		t.Fatalf("GetViewSectionsConfig = %+v", secs)
	}
	if secs[0].Title != "Open Pull Requests" || secs[1].Title != "Closed Pull Requests" {
		t.Fatalf("default PR section titles = %+v, want open + closed history tabs", secs)
	}
	// The default PR sections are filter-driven: me-scoped open and closed state.
	if secs[0].Filter.CreatedBy != "@me" || secs[0].Filter.State != "open" {
		t.Fatalf("default open PR filter = %+v, want CreatedBy=@me State=open", secs[0].Filter)
	}
	if secs[1].Filter.CreatedBy != "@me" || secs[1].Filter.State != "closed" {
		t.Fatalf("default closed PR filter = %+v, want CreatedBy=@me State=closed", secs[1].Filter)
	}
}

func TestGetViewSectionsConfigIssues(t *testing.T) {
	ctx := &ProgramContext{View: IssuesView}
	secs := ctx.GetViewSectionsConfig()
	if len(secs) != 1 || secs[0].Title != "My Issues" {
		t.Fatalf("GetViewSectionsConfig(IssuesView) = %+v", secs)
	}
	if secs[0].Filter.CreatedBy != "@me" || secs[0].Filter.State != "open" {
		t.Fatalf("default issues filter = %+v, want CreatedBy=@me State=open", secs[0].Filter)
	}
}

func TestGetViewSectionsConfigActions(t *testing.T) {
	ctx := &ProgramContext{View: ActionsView}
	secs := ctx.GetViewSectionsConfig()
	if len(secs) != 1 || secs[0].Title != "Actions" {
		t.Fatalf("GetViewSectionsConfig(ActionsView) = %+v, want one empty-state section", secs)
	}
	if secs[0].Repo != "" {
		t.Fatalf("default actions repo = %q, want blank repo for no-config empty state", secs[0].Repo)
	}

	ctx = &ProgramContext{
		View: ActionsView,
		Config: &config.Config{ActionsSections: []config.SectionConfig{{
			Title: "CI",
			Repo:  "acme/widgets",
		}}},
	}
	secs = ctx.GetViewSectionsConfig()
	if len(secs) != 1 || secs[0].Title != "CI" || secs[0].Repo != "acme/widgets" {
		t.Fatalf("configured actions sections = %+v", secs)
	}
}
