// Package section defines the Section contract every dashboard section
// satisfies, plus an embeddable BaseModel that owns the table/spinner and
// renders the loading/error/empty/table body. It also provides a generic
// Model[T RowData] that implements Section for any row type, which the
// pullsection/issuesection packages specialize.
package section

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/components/search"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Section is implemented by *pointer* section types (they mutate on Update).
type Section interface {
	GetId() int
	GetType() string
	GetTitle() string
	GetTotalCount() int
	GetIsLoading() bool
	GetError() error

	NumRows() int
	GetCurrRow() data.RowData
	SelectRow(index int)
	FetchRows() tea.Cmd
	MaybeFetchNextPage() tea.Cmd

	GetItemSingular() string
	GetItemPlural() string

	IsSearchFocused() bool
	SetIsSearching(v bool) tea.Cmd

	Update(msg tea.Msg) (Section, tea.Cmd)
	View() string
	UpdateProgramContext(ctx *context.ProgramContext)
}

// NewOptions parameterizes NewBaseModel.
type NewOptions struct {
	Id      int
	Type    string
	Ctx     *context.ProgramContext
	Config  config.SectionConfig
	Columns []table.Column

	// Body copy for the loading/empty states, so the generic base carries no
	// section-specific wording.
	LoadingText string
	EmptyText   string
	EmptyHint   string

	// Item wording used by the root's status line ("3 pull requests" / "3 issues").
	SingularForm string
	PluralForm   string
}

// BaseModel provides the shared machinery concrete sections embed.
type BaseModel struct {
	id           int
	sectionType  string
	totalCount   int
	numRows      int
	isLoading    bool
	lastFetchID  string
	err          error
	loadingText  string
	emptyText    string
	emptyHint    string
	singularForm string
	pluralForm   string
	isSearching  bool

	Ctx       *context.ProgramContext
	Config    config.SectionConfig
	Table     table.Model
	Spinner   spinner.Model
	Columns   []table.Column
	SearchBar search.Model
}

// NewBaseModel builds a BaseModel with an empty focused table and a spinner,
// starting in the loading state.
func NewBaseModel(o NewOptions) BaseModel {
	t := table.New(
		table.WithColumns(o.Columns),
		table.WithFocused(true),
	)
	t.SetStyles(o.Ctx.Styles.Table)
	sp := spinner.New(spinner.WithStyle(o.Ctx.Styles.Spinner))
	return BaseModel{
		id:           o.Id,
		sectionType:  o.Type,
		Ctx:          o.Ctx,
		Config:       o.Config,
		Table:        t,
		Spinner:      sp,
		Columns:      o.Columns,
		SearchBar:    search.New(o.Ctx),
		isLoading:    true,
		loadingText:  o.LoadingText,
		emptyText:    o.EmptyText,
		emptyHint:    o.EmptyHint,
		singularForm: o.SingularForm,
		pluralForm:   o.PluralForm,
	}
}

func (m *BaseModel) GetId() int               { return m.id }
func (m *BaseModel) GetType() string          { return m.sectionType }
func (m *BaseModel) GetTitle() string         { return m.Config.Title }
func (m *BaseModel) GetItemSingular() string  { return m.singularForm }
func (m *BaseModel) GetItemPlural() string    { return m.pluralForm }
func (m *BaseModel) GetTotalCount() int       { return m.totalCount }
func (m *BaseModel) SetTotalCount(n int)      { m.totalCount = n }
func (m *BaseModel) GetIsLoading() bool       { return m.isLoading }
func (m *BaseModel) SetIsLoading(v bool)      { m.isLoading = v }
func (m *BaseModel) GetError() error          { return m.err }
func (m *BaseModel) SetError(err error)       { m.err = err }
func (m *BaseModel) LastFetchID() string      { return m.lastFetchID }
func (m *BaseModel) SetLastFetchID(id string) { m.lastFetchID = id }

func (m *BaseModel) NumRows() int { return m.numRows }
func (m *BaseModel) CurrRow() int { return m.Table.Cursor() }

// SelectRow moves the table cursor to index, clamping to the available rows.
func (m *BaseModel) SelectRow(index int) { m.Table.SetCursor(index) }

// MaybeFetchNextPage is a no-op for sections that do not support progressive
// pagination.
func (m *BaseModel) MaybeFetchNextPage() tea.Cmd { return nil }

// IsSearchFocused reports whether the embedded search bar is currently active.
func (m *BaseModel) IsSearchFocused() bool { return m.isSearching }

// SetIsSearching toggles the search bar. Focusing it returns the textinput's
// focus command; blurring it returns nil. Toggling immediately reserves (or
// restores) the row the search bar occupies so the table height stays within
// the exact content budget without waiting for the next terminal resize.
func (m *BaseModel) SetIsSearching(v bool) tea.Cmd {
	m.isSearching = v
	m.syncTableDimensions()
	if v {
		return m.SearchBar.Focus()
	}
	m.SearchBar.Blur()
	return nil
}

// SetRows updates the table rows and records the count for the empty-state check.
func (m *BaseModel) SetRows(rows []table.Row) {
	m.Table.SetRows(rows)
	m.numRows = len(rows)
}

// SafelySetColumnsAndRows applies newColumns and (if there are any rows)
// rebuilds them via buildRows, without ever exposing bubbles/table to a
// mismatched intermediate state.
//
// bubbles/table's SetRows and SetColumns each immediately re-render
// (UpdateViewport -> renderRow) using whatever the OTHER of the pair
// currently is, and renderRow indexes each row's cells by column position —
// so calling them in either order across a column-count CHANGE (e.g. a
// resize responsively dropping a column per SixColumnSpec.Fit) risks a
// moment where old (wider) rows are rendered against new (narrower)
// columns, or vice versa, and panics with an index-out-of-range.
//
// The fix: clear rows first (always column-count-safe — an empty table
// renders nothing per-row), apply the new columns, then refill with
// freshly-built rows. The cursor is saved and restored around this,
// because SetRows only clamps a cursor DOWN when it now exceeds the row
// count — it never clamps a negative cursor back up, so a naive clear
// would otherwise strand it at -1 (GetCurrRow would then nil-deref
// forever, even once real rows are refilled).
func (m *BaseModel) SafelySetColumnsAndRows(newColumns []table.Column, buildRows func() []table.Row) {
	if m.numRows == 0 {
		m.Columns = newColumns
		m.Table.SetColumns(m.Columns)
		return
	}
	cursor := m.Table.Cursor()
	m.Table.SetRows(nil)
	m.Columns = newColumns
	m.Table.SetColumns(m.Columns)
	m.SetRows(buildRows())
	m.Table.SetCursor(cursor)
}

// UpdateProgramContext recaches the context and resizes the table to the main
// content area. Concrete sections may override to also refresh columns.
func (m *BaseModel) UpdateProgramContext(ctx *context.ProgramContext) {
	m.Ctx = ctx
	m.SearchBar.UpdateProgramContext(ctx)
	m.syncTableDimensions()
}

// syncTableDimensions sizes the table to the current main content area. When
// searching, it reserves one row for the search bar rendered above the body so
// the combined height stays within the (slackless) content budget.
func (m *BaseModel) syncTableDimensions() {
	m.Table.SetWidth(m.Ctx.MainContentWidth)
	h := m.Ctx.MainContentHeight
	if m.isSearching {
		if h -= 1; h < 1 {
			h = 1
		}
	}
	m.Table.SetHeight(h)
}

// View renders the section body, preserving the pre-refactor layout exactly.
// While searching, the search bar is prepended above the body.
func (m *BaseModel) View() string {
	st := m.Ctx.Styles
	var body string
	switch {
	case m.isLoading:
		body = "\n  " + m.Spinner.View() + " " + m.loadingText
	case m.err != nil:
		body = "\n" + st.ErrorText.Render("  Error: "+m.err.Error()) + "\n\n" +
			st.DimText.Render("  Check your Gitea login (run `tea login add`) and network.")
	case m.numRows == 0:
		body = "\n  " + m.emptyText + "\n\n" +
			st.DimText.Render("  "+m.emptyHint)
	default:
		body = m.Table.View()
	}
	if m.isSearching {
		return m.SearchBar.View() + "\n" + body
	}
	return body
}

// ColumnDefinition is one known table column a section can expose.
type ColumnDefinition struct {
	Name  string
	Title string
	Width int
}

// ColumnsFromConfig turns a section's configured column list into table
// columns. Unknown names are ignored defensively; config validation rejects
// them before user config reaches the UI.
func ColumnsFromConfig(configured []config.ColumnConfig, defaults []ColumnDefinition) []table.Column {
	if len(configured) == 0 {
		return columnsFromDefinitions(defaults)
	}
	byName := make(map[string]ColumnDefinition, len(defaults))
	for _, def := range defaults {
		byName[def.Name] = def
	}
	out := make([]table.Column, 0, len(configured))
	for _, col := range configured {
		def, ok := byName[col.Name]
		if !ok {
			continue
		}
		title := def.Title
		if col.Title != "" {
			title = col.Title
		}
		width := def.Width
		if col.Width > 0 {
			width = col.Width
		}
		out = append(out, table.Column{Title: title, Width: width})
	}
	if len(out) == 0 {
		return columnsFromDefinitions(defaults)
	}
	return out
}

// ColumnNamesFromConfig returns configured column names in render order. It
// falls back to defaults defensively when no configured names are valid.
func ColumnNamesFromConfig(configured []config.ColumnConfig, defaults []ColumnDefinition) []string {
	defaultNames := columnNamesFromDefinitions(defaults)
	byName := make(map[string]bool, len(defaults))
	for _, def := range defaults {
		byName[def.Name] = true
	}
	if len(configured) == 0 {
		return defaultNames
	}
	names := make([]string, 0, len(configured))
	for _, col := range configured {
		if byName[col.Name] {
			names = append(names, col.Name)
		}
	}
	if len(names) == 0 {
		return defaultNames
	}
	return names
}

func columnNamesFromDefinitions(defs []ColumnDefinition) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

func columnsFromDefinitions(defs []ColumnDefinition) []table.Column {
	cols := make([]table.Column, 0, len(defs))
	for _, def := range defs {
		cols = append(cols, table.Column{Title: def.Title, Width: def.Width})
	}
	return cols
}

// ColumnsFromDefinitions converts defs (already fitted to a width, e.g. via
// SixColumnSpec.Fit) into table.Columns, in order. Exported for sections
// with their own SixColumnSpec (actionsection, branchsection) that aren't
// PR/issue-shaped.
func ColumnsFromDefinitions(defs []ColumnDefinition) []table.Column {
	return columnsFromDefinitions(defs)
}

// ColumnNamesFromDefinitions returns defs' names, in order — the column set
// a matching row builder must emit cells for, in the same order.
func ColumnNamesFromDefinitions(defs []ColumnDefinition) []string {
	return columnNamesFromDefinitions(defs)
}

// SixColumnSpec describes one section's default 6-column table layout: a
// leading index/mark column, a "grow" column that absorbs whatever width is
// left over (title/branch — its Width doubles as the PREFERRED minimum: it
// grows past that freely when there's room, exactly like the historical
// unbounded title-width formula), and three further fixed-width columns in
// drop priority order (Fourth dropped first, Updated second, Repo third)
// for when the interior can't fit all six even with Grow at its preferred
// minimum. Index, Grow, and State never drop — gh-dash's own convention
// keeps #/Title/State visible at any width.
type SixColumnSpec struct {
	Index   ColumnDefinition // e.g. "#" — never dropped
	Grow    ColumnDefinition // e.g. Title, Branch — never dropped; Width is its preferred minimum
	Repo    ColumnDefinition // dropped third (last)
	Fourth  ColumnDefinition // e.g. Author, Actor/Event, Upstream — dropped first
	State   ColumnDefinition // e.g. State, Status — never dropped
	Updated ColumnDefinition // e.g. Updated, Commit — dropped second
}

// minGrowWidth is the hard floor for the grow column: 0, not 1. At
// layout.Compute's own minListInterior floor (20 columns), the PR/issue
// spec's Index+State columns alone already consume the entire budget
// (6+8 content + 2*3 padding for all three surviving essential columns ==
// 20), leaving no room for Grow at all. Rather than force it up to some
// positive minimum and overflow the panel border by that amount, Grow is
// allowed to bottom out at 0 (bubbles/table skips rendering a col.Width<=0
// column entirely, so it simply disappears) — the same "keep #/Title/State"
// promise is honored in the row/column-name sense (a row builder can still
// ask for the "title" column's value), just not the visual one, in this
// one truly pathological corner.
const minGrowWidth = 0

// Fit lays out spec's six columns for mainWidth, accounting for the real
// per-column rendering overhead: bubbles/table's DefaultStyles pads every
// header/cell by 1 column on each side (Padding(0,1)), so each surviving
// column costs mainWidth-budget = its own Width + 2, not just its Width.
// When even Grow at its preferred Width doesn't fit alongside the other
// five, columns are dropped by priority — Fourth, then Updated, then Repo —
// until it does; Index/Grow/State always survive. Grow's actual width is
// whatever's left after the surviving fixed columns and their padding
// overhead: when that's at least the preferred Width, Grow absorbs all of
// it (unbounded — the same "let the title fill the rest" behavior the
// original per-section formulas had); otherwise it's floored at
// minGrowWidth rather than clamped up to the (now unaffordable) preferred
// Width — a too-narrow (or, at the hard floor, invisible) title beats a
// table header that overflows the panel border.
func (spec SixColumnSpec) Fit(mainWidth int) []ColumnDefinition {
	order := []ColumnDefinition{spec.Index, spec.Grow, spec.Repo, spec.Fourth, spec.State, spec.Updated}
	kept := make(map[string]bool, len(order))
	for _, def := range order {
		kept[def.Name] = true
	}
	dropOrder := []string{spec.Fourth.Name, spec.Updated.Name, spec.Repo.Name}

	fixedWidthExceptGrow := func() int {
		w := 0
		for _, def := range order {
			if def.Name == spec.Grow.Name || !kept[def.Name] {
				continue
			}
			w += def.Width
		}
		return w
	}
	countKept := func() int {
		n := 0
		for _, ok := range kept {
			if ok {
				n++
			}
		}
		return n
	}
	fitsWithPreferredGrow := func() bool {
		return fixedWidthExceptGrow()+spec.Grow.Width+2*countKept() <= mainWidth
	}

	for _, name := range dropOrder {
		if fitsWithPreferredGrow() {
			break
		}
		kept[name] = false
	}

	overhead := 2 * countKept()
	growW := mainWidth - fixedWidthExceptGrow() - overhead // unbounded: absorbs all remaining width when there's room
	if growW < minGrowWidth {
		growW = minGrowWidth
	}

	out := make([]ColumnDefinition, 0, len(order))
	for _, def := range order {
		if !kept[def.Name] {
			continue
		}
		if def.Name == spec.Grow.Name {
			def.Width = growW
		}
		out = append(out, def)
	}
	return out
}

// DefaultColumnDefinitions defines the shared column widths and title-grow
// formula for every PR/issue-like section (# / Title / Repo / Author / State /
// Updated), responsively dropping Author, then Updated, then Repo when
// mainWidth can't fit them all (see SixColumnSpec.Fit).
func DefaultColumnDefinitions(mainWidth int) []ColumnDefinition {
	return DefaultColumnSpec().Fit(mainWidth)
}

// DefaultColumnSpec is the PR/issue-like SixColumnSpec, exposed so sections
// that need the raw spec (rather than an already-fitted slice) can share it.
func DefaultColumnSpec() SixColumnSpec {
	return SixColumnSpec{
		Index:   ColumnDefinition{Name: "number", Title: "#", Width: 6},
		Grow:    ColumnDefinition{Name: "title", Title: "Title", Width: 20},
		Repo:    ColumnDefinition{Name: "repo", Title: "Repo", Width: 22},
		Fourth:  ColumnDefinition{Name: "author", Title: "Author", Width: 16},
		State:   ColumnDefinition{Name: "state", Title: "State", Width: 8},
		Updated: ColumnDefinition{Name: "updated", Title: "Updated", Width: 10},
	}
}

// DefaultColumns defines the shared table columns for PR/issue-like sections.
func DefaultColumns(mainWidth int) []table.Column {
	return columnsFromDefinitions(DefaultColumnDefinitions(mainWidth))
}

// HumanizeTime renders a coarse "just now / Xm / Xh / Xd ago" relative time,
// returning "" for the zero time.
func HumanizeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
