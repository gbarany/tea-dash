package section

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	appctx "github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// wantGlyphStates covers every state StateCell/ClassifyState is documented
// to map: PR/issue open/draft/merged/closed, CI/action
// success/failure/running/cancelled/waiting, and the notification
// unread/read/pinned trio.
var wantGlyphStates = []string{
	"open", "draft", "merged", "closed",
	"success", "failure", "running", "cancelled", "waiting",
	"unread", "read", "pinned",
}

func TestStateCell_EveryDocumentedStateGetsAGlyph(t *testing.T) {
	styles := appctx.DefaultStyles()
	for _, set := range []icons.Set{icons.Unicode, icons.Nerd, icons.ASCII} {
		for _, state := range wantGlyphStates {
			got := StateCell(state, set, styles)
			st, ok := ClassifyState(state)
			if !ok {
				t.Fatalf("ClassifyState(%q) = false, want a match", state)
			}
			glyph := icons.Glyph(set, st)
			if !strings.Contains(got, glyph) {
				t.Fatalf("StateCell(%q, %v) = %q, want it to contain glyph %q", state, set, got, glyph)
			}
			if !strings.Contains(got, state) {
				t.Fatalf("StateCell(%q, %v) = %q, want it to still contain the original word", state, set, got)
			}
		}
	}
}

func TestStateCell_ASCIISetIsPureASCIIPlusWord(t *testing.T) {
	styles := appctx.DefaultStyles()
	for _, state := range wantGlyphStates {
		got := StateCell(state, icons.ASCII, styles)
		// Strip the lipgloss/ANSI styling that colors most of these cells —
		// only the underlying glyph+word content must be pure ASCII.
		plain := stripANSI(got)
		for _, r := range plain {
			if r >= 128 {
				t.Fatalf("StateCell(%q, ASCII) = %q, contains non-ASCII rune %q", state, plain, r)
			}
		}
	}
}

// TestStateCell_ColoredVsPlain locks in the Task 9 spike's outcome
// (bubbles/table truncates ANSI-styled cells correctly): StateCell applies
// real lipgloss color codes to every classified state's cell, not just the
// glyph-only fallback the plan would have required had the spike failed.
func TestStateCell_ColoredVsPlain(t *testing.T) {
	styles := appctx.DefaultStyles()
	for _, state := range wantGlyphStates {
		got := StateCell(state, icons.Unicode, styles)
		if !strings.Contains(got, "\x1b[") {
			t.Fatalf("StateCell(%q) = %q, want ANSI color codes present (spike greenlit colored cells)", state, got)
		}
	}
}

func TestStateCell_UnreadUsesExplicitStyleNotStateColorsMap(t *testing.T) {
	styles := appctx.DefaultStyles()
	if _, ok := styles.StateColors[icons.Unread]; ok {
		t.Fatalf("styles.StateColors has an Unread entry; StateCell's explicit-style path is meant for exactly the states StateColors DOESN'T cover")
	}
	got := StateCell("unread", icons.Unicode, styles)
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("StateCell(\"unread\", ...) = %q, want it colored via the explicit ActionButton style despite no StateColors entry", got)
	}
}

func TestStateCell_UnclassifiedStateReturnedPlain(t *testing.T) {
	styles := appctx.DefaultStyles()
	got := StateCell("bogus-state", icons.Unicode, styles)
	if got != "bogus-state" {
		t.Fatalf("StateCell(%q) = %q, want the state echoed unchanged for an unclassified word", "bogus-state", got)
	}
}

func TestClassifyState_CompositeActionStatusPrefersConclusion(t *testing.T) {
	cases := map[string]icons.State{
		"completed/success":   icons.Success,
		"completed/failure":   icons.Failure,
		"completed/cancelled": icons.Failure,
		"queued":              icons.Running,
		"in_progress":         icons.Running,
	}
	for in, want := range cases {
		got, ok := ClassifyState(in)
		if !ok || got != want {
			t.Fatalf("ClassifyState(%q) = (%v, %v), want (%v, true)", in, got, ok, want)
		}
	}
}

func TestGlyphText_AppliesExplicitStyle(t *testing.T) {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	got := GlyphText(icons.Unicode, icons.AheadArrow, "ahead 2", dim)
	if !strings.Contains(got, "ahead 2") {
		t.Fatalf("GlyphText(...) = %q, want it to contain the original text", got)
	}
	if !strings.Contains(got, icons.Glyph(icons.Unicode, icons.AheadArrow)) {
		t.Fatalf("GlyphText(...) = %q, want it to contain the AheadArrow glyph", got)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("GlyphText(...) = %q, want the explicit style's ANSI codes applied", got)
	}
}

// stripANSI removes CSI escape sequences (\x1b[...letter) for plain-text
// assertions, mirroring the ansi.Strip helper used in spike_test.go without
// adding another import for a single strip in this file.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case inEscape:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
		case r == 0x1b:
			inEscape = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
