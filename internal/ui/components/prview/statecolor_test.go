package prview

import (
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/data"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// TestRenderPullHeaderHasGlyphAndColor is Task 9's preview golden-ish
// substring test: the state pill contains both the icon glyph and real
// lipgloss color codes, colored from ctx.Styles.StateColors (theme.colors.
// state.*), and switches glyph family when a non-default icon set is
// configured — proving the preview reads set/styles rather than a
// hardcoded palette.
func TestRenderPullHeaderHasGlyphAndColor(t *testing.T) {
	styles := appctx.DefaultStyles()
	out := RenderPull(samplePull(), nil, 60, false, styles, icons.Unicode)
	glyph := icons.Glyph(icons.Unicode, icons.Open)
	if !strings.Contains(out, glyph) {
		t.Fatalf("PR preview missing Open glyph %q:\n%s", glyph, out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("PR preview header has no ANSI color codes:\n%s", out)
	}

	ascii := RenderPull(samplePull(), nil, 60, false, styles, icons.ASCII)
	if !strings.Contains(ascii, icons.Glyph(icons.ASCII, icons.Open)) {
		t.Fatalf("ASCII PR preview missing ASCII Open glyph:\n%s", ascii)
	}
	if strings.Contains(ascii, glyph) {
		t.Fatalf("ASCII PR preview should not contain the Unicode glyph:\n%s", ascii)
	}
}

// TestRenderPullDraftPillUsesDraftGlyph pins the DRAFT pill to the Draft
// state's glyph specifically (not just any glyph).
func TestRenderPullDraftPillUsesDraftGlyph(t *testing.T) {
	row := samplePull()
	row.Draft = true
	out := RenderPull(row, nil, 60, false, appctx.DefaultStyles(), icons.Unicode)
	if !strings.Contains(out, icons.Glyph(icons.Unicode, icons.Draft)) {
		t.Fatalf("draft preview missing Draft glyph:\n%s", out)
	}
}

// TestRenderIssueHeaderClosedUsesClosedGlyph exercises the issue header's
// state pill for a non-default state.
func TestRenderIssueHeaderClosedUsesClosedGlyph(t *testing.T) {
	row := data.Issue{Number: 7, Title: "Broke", RepoNameWithOwner: "a/b", State: "closed"}
	out := RenderIssue(row, nil, 60, false, appctx.DefaultStyles(), icons.Unicode)
	if !strings.Contains(out, icons.Glyph(icons.Unicode, icons.Closed)) {
		t.Fatalf("issue preview missing Closed glyph:\n%s", out)
	}
}

// TestRenderNotificationUnreadDotUsesExplicitStyle: Unread has no
// styles.StateColors entry, so its pill must still render (via the
// explicit ActionButton-foreground fallback in statePill), not silently
// drop color.
func TestRenderNotificationUnreadDotUsesExplicitStyle(t *testing.T) {
	styles := appctx.DefaultStyles()
	if _, ok := styles.StateColors[icons.Unread]; ok {
		t.Fatalf("styles.StateColors unexpectedly has an Unread entry")
	}
	row := data.Notification{ID: 1, Unread: true, RepoNameWithOwner: "a/b", SubjectTitle: "t"}
	out := RenderNotification(row, 60, styles, icons.Unicode)
	if !strings.Contains(out, icons.Glyph(icons.Unicode, icons.Unread)) {
		t.Fatalf("notification preview missing Unread glyph:\n%s", out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("notification preview pill has no color codes:\n%s", out)
	}
}

// TestRenderCIChecksLineUsesConfiguredGlyphs verifies the "Checks: ✓N ✗M
// ◐K" summary line's glyphs come from set (Task 9: previously hardcoded
// literal "✓"/"✗"/"•" regardless of theme.icons).
func TestRenderCIChecksLineUsesConfiguredGlyphs(t *testing.T) {
	detail := &data.PullDetail{
		CI: data.CIStatus{
			State: data.CIStateFailure,
			Checks: []data.Check{
				{Context: "build", State: data.CheckStateSuccess},
				{Context: "test", State: data.CheckStateFailure},
			},
		},
	}
	styles := appctx.DefaultStyles()
	unicodeOut := RenderPull(samplePull(), detail, 60, false, styles, icons.Unicode)
	for _, glyph := range []string{
		icons.Glyph(icons.Unicode, icons.Success),
		icons.Glyph(icons.Unicode, icons.Failure),
	} {
		if !strings.Contains(unicodeOut, glyph) {
			t.Fatalf("unicode CI block missing glyph %q:\n%s", glyph, unicodeOut)
		}
	}

	asciiOut := RenderPull(samplePull(), detail, 60, false, styles, icons.ASCII)
	for _, glyph := range []string{
		icons.Glyph(icons.ASCII, icons.Success),
		icons.Glyph(icons.ASCII, icons.Failure),
	} {
		if !strings.Contains(asciiOut, glyph) {
			t.Fatalf("ascii CI block missing glyph %q:\n%s", glyph, asciiOut)
		}
	}
}

// TestRenderActionHeaderUsesConclusionState verifies actionHeader's pill
// prefers the conclusion (success) over the bare status (completed) when
// classifying the composite "status/conclusion" state — mirroring
// section.ClassifyState's right-to-left precedence.
func TestRenderActionHeaderUsesConclusionState(t *testing.T) {
	run := data.ActionRun{RepoNameWithOwner: "a/b", RunNumber: 1, Status: "completed", Conclusion: "success"}
	out := RenderAction(run, nil, 80, appctx.DefaultStyles(), icons.Unicode)
	if !strings.Contains(out, icons.Glyph(icons.Unicode, icons.Success)) {
		t.Fatalf("action preview header missing Success glyph:\n%s", out)
	}
}
