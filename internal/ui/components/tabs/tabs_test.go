package tabs

import (
	"strings"
	"testing"

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
