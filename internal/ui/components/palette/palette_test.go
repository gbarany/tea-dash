package palette

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

func key(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func typeString(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		var ev Event
		m, _, ev = m.Update(key(r))
		if ev.Kind != EventNone {
			t.Fatalf("typing %q produced unexpected event %+v", r, ev)
		}
	}
	return m
}

func sampleItems() []Item {
	return []Item{
		{Label: "Open", Kind: KindAction, Builtin: "open"},
		{Label: "Refresh", Kind: KindAction, Builtin: "refresh"},
		{Label: "Merge", Kind: KindAction, Builtin: "merge"},
		{Label: "Reopen", Kind: KindAction, Builtin: "reopen"},
	}
}

// TestIsSubsequence covers the fzf-like "mrg matches Merge" requirement
// directly, independent of ranking.
func TestIsSubsequence(t *testing.T) {
	cases := []struct {
		q, s string
		want bool
	}{
		{"mrg", "merge", true},
		{"merge", "merge", true},
		{"gm", "merge", false}, // wrong order
		{"", "merge", true},
		{"mergex", "merge", false}, // query longer than target
		{"pr", "open", false},
	}
	for _, c := range cases {
		if got := isSubsequence(c.q, c.s); got != c.want {
			t.Errorf("isSubsequence(%q, %q) = %v, want %v", c.q, c.s, got, c.want)
		}
	}
}

// TestFilterItems_EmptyQueryShowsAll covers the plan's explicit
// "empty-query shows all" requirement.
func TestFilterItems_EmptyQueryShowsAll(t *testing.T) {
	items := sampleItems()
	got := filterItems(items, "")
	if len(got) != len(items) {
		t.Fatalf("empty query = %d items, want all %d", len(got), len(items))
	}
	for i, it := range got {
		if it.Label != items[i].Label {
			t.Fatalf("empty query reordered items: got %q at %d, want %q", it.Label, i, items[i].Label)
		}
	}
}

// TestFilterItems_SubsequenceMatch covers "mrg matches Merge" end to end
// through filterItems (case-insensitive).
func TestFilterItems_SubsequenceMatch(t *testing.T) {
	got := filterItems(sampleItems(), "MRG")
	if len(got) != 1 || got[0].Label != "Merge" {
		t.Fatalf("filterItems(_, %q) = %+v, want just Merge", "MRG", got)
	}
}

// TestFilterItems_RankingPrefixBeforeScattered covers the plan's explicit
// ranking requirement: prefix matches sort before scattered ones, stable
// otherwise. Query "re": "Refresh" and "Reopen" both start with "re"
// (prefix tier, kept in their original relative order); "Merge" only
// matches scattered ("Me-r-g-e" — r then e out of query order... actually
// "merge" contains "r" at index 3 and "e" at index 1 and 4; the query is
// "re" so it needs an 'r' then an 'e' after it — merge has r(3) then e(4),
// a scattered-but-valid subsequence, and does NOT start with "re").
func TestFilterItems_RankingPrefixBeforeScattered(t *testing.T) {
	got := filterItems(sampleItems(), "re")
	var labels []string
	for _, it := range got {
		labels = append(labels, it.Label)
	}
	want := []string{"Refresh", "Reopen", "Merge"}
	if strings.Join(labels, ",") != strings.Join(want, ",") {
		t.Fatalf("ranking = %v, want %v (prefix matches first, stable order within each tier)", labels, want)
	}
}

// TestFilterItems_StableWithinTier confirms two same-tier (both scattered,
// or both prefix) matches keep their original relative order rather than
// being reordered by the sort.
func TestFilterItems_StableWithinTier(t *testing.T) {
	items := []Item{
		{Label: "Zebra open"}, // scattered match for "open" (contains it, not a prefix)
		{Label: "Apple open"}, // scattered match too, originally AFTER Zebra
	}
	got := filterItems(items, "open")
	if len(got) != 2 || got[0].Label != "Zebra open" || got[1].Label != "Apple open" {
		t.Fatalf("stable order within a tier not preserved: %+v", got)
	}
}

func TestOpen_ResetsQueryAndSelection(t *testing.T) {
	var m Model = New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())
	m = typeString(t, m, "merge")
	if got, ok := m.Selected(); !ok || got.Label != "Merge" {
		t.Fatalf("after filtering to Merge, Selected() = %+v, %v", got, ok)
	}

	m.Open(sampleItems())
	if m.Value() != "" {
		t.Fatalf("Open should clear the query, got %q", m.Value())
	}
	items, _ := m.Visible()
	if len(items) != len(sampleItems()) {
		t.Fatalf("Open should show every item again, got %d", len(items))
	}
}

// TestUpdate_SelectionMovement covers "selection via up/down + enter".
func TestUpdate_SelectionMovement(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())

	if got, ok := m.Selected(); !ok || got.Label != "Open" {
		t.Fatalf("initial selection = %+v, want Open", got)
	}

	var ev Event
	m, _, ev = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if ev.Kind != EventNone {
		t.Fatalf("down should not emit an event, got %+v", ev)
	}
	if got, ok := m.Selected(); !ok || got.Label != "Refresh" {
		t.Fatalf("after down, Selected() = %+v, want Refresh", got)
	}

	m, _, ev = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got, ok := m.Selected(); !ok || got.Label != "Open" {
		t.Fatalf("after up, Selected() = %+v, want Open", got)
	}

	// Up at the top clamps rather than wrapping/going negative.
	m, _, ev = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got, ok := m.Selected(); !ok || got.Label != "Open" {
		t.Fatalf("up at the top should clamp, got %+v", got)
	}
}

func TestUpdate_EnterRunsSelected(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())
	m = typeString(t, m, "merge")

	_, _, ev := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if ev.Kind != EventRun || ev.Item.Label != "Merge" {
		t.Fatalf("enter = %+v, want EventRun on Merge", ev)
	}
}

func TestUpdate_EscDismisses(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())

	_, _, ev := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if ev.Kind != EventDismiss {
		t.Fatalf("esc = %+v, want EventDismiss", ev)
	}
}

// TestUpdate_JKAreFilterCharsNotNavigation documents the deviation from a
// literal "j/k navigate" reading: since the palette filters as you type,
// "j"/"k" must reach the text input like any other letter (labels such as
// "Rerun"/"Checkout" contain them), so only the arrow keys navigate.
func TestUpdate_JKAreFilterCharsNotNavigation(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())

	m, _, _ = m.Update(key('j'))
	if m.Value() != "j" {
		t.Fatalf("'j' should be typed into the filter, got value %q", m.Value())
	}
}

func TestSelected_EmptyFilteredListIsFalse(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())
	m = typeString(t, m, "zzzznomatch")

	if _, ok := m.Selected(); ok {
		t.Fatal("Selected() should report false when nothing matches")
	}
	if _, _, ev := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); ev.Kind != EventNone {
		t.Fatalf("enter with no matches should be a no-op, got %+v", ev)
	}
}

// TestVisible_WindowsAndTracksSelection covers scrolling: with a 2-row
// window and 4 items, moving the selection past the bottom of the window
// scrolls it into view.
func TestVisible_WindowsAndTracksSelection(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())
	// height 5: SetSize reserves HeaderRows(2) + FooterRows(1), leaving a
	// 2-item window.
	m.SetSize(40, 5)

	items, sel := m.Visible()
	if len(items) != 2 || sel != 0 || items[0].Label != "Open" {
		t.Fatalf("initial window = %+v sel=%d, want [Open, Refresh] sel=0", items, sel)
	}

	m = m.MoveSelection(1) // Refresh
	m = m.MoveSelection(1) // Merge — should scroll
	items, sel = m.Visible()
	if len(items) != 2 || sel != 1 || items[1].Label != "Merge" {
		t.Fatalf("after scrolling down, window = %+v sel=%d, want Merge visible and selected", items, sel)
	}
}

func TestItemAtVisibleIndex(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())
	// height 5: SetSize reserves HeaderRows(2) + FooterRows(1), leaving a
	// 2-item window.
	m.SetSize(40, 5)

	item, ok := m.ItemAtVisibleIndex(1)
	if !ok || item.Label != "Refresh" {
		t.Fatalf("ItemAtVisibleIndex(1) = %+v, %v, want Refresh", item, ok)
	}
	if _, ok := m.ItemAtVisibleIndex(5); ok {
		t.Fatal("ItemAtVisibleIndex out of range should report false")
	}
}

func TestView_RendersLabelsAndKeyHints(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open([]Item{{Label: "Merge", KeyHint: "m"}})
	m.SetSize(40, 5)

	view := m.View()
	if !strings.Contains(view, "Merge") || !strings.Contains(view, "m") {
		t.Fatalf("view missing label/key hint:\n%s", view)
	}
}

func TestView_NoMatches(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())
	m = typeString(t, m, "zzzznomatch")

	if !strings.Contains(m.View(), "No matches") {
		t.Fatalf("view should say no matches:\n%s", m.View())
	}
}

// TestView_FooterHint covers Step 0(c) from the T7 review: a one-line
// key-hint footer always renders, in every list-content state.
func TestView_FooterHint(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.Open(sampleItems())
	m.SetSize(40, 5)

	if !strings.Contains(m.View(), footerHint) {
		t.Fatalf("view missing footer hint %q:\n%s", footerHint, m.View())
	}

	empty := typeString(t, m, "zzzznomatch")
	if !strings.Contains(empty.View(), footerHint) {
		t.Fatalf("no-matches view missing footer hint:\n%s", empty.View())
	}
}

// TestSetSize_FooterSurvivesFullItemList is a regression test for a bug
// caught live at 80×24 after this package's initial review: SetSize
// reserved FooterRows out of h but not HeaderRows (the input line + blank
// separator View() always emits first), so whenever the item list filled
// its window View() produced h+2 lines — 2 more than the caller's height
// budget. app.go's fitBlock crops every overlay's content to exactly that
// budget, so the overflow (the footer line, plus the last visible item)
// was silently cropped away instead of anything genuinely not fitting.
// TestView_FooterHint didn't catch this: it never constrains the height
// against a list big enough to fill the window, so the pre-fix h+2 lines
// still contained the footer — the bug only shows up once something
// downstream (fitBlock) enforces the exact-h budget this test checks
// directly instead.
func TestSetSize_FooterSurvivesFullItemList(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	items := make([]Item, 40) // comfortably more than any h below can show
	for i := range items {
		items[i] = Item{Label: fmt.Sprintf("Item %d", i)}
	}
	m.Open(items)

	for _, h := range []int{4, 5, 6, 10, 24} {
		m.SetSize(40, h)
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) != h {
			t.Errorf("h=%d: View() produced %d lines, want exactly %d (HeaderRows+items+FooterRows must fit the caller's budget exactly, or fitBlock crops the overflow)", h, len(lines), h)
			continue
		}
		if !strings.Contains(view, footerHint) {
			t.Errorf("h=%d: footer hint missing when a full item list fills the window:\n%s", h, view)
		}
	}
}

// TestRenderRow_MultibyteLabelAlignsKeyHint covers Step 0(b) from the T7
// review: renderRow must measure with lipgloss.Width, not len (a byte
// count) — a multibyte label would otherwise be over-measured (more bytes
// than display columns), under-padding the gap before the key hint.
func TestRenderRow_MultibyteLabelAlignsKeyHint(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	// "Wüst review" is 11 runes / 11 display columns, but 12 bytes (ü is
	// 2 bytes in UTF-8) — len() would compute a width one too wide.
	label := "Wüst review"
	m.Open([]Item{{Label: label, KeyHint: "v"}})
	m.SetSize(40, 5)

	lines := strings.Split(m.View(), "\n")
	row := lines[HeaderRows]
	// The meaningful assertion: with a byte-vs-rune-count bug (len()
	// instead of lipgloss.Width), the pad math would be off by however
	// many extra bytes the multibyte rune(s) contribute, throwing off the
	// selected row's full-width padding — this exact-width check catches
	// that even though the row is ANSI-styled (lipgloss.Width ignores
	// escape codes).
	if got := lipgloss.Width(row); got != 40 {
		t.Fatalf("row with multibyte label should still be exactly 40 columns wide, got %d:\n%q", got, row)
	}
	if !strings.Contains(row, label) {
		t.Fatalf("row should contain the label:\n%q", row)
	}
}
