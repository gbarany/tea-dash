package statusbar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

func TestView_ExactWidth(t *testing.T) {
	styles := context.DefaultStyles()
	for _, w := range []int{40, 50, 80, 120, 200} {
		row := View(w, "Merging #42…", "12 PRs · refreshed 2m ago", "? help · q quit", styles)
		if got := lipgloss.Width(row); got != w {
			t.Errorf("width %d: rendered width = %d, want %d\nrow=%q", w, got, w, row)
		}
	}
}

func TestView_ContainsAllThreeSegmentsWhenRoomy(t *testing.T) {
	styles := context.DefaultStyles()
	row := View(120, "Merging #42…", "12 PRs · refreshed 2m ago", "? help · q quit", styles)
	for _, want := range []string{"Merging #42…", "12 PRs", "refreshed 2m ago", "? help", "q quit"} {
		if !strings.Contains(row, want) {
			t.Errorf("status row missing %q:\n%s", want, row)
		}
	}
	if !strings.Contains(row, "└") || !strings.Contains(row, "┘") {
		t.Errorf("status row should carry the bottom-border corners:\n%s", row)
	}
}

func TestView_DropsMiddleBeforeLeftAtNarrowWidths(t *testing.T) {
	styles := context.DefaultStyles()
	left := "Merging #42…"
	middle := "12 PRs · refreshed 2m ago"
	right := "? help · q quit"

	// Narrow enough that all three no longer fit, but left+right (joined by
	// " ─ ", plus the row's 2 outer padding cells and 2 border corners) do:
	// middle must be the one dropped, and right must always survive.
	w := lipgloss.Width(left) + lipgloss.Width(right) + len(" ─ ") + 2 + 2
	row := View(w, left, middle, right, styles)
	if !strings.Contains(row, right) {
		t.Fatalf("right segment must never be dropped:\n%s", row)
	}
	if !strings.Contains(row, left) {
		t.Fatalf("left segment should survive once middle is dropped:\n%s", row)
	}
	if strings.Contains(row, "refreshed 2m ago") {
		t.Fatalf("middle segment should have been dropped first at width %d:\n%s", w, row)
	}
}

func TestView_DropsLeftWhenStillTooNarrow(t *testing.T) {
	styles := context.DefaultStyles()
	left := "Merging #42…"
	middle := "12 PRs · refreshed 2m ago"
	right := "? help · q quit"

	w := lipgloss.Width(right) + 4
	row := View(w, left, middle, right, styles)
	if !strings.Contains(row, right) {
		t.Fatalf("right segment must never be dropped:\n%s", row)
	}
	if strings.Contains(row, left) {
		t.Fatalf("left segment should have been dropped at width %d:\n%s", w, row)
	}
	if got := lipgloss.Width(row); got != w {
		t.Fatalf("width = %d, want %d\nrow=%q", got, w, row)
	}
}

// TestView_RightSurvivesEvenAtPathologicallyNarrowWidths starts at 2 (the
// practical floor: the two border corners alone already need that much) —
// the status bar is never asked to render below 40 columns in production
// (layout.Compute zeroes StatusBar entirely below that), but this confirms
// graceful, exact-width degradation and no panics through the narrow band.
func TestView_RightSurvivesEvenAtPathologicallyNarrowWidths(t *testing.T) {
	styles := context.DefaultStyles()
	for w := 2; w <= 20; w++ {
		row := View(w, "left", "middle", "? help · q quit", styles)
		if got := lipgloss.Width(row); got != w {
			t.Fatalf("w=%d: rendered width = %d, want %d\nrow=%q", w, got, w, row)
		}
	}
}

func TestView_EmptySegmentsRenderCleanRow(t *testing.T) {
	styles := context.DefaultStyles()
	row := View(60, "", "", "q quit", styles)
	if got := lipgloss.Width(row); got != 60 {
		t.Fatalf("width = %d, want 60", got)
	}
	if !strings.Contains(row, "q quit") {
		t.Fatalf("row missing right segment:\n%s", row)
	}
	if strings.Contains(row, " ─ ─ ") {
		t.Fatalf("empty segments should not leave stray separators:\n%s", row)
	}
}
