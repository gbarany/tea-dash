// Package section defines the Section contract every dashboard section
// satisfies, plus an embeddable BaseModel that owns the table/spinner and
// renders the loading/error/empty/table body.
package section

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
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
	FetchRows() tea.Cmd

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
}

// BaseModel provides the shared machinery concrete sections embed.
type BaseModel struct {
	id          int
	sectionType string
	totalCount  int
	numRows     int
	isLoading   bool
	lastFetchID string
	err         error
	loadingText string
	emptyText   string
	emptyHint   string

	Ctx     *context.ProgramContext
	Config  config.SectionConfig
	Table   table.Model
	Spinner spinner.Model
	Columns []table.Column
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
		id:          o.Id,
		sectionType: o.Type,
		Ctx:         o.Ctx,
		Config:      o.Config,
		Table:       t,
		Spinner:     sp,
		Columns:     o.Columns,
		isLoading:   true,
		loadingText: o.LoadingText,
		emptyText:   o.EmptyText,
		emptyHint:   o.EmptyHint,
	}
}

func (m *BaseModel) GetId() int               { return m.id }
func (m *BaseModel) GetType() string          { return m.sectionType }
func (m *BaseModel) GetTitle() string         { return m.Config.Title }
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

// SetRows updates the table rows and records the count for the empty-state check.
func (m *BaseModel) SetRows(rows []table.Row) {
	m.Table.SetRows(rows)
	m.numRows = len(rows)
}

// UpdateProgramContext recaches the context and resizes the table to the main
// content area. Concrete sections may override to also refresh columns.
func (m *BaseModel) UpdateProgramContext(ctx *context.ProgramContext) {
	m.Ctx = ctx
	m.Table.SetWidth(ctx.MainContentWidth)
	m.Table.SetHeight(ctx.MainContentHeight)
}

// View renders the section body, preserving the pre-refactor layout exactly.
func (m *BaseModel) View() string {
	st := m.Ctx.Styles
	switch {
	case m.isLoading:
		return "\n  " + m.Spinner.View() + " " + m.loadingText
	case m.err != nil:
		return "\n" + st.ErrorText.Render("  Error: "+m.err.Error()) + "\n\n" +
			st.DimText.Render("  Check your Gitea login (run `tea login add`) and network.")
	case m.numRows == 0:
		return "\n  " + m.emptyText + "\n\n" +
			st.DimText.Render("  "+m.emptyHint)
	default:
		return m.Table.View()
	}
}
