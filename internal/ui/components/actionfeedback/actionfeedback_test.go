package actionfeedback

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

func TestFeedbackRendersKinds(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want string
	}{
		{name: "start", msg: Start("Starting merge"), want: "Starting merge"},
		{name: "success", msg: Success("Merged"), want: "Merged"},
		{name: "error", msg: Error("Merge failed"), want: "Merge failed"},
		{name: "cancel", msg: Cancel("Action cancelled"), want: "Action cancelled"},
		{name: "info", msg: Info("Copied #42"), want: "Copied #42"},
	}
	styles := context.DefaultStyles()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := New().Set(tt.msg)
			view := m.View(80, styles, icons.Unicode)
			if !strings.Contains(view, tt.want) {
				t.Fatalf("feedback view missing %q:\n%s", tt.want, view)
			}
		})
	}
}

func TestFeedbackSmallWidthRender(t *testing.T) {
	styles := context.DefaultStyles()
	m, _ := New().Set(Error("This message is too long for the footer"))
	view := m.View(10, styles, icons.Unicode)
	if view == "" {
		t.Fatal("feedback with a message should render")
	}
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > 10 {
			t.Fatalf("line %q is %d columns wide, want <= 10 in:\n%s", line, got, view)
		}
	}
	if !strings.Contains(view, "...") {
		t.Fatalf("small-width render should truncate with ..., got:\n%s", view)
	}
}

func TestEmpty_NoMessageRendersNothing(t *testing.T) {
	styles := context.DefaultStyles()
	if got := New().View(80, styles, icons.Unicode); got != "" {
		t.Fatalf("empty model should render \"\", got %q", got)
	}
	if !New().Empty() {
		t.Fatal("New() should be Empty()")
	}
}

func TestView_IconPerKind(t *testing.T) {
	styles := context.DefaultStyles()
	tests := []struct {
		msg  Message
		want string
	}{
		{Success("ok"), "✓"},
		{Error("bad"), "✗"},
		{Start("working"), "◐"},
	}
	for _, tt := range tests {
		m, _ := New().Set(tt.msg)
		view := m.View(80, styles, icons.Unicode)
		if !strings.Contains(view, tt.want) {
			t.Errorf("Set(%+v).View() = %q, want it to contain icon %q", tt.msg, view, tt.want)
		}
	}
}

// TestSet_SuccessAutoExpires covers the plan's explicit requirement: "Set ->
// tick before deadline keeps it, after clears it" — modeled as generation
// matching rather than real elapsed time (Set's returned tea.Cmd carries the
// real timer; this test just proves the gen-matching logic Expire relies on
// is correct without waiting on a real clock).
func TestSet_SuccessAutoExpires(t *testing.T) {
	m := New().WithExpiry(time.Millisecond) // keep this test fast; see WithExpiry's doc comment
	m, cmd := m.Set(Success("Merged #42"))
	if cmd == nil {
		t.Fatal("Set(Success) should return a non-nil expiry cmd")
	}
	msg := cmd()
	expireMsg, ok := msg.(ExpireMsg)
	if !ok {
		t.Fatalf("expiry cmd produced %T, want ExpireMsg", msg)
	}

	// "Before the deadline": a stale/mismatched generation must not clear a
	// still-fresh message. Simulate by expiring a generation that isn't the
	// current one (e.g. an earlier, already-superseded Set).
	stale := m.Expire(expireMsg.Gen - 1)
	if stale.Empty() {
		t.Fatal("Expire with a stale generation cleared a fresh message")
	}

	// "After the deadline": the matching generation clears it.
	expired := m.Expire(expireMsg.Gen)
	if !expired.Empty() {
		t.Fatal("Expire with the matching generation should clear the message")
	}
}

// TestSet_StaleExpireDoesNotClearNewerToast is the exact scenario the plan
// calls out by name: a stale tick from an earlier Set must not clear a
// later Set's message.
func TestSet_StaleExpireDoesNotClearNewerToast(t *testing.T) {
	m := New().WithExpiry(time.Millisecond) // keep this test fast; see WithExpiry's doc comment
	m, firstCmd := m.Set(Success("first"))
	firstGen := firstCmd().(ExpireMsg).Gen

	m, _ = m.Set(Info("second"))

	m = m.Expire(firstGen)
	if m.Empty() {
		t.Fatal("stale expire (from the first Set) cleared the second, newer message")
	}
	view := m.View(80, context.DefaultStyles(), icons.Unicode)
	if !strings.Contains(view, "second") {
		t.Fatalf("newer message should survive a stale expire, got view %q", view)
	}
}

func TestSet_ErrorDoesNotAutoExpire(t *testing.T) {
	m := New()
	_, cmd := m.Set(Error("boom"))
	if cmd != nil {
		t.Fatal("Set(Error) should return a nil cmd — errors persist until a keypress")
	}
}

func TestSet_StartDoesNotAutoExpire(t *testing.T) {
	m := New()
	_, cmd := m.Set(Start("Merging…"))
	if cmd != nil {
		t.Fatal("Set(Start) should return a nil cmd — in-flight state is superseded, not timed")
	}
}

func TestDismissError_ClearsOnlyError(t *testing.T) {
	m := New()
	m, _ = m.Set(Error("boom"))
	m = m.DismissError()
	if !m.Empty() {
		t.Fatal("DismissError should clear an Error message")
	}

	m = New()
	m, _ = m.Set(Info("fyi"))
	m = m.DismissError()
	if m.Empty() {
		t.Fatal("DismissError should leave a non-Error message alone")
	}
}

func TestWithExpiry_ShortensRealTimer(t *testing.T) {
	m := New().WithExpiry(time.Millisecond)
	m, cmd := m.Set(Success("quick"))
	if cmd == nil {
		t.Fatal("expected an expiry cmd")
	}
	start := time.Now()
	msg := cmd()
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("WithExpiry(1ms) took %s to fire — too slow for a fast test", elapsed)
	}
	expireMsg := msg.(ExpireMsg)
	if m.Expire(expireMsg.Gen).Empty() != true {
		t.Fatal("expected the short-expiry toast to clear")
	}
}
