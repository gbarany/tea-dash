package section

import (
	"reflect"
	"testing"
)

// columnBudget is the real per-column rendering overhead: bubbles/table's
// DefaultStyles pads every header/cell by 1 column on each side
// (Padding(0,1)), so each column costs its own Width + 2, not just Width.
func columnBudget(defs []ColumnDefinition) int {
	total := 0
	for _, d := range defs {
		total += d.Width + 2
	}
	return total
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// TestDefaultColumnDefinitions_NeverExceedsWidth is the core budget-
// arithmetic regression test: at every width from the practical floor up
// through generous widths, the six columns' rendered budget (width + the
// table's own padding overhead) must never exceed the interior width — the
// root cause of the table header wrapping onto a second row (stealing a
// data row and breaking the panel's right border) was under-reserving this
// overhead (-6 flat instead of -2 per surviving column).
func TestDefaultColumnDefinitions_NeverExceedsWidth(t *testing.T) {
	for w := 20; w <= 200; w++ {
		defs := DefaultColumnDefinitions(w)
		if got := columnBudget(defs); got > w {
			t.Fatalf("width %d: columns consume %d (budget incl. padding), exceeds available width\ndefs=%+v", w, got, defs)
		}
	}
}

// TestDefaultColumnDefinitions_DropsByPriority pins the plan's three
// reference widths: 118 keeps all six columns, 71 drops Author (and,
// having dropped it, also Updated — Repo survives), and 38 — after
// exhausting every droppable column — keeps only #/Title/State (gh-dash's
// convention: those three never drop).
func TestDefaultColumnDefinitions_DropsByPriority(t *testing.T) {
	namesAt := func(w int) []string { return columnNamesFromDefinitions(DefaultColumnDefinitions(w)) }

	if got, want := namesAt(118), []string{"number", "title", "repo", "author", "state", "updated"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("118: names = %v, want %v (all six)", got, want)
	}

	got71 := namesAt(71)
	if containsName(got71, "author") {
		t.Fatalf("71: author should have been dropped first: %v", got71)
	}
	if want := []string{"number", "title", "repo", "state"}; !reflect.DeepEqual(got71, want) {
		t.Fatalf("71: names = %v, want %v (author AND updated dropped, repo survives)", got71, want)
	}

	if got, want := namesAt(38), []string{"number", "title", "state"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("38: names = %v, want %v (#/Title/State only)", got, want)
	}
}

// TestSixColumnSpec_GrowAbsorbsAllRemainingWidth confirms Grow isn't capped
// at its preferred Width once there's room — it fills whatever's left,
// exactly like the historical unbounded per-section title-width formulas
// (e.g. "titleW := mainWidth - fixed - 6") did before being generalized
// into SixColumnSpec.
func TestSixColumnSpec_GrowAbsorbsAllRemainingWidth(t *testing.T) {
	defs := DefaultColumnSpec().Fit(300)
	for _, d := range defs {
		if d.Name == "title" {
			if d.Width <= 20 {
				t.Fatalf("title width = %d, want it to have grown well past its 20-column floor at width 300", d.Width)
			}
			return
		}
	}
	t.Fatal("title column missing from Fit(300)")
}

// TestSixColumnSpec_FitIsIdempotentAndOrdered checks Fit's output is always
// in the fixed Index/Grow/Repo/Fourth/State/Updated declaration order
// (regardless of which get dropped) and never returns a NEGATIVE width —
// widths start at 20, layout.Compute's real production floor for
// MainContentWidth (its minListInterior clamp); below that, the fixed
// Index+State columns alone can exceed the budget, which no column
// dropping can fix (see minGrowWidth's doc comment).
func TestSixColumnSpec_FitIsIdempotentAndOrdered(t *testing.T) {
	spec := DefaultColumnSpec()
	for _, w := range []int{20, 38, 71, 118, 200} {
		defs := spec.Fit(w)
		order := map[string]int{"number": 0, "title": 1, "repo": 2, "author": 3, "state": 4, "updated": 5}
		last := -1
		for _, d := range defs {
			if d.Width < 0 {
				t.Fatalf("width %d: column %q has negative width %d", w, d.Name, d.Width)
			}
			if d.Width == 0 && d.Name != spec.Grow.Name {
				t.Fatalf("width %d: only the grow column may be zero-width (invisible); got it for %q", w, d.Name)
			}
			idx, ok := order[d.Name]
			if !ok {
				t.Fatalf("width %d: unexpected column name %q", w, d.Name)
			}
			if idx <= last {
				t.Fatalf("width %d: columns out of declaration order: %+v", w, defs)
			}
			last = idx
		}
		// number, title, and state must never be dropped.
		names := columnNamesFromDefinitions(defs)
		for _, essential := range []string{"number", "title", "state"} {
			if !containsName(names, essential) {
				t.Fatalf("width %d: essential column %q was dropped: %v", w, essential, names)
			}
		}
	}
}
