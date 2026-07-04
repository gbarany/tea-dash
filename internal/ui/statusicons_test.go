package ui

import (
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/ui/components/actionfeedback"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// TestStatusLeftSegmentUsesConfiguredIconSet is Task 9's statusLeftSegment
// threading test: an ascii theme.icons config renders the ASCII success
// glyph in the toast, not the Unicode default — statusLeftSegment reads
// m.ctx.Icons (previously hardcoded to icons.Unicode, per the T8 review
// note this task closes) rather than a fixed set.
func TestStatusLeftSegmentUsesConfiguredIconSet(t *testing.T) {
	m := New(&config.Config{Theme: config.Theme{Icons: "ascii"}}, nil)
	if m.ctx.Icons != icons.ASCII {
		t.Fatalf("ctx.Icons = %v, want icons.ASCII", m.ctx.Icons)
	}
	msg, cmd := m.actionFeedback.Set(actionfeedback.Success("Done."))
	m.actionFeedback = msg
	_ = cmd

	got := m.statusLeftSegment()
	asciiGlyph := icons.Glyph(icons.ASCII, icons.Success)
	unicodeGlyph := icons.Glyph(icons.Unicode, icons.Success)
	if !strings.Contains(got, asciiGlyph) {
		t.Fatalf("statusLeftSegment() = %q, want the ASCII success glyph %q", got, asciiGlyph)
	}
	if strings.Contains(got, unicodeGlyph) {
		t.Fatalf("statusLeftSegment() = %q, contains the Unicode glyph despite an ASCII icon set configured", got)
	}
}

// TestStatusLeftSegmentDefaultsToUnicodeIcons pins the default (no
// theme.icons configured) behavior: icons.Unicode, matching the pre-Task-9
// hardcoded call site's actual rendering (just no longer hardcoded).
func TestStatusLeftSegmentDefaultsToUnicodeIcons(t *testing.T) {
	m := New(&config.Config{}, nil)
	if m.ctx.Icons != icons.Unicode {
		t.Fatalf("ctx.Icons = %v, want icons.Unicode by default", m.ctx.Icons)
	}
	msg, _ := m.actionFeedback.Set(actionfeedback.Success("Done."))
	m.actionFeedback = msg
	got := m.statusLeftSegment()
	unicodeGlyph := icons.Glyph(icons.Unicode, icons.Success)
	if !strings.Contains(got, unicodeGlyph) {
		t.Fatalf("statusLeftSegment() = %q, want the Unicode success glyph %q", got, unicodeGlyph)
	}
}
