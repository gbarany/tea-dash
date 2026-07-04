// Package tabs renders the section tab bar, embedded in the list panel's
// top border line (spec §1: "├─ Open (12) ── Closed (3) ─..."). It is
// hidden (empty) while there are fewer than two sections, so a
// single-section view's border row is a plain dash rule.
package tabs

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Sectioner is the minimal view of a section the tab bar needs. (section.Section
// satisfies it; declared here to avoid importing section and risking a cycle.)
type Sectioner interface {
	GetTitle() string
	GetTotalCount() int
	GetIsLoading() bool
}

// Model is the tab bar.
type Model struct {
	ctx      *context.ProgramContext
	sections []Sectioner
	cursor   int
}

// New builds a tab bar bound to ctx.
func New(ctx *context.ProgramContext) Model {
	return Model{ctx: ctx}
}

// SetSections replaces the sections the bar renders.
func (m *Model) SetSections(s []Sectioner) { m.sections = s }

// SetCurrSectionId marks the active tab.
func (m *Model) SetCurrSectionId(i int) { m.cursor = i }

// CurrSectionId returns the active tab index.
func (m Model) CurrSectionId() int { return m.cursor }

// UpdateProgramContext recaches the shared context.
func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) { m.ctx = ctx }

func (m Model) labelAt(i int) string {
	s := m.sections[i]
	return fmt.Sprintf("%s (%d)", s.GetTitle(), s.GetTotalCount())
}

func (m Model) renderedTabAt(i int) string {
	label := m.labelAt(i)
	if i == m.cursor {
		return m.ctx.Styles.ActiveTab.Render(label)
	}
	return m.ctx.Styles.Tab.Render(label)
}

// TabWidth returns the rendered cell width of tab i. It returns 0 for out of
// range tabs.
func (m Model) TabWidth(i int) int {
	if i < 0 || i >= len(m.sections) {
		return 0
	}
	return lipgloss.Width(m.renderedTabAt(i))
}

// TabAt maps a cell offset relative to the embedded segment's own left edge
// (the leading "─", i.e. offset 0) to a section index. It returns false when
// the tab bar is hidden or the offset is outside all rendered tabs.
func (m Model) TabAt(x int) (int, bool) {
	if len(m.sections) < 2 || x < 0 {
		return 0, false
	}
	pos := 1 // skip the leading "─"
	for i := range m.sections {
		w := m.TabWidth(i)
		if x >= pos && x < pos+w {
			return i, true
		}
		pos += w + 2 // "──" separator before the next tab
	}
	return 0, false
}

// View renders the tab bar as a segment embeddable in the list panel's top
// border line — "─ Open (12) ── Closed (3) ─" — or "" when there are fewer
// than two sections (the border row is then a plain dash rule).
func (m Model) View() string {
	if len(m.sections) < 2 {
		return ""
	}
	rendered := make([]string, len(m.sections))
	for i := range m.sections {
		rendered[i] = m.renderedTabAt(i)
	}
	return "─" + strings.Join(rendered, "──")
}
