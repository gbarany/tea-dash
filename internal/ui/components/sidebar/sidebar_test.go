package sidebar

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

func longContent(lines int) string {
	rows := make([]string, lines)
	for i := range rows {
		rows[i] = "line of preview content"
	}
	return strings.Join(rows, "\n")
}

// TestViewEmptyWhenClosed verifies the pane draws nothing while the preview is
// closed, and non-empty content once opened.
func TestViewEmptyWhenClosed(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   false,
		PreviewWidth:  40,
		PreviewHeight: 10,
	}
	m := New(ctx)
	m.SetContent("hello preview")

	if got := m.View(); got != "" {
		t.Fatalf("closed preview View() = %q, want empty", got)
	}

	ctx.PreviewOpen = true
	m.UpdateProgramContext(ctx)
	if got := m.View(); strings.TrimSpace(got) == "" {
		t.Fatalf("open preview View() = %q, want non-empty", got)
	}
	if got := m.View(); !strings.Contains(got, "hello preview") {
		t.Fatalf("open preview View() = %q, want it to contain the content", got)
	}
}

// TestFocusedPreviewKeysScroll covers spec §2's "Preview (focused)" row:
// j/k line scroll, d/u half-page, g/G top/bottom all move the viewport and
// report handled=true; an unrelated key (not part of that row) leaves the
// viewport untouched and reports handled=false so app.go knows to fall
// through to its own routing (view jumps, esc, tab/enter to unfocus, ...).
func TestFocusedPreviewKeysScroll(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetContent(longContent(200))

	before := m.vp.ScrollPercent()

	// An unrelated key is left alone and reported unhandled.
	next, cmd, handled := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if handled {
		t.Fatalf("unrelated key '1' should not be handled")
	}
	if cmd != nil {
		t.Fatalf("unrelated key should return a nil cmd, got %v", cmd)
	}
	if got := next.vp.ScrollPercent(); got != before {
		t.Fatalf("unrelated key changed scroll: before=%v after=%v", before, got)
	}
	m = next

	// j/k line-scroll.
	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if !handled {
		t.Fatal("'j' should be handled")
	}
	afterJ := m.vp.ScrollPercent()
	if afterJ <= before {
		t.Fatalf("'j' did not advance scroll: before=%v after=%v", before, afterJ)
	}
	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if !handled {
		t.Fatal("'k' should be handled")
	}
	if got := m.vp.ScrollPercent(); got >= afterJ {
		t.Fatalf("'k' did not reverse scroll: before=%v after=%v", afterJ, got)
	}

	// d/u half-page (bare, per the remap — ctrl+d/ctrl+u also still work,
	// for custom keybindings built around the pre-remap key).
	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !handled {
		t.Fatal("'d' should be handled")
	}
	afterD := m.vp.ScrollPercent()
	if afterD <= before {
		t.Fatalf("'d' (half-page down) did not advance scroll: before=%v after=%v", before, afterD)
	}
	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if !handled {
		t.Fatal("ctrl+d should still be handled (legacy custom-keybinding compatibility)")
	}
	afterCtrlD := m.vp.ScrollPercent()
	if afterCtrlD <= afterD {
		t.Fatalf("ctrl+d did not advance scroll further: before=%v after=%v", afterD, afterCtrlD)
	}
	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if !handled {
		t.Fatal("'u' should be handled")
	}
	if got := m.vp.ScrollPercent(); got >= afterCtrlD {
		t.Fatalf("'u' (half-page up) did not reverse scroll: before=%v after=%v", afterCtrlD, got)
	}

	// g/G top/bottom.
	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !handled {
		t.Fatal("'G' should be handled")
	}
	if got := m.vp.ScrollPercent(); got != 1 {
		t.Fatalf("'G' should scroll to the bottom (100%%), got %v", got)
	}
	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if !handled {
		t.Fatal("'g' should be handled")
	}
	if got := m.vp.ScrollPercent(); got != 0 {
		t.Fatalf("'g' should scroll to the top (0%%), got %v", got)
	}
}

// TestFocusedPreviewKeysCycleTabs covers the "[/]" preview-tab-cycling part
// of spec §2's "Preview (focused)" row, through the same Update path (not
// the direct PrevTab/NextTab methods TestTabsSwitchContent already covers).
func TestFocusedPreviewKeysCycleTabs(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token"},
		{Title: "Checks", Content: "checks-token"},
	})

	m, _, handled := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	if !handled {
		t.Fatal("']' should be handled")
	}
	if got := m.CurrentTabTitle(); got != "Checks" {
		t.Fatalf("after ']' CurrentTabTitle = %q, want Checks", got)
	}
	m, _, handled = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	if !handled {
		t.Fatal("'[' should be handled")
	}
	if got := m.CurrentTabTitle(); got != "Overview" {
		t.Fatalf("after '[' CurrentTabTitle = %q, want Overview", got)
	}
}

func TestTabsSwitchContent(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token"},
		{Title: "Checks", Content: "checks-token"},
	})

	view := m.View()
	if !strings.Contains(view, "overview-token") {
		t.Fatalf("initial tab view missing content:\n%s", view)
	}
	segment := m.TabsBorderSegment()
	for _, want := range []string{"Overview", "Checks"} {
		if !strings.Contains(segment, want) {
			t.Fatalf("tabs border segment missing %q:\n%s", want, segment)
		}
	}

	m.NextTab()
	if got := m.CurrentTabTitle(); got != "Checks" {
		t.Fatalf("CurrentTabTitle after NextTab = %q, want Checks", got)
	}
	if want := ctx.Styles.ActiveTab.Render("Checks"); !strings.Contains(m.TabsBorderSegment(), want) {
		t.Fatalf("tabs border segment should highlight the active tab:\n%s", m.TabsBorderSegment())
	}
	view = m.View()
	if !strings.Contains(view, "checks-token") || strings.Contains(view, "overview-token") {
		t.Fatalf("next tab should show checks content only:\n%s", view)
	}

	m.PrevTab()
	if got := m.CurrentTabTitle(); got != "Overview" {
		t.Fatalf("CurrentTabTitle after PrevTab = %q, want Overview", got)
	}
	view = m.View()
	if !strings.Contains(view, "overview-token") || strings.Contains(view, "checks-token") {
		t.Fatalf("prev tab should show overview content only:\n%s", view)
	}
}

// TestSelectTabJumpsDirectly covers Task 6's ZonePreviewTab click handler:
// SelectTab jumps straight to a tab by index (unlike Next/PrevTab's
// relative cycling), switches content, and a redundant SelectTab on the
// tab already showing is a no-op (doesn't reset scroll via GotoTop).
func TestSelectTabJumpsDirectly(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token"},
		{Title: "Checks", Content: "checks-token"},
		{Title: "Reviews", Content: "reviews-token"},
	})

	m.SelectTab(2)
	if got := m.CurrentTabTitle(); got != "Reviews" {
		t.Fatalf("CurrentTabTitle after SelectTab(2) = %q, want Reviews", got)
	}
	view := m.View()
	if !strings.Contains(view, "reviews-token") || strings.Contains(view, "overview-token") {
		t.Fatalf("SelectTab(2) should show only the reviews content:\n%s", view)
	}

	m.SelectTab(2) // already there: no-op, must not panic or change tab
	if got := m.CurrentTabTitle(); got != "Reviews" {
		t.Fatalf("redundant SelectTab(2) changed tab to %q", got)
	}

	m.SelectTab(-1) // out of range: no-op
	m.SelectTab(99) // out of range: no-op
	if got := m.CurrentTabTitle(); got != "Reviews" {
		t.Fatalf("out-of-range SelectTab changed tab to %q, want unchanged Reviews", got)
	}
}

// TestTabsBorderSegmentEllipsizesAndDropsTrailingTabs mirrors
// components/tabs' identical truncation test: once ctx.PreviewWidth
// constrains TabsBorderSegment, a tab that doesn't fully fit is
// ellipsized (never cut bare mid-word) and every tab after it is dropped
// entirely — and TabRanges() must describe exactly what's rendered, since
// Task 6's ZonePreviewTab registration builds its click Rects from it.
func TestTabsBorderSegmentEllipsizesAndDropsTrailingTabs(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "o"},
		{Title: "Review Comments", Content: "r"},
		{Title: "Checks", Content: "c"},
	})

	firstFullWidth := lipgloss.Width(ctx.Styles.ActiveTab.Render("Overview"))
	ctx.PreviewWidth = 1 + firstFullWidth + 2 + 4

	segment := m.TabsBorderSegment()
	if !strings.Contains(segment, "Overview") {
		t.Fatalf("first (fully-fitting) tab missing:\n%q", segment)
	}
	if strings.Contains(segment, "Comments") {
		t.Fatalf("second tab should be ellipsized before \"Comments\", got:\n%q", segment)
	}
	if !strings.Contains(segment, "…") {
		t.Fatalf("truncated tab should end in an ellipsis, got:\n%q", segment)
	}
	if strings.Contains(segment, "Checks") {
		t.Fatalf("third tab should be dropped entirely once the second is truncated, got:\n%q", segment)
	}

	ranges := m.TabRanges()
	if len(ranges) != 2 {
		t.Fatalf("TabRanges() = %+v, want exactly 2 (third tab dropped)", ranges)
	}
	if lipgloss.Width(segment) != ranges[1].End {
		t.Fatalf("rendered width = %d, want to end exactly at the truncated tab's End %d", lipgloss.Width(segment), ranges[1].End)
	}
}

// TestSetTabsPreservesSelectedTabByTitle: re-rendering the preview (e.g. on a
// refresh) must keep the user's selected tab, not snap back to Overview.
func TestSetTabsPreservesSelectedTabByTitle(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token"},
		{Title: "Checks", Content: "checks-token"},
	})
	m.NextTab() // user selects Checks

	// Re-render with the same titles (what a refresh does once detail reloads).
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token-2"},
		{Title: "Checks", Content: "checks-token-2"},
	})
	if got := m.CurrentTabTitle(); got != "Checks" {
		t.Fatalf("SetTabs should preserve the selected tab by title, got %q, want Checks", got)
	}
}

// TestSetTabsRestoresTabAfterTransientCollapse reproduces the refresh shape: the
// row's detail is cleared first (so only the Overview tab renders), then the full
// set returns when the re-fetch lands. The selection must survive the round trip.
// This is the case a naive "preserve the current title" approach fails, because
// the collapse rewrites the current title to Overview.
func TestSetTabsRestoresTabAfterTransientCollapse(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token"},
		{Title: "Checks", Content: "checks-token"},
	})
	m.NextTab() // user selects Checks

	// Detail cleared: only Overview renders. Must show Overview (Checks is gone).
	m.SetTabs([]Tab{{Title: "Overview", Content: "overview-only"}})
	if got := m.CurrentTabTitle(); got != "Overview" {
		t.Fatalf("with only Overview present, want Overview, got %q", got)
	}

	// Detail re-lands: full set returns. Must snap back to Checks.
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token"},
		{Title: "Checks", Content: "checks-token"},
	})
	if got := m.CurrentTabTitle(); got != "Checks" {
		t.Fatalf("selected tab should be restored after the set returns, got %q, want Checks", got)
	}
}

// TestSetTabsDefaultsToFirstWithoutExplicitSelection: a user who never switches
// tabs stays on Overview across re-renders (today's behavior, preserved).
func TestSetTabsDefaultsToFirstWithoutExplicitSelection(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "a"},
		{Title: "Checks", Content: "b"},
	})
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "c"},
		{Title: "Checks", Content: "d"},
	})
	if got := m.CurrentTabTitle(); got != "Overview" {
		t.Fatalf("no explicit selection should stay on Overview, got %q", got)
	}
}
