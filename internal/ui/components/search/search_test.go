package search

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

func newTestModel() Model {
	ctx := &appctx.ProgramContext{Styles: appctx.DefaultStyles()}
	return New(ctx)
}

func TestFreshModelIsBlurred(t *testing.T) {
	m := newTestModel()
	if m.Focused() {
		t.Fatal("a fresh model should not be focused")
	}
}

func TestFocusFocuses(t *testing.T) {
	m := newTestModel()
	m.Focus()
	if !m.Focused() {
		t.Fatal("after Focus() the model should be focused")
	}
}

func TestBlurUnfocuses(t *testing.T) {
	m := newTestModel()
	m.Focus()
	m.Blur()
	if m.Focused() {
		t.Fatal("after Blur() the model should not be focused")
	}
}

func TestSetValueValueRoundTrips(t *testing.T) {
	m := newTestModel()
	m.SetValue("bug")
	if got := m.Value(); got != "bug" {
		t.Fatalf("Value() = %q, want %q", got, "bug")
	}
}

func TestTypingWhileFocusedUpdatesValue(t *testing.T) {
	m := newTestModel()
	m.Focus()

	for _, r := range "bug" {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		_ = cmd
	}

	if got := m.Value(); got != "bug" {
		t.Fatalf("after typing, Value() = %q, want %q", got, "bug")
	}
}

func TestUpdateWhileBlurredDoesNotPanic(t *testing.T) {
	m := newTestModel()
	// Not focused: Update should be a no-op, not a panic.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := m.Value(); got != "" {
		t.Fatalf("blurred Update should not accept input, Value() = %q", got)
	}
}

func TestViewNonEmptyWhenFocused(t *testing.T) {
	m := newTestModel()
	m.Focus()
	if m.View() == "" {
		t.Fatal("View() should be non-empty when focused")
	}
}

func TestCursorEndDoesNotPanic(t *testing.T) {
	m := newTestModel()
	m.SetValue("bug")
	m.CursorEnd()
}

func TestUpdateProgramContextDoesNotPanic(t *testing.T) {
	m := newTestModel()
	ctx := &appctx.ProgramContext{Styles: appctx.DefaultStyles()}
	m.UpdateProgramContext(ctx)
}
