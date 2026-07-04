package notificationsection

import (
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// TestBuildRowStateCellHasGlyphPerState is Task 9's per-section BuildRow
// glyph-presence assertion for notifications: unread/read/pinned all
// render a section.StateCell glyph in the "state" column. Unread has no
// styles.StateColors entry (T2/T9 note) but still gets a glyph via the
// explicit-style path.
func TestBuildRowStateCellHasGlyphPerState(t *testing.T) {
	cases := []struct {
		name string
		n    data.Notification
		st   icons.State
	}{
		{"unread", data.Notification{ID: 1, Unread: true}, icons.Unread},
		{"read", data.Notification{ID: 2}, icons.Neutral},
		{"pinned", data.Notification{ID: 3, Pinned: true}, icons.Neutral},
	}
	ctx := &context.ProgramContext{Styles: context.DefaultStyles(), MainContentWidth: 120, MainContentHeight: 20}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel(0, ctx, config.SectionConfig{Title: "Notifications"})
			m.SetLastFetchID("t")
			next, _ := m.Update(SectionNotificationsFetchedMsg{Rows: []data.Notification{c.n}, TotalCount: 1, TaskId: "t"})
			m = next.(*Model)
			row := m.BuildRows()[0]
			joined := strings.Join([]string(row), "|")
			glyph := icons.Glyph(icons.Unicode, c.st)
			if !strings.Contains(joined, glyph) {
				t.Fatalf("row %q missing glyph %q for state %s", joined, glyph, c.name)
			}
		})
	}
}

// TestBuildRowUsesConfiguredASCIIIconSet is the ctx.Icons threading test.
func TestBuildRowUsesConfiguredASCIIIconSet(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles: context.DefaultStyles(), MainContentWidth: 120, MainContentHeight: 20,
		Icons: icons.ASCII,
	}
	m := NewModel(0, ctx, config.SectionConfig{Title: "Notifications"})
	m.SetLastFetchID("t")
	next, _ := m.Update(SectionNotificationsFetchedMsg{
		Rows:       []data.Notification{{ID: 1, Unread: true}},
		TotalCount: 1, TaskId: "t",
	})
	m = next.(*Model)
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	if !strings.Contains(joined, icons.Glyph(icons.ASCII, icons.Unread)) {
		t.Fatalf("row %q missing ASCII Unread glyph %q", joined, icons.Glyph(icons.ASCII, icons.Unread))
	}
}
