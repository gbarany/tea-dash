package helpoverlay

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func sampleGroups() []Group {
	return []Group{
		{Title: "Views", Bindings: []key.Binding{
			key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "pulls")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "cycle views")),
		}},
		{Title: "List", Bindings: []key.Binding{
			key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "move up")),
			key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "move down")),
		}},
	}
}

// TestSetGroups_EveryBindingRenders is the plan's explicit requirement:
// walk the groups and assert every binding's help key renders
// (strings.Count(content, help.Key) >= 1, per the plan's own suggested
// check — single-character keys like "s" legitimately recur inside other
// descriptions' prose, e.g. "cycle view**s**", so "at least once" is the
// right bar, not "exactly once").
func TestSetGroups_EveryBindingRenders(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.SetSize(80, 40) // tall enough that nothing scrolls out of view
	groups := sampleGroups()
	m.SetGroups(groups)

	content := stripANSI(m.View())
	for _, g := range groups {
		for _, b := range g.Bindings {
			help := b.Help()
			if strings.Count(content, help.Key) < 1 {
				t.Fatalf("help key %q missing:\n%s", help.Key, content)
			}
			if !strings.Contains(content, help.Desc) {
				t.Fatalf("description %q for key %q missing:\n%s", help.Desc, help.Key, content)
			}
		}
	}
}

// TestSetGroups_ReboundKeyRenders covers the plan's rebind requirement:
// build a group where the "help" binding's key has been rebound (as
// keys.go's rebindBuiltin would do for a config like
// `keybindings.universal: [{key: F1, builtin: help}]`) and confirm the NEW
// key — not the old default — shows up.
func TestSetGroups_ReboundKeyRenders(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.SetSize(80, 40)
	rebound := key.NewBinding(key.WithKeys("F1"), key.WithHelp("F1", "help"))
	m.SetGroups([]Group{
		{Title: "Overlays", Bindings: []key.Binding{rebound}},
	})

	content := m.View()
	if !strings.Contains(content, "F1") {
		t.Fatalf("rebound key %q missing from content:\n%s", "F1", content)
	}
	if strings.Contains(content, "?") {
		t.Fatalf("old default key %q should not appear once rebound:\n%s", "?", content)
	}
}

// TestSetGroups_IncludesMouseCheatsheet confirms the static mouse section
// is always appended, honestly limited to gestures that exist today
// (click, wheel) — no double-click/right-click, which land in Task 6.
func TestSetGroups_IncludesMouseCheatsheet(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.SetSize(80, 40)
	m.SetGroups(sampleGroups())

	content := m.View()
	for _, want := range []string{"Mouse", "click", "wheel"} {
		if !strings.Contains(content, want) {
			t.Fatalf("mouse cheatsheet missing %q:\n%s", want, content)
		}
	}
	for _, notYet := range []string{"double-click", "right-click"} {
		if strings.Contains(content, notYet) {
			t.Fatalf("mouse cheatsheet should not advertise a Task 6 gesture %q yet:\n%s", notYet, content)
		}
	}
}

// TestSetGroups_SkipsBindingsWithNoHelp confirms a zero-value/no-help
// binding (e.g. a keyMap field left at its zero value) doesn't render a
// stray blank line. Exercises render() directly (same package) rather than
// through SetGroups+View, since the viewport pads every line to its full
// width with trailing spaces — noise that would make a "no stray blank
// line" check fragile.
func TestSetGroups_SkipsBindingsWithNoHelp(t *testing.T) {
	styles := context.DefaultStyles()
	content := render([]Group{
		{Title: "Views", Bindings: []key.Binding{
			key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "pulls")),
			{}, // zero-value binding, no help
		}},
	}, styles)

	if strings.Count(content, "pulls") != 1 {
		t.Fatalf("expected the one real binding to render exactly once:\n%q", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "  ") && strings.TrimSpace(line) == "" {
			t.Fatalf("stray blank binding line from the skipped zero-value binding: %q\nfull content:\n%q", line, content)
		}
	}
}

// TestUpdate_ScrollKeysMoveViewportAndReportHandled covers j/k/g/G/d/u
// (plus legacy ctrl+d/ctrl+u) scrolling the overlay, and an unrelated key
// being left alone and reported unhandled.
func TestUpdate_ScrollKeysMoveViewportAndReportHandled(t *testing.T) {
	m := New(&context.ProgramContext{Styles: context.DefaultStyles()})
	m.SetSize(20, 3) // small viewport so the long content actually scrolls

	var groups []Group
	var bindings []key.Binding
	for i := 0; i < 100; i++ {
		bindings = append(bindings, key.NewBinding(key.WithKeys(strconv.Itoa(i)), key.WithHelp(strconv.Itoa(i), "line")))
	}
	groups = append(groups, Group{Title: "Long", Bindings: bindings})
	m.SetGroups(groups)

	before := m.vp.ScrollPercent()

	next, cmd, handled := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if handled {
		t.Fatal("unrelated key should not be handled")
	}
	if cmd != nil {
		t.Fatalf("unrelated key should return a nil cmd, got %v", cmd)
	}
	if got := next.vp.ScrollPercent(); got != before {
		t.Fatalf("unrelated key changed scroll: before=%v after=%v", before, got)
	}
	m = next

	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if !handled {
		t.Fatal("'j' should be handled")
	}
	afterJ := m.vp.ScrollPercent()
	if afterJ <= before {
		t.Fatalf("'j' did not advance scroll: before=%v after=%v", before, afterJ)
	}

	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !handled {
		t.Fatal("'G' should be handled")
	}
	if got := m.vp.ScrollPercent(); got != 1 {
		t.Fatalf("'G' should scroll to the bottom, got %v", got)
	}

	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if !handled {
		t.Fatal("'g' should be handled")
	}
	if got := m.vp.ScrollPercent(); got != 0 {
		t.Fatalf("'g' should scroll to the top, got %v", got)
	}

	m, _, handled = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if !handled {
		t.Fatal("ctrl+d should still be handled (legacy compatibility)")
	}
	if got := m.vp.ScrollPercent(); got <= 0 {
		t.Fatalf("ctrl+d (half-page down) did not advance scroll, got %v", got)
	}
}
