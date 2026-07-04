package tabs

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func TestTabBarHiddenForSingleSection(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{pullsection.NewModel(0, ctx, config.SectionConfig{Title: "PRs"})})
	if got := tb.View(); got != "" {
		t.Fatalf("single-section tab bar = %q, want empty", got)
	}
}

func TestTabBarShowsTwoSections(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{
		pullsection.NewModel(0, ctx, config.SectionConfig{Title: "PRs"}),
		pullsection.NewModel(1, ctx, config.SectionConfig{Title: "Issues"}),
	})
	v := tb.View()
	if !strings.Contains(v, "PRs") || !strings.Contains(v, "Issues") {
		t.Fatalf("two-section tab bar = %q", v)
	}
}

func TestTabAtMapsRenderedCellsToSections(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{
		pullsection.NewModel(0, ctx, config.SectionConfig{Title: "Open Pull Requests"}),
		pullsection.NewModel(1, ctx, config.SectionConfig{Title: "Closed Pull Requests"}),
	})

	firstWidth := lipgloss.Width(ctx.Styles.ActiveTab.Render("Open Pull Requests (0)"))
	secondWidth := lipgloss.Width(ctx.Styles.Tab.Render("Closed Pull Requests (0)"))

	// Offsets are relative to the embedded segment's own left edge: a
	// leading "─" occupies column 0, the first tab starts at column 1, and
	// a "──" separator (2 columns) precedes the next tab.
	if idx, ok := tb.TabAt(1); !ok || idx != 0 {
		t.Fatalf("TabAt(first tab cell) = %d, %v; want 0, true", idx, ok)
	}
	if idx, ok := tb.TabAt(1 + firstWidth + 2); !ok || idx != 1 {
		t.Fatalf("TabAt(second tab cell) = %d, %v; want 1, true", idx, ok)
	}
	if idx, ok := tb.TabAt(1 + firstWidth + 2 + secondWidth); ok {
		t.Fatalf("TabAt(after tabs) = %d, %v; want no tab", idx, ok)
	}
}

func TestTabAtIgnoresHiddenSingleSectionBar(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{pullsection.NewModel(0, ctx, config.SectionConfig{Title: "Only"})})
	if idx, ok := tb.TabAt(1); ok {
		t.Fatalf("TabAt(single-section bar) = %d, %v; want no tab", idx, ok)
	}
}
