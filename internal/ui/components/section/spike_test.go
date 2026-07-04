package section

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

// TestSpike_ANSIStyledCellTruncation is the Task-9 Step-1 time-boxed spike:
// does bubbles/v2 table.Model truncate an ANSI-styled (lipgloss-colored)
// cell string correctly at a narrow column width, and do the color codes
// survive? Answer: yes on both counts. table.renderRow truncates via
// ansi.Truncate (width-aware: counts visible cells, not escape bytes or
// raw runes) BEFORE re-wrapping in the column/cell lipgloss styles, so a
// colored glyph+word cell truncates to the right visible width and its
// color survives (lipgloss nests foreground SGR codes without a
// destructive reset). This greenlights per-cell styled strings for
// stateCell (Task 9 Step 2) instead of the glyph-only fallback.
func TestSpike_ANSIStyledCellTruncation(t *testing.T) {
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styled := red.Render("✗") + " Failing check on a very long CI job name"

	// The table's SetWidth is the VIEWPORT width, which must cover the
	// column's rendered budget (Width + 2, per bubbles/table's
	// Padding(0,1) cell style — see columnBudget in columns_test.go) or
	// the viewport itself clips the already-correctly-truncated cell,
	// which is a real footgun for stateCell callers to note.
	const colWidth = 8
	tbl := table.New(
		table.WithColumns([]table.Column{{Title: "State", Width: colWidth}}),
		table.WithRows([]table.Row{{styled}}),
		table.WithFocused(true),
	)
	tbl.SetWidth(colWidth + 2)
	tbl.SetHeight(3)
	out := tbl.View()

	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI color codes to survive truncation, got plain output: %q", out)
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("expected the overlong cell to be truncated with an ellipsis, got %q", out)
	}

	// The rendered row's VISIBLE width (lipgloss.Width already ignores ANSI
	// escapes when measuring) must match the column width exactly (no
	// SetWidth call here, so bubbles/table doesn't stretch/pad beyond the
	// column) — a byte-counted (ANSI-unaware) truncation would instead blow
	// this up by however many escape bytes got miscounted as visible cells.
	const wantWidth = colWidth + 2 // + bubbles/table's Cell Padding(0,1)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if w := lipgloss.Width(line); w != wantWidth {
			t.Fatalf("line %q: visible width %d, want %d — ANSI truncation is broken", line, w, wantWidth)
		}
	}
}
