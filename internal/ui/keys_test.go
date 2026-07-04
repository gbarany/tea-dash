package ui

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

// allViews lists every ViewType Groups must produce a valid, fully-covered
// result for.
var allViews = []context.ViewType{
	context.PullsView, context.IssuesView, context.NotificationsView,
	context.ActionsView, context.BranchesView,
}

// TestGroups_EveryBindingHasHelpAndIsNotDuplicatedWithinAGroup is the plan's
// Task 5 prerequisite: every binding Groups(view) returns must carry help
// text (key.WithHelp), and no group lists the same binding twice. Help key
// text CAN legitimately repeat ACROSS groups within one call — e.g. "u" is
// both "Preview"'s half-page-up (only live while focused) and "PRs"'s
// update-branch (only live while not) — the two are disambiguated by focus
// state, not by view, so both are correctly documented in their own group
// rather than one shadowing the other.
func TestGroups_EveryBindingHasHelpAndIsNotDuplicatedWithinAGroup(t *testing.T) {
	k := defaultKeyMap()
	for _, view := range allViews {
		for _, g := range k.Groups(view) {
			if g.Title == "" {
				t.Fatalf("view %v: group with empty title: %+v", view, g)
			}
			seen := map[string]bool{}
			for _, b := range g.Bindings {
				help := b.Help()
				if help.Key == "" {
					t.Fatalf("view %v, group %q: binding with no help text: %+v", view, g.Title, b)
				}
				if seen[help.Key] {
					t.Fatalf("view %v, group %q: help key %q listed twice", view, g.Title, help.Key)
				}
				seen[help.Key] = true
			}
		}
	}
}

// TestGroups_ViewsGroupHasAllFiveJumpsAndCycle spot-checks spec §2's Views
// row: 1-5 direct jumps plus "s" to cycle.
func TestGroups_ViewsGroupHasAllFiveJumpsAndCycle(t *testing.T) {
	k := defaultKeyMap()
	groups := k.Groups(context.PullsView)
	views := groupByTitle(t, groups, "Views")
	wantKeys := []string{"1", "2", "3", "4", "5", "s"}
	for _, want := range wantKeys {
		if !anyBindingHasKey(views, want) {
			t.Fatalf("Views group missing key %q: %+v", want, helpKeys(views))
		}
	}
}

// TestGroups_ListGroupHasMoveAndFirstLast spot-checks spec §2's List row.
func TestGroups_ListGroupHasMoveAndFirstLast(t *testing.T) {
	k := defaultKeyMap()
	list := groupByTitle(t, k.Groups(context.PullsView), "List")
	for _, want := range []string{"up", "k", "down", "j", "g", "G"} {
		if !anyBindingHasKey(list, want) {
			t.Fatalf("List group missing key %q: %+v", want, helpKeys(list))
		}
	}
}

// TestGroups_PreviewGroupHasFocusAndScroll spot-checks spec §2's Preview
// rows (both the always-shown toggle and the focused-only scroll/tab keys).
func TestGroups_PreviewGroupHasFocusAndScroll(t *testing.T) {
	k := defaultKeyMap()
	preview := groupByTitle(t, k.Groups(context.PullsView), "Preview")
	for _, want := range []string{"enter", "tab", "u", "d", "[", "]", "p", "e"} {
		if !anyBindingHasKey(preview, want) {
			t.Fatalf("Preview group missing key %q: %+v", want, helpKeys(preview))
		}
	}
}

// TestGroups_GlobalGroupDropsCtrlRAndIncludesEsc covers spec §2's migration
// table: ctrl+r is gone from RefreshAll (R only), and esc (the universal
// dismiss) is a real, listed binding.
func TestGroups_GlobalGroupDropsCtrlRAndIncludesEsc(t *testing.T) {
	k := defaultKeyMap()
	global := groupByTitle(t, k.Groups(context.PullsView), "Global")
	if !anyBindingHasKey(global, "esc") {
		t.Fatalf("Global group missing esc: %+v", helpKeys(global))
	}
	for _, b := range global {
		if b.Help().Key != "R" {
			continue
		}
		for _, key := range b.Keys() {
			if key == "ctrl+r" {
				t.Fatalf("RefreshAll should no longer include ctrl+r (spec §2 migration table): %+v", b.Keys())
			}
		}
	}
}

// TestGroups_OpenIsBrowserOnly covers spec §2's migration table: enter no
// longer opens the browser — only "o" does (enter now focuses the preview).
func TestGroups_OpenIsBrowserOnly(t *testing.T) {
	k := defaultKeyMap()
	global := groupByTitle(t, k.Groups(context.PullsView), "Global")
	for _, b := range global {
		if b.Help().Key != "o" {
			continue
		}
		for _, key := range b.Keys() {
			if key == "enter" {
				t.Fatalf("Open should no longer include enter (spec §2 migration table): %+v", b.Keys())
			}
		}
		return
	}
	t.Fatal("Global group missing the open binding")
}

// TestGroups_ViewScopedGroupChangesWithView confirms the trailing group is
// specific to the given view (PRs/Issues/Inbox/CI/Branches), not a fixed
// extra group.
func TestGroups_ViewScopedGroupChangesWithView(t *testing.T) {
	tests := []struct {
		view  context.ViewType
		title string
		key   string
	}{
		{context.PullsView, "PRs", "m"},
		{context.IssuesView, "Issues", "M"},
		{context.NotificationsView, "Inbox", "m"},
		{context.ActionsView, "CI", "R"},
		{context.BranchesView, "Branches", "P"},
	}
	k := defaultKeyMap()
	for _, tt := range tests {
		groups := k.Groups(tt.view)
		last := groups[len(groups)-1]
		if last.Title != tt.title {
			t.Fatalf("view %v: trailing group title = %q, want %q", tt.view, last.Title, tt.title)
		}
		if !anyBindingHasKey(last.Bindings, tt.key) {
			t.Fatalf("view %v: %q group missing key %q: %+v", tt.view, tt.title, tt.key, helpKeys(last.Bindings))
		}
	}
}

func groupByTitle(t *testing.T, groups []BindingGroup, title string) []key.Binding {
	t.Helper()
	for _, g := range groups {
		if g.Title == title {
			return g.Bindings
		}
	}
	t.Fatalf("no group titled %q in %+v", title, groups)
	return nil
}

func anyBindingHasKey(bindings []key.Binding, want string) bool {
	for _, b := range bindings {
		for _, k := range b.Keys() {
			if k == want {
				return true
			}
		}
	}
	return false
}

func helpKeys(bindings []key.Binding) []string {
	out := make([]string, len(bindings))
	for i, b := range bindings {
		out[i] = b.Help().Key
	}
	return out
}

// TestRebindBuiltin_UpdatesKnownFields is the plan's explicit rebind
// regression coverage: rebinding open/nextSection/removeReviewers to a new
// key replaces (not appends to) that field's key list, and the field no
// longer matches its old default key.
func TestRebindBuiltin_UpdatesKnownFields(t *testing.T) {
	tests := []struct {
		builtin string
		oldKey  string
		newKey  string
		fieldOf func(k keyMap) key.Binding
	}{
		{"open", "o", "X", func(k keyMap) key.Binding { return k.Open }},
		{"nextSection", "l", "N", func(k keyMap) key.Binding { return k.NextSection }},
		{"removeReviewers", "#", "Z", func(k keyMap) key.Binding { return k.RemoveReviewers }},
	}
	for _, tt := range tests {
		t.Run(tt.builtin, func(t *testing.T) {
			k := defaultKeyMap()
			if !anyBindingHasKey([]key.Binding{tt.fieldOf(k)}, tt.oldKey) {
				t.Fatalf("precondition failed: default %s binding missing %q", tt.builtin, tt.oldKey)
			}
			k.rebindBuiltin(tt.builtin, tt.newKey)
			got := tt.fieldOf(k)
			if !anyBindingHasKey([]key.Binding{got}, tt.newKey) {
				t.Fatalf("rebindBuiltin(%q, %q) did not land: keys = %v", tt.builtin, tt.newKey, got.Keys())
			}
			if anyBindingHasKey([]key.Binding{got}, tt.oldKey) {
				t.Fatalf("rebindBuiltin(%q, %q) should replace the old key %q, got keys = %v", tt.builtin, tt.newKey, tt.oldKey, got.Keys())
			}
		})
	}
}

// TestRebindBuiltin_IgnoresBlankKey confirms an empty configured key is a
// no-op (leaves the default untouched) rather than clearing the binding.
func TestRebindBuiltin_IgnoresBlankKey(t *testing.T) {
	k := defaultKeyMap()
	before := k.Open.Keys()
	k.rebindBuiltin("open", "  ")
	if got := k.Open.Keys(); len(got) != len(before) || got[0] != before[0] {
		t.Fatalf("blank key should be a no-op: before=%v after=%v", before, got)
	}
}
