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

// SelectTab jumps directly to tab i (Task 6's ZonePreviewTab click
// handler; NextTab/PrevTab are relative, for the keyboard's "["/"]"). Out
// of range or already-selected is a no-op — clicking the tab you're
// already on shouldn't reset its scroll position back to the top.
func (m *Model) SelectTab(i int) {
	if i < 0 || i >= len(m.tabs) || i == m.tab {
		return
	}
	m.tab = i
	m.syncViewport()
	m.vp.GotoTop()
}

// Update handles preview-focused navigation (spec §2's "Preview (focused)"
// row): j/k line scroll, g/G top/bottom, d/u half-page, [/] tab cycling
// (ctrl+d/ctrl+u are also accepted, for custom keybindings still built
// around the pre-remap key — see app.go's "pageup"/"pagedown" builtin
// cases). It reports whether it consumed the key so the caller (app.go,
// only while the preview is focused) knows whether to fall through to its
// own key routing for anything else — view jumps, esc, tab/enter to
// unfocus, and so on all pass straight through unrecognized.
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
	case "[":
		m.PrevTab()
	case "]":
		m.NextTab()
	default:
		return m, nil, false
	}
	return m, nil, true
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

// TabRange is the 0-based column range (relative to TabsBorderSegment's own
// left edge — the leading "─", offset 0) a preview tab occupies in whatever
// TabsBorderSegment() currently renders. Mirrors components/tabs.Range;
// Task 6's ZonePreviewTab mouse-hit registration (app.go) builds its Rects
// straight from these.
type TabRange struct {
	Index      int
	Start, End int
}

// tabsBorder lays out every preview tab exactly like
// components/tabs.Model.render does (same whole-label priority truncation,
// ellipsizing the first tab that doesn't fit and dropping every one after
// it — see that function's doc comment for the full rationale, including
// why width is measured on the styled text) once ctx.PreviewWidth is set
// and positive; <= 0 (the zero value plain-struct test callers get when
// they don't set it) means unconstrained, matching pre-Task-6 behavior.
func (m Model) tabsBorder() (string, []TabRange) {
	if len(m.tabs) <= 1 {
		return "", nil
	}
	maxW := 0
	if m.ctx != nil {
		maxW = m.ctx.PreviewWidth
	}

	var b strings.Builder
	b.WriteString("─")
	pos := 1
	var ranges []TabRange
	for i, t := range m.tabs {
		style := m.ctx.Styles.Tab
		if i == m.tab {
			style = m.ctx.Styles.ActiveTab
		}
		rendered := style.Render(t.Title)
		renderedW := lipgloss.Width(rendered)

		sepW := 0
		if i > 0 {
			sepW = 2
		}
		truncated := false
		if maxW > 0 {
			remaining := maxW - pos - sepW
			if remaining <= 0 {
				break
			}
			if renderedW > remaining {
				if remaining < 2 {
					break
				}
				rendered = ellipsize(rendered, remaining)
				renderedW = lipgloss.Width(rendered)
				truncated = true
			}
		}

		if i > 0 {
			b.WriteString("──")
			pos += 2
		}
		ranges = append(ranges, TabRange{Index: i, Start: pos, End: pos + renderedW})
		b.WriteString(rendered)
		pos += renderedW

		if truncated {
			break
		}
	}
	return b.String(), ranges
}

// ellipsize trims a (possibly already ANSI-styled) rendered string to w-1
// cells and appends "…" (1 cell) — or just "…" alone when w == 1. Mirrors
// components/tabs' identical helper (app.go's embedInBorderRow/
// padOrTruncateLine already truncate pre-styled segments the same way).
func ellipsize(rendered string, w int) string {
	if lipgloss.Width(rendered) <= w {
		return rendered
	}
	if w <= 1 {
		return "…"
	}
	return lipgloss.NewStyle().MaxWidth(w-1).Render(rendered) + "…"
}

// TabsBorderSegment renders the sidebar's tabs as a segment embeddable in
// the preview panel's top border line — "─ Overview ── Checks ─" — or ""
// when there are fewer than two tabs (the border row is then a plain dash
// rule) or truncation has dropped every tab, mirroring components/tabs'
// embedding format.
func (m Model) TabsBorderSegment() string {
	s, _ := m.tabsBorder()
	return s
}

// TabRanges returns the column range each currently-rendered preview tab
// occupies — see TabRange's doc comment. Empty when the bar is hidden.
func (m Model) TabRanges() []TabRange {
	_, r := m.tabsBorder()
	return r
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
