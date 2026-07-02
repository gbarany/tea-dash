package context

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
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
	assertColor(t, s.HelpText.GetForeground(), lipgloss.Color("#8A889D"), "help foreground")
	assertColor(t, s.Tab.GetForeground(), lipgloss.Color("#8A889D"), "tab foreground")
	assertColor(t, s.TabSeparator.GetForeground(), lipgloss.Color("#8A889D"), "tab separator foreground")
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
