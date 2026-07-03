package section

import (
	stdctx "context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

const fetchTimeout = 30 * time.Second

// RowsFetchedMsg is the generic fetch payload carried in TaskFinishedMsg.Msg.
type RowsFetchedMsg[T data.RowData] struct {
	Rows       []T
	TotalCount int
	TaskId     string
	Page       int
	Append     bool
	Err        error
}

// Options parameterizes New, carrying the four per-type seams (fetch, row
// build, limit, filter kind) plus the section metadata forwarded to
// NewBaseModel.
type Options[T data.RowData] struct {
	Id     int
	Ctx    *appctx.ProgramContext
	Config config.SectionConfig
	// Type MUST correspond to the type parameter T: it drives app-level fetch
	// routing, so a Type/T mismatch would silently misroute fetch results.
	Type       string
	FilterKind string

	LoadingText  string
	EmptyText    string
	EmptyHint    string
	SingularForm string
	PluralForm   string

	Fetch    func(stdctx.Context, *gitea.Client, config.PrIssueFilter, int, int) ([]T, int, error)
	BuildRow func(T) table.Row
	Columns  func(int) []table.Column
	Limit    func(*config.Config) int
	Pageable bool
}

// Model is the generic dashboard section shared by the pull-request and issue
// views. It embeds BaseModel and holds the per-type seams.
type Model[T data.RowData] struct {
	BaseModel
	rows        []T
	fetch       func(stdctx.Context, *gitea.Client, config.PrIssueFilter, int, int) ([]T, int, error)
	buildRow    func(T) table.Row
	columns     func(int) []table.Column
	limitFn     func(*config.Config) int
	filterKind  string
	page        int
	loadingMore bool
	pageable    bool
}

// compile-time interface assertions
var (
	_ Section = (*Model[data.PullRequest])(nil)
	_ Section = (*Model[data.Issue])(nil)
)

// New builds a generic section from Options.
func New[T data.RowData](o Options[T]) *Model[T] {
	if o.Fetch == nil {
		panic("section.New: Options.Fetch is required")
	}
	if o.BuildRow == nil {
		panic("section.New: Options.BuildRow is required")
	}
	if o.Limit == nil {
		panic("section.New: Options.Limit is required")
	}
	columns := o.Columns
	if columns == nil {
		columns = DefaultColumns
	}
	m := &Model[T]{}
	m.BaseModel = NewBaseModel(NewOptions{
		Id:           o.Id,
		Type:         o.Type,
		Ctx:          o.Ctx,
		Config:       o.Config,
		Columns:      columns(o.Ctx.MainContentWidth),
		LoadingText:  o.LoadingText,
		EmptyText:    o.EmptyText,
		EmptyHint:    o.EmptyHint,
		SingularForm: o.SingularForm,
		PluralForm:   o.PluralForm,
	})
	m.fetch = o.Fetch
	m.buildRow = o.BuildRow
	m.columns = columns
	m.limitFn = o.Limit
	m.filterKind = o.FilterKind
	m.pageable = o.Pageable
	return m
}

// FetchRows fetches the current user's rows across all repos.
func (m *Model[T]) FetchRows() tea.Cmd {
	m.page = 0
	m.loadingMore = false
	return m.fetchPage(1, false)
}

func (m *Model[T]) MaybeFetchNextPage() tea.Cmd {
	if m.GetIsLoading() || m.GetError() != nil || m.loadingMore {
		return nil
	}
	if !m.pageable {
		return nil
	}
	if len(m.rows) == 0 || len(m.rows) >= m.GetTotalCount() {
		return nil
	}
	if m.CurrRow() < len(m.rows)-1 {
		return nil
	}
	nextPage := m.page + 1
	if nextPage <= 1 {
		nextPage = 2
	}
	return m.fetchPage(nextPage, true)
}

func (m *Model[T]) fetchPage(page int, appendRows bool) tea.Cmd {
	taskId := fmt.Sprintf("fetch-%s-%d-%d", m.GetType(), m.GetId(), time.Now().UnixNano())
	m.SetLastFetchID(taskId)
	if appendRows {
		m.loadingMore = true
	} else {
		m.SetIsLoading(true)
	}
	client := m.Ctx.Client
	id, sType := m.GetId(), m.GetType()
	var start tea.Cmd
	if m.Ctx.StartTask != nil {
		start = m.Ctx.StartTask(appctx.Task{Id: taskId, StartText: m.loadingText, State: appctx.TaskStart})
	}
	f := m.Config.Filter.WithDefaults(m.filterKind)
	limit := m.Config.Limit
	if limit == 0 && m.Ctx.Config != nil {
		limit = m.limitFn(m.Ctx.Config)
	}
	fetch := func() tea.Msg {
		ctx, cancel := stdctx.WithTimeout(stdctx.Background(), fetchTimeout)
		defer cancel()
		rows, total, err := m.fetch(ctx, client, f, limit, page)
		return appctx.TaskFinishedMsg{
			SectionId: id, SectionType: sType, TaskId: taskId,
			Msg: RowsFetchedMsg[T]{
				Rows: rows, TotalCount: total, TaskId: taskId,
				Page: page, Append: appendRows, Err: err,
			},
		}
	}
	return tea.Batch(start, m.Spinner.Tick, fetch)
}

// Update applies fetched rows (behind a stale-fetch guard), advances the
// spinner while loading, and otherwise delegates to the table (row nav).
func (m *Model[T]) Update(msg tea.Msg) (Section, tea.Cmd) {
	// Only divert key presses to the search bar while it is focused. Other
	// messages (fetch payloads, spinner ticks, etc.) must fall through to the
	// normal switch so they are applied even during an active search.
	if key, ok := msg.(tea.KeyPressMsg); ok && m.IsSearchFocused() {
		switch key.String() {
		case "enter":
			m.Config.Filter.Q = m.SearchBar.Value()
			m.SetIsSearching(false)
			return m, m.FetchRows()
		case "esc", "ctrl+c":
			m.SearchBar.SetValue(m.Config.Filter.Q) // revert to the applied keyword
			m.SetIsSearching(false)
			return m, nil
		}
		var cmd tea.Cmd
		m.SearchBar, cmd = m.SearchBar.Update(msg)
		return m, cmd
	}
	switch msg := msg.(type) {
	case RowsFetchedMsg[T]:
		if m.LastFetchID() != "" && m.LastFetchID() != msg.TaskId {
			return m, nil // stale/superseded fetch
		}
		if msg.Append {
			m.loadingMore = false
		} else {
			m.SetIsLoading(false)
		}
		if msg.Err != nil {
			m.SetError(msg.Err)
			return m, nil
		}
		if msg.Append {
			m.rows = append(m.rows, msg.Rows...)
		} else {
			m.rows = msg.Rows
		}
		if msg.Page > 0 {
			m.page = msg.Page
		} else if !msg.Append {
			m.page = 1
		}
		m.SetTotalCount(msg.TotalCount)
		m.SetError(nil)
		// SetRows clamps but does not reset the cursor, so a refresh keeps the
		// selected row (intentional: preserves the user's position rather than
		// jumping to the top).
		m.SetRows(m.BuildRows())
		return m, nil
	case spinner.TickMsg:
		if m.GetIsLoading() {
			var cmd tea.Cmd
			m.Spinner, cmd = m.Spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	// Only forward navigation/other messages to the table when ready: keys are
	// ignored while loading or in the error state.
	if m.GetIsLoading() || m.GetError() != nil {
		return m, nil
	}
	var cmd tea.Cmd
	m.Table, cmd = m.Table.Update(msg)
	return m, tea.Batch(cmd, m.MaybeFetchNextPage())
}

// UpdateProgramContext resizes the table and refreshes columns for the new width.
func (m *Model[T]) UpdateProgramContext(ctx *appctx.ProgramContext) {
	m.BaseModel.UpdateProgramContext(ctx)
	m.Columns = m.columns(ctx.MainContentWidth)
	m.Table.SetColumns(m.Columns)
}

// GetCurrRow returns the selected row, or nil when there are no rows.
func (m *Model[T]) GetCurrRow() data.RowData {
	if len(m.rows) == 0 {
		return nil
	}
	i := m.CurrRow()
	if i < 0 || i >= len(m.rows) {
		return nil
	}
	return m.rows[i]
}

// BuildRows maps the fetched rows into the table's rows via the per-type BuildRow.
func (m *Model[T]) BuildRows() []table.Row {
	rows := make([]table.Row, len(m.rows))
	for i, r := range m.rows {
		rows[i] = m.buildRow(r)
	}
	return rows
}
