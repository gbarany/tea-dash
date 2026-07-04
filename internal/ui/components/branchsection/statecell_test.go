package branchsection

import (
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/config"
	localgit "github.com/gbarany/tea-dash/internal/git"
	"github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// TestBuildRowStateCellHasGlyphPerState is Task 9's per-section BuildRow
// glyph-presence assertion for branches: ahead-only and behind-only both
// render their AheadArrow/BehindArrow glyph in the "state" column.
// AheadArrow/BehindArrow have no styles.StateColors entry (T2/T9 note) —
// branchStateCell colors them via the explicit DimText style instead.
func TestBuildRowStateCellHasGlyphPerState(t *testing.T) {
	cases := []struct {
		name   string
		branch localgit.Branch
		wantSt icons.State
	}{
		{"ahead", localgit.Branch{Name: "feature/x", Ahead: 2}, icons.AheadArrow},
		{"behind", localgit.Branch{Name: "feature/y", Behind: 3}, icons.BehindArrow},
	}
	ctx := &context.ProgramContext{Config: &config.Config{}, Styles: context.DefaultStyles(), MainContentWidth: 120, MainContentHeight: 20}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel(0, ctx, config.SectionConfig{Title: "Local Branches"})
			m.SetLastFetchID("t")
			next, _ := m.Update(SectionBranchesFetchedMsg{Rows: []localgit.Branch{c.branch}, TotalCount: 1, TaskId: "t"})
			m = next.(*Model)
			row := m.BuildRows()[0]
			joined := strings.Join([]string(row), "|")
			glyph := icons.Glyph(icons.Unicode, c.wantSt)
			if !strings.Contains(joined, glyph) {
				t.Fatalf("row %q missing glyph %q for state %s", joined, glyph, c.name)
			}
		})
	}
}

// TestBuildRowUsesConfiguredASCIIIconSet is the ctx.Icons threading test.
func TestBuildRowUsesConfiguredASCIIIconSet(t *testing.T) {
	ctx := &context.ProgramContext{
		Config: &config.Config{}, Styles: context.DefaultStyles(), MainContentWidth: 120, MainContentHeight: 20,
		Icons: icons.ASCII,
	}
	m := NewModel(0, ctx, config.SectionConfig{Title: "Local Branches"})
	m.SetLastFetchID("t")
	next, _ := m.Update(SectionBranchesFetchedMsg{
		Rows:       []localgit.Branch{{Name: "feature/x", Ahead: 1}},
		TotalCount: 1, TaskId: "t",
	})
	m = next.(*Model)
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	if !strings.Contains(joined, icons.Glyph(icons.ASCII, icons.AheadArrow)) {
		t.Fatalf("row %q missing ASCII AheadArrow glyph %q", joined, icons.Glyph(icons.ASCII, icons.AheadArrow))
	}
}
