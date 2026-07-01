// Package ui contains the Bubble Tea models that make up the tea-dash TUI.
package ui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/components/tabs"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Model is the root tea-dash model: a set of sections rendered as tabs.
type Model struct {
	ctx           *context.ProgramContext
	keys          keyMap
	tabs          tabs.Model
	tasks         map[string]context.Task
	currSectionId int
	sections      []section.Section
	notice        string // transient status message (e.g. browser-open failure)
}

// openFailedMsg reports that opening a URL in the browser failed, so the UI can
// surface the error (and the URL, to copy) instead of failing silently.
type openFailedMsg struct {
	url string
	err error
}

// New builds the root model. client may be nil in tests (only FetchRows uses it).
func New(cfg *config.Config, client *gitea.Client) Model {
	tasks := map[string]context.Task{}
	user := ""
	if client != nil {
		user = client.Me()
	}
	ctx := &context.ProgramContext{
		Config: cfg,
		Client: client,
		User:   user,
		View:   context.PullsView,
		Styles: context.DefaultStyles(),
	}
	ctx.StartTask = func(t context.Task) tea.Cmd {
		tasks[t.Id] = t
		return nil
	}

	sections := []section.Section{
		pullsection.NewModel(0, ctx, ctx.GetViewSectionsConfig()[0]),
	}

	tb := tabs.New(ctx)
	tb.SetSections(toSectioners(sections))

	return Model{
		ctx:      ctx,
		keys:     defaultKeyMap(),
		tabs:     tb,
		tasks:    tasks,
		sections: sections,
	}
}

// Init starts the initial fetch for the current section.
func (m Model) Init() tea.Cmd {
	return m.getCurrSection().FetchRows()
}

// Update routes messages: layout, async results, keys, then generic fallthrough.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.ctx.ScreenWidth = msg.Width
		m.ctx.ScreenHeight = msg.Height
		m.syncMainContentDimensions()
		m.syncProgramContext()
		return m, nil

	case context.TaskFinishedMsg:
		delete(m.tasks, msg.TaskId)
		return m, m.updateSection(msg.SectionId, msg.SectionType, msg.Msg)

	case openFailedMsg:
		m.notice = fmt.Sprintf("Couldn't open browser: %v — copy: %s", msg.err, msg.url)
		return m, nil

	case tea.KeyPressMsg:
		m.notice = "" // any key dismisses a transient notice
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Refresh):
			if !m.getCurrSection().GetIsLoading() {
				return m, m.getCurrSection().FetchRows()
			}
			return m, nil
		case key.Matches(msg, m.keys.Open):
			return m, m.openSelected()
		}
	}

	return m, m.updateCurrentSection(msg)
}

// View composes the same shell as before: title, (tab bar), section body,
// status line, help.
func (m Model) View() tea.View {
	title := titleStyle.Render("tea-dash") + m.ctx.Styles.DimText.Render("  my pull requests")

	parts := []string{title}
	if tv := m.tabs.View(); tv != "" {
		parts = append(parts, tv)
	}
	status := m.statusLine()
	if m.notice != "" {
		status = m.ctx.Styles.ErrorText.Render(m.notice)
	}
	parts = append(parts, m.getCurrSection().View(), status,
		helpStyle.Render("↑/↓ move · r refresh · o/enter open in browser · q quit"))

	content := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return tea.View{Content: content, AltScreen: true}
}

func (m Model) getCurrSection() section.Section { return m.sections[m.currSectionId] }

func (m Model) getCurrRowData() data.RowData { return m.getCurrSection().GetCurrRow() }

func (m Model) openSelected() tea.Cmd {
	row := m.getCurrRowData()
	if row == nil {
		return nil
	}
	url := row.GetUrl()
	if url == "" {
		return nil
	}
	return func() tea.Msg {
		if err := openURL(url); err != nil {
			return openFailedMsg{url: url, err: err}
		}
		return nil
	}
}

func (m *Model) updateSection(id int, sType string, msg tea.Msg) tea.Cmd {
	for i, s := range m.sections {
		if s.GetId() == id && s.GetType() == sType {
			var cmd tea.Cmd
			m.sections[i], cmd = s.Update(msg)
			return cmd
		}
	}
	return nil
}

func (m *Model) updateCurrentSection(msg tea.Msg) tea.Cmd {
	s := m.getCurrSection()
	return m.updateSection(s.GetId(), s.GetType(), msg)
}

func (m *Model) syncProgramContext() {
	for _, s := range m.sections {
		s.UpdateProgramContext(m.ctx)
	}
	m.tabs.UpdateProgramContext(m.ctx)
}

func (m *Model) syncMainContentDimensions() {
	m.ctx.MainContentWidth = m.ctx.ScreenWidth - 4
	h := m.ctx.ScreenHeight - 6
	if h < 3 {
		h = 3
	}
	m.ctx.MainContentHeight = h
}

func (m Model) statusLine() string {
	s := m.getCurrSection()
	if s.GetIsLoading() || s.GetError() != nil {
		return ""
	}
	total := s.GetTotalCount()
	shown := s.NumRows()
	if total > shown {
		return m.ctx.Styles.DimText.Render(fmt.Sprintf("showing %d of %d pull requests", shown, total))
	}
	return m.ctx.Styles.DimText.Render(fmt.Sprintf("%d pull requests", total))
}

// toSectioners adapts sections to the tab bar's minimal interface.
func toSectioners(sections []section.Section) []tabs.Sectioner {
	out := make([]tabs.Sectioner, len(sections))
	for i, s := range sections {
		out[i] = s
	}
	return out
}
