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

func (m Model) styleFor(i int) lipgloss.Style {
	if i == m.cursor {
		return m.ctx.Styles.ActiveTab
	}
	return m.ctx.Styles.Tab
}

// Range is the 0-based column range (relative to the embedded segment's own
// left edge — the leading "─", offset 0) a tab occupies in whatever View()
// currently renders. Task 6's ZoneSectionTab mouse-hit registration
// (app.go) builds its Rects straight from these, so a click always lands on
// exactly what's drawn — including when a narrow terminal truncates or
// drops trailing tabs (see render's doc comment).
type Range struct {
	Index      int
	Start, End int
}

// render lays out every section as a "Title (N)" tab joined by "──",
// applying whole-label priority truncation once ctx.MainContentWidth is
// set and positive: tabs that fit render in full; the first tab that
// doesn't is ellipsized to fit the remaining space (carrying forward the
// T3 review's ask — drop the whole label rather than cut it mid-word, e.g.
// the old "Needs My Rev" glitch); every tab after that is dropped
// entirely. ctx.MainContentWidth <= 0 (the zero value plain-struct test
// callers get when they don't set it) means unconstrained: render every
// tab in full, exactly matching pre-Task-6 behavior.
//
// Width is measured on the STYLED (already Render'd) text, not the raw
// label — ActiveTab carries its own padding (see header.go's build doc
// comment for the same gotcha), so the rendered cell width can exceed the
// label's own rune count.
func (m Model) render() (string, []Range) {
	if len(m.sections) < 2 {
		return "", nil
	}
	maxW := 0
	if m.ctx != nil {
		maxW = m.ctx.MainContentWidth
	}

	var b strings.Builder
	b.WriteString("─")
	pos := 1
	var ranges []Range
	for i := range m.sections {
		rendered := m.styleFor(i).Render(m.labelAt(i))
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
				if remaining < 2 { // not even room for "x…"
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
		ranges = append(ranges, Range{Index: i, Start: pos, End: pos + renderedW})
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
// app.go's embedInBorderRow/padOrTruncateLine, which already truncate
// pre-styled segments the same way (lipgloss.Style.MaxWidth is ANSI-aware,
// so it won't cut mid-escape-sequence).
func ellipsize(rendered string, w int) string {
	if lipgloss.Width(rendered) <= w {
		return rendered
	}
	if w <= 1 {
		return "…"
	}
	return lipgloss.NewStyle().MaxWidth(w-1).Render(rendered) + "…"
}

// View renders the tab bar as a segment embeddable in the list panel's top
// border line — "─ Open (12) ── Closed (3) ─" — or "" when there are fewer
// than two sections (the border row is then a plain dash rule) or once
// truncation (see render) has dropped every tab (pathologically narrow
// width).
func (m Model) View() string {
	s, _ := m.render()
	return s
}

// Ranges returns the column range each currently-rendered tab occupies —
// see Range's doc comment. Empty when the bar is hidden.
func (m Model) Ranges() []Range {
	_, r := m.render()
	return r
}

// TabWidth returns the rendered cell width of tab i as View() currently
// renders it — 0 if it's out of range or has been truncated away entirely.
func (m Model) TabWidth(i int) int {
	for _, r := range m.Ranges() {
		if r.Index == i {
			return r.End - r.Start
		}
	}
	return 0
}

// TabAt maps a cell offset relative to the embedded segment's own left edge
// (the leading "─", i.e. offset 0) to a section index, from the SAME render
// pass View() uses — so a truncated/dropped trailing tab is correctly
// never clickable past where it visually ends (or not clickable at all).
// It returns false when the tab bar is hidden or the offset is outside
// every rendered tab.
func (m Model) TabAt(x int) (int, bool) {
	if x < 0 {
		return 0, false
	}
	for _, r := range m.Ranges() {
		if x >= r.Start && x < r.End {
			return r.Index, true
		}
	}
	return 0, false
}
