package header

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestView_ExactWidth(t *testing.T) {
	styles := context.DefaultStyles()
	for _, w := range []int{40, 50, 59, 60, 80, 120, 200} {
		row := View(w, context.PullsView, "demo.gitea.local", "gabor", styles)
		if got := lipgloss.Width(row); got != w {
			t.Errorf("width %d: rendered width = %d, want %d\nrow=%q", w, got, w, row)
		}
	}
}

func TestView_ContainsAppNameLabelsAndHost(t *testing.T) {
	styles := context.DefaultStyles()
	row := View(120, context.PullsView, "demo.gitea.local", "gabor", styles)
	for _, want := range []string{"tea-dash", "1 Pulls", "2 Issues", "3 Inbox", "4 CI", "5 Branches", "demo.gitea.local", "gabor"} {
		if !strings.Contains(row, want) {
			t.Errorf("header row missing %q:\n%s", want, row)
		}
	}
	plain := stripANSI(row)
	if !strings.HasPrefix(plain, "┌") {
		t.Errorf("header row should start with the top-left corner: %q", plain)
	}
	if !strings.HasSuffix(plain, "┐") {
		t.Errorf("header row should end with the top-right corner: %q", plain)
	}
}

func TestView_ActiveLabelUsesPanelTitleStyle(t *testing.T) {
	styles := context.DefaultStyles()
	row := View(120, context.BranchesView, "demo.gitea.local", "gabor", styles)
	// PanelTitle, not ActiveTab: ActiveTab's Padding(0,1) would put stray
	// extra spaces around the label next to this header's own explicit
	// " · " separators.
	want := styles.PanelTitle.Render("5 Branches")
	if !strings.Contains(row, want) {
		t.Errorf("active label should be styled with PanelTitle:\nwant substr %q\nrow=%q", want, row)
	}
	// A non-active label must NOT carry the active style.
	notWant := styles.PanelTitle.Render("1 Pulls")
	if strings.Contains(row, notWant) {
		t.Errorf("inactive label should not carry the active style: %q", row)
	}
}

func TestView_MockHostWinsOverInstanceHost(t *testing.T) {
	// The caller (app.go) decides which of MockHost/InstanceHost to pass in;
	// this just confirms whatever host string is given renders.
	styles := context.DefaultStyles()
	row := View(120, context.PullsView, "demo.gitea.local", "", styles)
	if !strings.Contains(row, "demo.gitea.local") {
		t.Errorf("row missing mock host:\n%s", row)
	}
}

func TestView_NoHostOrUserOmitsHostBlock(t *testing.T) {
	styles := context.DefaultStyles()
	row := View(80, context.PullsView, "", "", styles)
	if lipgloss.Width(row) != 80 {
		t.Fatalf("width = %d, want 80", lipgloss.Width(row))
	}
	if !strings.Contains(row, "tea-dash") {
		t.Errorf("row missing app name:\n%s", row)
	}
}

// TestView_NarrowWidthDoesNotPanicAndFitsExactly exercises widths from the
// practical floor (corners + " tea-dash " with no fill) up through the
// layout package's TooSmall gate (40): the header is never asked to render
// below 40 columns in production (layout.Compute zeroes Header entirely
// below that), but this still confirms graceful, exact-width degradation
// through the narrow band just above it.
func TestView_NarrowWidthDoesNotPanicAndFitsExactly(t *testing.T) {
	styles := context.DefaultStyles()
	for w := 12; w <= 60; w++ {
		row := View(w, context.PullsView, "demo.gitea.local", "gabor", styles)
		if got := lipgloss.Width(row); got != w {
			t.Fatalf("w=%d: rendered width = %d, want %d\nrow=%q", w, got, w, row)
		}
	}
}

func TestLabels_RangesAreOrderedAndWithinRow(t *testing.T) {
	styles := context.DefaultStyles()
	w := 120
	ranges := Labels(w, context.PullsView, "demo.gitea.local", "gabor", styles)
	if len(ranges) != 5 {
		t.Fatalf("len(ranges) = %d, want 5", len(ranges))
	}
	prevEnd := 0
	for i, r := range ranges {
		if r.Start < prevEnd {
			t.Fatalf("range %d starts (%d) before previous ended (%d)", i, r.Start, prevEnd)
		}
		if r.End > w || r.Start < 0 {
			t.Fatalf("range %d = %+v out of bounds for width %d", i, r, w)
		}
		if r.End <= r.Start {
			t.Fatalf("range %d = %+v has non-positive width", i, r)
		}
		prevEnd = r.End
	}
	if ranges[0].View != context.PullsView || ranges[4].View != context.BranchesView {
		t.Fatalf("ranges out of expected view order: %+v", ranges)
	}
}

func TestLabels_NilWhenTooNarrowForLabelBlock(t *testing.T) {
	styles := context.DefaultStyles()
	ranges := Labels(20, context.PullsView, "demo.gitea.local", "gabor", styles)
	if ranges != nil {
		t.Errorf("expected nil ranges when the row is too narrow, got %+v", ranges)
	}
}
