package actionsection

import (
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// TestBuildRowStateCellHasGlyphPerState is Task 9's per-section BuildRow
// glyph-presence assertion for Actions: success/failure/running/cancelled/
// waiting all render a section.StateCell glyph in the "state" column.
// ClassifyState prefers the rightmost ("conclusion") token in a
// "status/conclusion" pair, so cases combine a completed status with each
// conclusion, plus a bare in-flight status for running/waiting.
func TestBuildRowStateCellHasGlyphPerState(t *testing.T) {
	cases := []struct {
		name   string
		run    data.ActionRun
		wantSt icons.State
	}{
		{"success", data.ActionRun{ID: 1, Status: "completed", Conclusion: "success"}, icons.Success},
		{"failure", data.ActionRun{ID: 2, Status: "completed", Conclusion: "failure"}, icons.Failure},
		{"cancelled", data.ActionRun{ID: 3, Status: "completed", Conclusion: "cancelled"}, icons.Failure},
		{"running", data.ActionRun{ID: 4, Status: "in_progress"}, icons.Running},
		{"waiting", data.ActionRun{ID: 5, Status: "waiting"}, icons.Running},
	}
	ctx := &context.ProgramContext{Styles: context.DefaultStyles(), MainContentWidth: 140, MainContentHeight: 20}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel(0, ctx, config.SectionConfig{Title: "CI", Repo: "acme/widgets"})
			m.SetLastFetchID("t")
			next, _ := m.Update(SectionActionsFetchedMsg{Rows: []data.ActionRun{c.run}, TotalCount: 1, TaskId: "t"})
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
		Styles: context.DefaultStyles(), MainContentWidth: 140, MainContentHeight: 20,
		Icons: icons.ASCII,
	}
	m := NewModel(0, ctx, config.SectionConfig{Title: "CI", Repo: "acme/widgets"})
	m.SetLastFetchID("t")
	next, _ := m.Update(SectionActionsFetchedMsg{
		Rows:       []data.ActionRun{{ID: 1, Status: "completed", Conclusion: "success"}},
		TotalCount: 1, TaskId: "t",
	})
	m = next.(*Model)
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	if !strings.Contains(joined, icons.Glyph(icons.ASCII, icons.Success)) {
		t.Fatalf("row %q missing ASCII Success glyph %q", joined, icons.Glyph(icons.ASCII, icons.Success))
	}
}
