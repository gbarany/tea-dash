// Package palette implements the command palette (spec §4's ":"/ctrl+p
// overlay): a text-filtered list of actions, view jumps, section jumps, and
// user custom commands.
//
// Like components/actionprompt and components/helpoverlay, it's
// content-agnostic — app.go builds the concrete Item list (from
// availableActions, the fixed view list, the current view's sections, and
// cfg.Keybindings custom commands) and dispatches whatever Item a run event
// carries; this package only knows how to filter, navigate, and render a
// slice of Items. It can't import internal/ui/context back for anything
// beyond Styles (same import-cycle reasoning as helpoverlay's package doc),
// so KindView/KindSection carry an opaque Index the caller casts back to
// its own types.
package palette

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

// Kind is what an Item runs when selected — app.go's dispatchPaletteItem
// switches on it to decide which of handleBuiltinKeybinding/switchToView/
// switchSectionTo/startCustomCommand to call.
type Kind int

const (
	KindAction Kind = iota
	KindView
	KindSection
	KindCustom
)

// Item is one palette row. Which fields are meaningful depends on Kind:
// KindAction reads Builtin (a builtin name, dispatched exactly like a
// keybinding through handleBuiltinKeybinding); KindView and KindSection
// read Index (a context.ViewType value / section id, cast by the caller);
// KindCustom reads Command and reuses Label as the custom command's
// configured Name (see config.Keybinding.Name).
type Item struct {
	Label   string
	KeyHint string
	Kind    Kind
	Builtin string
	Command string
	Index   int
}

// EventKind is what a key press did to the palette that the caller
// (app.go's updatePaletteOverlay) needs to react to — closing the overlay,
// dispatching an item, or neither (typing, navigation).
type EventKind int

const (
	EventNone EventKind = iota
	EventDismiss
	EventRun
)

// Event reports the outcome of a key press; Item is only meaningful for
// EventRun.
type Event struct {
	Kind EventKind
	Item Item
}

// HeaderRows is how many lines of View()'s output precede the first item
// row (the input line, then one blank separator line). app.go's
// rebuildZones uses this to register a mouse zone per visible item at the
// same Y offset View() renders them at, so the two must stay in sync.
const HeaderRows = 2

// Model is the palette's filter box plus its scrollable, filtered item list.
type Model struct {
	ctx   *appctx.ProgramContext
	input textinput.Model

	all      []Item
	filtered []Item
	selected int
	offset   int

	width  int
	height int // rows available for the item list; 0 = unbounded (tests that skip SetSize)
}

// New builds a palette bound to the shared program context. It starts with
// no items — Open supplies them each time the palette is shown.
func New(ctx *appctx.ProgramContext) Model {
	ti := textinput.New()
	ti.Prompt = ": "
	ti.Placeholder = "type to filter…"
	return Model{ctx: ctx, input: ti}
}

// Open resets the palette to items, clears any previous query, resets the
// selection to the top, and focuses the input — called every time the
// palette opens, since the valid item set depends on the current
// view/row/scope.
func (m *Model) Open(items []Item) tea.Cmd {
	m.all = items
	m.input.SetValue("")
	cmd := m.input.Focus()
	m.input.CursorEnd()
	m.applyFilter()
	return cmd
}

// SetSize sizes the item list's visible window. Safe to call every render
// (idempotent), mirroring helpoverlay.Model.SetSize.
func (m *Model) SetSize(w, h int) {
	if w < 0 {
		w = 0
	}
	if h < 1 {
		h = 1
	}
	m.width = w
	m.height = h
	m.input.SetWidth(w)
	m.ensureVisible()
}

// Value returns the current filter text.
func (m Model) Value() string { return m.input.Value() }

// Selected returns the item the cursor is currently on, if the filtered
// list is non-empty.
func (m Model) Selected() (Item, bool) {
	if m.selected < 0 || m.selected >= len(m.filtered) {
		return Item{}, false
	}
	return m.filtered[m.selected], true
}

// Visible returns the currently visible window of filtered items (honoring
// scroll offset and the height set by SetSize) plus which index within that
// window is selected (-1 if the selection isn't in view, which shouldn't
// happen since ensureVisible keeps it there). Rendering (View) and mouse
// hit-testing (app.go's rebuildZones, via ItemAtVisibleIndex) both read
// this so they never disagree about what's on screen.
func (m Model) Visible() ([]Item, int) {
	if len(m.filtered) == 0 {
		return nil, -1
	}
	start := m.offset
	end := len(m.filtered)
	if m.height > 0 && start+m.height < end {
		end = start + m.height
	}
	if start > end {
		start = end
	}
	items := m.filtered[start:end]
	sel := m.selected - start
	if sel < 0 || sel >= len(items) {
		sel = -1
	}
	return items, sel
}

// ItemAtVisibleIndex maps a click's row (relative to the visible window
// Visible returns, i.e. what app.go's rebuildZones registered a
// layout.ZonePaletteItem zone for) back to the Item it represents.
func (m Model) ItemAtVisibleIndex(i int) (Item, bool) {
	items, _ := m.Visible()
	if i < 0 || i >= len(items) {
		return Item{}, false
	}
	return items[i], true
}

// MoveSelection moves the cursor by delta (clamped to the filtered list's
// bounds) and scrolls to keep it visible — the mouse-wheel path
// (app.go's handleMouseWheel) calls this directly instead of routing a
// synthetic key through Update, since plain "j"/"k" are ordinary filter
// characters here (see Update's doc comment), not navigation keys.
func (m Model) MoveSelection(delta int) Model {
	m.moveSelection(delta)
	return m
}

func (m *Model) moveSelection(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.filtered) {
		m.selected = len(m.filtered) - 1
	}
	m.ensureVisible()
}

func (m *Model) ensureVisible() {
	if m.height <= 0 {
		return
	}
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+m.height {
		m.offset = m.selected - m.height + 1
	}
}

// Update handles one message. Only up/down arrows navigate, enter runs the
// selected item, and esc dismisses — everything else (including plain "j"/
// "k", unlike helpoverlay's vim-style scroll keys) is forwarded to the text
// input as ordinary filter text, since typing IS the palette's primary
// interaction and reserving letters for navigation would make some labels
// unfilterable by their own contents.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Event) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd, Event{}
	}
	switch key.String() {
	case "esc":
		return m, nil, Event{Kind: EventDismiss}
	case "enter":
		if item, ok := m.Selected(); ok {
			return m, nil, Event{Kind: EventRun, Item: item}
		}
		return m, nil, Event{}
	case "up":
		m.moveSelection(-1)
		return m, nil, Event{}
	case "down":
		m.moveSelection(1)
		return m, nil, Event{}
	}
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.applyFilter()
	}
	return m, cmd, Event{}
}

// View renders the input line, a blank separator (HeaderRows), then the
// visible window of filtered items — the selected row highlighted, each
// row's KeyHint (if any) right-aligned. The caller (app.go's fitBlock
// discipline) pads/truncates this to its exact interior rect, so View
// doesn't need to produce exact dimensions itself, matching
// components/helpoverlay and components/sidebar.
func (m Model) View() string {
	lines := make([]string, 0, HeaderRows+len(m.filtered))
	lines = append(lines, m.input.View(), "")
	if len(m.all) == 0 {
		return strings.Join(lines, "\n")
	}
	if len(m.filtered) == 0 {
		lines = append(lines, m.dim("No matches."))
		return strings.Join(lines, "\n")
	}
	items, selIdx := m.Visible()
	for i, it := range items {
		lines = append(lines, m.renderRow(it, i == selIdx))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderRow(it Item, selected bool) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}
	line := prefix + it.Label
	if it.KeyHint != "" {
		pad := 2
		if m.width > 0 {
			if gap := m.width - len(line) - len(it.KeyHint); gap > pad {
				pad = gap
			}
		}
		line += strings.Repeat(" ", pad) + it.KeyHint
	}
	if !selected {
		return line
	}
	if m.width > len(line) {
		line += strings.Repeat(" ", m.width-len(line))
	}
	if m.ctx == nil {
		return line
	}
	return m.ctx.Styles.Table.Selected.Render(line)
}

// dim renders s in the dim/faint style, or unstyled when ctx is nil
// (component unit tests that don't wire a ProgramContext).
func (m Model) dim(s string) string {
	if m.ctx == nil {
		return s
	}
	return m.ctx.Styles.DimText.Render(s)
}

// applyFilter recomputes m.filtered from m.all and the current input value,
// resetting the selection and scroll to the top — standard fuzzy-picker
// behavior (fzf, VS Code's palette, ...): a previously selected item may not
// even be in the new result set, so keeping its index would either point at
// an unrelated row or need extra bookkeeping to track identity across
// filters for no real benefit.
func (m *Model) applyFilter() {
	m.filtered = filterItems(m.all, m.input.Value())
	m.selected = 0
	m.offset = 0
}

// filterItems keeps items whose Label case-insensitively contains query as
// a subsequence (fzf's default feel: "mrg" matches "Merge"), then sorts
// prefix matches before scattered ones — stable otherwise, so ties (both
// prefix or both scattered) keep their original relative order. An empty
// query matches everything, unsorted, so "show all" doesn't depend on the
// scoring below ever agreeing that everything ties.
func filterItems(items []Item, query string) []Item {
	if strings.TrimSpace(query) == "" {
		out := make([]Item, len(items))
		copy(out, items)
		return out
	}
	q := strings.ToLower(query)
	type scored struct {
		item   Item
		prefix bool
	}
	matched := make([]scored, 0, len(items))
	for _, it := range items {
		label := strings.ToLower(it.Label)
		if !isSubsequence(q, label) {
			continue
		}
		matched = append(matched, scored{item: it, prefix: strings.HasPrefix(label, q)})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].prefix && !matched[j].prefix
	})
	out := make([]Item, len(matched))
	for i, s := range matched {
		out[i] = s.item
	}
	return out
}

// isSubsequence reports whether every rune of q appears in s in order
// (not necessarily contiguous) — q and s are both assumed already
// lower-cased by the caller.
func isSubsequence(q, s string) bool {
	qi := 0
	qr := []rune(q)
	for _, r := range s {
		if qi >= len(qr) {
			break
		}
		if r == qr[qi] {
			qi++
		}
	}
	return qi >= len(qr)
}
