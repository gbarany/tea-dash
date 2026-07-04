package palette

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	m.SetSize(40, 2)

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
	m.SetSize(40, 2)

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
