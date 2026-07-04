// Package helpoverlay renders the full-keymap help modal (spec §4): every
// key-binding group (spec §2's Views/Sections/List/Preview/Search/
// Overlays/Global groups, plus the current view's scoped group) plus a
// static mouse cheatsheet, scrollable when it overflows.
//
// It's content-agnostic, like components/sidebar: internal/ui/keys.go's
// keyMap.Groups(view) is what actually generates the groups, but keyMap is
// unexported and lives in package ui, which imports this package to render
// it — so this package can't import package ui back (that would be a
// cycle). Group below has the exact same shape as keys.go's exported
// BindingGroup; app.go does the (trivial, one-line-per-group) conversion
// before calling SetGroups.
package helpoverlay

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Group is one titled cluster of bindings — the same shape as
// internal/ui/keys.go's BindingGroup (see the package doc for why this
// package can't just use that type directly).
type Group struct {
	Title    string
	Bindings []key.Binding
}

// mouseCheatsheet is the static mouse section appended after the keymap
// groups. It documents only gestures that work TODAY — double-click/
// right-click/wheel-over-preview are Task 6 additions and are deliberately
// left out rather than listed as "(soon)", to avoid the overlay describing
// features that don't exist yet.
var mouseCheatsheet = []struct{ gesture, action string }{
	{"click", "select row / switch section or view"},
	{"wheel", "scroll the list"},
}

// Model is a scrollable modal listing every keybinding group plus the
// mouse cheatsheet.
type Model struct {
	vp  viewport.Model
	ctx *context.ProgramContext
}

// New builds a help overlay bound to the shared program context.
func New(ctx *context.ProgramContext) Model {
	return Model{vp: viewport.New(), ctx: ctx}
}

// SetGroups renders groups (plus the static mouse cheatsheet) as the
// overlay's content and scrolls back to the top — called every time the
// overlay opens, so it always reflects the current view and any rebound
// keys.
func (m *Model) SetGroups(groups []Group) {
	m.vp.SetContent(render(groups, m.ctx.Styles))
	m.vp.GotoTop()
}

// SetSize sizes the underlying viewport to the interior it's rendered
// into. Safe to call every render (idempotent) — mirrors
// components/sidebar's resize().
func (m *Model) SetSize(w, h int) {
	if w < 0 {
		w = 0
	}
	if h < 1 {
		h = 1
	}
	m.vp.SetWidth(w)
	m.vp.SetHeight(h)
}

// Update handles scroll keys: j/k line, g/G top/bottom, d/u half-page
// (ctrl+d/ctrl+u also accepted). It reports whether it consumed the key,
// mirroring components/sidebar.Update's contract — though app.go's caller
// (while the overlay is open) swallows every key regardless, per spec §4
// ("overlay intercepts all keys while open").
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, bool) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil, false
	}
	switch key.String() {
	case "j", "down":
		m.vp.ScrollDown(1)
	case "k", "up":
		m.vp.ScrollUp(1)
	case "g":
		m.vp.GotoTop()
	case "G":
		m.vp.GotoBottom()
	case "d", "ctrl+d":
		m.vp.HalfPageDown()
	case "u", "ctrl+u":
		m.vp.HalfPageUp()
	default:
		return m, nil, false
	}
	return m, nil, true
}

// View renders the overlay's current viewport window. The caller
// (app.go's fitBlock discipline) pads/truncates this to the exact interior
// rect it's composited into — View doesn't need to produce exact
// dimensions itself, matching components/sidebar's View().
func (m Model) View() string {
	return m.vp.View()
}

func render(groups []Group, styles context.Styles) string {
	var b strings.Builder
	for i, g := range groups {
		if i > 0 {
			b.WriteString("\n\n")
		}
		writeGroup(&b, g.Title, styles)
		for _, binding := range g.Bindings {
			help := binding.Help()
			if help.Key == "" {
				continue
			}
			fmt.Fprintf(&b, "  %-14s %s\n", help.Key, help.Desc)
		}
	}
	if len(mouseCheatsheet) > 0 {
		b.WriteString("\n\n")
		writeGroup(&b, "Mouse", styles)
		for _, m := range mouseCheatsheet {
			fmt.Fprintf(&b, "  %-14s %s\n", m.gesture, m.action)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeGroup(b *strings.Builder, title string, styles context.Styles) {
	b.WriteString(styles.PanelTitle.Render(title))
	b.WriteString("\n")
}
