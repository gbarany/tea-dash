// Package ui contains the Bubble Tea models that make up the tea-dash TUI.
package ui

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
)

const loadTimeout = 30 * time.Second

type status int

const (
	statusLoading status = iota
	statusReady
	statusError
)

// Messages emitted by background commands.
type (
	pullsLoadedMsg struct{ items []data.PullRequest }
	errMsg         struct{ err error }
)

// Model is the root tea-dash model: a table of the current user's pull requests.
type Model struct {
	cfg    *config.Config
	client *gitea.Client
	keys   keyMap

	table   table.Model
	spinner spinner.Model
	items   []data.PullRequest

	status  status
	loadErr error
	width   int
	height  int
}

// New builds the root model. client may be nil in tests that drive Update
// directly (loadPulls is the only consumer of the client).
func New(cfg *config.Config, client *gitea.Client) Model {
	return Model{
		cfg:     cfg,
		client:  client,
		keys:    defaultKeyMap(),
		spinner: spinner.New(spinner.WithStyle(spinnerStyle)),
		status:  statusLoading,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadPulls())
}

// loadPulls fetches the authenticated user's open pull requests across all
// accessible repositories via the me-scoped search endpoint.
func (m Model) loadPulls() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		prs, err := client.SearchMyPulls(ctx, "open")
		if err != nil {
			return errMsg{err}
		}
		return pullsLoadedMsg{items: prs}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case spinner.TickMsg:
		if m.status == statusLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case pullsLoadedMsg:
		m.items = msg.items
		m.status = statusReady
		m.rebuildTable()
		return m, nil

	case errMsg:
		m.status = statusError
		m.loadErr = msg.err
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Refresh):
			if m.status != statusLoading {
				m.status = statusLoading
				return m, tea.Batch(m.spinner.Tick, m.loadPulls())
			}
			return m, nil
		case key.Matches(msg, m.keys.Open):
			return m, m.openSelected()
		}
	}

	if m.status == statusReady {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) openSelected() tea.Cmd {
	if m.status != statusReady || len(m.items) == 0 {
		return nil
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.items) {
		return nil
	}
	url := m.items[idx].HTMLURL
	return func() tea.Msg {
		_ = openURL(url)
		return nil
	}
}

func (m *Model) rebuildTable() {
	rows := make([]table.Row, len(m.items))
	for i, pr := range m.items {
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
	t := table.New(
		table.WithColumns(m.columns()),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	t.SetStyles(tableStyles())
	m.table = t
	m.layout()
}

// layout resizes the table to the current terminal dimensions.
func (m *Model) layout() {
	if m.status != statusReady || m.width == 0 || m.height == 0 {
		return
	}
	tableHeight := m.height - 6 // title, blank, status, help + padding
	if tableHeight < 3 {
		tableHeight = 3
	}
	m.table.SetHeight(tableHeight)
	m.table.SetWidth(m.width - 4)
	m.table.SetColumns(m.columns())
}

func (m Model) columns() []table.Column {
	const (
		numW     = 6
		repoW    = 22
		authorW  = 16
		stateW   = 8
		updatedW = 10
	)
	total := m.width - 4
	titleW := total - (numW + repoW + authorW + stateW + updatedW) - 6
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

// View implements tea.Model.
func (m Model) View() tea.View {
	title := titleStyle.Render("tea-dash") + dimStyle.Render("  my pull requests")

	var body string
	switch m.status {
	case statusLoading:
		body = fmt.Sprintf("\n  %s Loading pull requests…", m.spinner.View())
	case statusError:
		body = "\n" + errorStyle.Render("  Error: "+m.loadErr.Error()) + "\n\n" +
			dimStyle.Render("  Check your Gitea login (run `tea login add`) and network.")
	case statusReady:
		if len(m.items) == 0 {
			body = "\n" + m.emptyState()
		} else {
			body = m.table.View()
		}
	}

	help := helpStyle.Render("↑/↓ move · r refresh · o/enter open in browser · q quit")

	content := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, body, m.statusLine(), help))
	return tea.View{Content: content, AltScreen: true}
}

func (m Model) statusLine() string {
	if m.status != statusReady {
		return ""
	}
	return dimStyle.Render(fmt.Sprintf("%d pull requests", len(m.items)))
}

func (m Model) emptyState() string {
	return "  No open pull requests authored by you.\n\n" +
		dimStyle.Render("  This board shows PRs you created across all repos on your Gitea instance.")
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
