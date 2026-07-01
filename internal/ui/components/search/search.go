// Package search provides a minimal keyword search input: a thin wrapper around
// bubbles' textinput that the issues/pulls sections use to filter by free text.
// It is deliberately plain (no DSL, no autocomplete) — just a focusable box.
package search

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

// Model is the keyword search box.
type Model struct {
	ctx   *appctx.ProgramContext
	input textinput.Model
}

// New builds a blurred search box bound to ctx.
func New(ctx *appctx.ProgramContext) Model {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "filter by keyword…"
	return Model{ctx: ctx, input: ti}
}

// Focus focuses the input, moves the cursor to the end, and returns the
// textinput's focus command (e.g. to start the cursor blinking).
func (m *Model) Focus() tea.Cmd {
	cmd := m.input.Focus()
	m.input.CursorEnd()
	return cmd
}

// Blur removes focus from the input.
func (m *Model) Blur() { m.input.Blur() }

// Focused reports whether the input currently has focus.
func (m Model) Focused() bool { return m.input.Focused() }

// Value returns the current keyword text.
func (m Model) Value() string { return m.input.Value() }

// SetValue replaces the keyword text.
func (m *Model) SetValue(s string) { m.input.SetValue(s) }

// CursorEnd moves the cursor to the end of the input.
func (m *Model) CursorEnd() { m.input.CursorEnd() }

// Update delegates to the underlying textinput (a no-op while blurred).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// View renders the input.
func (m Model) View() string { return m.input.View() }

// UpdateProgramContext recaches the shared context (kept for symmetry with the
// other components; the search box does not yet draw from ctx styles).
func (m *Model) UpdateProgramContext(ctx *appctx.ProgramContext) { m.ctx = ctx }
