package tabs

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

// TestTabBarShowsSingleSectionTitle is the review fix: a single-section
// view (e.g. Inbox, Branches) still renders its "Title (N)" segment in the
// border, rather than leaving it an unlabeled dash rule — the old "hidden
// below two sections" rule predates the framed shell.
func TestTabBarShowsSingleSectionTitle(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{pullsection.NewModel(0, ctx, config.SectionConfig{Title: "PRs"})})
	got := tb.View()
	if got == "" {
		t.Fatal("single-section tab bar should render its title, got empty")
	}
	if !strings.Contains(got, "PRs") {
		t.Fatalf("single-section tab bar = %q, want it to contain the title", got)
	}
}

func TestTabBarHiddenForZeroSections(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections(nil)
	if got := tb.View(); got != "" {
		t.Fatalf("zero-section tab bar = %q, want empty", got)
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

// TestViewEllipsizesTabsInsteadOfCuttingMidWordAndDropsTrailingOnes covers
// the plan's Task 6 carry-forward (T3 review, Minor): at a narrow width the
// tab bar used to hard-truncate the whole joined string wherever it landed
// (app.go's embedInBorderRow), producing glitches like "Needs My Rev". Once
// ctx.MainContentWidth constrains the bar, a tab that doesn't fully fit is
// ellipsized instead (whole-label priority, never a bare mid-word cut), and
// every tab after it is dropped entirely rather than partially shown.
func TestViewEllipsizesTabsInsteadOfCuttingMidWordAndDropsTrailingOnes(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{
		pullsection.NewModel(0, ctx, config.SectionConfig{Title: "My Pull Requests"}),
		pullsection.NewModel(1, ctx, config.SectionConfig{Title: "Needs My Review"}),
		pullsection.NewModel(2, ctx, config.SectionConfig{Title: "All Open"}),
	})

	firstFullWidth := lipgloss.Width(ctx.Styles.ActiveTab.Render("My Pull Requests (0)"))
	// leading "─" (1) + the first tab in full + "──" (2) + just enough of
	// the second tab's own width to prove it's mid-render, well short of
	// "Review" (let alone "Rev").
	ctx.MainContentWidth = 1 + firstFullWidth + 2 + 6

	v := tb.View()
	if !strings.Contains(v, "My Pull Requests") {
		t.Fatalf("first (fully-fitting) tab missing:\n%q", v)
	}
	if strings.Contains(v, "Rev") {
		t.Fatalf("second tab should be ellipsized well before \"Rev\" (the old mid-word-cut bug), got:\n%q", v)
	}
	if !strings.Contains(v, "…") {
		t.Fatalf("truncated tab should end in an ellipsis, got:\n%q", v)
	}
	if strings.Contains(v, "All Open") {
		t.Fatalf("third tab should be dropped entirely once the second is truncated, got:\n%q", v)
	}
	if got := lipgloss.Width(v); got > ctx.MainContentWidth {
		t.Fatalf("rendered tab bar width = %d, want <= MainContentWidth %d:\n%q", got, ctx.MainContentWidth, v)
	}
}

// TestRangesAndTabAtAgreeWithTruncatedView confirms TabAt (and the Ranges
// app.go's zone registration builds from) reflect exactly the truncated
// render from the test above — a click past the ellipsized tab's visible
// end must miss (never resolve to the dropped third tab), and the second
// tab's range must span exactly what's drawn for it.
func TestRangesAndTabAtAgreeWithTruncatedView(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{
		pullsection.NewModel(0, ctx, config.SectionConfig{Title: "My Pull Requests"}),
		pullsection.NewModel(1, ctx, config.SectionConfig{Title: "Needs My Review"}),
		pullsection.NewModel(2, ctx, config.SectionConfig{Title: "All Open"}),
	})
	firstFullWidth := lipgloss.Width(ctx.Styles.ActiveTab.Render("My Pull Requests (0)"))
	ctx.MainContentWidth = 1 + firstFullWidth + 2 + 6

	ranges := tb.Ranges()
	if len(ranges) != 2 {
		t.Fatalf("Ranges() = %+v, want exactly 2 (third tab dropped)", ranges)
	}
	if ranges[0].Index != 0 || ranges[1].Index != 1 {
		t.Fatalf("Ranges() indices = %d, %d; want 0, 1", ranges[0].Index, ranges[1].Index)
	}
	// Every cell inside the second (truncated) tab's own range must
	// resolve to it, and the cell immediately after must resolve to
	// nothing (not the dropped third tab).
	if idx, ok := tb.TabAt(ranges[1].Start); !ok || idx != 1 {
		t.Fatalf("TabAt(second tab's first cell) = %d, %v; want 1, true", idx, ok)
	}
	if idx, ok := tb.TabAt(ranges[1].End - 1); !ok || idx != 1 {
		t.Fatalf("TabAt(second tab's last cell) = %d, %v; want 1, true", idx, ok)
	}
	if idx, ok := tb.TabAt(ranges[1].End); ok {
		t.Fatalf("TabAt(just past truncated tab) = %d, %v; want no tab", idx, ok)
	}
	if lipgloss.Width(tb.View()) != ranges[1].End {
		t.Fatalf("rendered width = %d, want to end exactly at the truncated tab's End %d", lipgloss.Width(tb.View()), ranges[1].End)
	}
}

// TestTabAtHitsSingleSectionTab: since a single section now renders its
// title (see TestTabBarShowsSingleSectionTitle), clicking within that
// rendered range resolves to index 0 — there's nothing to switch to, but
// the hit-test itself must still work like any other tab, not be
// specially suppressed.
func TestTabAtHitsSingleSectionTab(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{pullsection.NewModel(0, ctx, config.SectionConfig{Title: "Only"})})
	ranges := tb.Ranges()
	if len(ranges) != 1 {
		t.Fatalf("Ranges() = %+v, want exactly 1 range for a single section", ranges)
	}
	if idx, ok := tb.TabAt(ranges[0].Start); !ok || idx != 0 {
		t.Fatalf("TabAt(single-section bar's own range) = %d, %v; want 0, true", idx, ok)
	}
}

func TestTabAtNoHitOnZeroSectionBar(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections(nil)
	if idx, ok := tb.TabAt(1); ok {
		t.Fatalf("TabAt(zero-section bar) = %d, %v; want no tab", idx, ok)
	}
}
