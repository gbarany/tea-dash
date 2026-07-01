// Package pullsection is the pull-requests dashboard section.
package pullsection

import (
	stdctx "context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

// SectionType is the routing type tag for pull-request sections.
const SectionType = "pr"

const fetchTimeout = 30 * time.Second

// Model is the pull-requests section.
type Model struct {
	section.BaseModel
	prs []data.PullRequest
}

// compile-time interface assertion
var _ section.Section = (*Model)(nil)

// SectionPullRequestsFetchedMsg is the fetch payload carried in TaskFinishedMsg.Msg.
type SectionPullRequestsFetchedMsg struct {
	Prs        []data.PullRequest
	TotalCount int
	TaskId     string
	Err        error
}

// NewModel builds a pull-requests section.
func NewModel(id int, ctx *appctx.ProgramContext, cfg config.SectionConfig) *Model {
	m := &Model{}
	m.BaseModel = section.NewBaseModel(section.NewOptions{
		Id:           id,
		Type:         SectionType,
		Ctx:          ctx,
		Config:       cfg,
		Columns:      columns(ctx.MainContentWidth),
		LoadingText:  "Loading pull requests…",
		EmptyText:    "No open pull requests authored by you.",
		EmptyHint:    "This board shows PRs you created across all repos on your Gitea instance.",
		SingularForm: "pull request",
		PluralForm:   "pull requests",
	})
	return m
}

// FetchRows fetches the current user's open PRs across all repos.
func (m *Model) FetchRows() tea.Cmd {
	taskId := fmt.Sprintf("fetch-pulls-%d-%d", m.GetId(), time.Now().UnixNano())
	m.SetLastFetchID(taskId)
	m.SetIsLoading(true)
	client := m.Ctx.Client
	id, sType := m.GetId(), m.GetType()
	start := m.Ctx.StartTask(appctx.Task{Id: taskId, StartText: "Loading pull requests…", State: appctx.TaskStart})
	f := m.Config.Filter.WithDefaults("pulls")
	fetch := func() tea.Msg {
		ctx, cancel := stdctx.WithTimeout(stdctx.Background(), fetchTimeout)
		defer cancel()
		prs, total, err := client.SearchPulls(ctx, f)
		return appctx.TaskFinishedMsg{
			SectionId: id, SectionType: sType, TaskId: taskId,
			Msg: SectionPullRequestsFetchedMsg{Prs: prs, TotalCount: total, TaskId: taskId, Err: err},
		}
	}
	return tea.Batch(start, m.Spinner.Tick, fetch)
}

// Update applies fetched rows (behind a stale-fetch guard), advances the
// spinner while loading, and otherwise delegates to the table (row nav).
func (m *Model) Update(msg tea.Msg) (section.Section, tea.Cmd) {
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
	case SectionPullRequestsFetchedMsg:
		if m.LastFetchID() != "" && m.LastFetchID() != msg.TaskId {
			return m, nil // stale/superseded fetch
		}
		m.SetIsLoading(false)
		if msg.Err != nil {
			m.SetError(msg.Err)
			return m, nil
		}
		m.prs = msg.Prs
		m.SetTotalCount(msg.TotalCount)
		m.SetError(nil)
		// SetRows clamps but does not reset the cursor, so a refresh keeps the
		// selected row (intentional; better UX than the pre-refactor reset-to-top).
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
	// Only forward navigation/other messages to the table when ready — matches the
	// pre-refactor gating, which ignored keys while loading or in the error state.
	if m.GetIsLoading() || m.GetError() != nil {
		return m, nil
	}
	var cmd tea.Cmd
	m.Table, cmd = m.Table.Update(msg)
	return m, cmd
}

// UpdateProgramContext resizes the table and refreshes columns for the new width.
func (m *Model) UpdateProgramContext(ctx *appctx.ProgramContext) {
	m.BaseModel.UpdateProgramContext(ctx)
	m.Columns = columns(ctx.MainContentWidth)
	m.Table.SetColumns(m.Columns)
}

// GetCurrRow returns the selected PR, or nil when there are no rows.
func (m *Model) GetCurrRow() data.RowData {
	if len(m.prs) == 0 {
		return nil
	}
	i := m.CurrRow()
	if i < 0 || i >= len(m.prs) {
		return nil
	}
	return m.prs[i]
}

// BuildRows maps the PRs into the table's 6-cell rows (unchanged formatting).
func (m *Model) BuildRows() []table.Row {
	rows := make([]table.Row, len(m.prs))
	for i, pr := range m.prs {
		author := ""
		if pr.Author != "" {
			author = "@" + pr.Author
		}
		rows[i] = table.Row{
			fmt.Sprintf("#%d", pr.Number),
			pr.Title,
			pr.RepoNameWithOwner,
			author,
			prState(pr),
			humanizeTime(pr.UpdatedAt),
		}
	}
	return rows
}

// columns reproduces the pre-refactor column widths and title-grow formula.
func columns(mainWidth int) []table.Column {
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

func prState(pr data.PullRequest) string {
	if pr.Draft {
		return "draft"
	}
	return pr.State
}

func humanizeTime(t time.Time) string {
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
