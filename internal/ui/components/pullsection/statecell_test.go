package pullsection

import (
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// TestBuildRowStateCellHasGlyphPerState is Task 9's per-section BuildRow
// glyph-presence assertion: every PR state (open/closed/merged, plus the
// draft special-case) renders its section.StateCell glyph in the "state"
// column, for the default unicode icon set.
func TestBuildRowStateCellHasGlyphPerState(t *testing.T) {
	cases := []struct {
		name  string
		pr    data.PullRequest
		state icons.State
	}{
		{"open", data.PullRequest{Number: 1, State: "open"}, icons.Open},
		{"closed", data.PullRequest{Number: 2, State: "closed"}, icons.Closed},
		{"merged", data.PullRequest{Number: 3, State: "merged"}, icons.Merged},
		{"draft", data.PullRequest{Number: 4, State: "open", Draft: true}, icons.Draft},
	}
	ctx := &context.ProgramContext{Styles: context.DefaultStyles(), MainContentWidth: 120, MainContentHeight: 20}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel(0, ctx, config.SectionConfig{Title: "PRs"})
			m.SetLastFetchID("t")
			next, _ := m.Update(SectionPullRequestsFetchedMsg{Rows: []data.PullRequest{c.pr}, TotalCount: 1, TaskId: "t"})
			m = next.(*Model)
			row := m.BuildRows()[0]
			joined := strings.Join([]string(row), "|")
			glyph := icons.Glyph(icons.Unicode, c.state)
			if !strings.Contains(joined, glyph) {
				t.Fatalf("row %q missing glyph %q for state %s", joined, glyph, c.name)
			}
		})
	}
}

// TestBuildRowUsesConfiguredASCIIIconSet is the ctx.Icons threading test:
// an ascii theme.icons config produces pure-ASCII glyphs in the rendered
// row, proving the section reads ctx.Icons (not a hardcoded default).
func TestBuildRowUsesConfiguredASCIIIconSet(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles: context.DefaultStyles(), MainContentWidth: 120, MainContentHeight: 20,
		Icons: icons.ASCII,
	}
	m := NewModel(0, ctx, config.SectionConfig{Title: "PRs"})
	m.SetLastFetchID("t")
	next, _ := m.Update(SectionPullRequestsFetchedMsg{
		Rows:       []data.PullRequest{{Number: 1, State: "open"}},
		TotalCount: 1, TaskId: "t",
	})
	m = next.(*Model)
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	if !strings.Contains(joined, icons.Glyph(icons.ASCII, icons.Open)) {
		t.Fatalf("row %q missing ASCII Open glyph %q", joined, icons.Glyph(icons.ASCII, icons.Open))
	}
	if strings.Contains(joined, icons.Glyph(icons.Unicode, icons.Open)) {
		t.Fatalf("row %q contains the Unicode glyph despite an ASCII icon set configured", joined)
	}
}
