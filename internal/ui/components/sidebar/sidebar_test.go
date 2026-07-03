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

// TestCtrlDScrolls verifies ctrl+d advances the viewport (scroll percent grows)
// on content taller than the pane, and that other keys are ignored.
func TestCtrlDScrolls(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetContent(longContent(200))

	before := m.vp.ScrollPercent()

	// An unrelated key must not move the viewport.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if got := m.vp.ScrollPercent(); got != before {
		t.Fatalf("unrelated key changed scroll: before=%v after=%v", before, got)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if after := m.vp.ScrollPercent(); after <= before {
		t.Fatalf("ctrl+d did not advance scroll: before=%v after=%v", before, after)
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
	for _, want := range []string{"Overview", "Checks", "overview-token"} {
		if !strings.Contains(view, want) {
			t.Fatalf("initial tab view missing %q:\n%s", want, view)
		}
	}

	m.NextTab()
	if got := m.CurrentTabTitle(); got != "Checks" {
		t.Fatalf("CurrentTabTitle after NextTab = %q, want Checks", got)
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
