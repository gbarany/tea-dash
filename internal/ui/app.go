// Package ui contains the Bubble Tea models that make up the tea-dash TUI.
package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/teacli"
)

const loadTimeout = 30 * time.Second

type status int

const (
	statusLoading status = iota
	statusReady
	statusError
)

// pullItem pairs a pull request with the repo it came from.
type pullItem struct {
	repo string
	pr   teacli.PullRequest
}

// Messages emitted by background commands.
type (
	pullsLoadedMsg struct {
		items    []pullItem
		warnings []string
	}
	errMsg struct{ err error }
)

// Model is the root tea-dash model: a table of pull requests.
type Model struct {
	cfg    *config.Config
	client *teacli.Client
	keys   keyMap

	table    table.Model
	spinner  spinner.Model
	items    []pullItem
	warnings []string

	status  status
	loadErr error
	width   int
	height  int
}

// New builds the root model from configuration.
func New(cfg *config.Config) Model {
	return Model{
		cfg:     cfg,
		client:  &teacli.Client{Binary: teacli.DefaultBinary, Login: cfg.Login},
		keys:    defaultKeyMap(),
		spinner: spinner.New(spinner.WithStyle(spinnerStyle)),
		status:  statusLoading,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadPulls())
}

// loadPulls fetches open PRs for every configured repo concurrently. With no
// configured repos it falls back to the repository in $PWD via tea's
// {owner}/{repo} placeholder expansion.
func (m Model) loadPulls() tea.Cmd {
	cfg, client := m.cfg, m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		repos, err := cfg.ParsedRepos()
		if err != nil {
			return errMsg{err}
		}

		if len(repos) == 0 {
			prs, err := client.ListCurrentRepoPulls(ctx, "open")
			if err != nil {
				return errMsg{fmt.Errorf("no repos configured and no gitea repo in current directory: %w", err)}
			}
			return pullsLoadedMsg{items: toItems("(current)", prs)}
		}

		type result struct {
			items   []pullItem
			warning string
		}
		ch := make(chan result, len(repos))
		for _, r := range repos {
			go func(r config.Repo) {
				prs, err := client.ListRepoPulls(ctx, r.Owner, r.Name, "open")
				if err != nil {
					ch <- result{warning: fmt.Sprintf("%s: %v", r, err)}
					return
				}
				ch <- result{items: toItems(r.String(), prs)}
			}(r)
		}

		var items []pullItem
		var warnings []string
		for range repos {
			res := <-ch
			items = append(items, res.items...)
			if res.warning != "" {
				warnings = append(warnings, res.warning)
			}
		}
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].pr.UpdatedAt.After(items[j].pr.UpdatedAt)
		})
		return pullsLoadedMsg{items: items, warnings: warnings}
	}
}

func toItems(repo string, prs []teacli.PullRequest) []pullItem {
	items := make([]pullItem, len(prs))
	for i, pr := range prs {
		items[i] = pullItem{repo: repo, pr: pr}
	}
	return items
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
		m.warnings = msg.warnings
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
	url := m.items[idx].pr.HTMLURL
	return func() tea.Msg {
		_ = openURL(url)
		return nil
	}
}

func (m *Model) rebuildTable() {
	rows := make([]table.Row, len(m.items))
	for i, it := range m.items {
		author := ""
		if it.pr.Poster != nil {
			author = "@" + it.pr.Poster.Login
		}
		rows[i] = table.Row{
			fmt.Sprintf("#%d", it.pr.Number),
			it.pr.Title,
			it.repo,
			author,
			prState(it.pr),
			humanizeTime(it.pr.UpdatedAt),
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
	title := titleStyle.Render("tea-dash") + dimStyle.Render("  pull requests")

	var body string
	switch m.status {
	case statusLoading:
		body = fmt.Sprintf("\n  %s Loading pull requests…", m.spinner.View())
	case statusError:
		body = "\n" + errorStyle.Render("  Error: "+m.loadErr.Error()) + "\n\n" +
			dimStyle.Render("  Check that `tea` is installed and you have run `tea login add`.")
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
	parts := []string{fmt.Sprintf("%d pull requests", len(m.items))}
	if n := len(m.warnings); n > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("%d repo(s) failed to load", n)))
	}
	return dimStyle.Render(strings.Join(parts, " · "))
}

func (m Model) emptyState() string {
	if len(m.cfg.Repos) == 0 {
		path, _ := config.Path()
		return "  No open pull requests in the current repository.\n\n" +
			dimStyle.Render("  To watch specific repos, create "+path+" with:\n\n"+
				"    repos:\n      - owner/name\n")
	}
	return "  No open pull requests in the configured repositories."
}

func prState(pr teacli.PullRequest) string {
	switch {
	case pr.Merged:
		return "merged"
	case pr.Draft:
		return "draft"
	default:
		return pr.State
	}
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
