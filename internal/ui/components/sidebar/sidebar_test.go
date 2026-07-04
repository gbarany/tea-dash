package sidebar

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
