// Package sidebar is a scrollable preview pane: a viewport wrapper that renders
// pre-formatted content, optionally split into tabs, with half-page scrolling
// and a bottom-right scroll indicator. It draws nothing while the preview is
// closed.
package sidebar

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Model wraps a viewport bound to the shared program context.
type Model struct {
	vp      viewport.Model
	ctx     *context.ProgramContext
	tabs    []Tab
	tab     int
	content string
}

// Tab is one selectable sidebar page.
type Tab struct {
	Title   string
	Content string
}

// New builds a sidebar sized to the context's current preview dimensions.
func New(ctx *context.ProgramContext) Model {
	m := Model{vp: viewport.New(), ctx: ctx}
	m.resize()
	return m
}

// SetContent replaces the previewed text and scrolls back to the top so a newly
// selected row starts at its header.
func (m *Model) SetContent(s string) {
	m.SetTabs([]Tab{{Title: "Overview", Content: s}})
}

// SetTabs replaces the preview tabs and scrolls back to the top. Empty tabs
// clear the viewport.
func (m *Model) SetTabs(tabs []Tab) {
	m.tabs = compactTabs(tabs)
	m.tab = 0
	m.resize()
	m.syncViewport()
	m.vp.GotoTop()
}

// ScrollToTop resets the viewport to the first line.
func (m *Model) ScrollToTop() { m.vp.GotoTop() }

// CurrentTabTitle returns the selected tab title, or "" when no tab exists.
func (m Model) CurrentTabTitle() string {
	if len(m.tabs) == 0 || m.tab < 0 || m.tab >= len(m.tabs) {
		return ""
	}
	return m.tabs[m.tab].Title
}

// NextTab selects the next sidebar tab, wrapping at the end.
func (m *Model) NextTab() {
	if len(m.tabs) <= 1 {
		return
	}
	m.tab = (m.tab + 1) % len(m.tabs)
	m.syncViewport()
	m.vp.GotoTop()
}

// PrevTab selects the previous sidebar tab, wrapping at the beginning.
func (m *Model) PrevTab() {
	if len(m.tabs) <= 1 {
		return
	}
	m.tab = (m.tab - 1 + len(m.tabs)) % len(m.tabs)
	m.syncViewport()
	m.vp.GotoTop()
}

// Update handles only half-page scrolling (ctrl+u / ctrl+d); every other
// message is ignored so the pane never steals navigation or quit keys.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "ctrl+u":
			m.vp.HalfPageUp()
		case "ctrl+d":
			m.vp.HalfPageDown()
		}
	}
	return m, nil
}

// UpdateProgramContext recaches the shared context and re-sizes the viewport to
// the current preview dimensions.
func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) {
	m.ctx = ctx
	m.resize()
}

// View renders the viewport plus a right-aligned "NN%" scroll indicator, or ""
// while the preview is closed. The indicator occupies the last row, so the whole
// pane fits within PreviewHeight. Sidebar tabs no longer render here — they're
// embedded in the preview panel's top border line (see TabsBorderSegment).
func (m Model) View() string {
	if !m.ctx.PreviewOpen {
		return ""
	}
	pct := fmt.Sprintf("%d%%", int(m.vp.ScrollPercent()*100))
	indicator := m.ctx.Styles.DimText.
		Width(m.ctx.PreviewWidth).
		Align(lipgloss.Right).
		Render(pct)
	return lipgloss.JoinVertical(lipgloss.Left, m.vp.View(), indicator)
}

// TabsBorderSegment renders the sidebar's tabs as a segment embeddable in
// the preview panel's top border line — "─ Overview ── Checks ─" — or ""
// when there are fewer than two tabs (the border row is then a plain dash
// rule), mirroring components/tabs' embedding format.
func (m Model) TabsBorderSegment() string {
	if len(m.tabs) <= 1 {
		return ""
	}
	rendered := make([]string, len(m.tabs))
	for i, t := range m.tabs {
		style := m.ctx.Styles.Tab
		if i == m.tab {
			style = m.ctx.Styles.ActiveTab
		}
		rendered[i] = style.Render(t.Title)
	}
	return "─" + strings.Join(rendered, "──")
}

// resize sizes the viewport to the preview area, reserving one row for the
// scroll indicator so View()'s total height stays within PreviewHeight.
func (m *Model) resize() {
	w := m.ctx.PreviewWidth
	if w < 0 {
		w = 0
	}
	h := m.ctx.PreviewHeight - 1 // reserve the last row for the scroll indicator
	if h < 1 {
		h = 1
	}
	m.vp.SetWidth(w)
	m.vp.SetHeight(h)
}

func (m *Model) syncViewport() {
	if len(m.tabs) == 0 {
		m.content = ""
		m.vp.SetContent("")
		return
	}
	if m.tab < 0 || m.tab >= len(m.tabs) {
		m.tab = 0
	}
	m.content = m.tabs[m.tab].Content
	m.vp.SetContent(m.content)
}

func compactTabs(tabs []Tab) []Tab {
	out := make([]Tab, 0, len(tabs))
	for _, t := range tabs {
		if t.Title == "" && t.Content == "" {
			continue
		}
		if t.Title == "" {
			t.Title = "Preview"
		}
		out = append(out, t)
	}
	return out
}
