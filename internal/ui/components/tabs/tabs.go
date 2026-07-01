// Package tabs renders the section tab bar. It is hidden (empty) while there is
// fewer than two sections, so a single-section view looks unchanged.
package tabs

import (
	"fmt"

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

// View renders the tab bar, or "" when there are fewer than two sections.
func (m Model) View() string {
	if len(m.sections) < 2 {
		return ""
	}
	st := m.ctx.Styles
	tabs := make([]string, len(m.sections))
	for i, s := range m.sections {
		label := fmt.Sprintf("%s (%d)", s.GetTitle(), s.GetTotalCount())
		if i == m.cursor {
			tabs[i] = st.ActiveTab.Render(label)
		} else {
			tabs[i] = st.Tab.Render(label)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}
