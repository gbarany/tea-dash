// Package sidebar is a dumb, scrollable preview pane: a viewport wrapper that
// renders a pre-formatted string and offers half-page scrolling plus a
// bottom-right scroll indicator. It owns no content of its own — callers push a
// rendered string via SetContent — and it draws nothing while the preview is
// closed.
package sidebar

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Model wraps a viewport bound to the shared program context.
type Model struct {
	vp  viewport.Model
	ctx *context.ProgramContext
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
	m.vp.SetContent(s)
	m.vp.GotoTop()
}

// ScrollToTop resets the viewport to the first line.
func (m *Model) ScrollToTop() { m.vp.GotoTop() }

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
// pane fits within PreviewHeight.
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
