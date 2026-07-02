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

// DefaultColumns defines the shared column widths and title-grow formula for
// every section (# / Title / Repo / Author / State / Updated).
func DefaultColumns(mainWidth int) []table.Column {
	const (
		numW     = 6
		repoW    = 22
		authorW  = 16
		stateW   = 8
		updatedW = 10
	)
	titleW := mainWidth - (numW + repoW + authorW + stateW + updatedW) - 6
	if titleW < 20 {
		titleW = 20
	}
	return []table.Column{
		{Title: "#", Width: numW},
		{Title: "Title", Width: titleW},
		{Title: "Repo", Width: repoW},
		{Title: "Author", Width: authorW},
		{Title: "State", Width: stateW},
		{Title: "Updated", Width: updatedW},
	}
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
