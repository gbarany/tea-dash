// Package ui contains the Bubble Tea models that make up the tea-dash TUI.
package ui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/issuesection"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/components/tabs"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Model is the root tea-dash model: a set of sections rendered as tabs. Each
// view (pulls, issues) owns its own section slice; the inactive view's slice is
// nil until the first switch (lazy build).
type Model struct {
	ctx           *context.ProgramContext
	keys          keyMap
	tabs          tabs.Model
	tasks         map[string]context.Task
	currSectionId int
	prs           []section.Section
	issues        []section.Section
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
	view := context.PullsView
	if cfg != nil && cfg.Defaults.View == "issues" {
		view = context.IssuesView
	}
	ctx := &context.ProgramContext{
		Config: cfg,
		Client: client,
		User:   user,
		View:   view,
		Styles: context.DefaultStyles(),
	}
	ctx.StartTask = func(t context.Task) tea.Cmd {
		tasks[t.Id] = t
		return nil
	}

	m := Model{
		ctx:   ctx,
		keys:  defaultKeyMap(),
		tabs:  tabs.New(ctx),
		tasks: tasks,
	}
	// Build only the starting view's sections; the other slice stays nil until
	// the first switch (lazy build). setCurrentViewSections wires the tab bar.
	m.setCurrentViewSections(buildSections(view, ctx))
	return m
}

// buildSections constructs the section models for a view from its config list.
func buildSections(view context.ViewType, ctx *context.ProgramContext) []section.Section {
	cfgs := ctx.GetViewSectionsConfig()
	sections := make([]section.Section, len(cfgs))
	for i, cfg := range cfgs {
		if view == context.IssuesView {
			sections[i] = issuesection.NewModel(i, ctx, cfg)
		} else {
			sections[i] = pullsection.NewModel(i, ctx, cfg)
		}
	}
	return sections
}

// Init starts the initial fetch for every section in the current view.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, s := range m.currentViewSections() {
		cmds = append(cmds, s.FetchRows())
	}
	return tea.Batch(cmds...)
}

// Update routes messages: layout, async results, keys, then generic fallthrough.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While the current section's search bar is focused, route key presses
	// straight to it so typing isn't eaten by nav/quit keys. Resize and async
	// messages still flow through the normal switch below.
	if _, ok := msg.(tea.KeyPressMsg); ok {
		if s := m.getCurrSection(); s != nil && s.IsSearchFocused() {
			return m, m.updateCurrentSection(msg)
		}
	}
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

	case spinner.TickMsg:
		// A tick belongs to whichever section started the spinner, but sibling
		// sections in the current view share one animation; route it to every one
		// so concurrent loaders stay animated (each ignores it once done loading).
		var cmds []tea.Cmd
		for _, s := range m.currentViewSections() {
			if cmd := m.updateSection(s.GetId(), s.GetType(), msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		m.notice = "" // any key dismisses a transient notice
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Refresh):
			if s := m.getCurrSection(); s != nil && !s.GetIsLoading() {
				return m, s.FetchRows()
			}
			return m, nil
		case key.Matches(msg, m.keys.Open):
			return m, m.openSelected()
		case key.Matches(msg, m.keys.NextSection):
			if last := len(m.currentViewSections()) - 1; m.currSectionId < last {
				m.currSectionId++
			}
			m.tabs.SetCurrSectionId(m.currSectionId)
			return m, nil
		case key.Matches(msg, m.keys.PrevSection):
			if m.currSectionId > 0 {
				m.currSectionId--
			}
			m.tabs.SetCurrSectionId(m.currSectionId)
			return m, nil
		case key.Matches(msg, m.keys.SwitchView):
			cmd := m.switchView()
			return m, cmd
		case key.Matches(msg, m.keys.Search):
			if s := m.getCurrSection(); s != nil {
				return m, s.SetIsSearching(true)
			}
			return m, nil
		}
	}

	return m, m.updateCurrentSection(msg)
}

// View composes the same shell as before: title, (tab bar), section body,
// status line, help.
func (m Model) View() tea.View {
	subtitle := "  my pull requests"
	if m.ctx.View == context.IssuesView {
		subtitle = "  my issues"
	}
	title := titleStyle.Render("tea-dash") + m.ctx.Styles.DimText.Render(subtitle)

	parts := []string{title}
	if tv := m.tabs.View(); tv != "" {
		parts = append(parts, tv)
	}
	status := m.statusLine()
	if m.notice != "" {
		status = m.ctx.Styles.ErrorText.Render(m.notice)
	}
	body := ""
	if s := m.getCurrSection(); s != nil {
		body = s.View()
	}
	parts = append(parts, body, status,
		helpStyle.Render("↑/↓ move · h/l section · s view · / search · r refresh · o/enter open · q quit"))

	content := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return tea.View{Content: content, AltScreen: true}
}

// currentViewSections returns the section slice for the active view.
func (m *Model) currentViewSections() []section.Section {
	switch m.ctx.View {
	case context.IssuesView:
		return m.issues
	default:
		return m.prs
	}
}

// setCurrentViewSections stores s under the active view and rewires the tab bar.
func (m *Model) setCurrentViewSections(s []section.Section) {
	switch m.ctx.View {
	case context.IssuesView:
		m.issues = s
	default:
		m.prs = s
	}
	m.tabs.SetSections(toSectioners(s))
}

// getCurrSection returns the active section, or nil when the view has none or
// the cursor is out of range.
func (m Model) getCurrSection() section.Section {
	s := m.currentViewSections()
	if m.currSectionId < 0 || m.currSectionId >= len(s) {
		return nil
	}
	return s[m.currSectionId]
}

func (m Model) getCurrRowData() data.RowData {
	s := m.getCurrSection()
	if s == nil {
		return nil
	}
	return s.GetCurrRow()
}

// switchView toggles between the pulls and issues views, lazily building and
// fetching the target view's sections on first visit.
func (m *Model) switchView() tea.Cmd {
	if m.ctx.View == context.IssuesView {
		m.ctx.View = context.PullsView
	} else {
		m.ctx.View = context.IssuesView
	}
	m.syncMainContentDimensions()
	var cmds []tea.Cmd
	s := m.currentViewSections()
	if len(s) == 0 {
		s = buildSections(m.ctx.View, m.ctx)
		for _, sec := range s {
			cmds = append(cmds, sec.FetchRows())
		}
	}
	m.setCurrentViewSections(s)
	m.currSectionId = 0
	m.tabs.SetCurrSectionId(0)
	m.syncProgramContext()
	return tea.Batch(cmds...)
}

func (m Model) openSelected() tea.Cmd {
	row := m.getCurrRowData()
	if row == nil {
		return nil
	}
	url := row.GetURL()
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
	var cmd tea.Cmd
	switch sType {
	case pullsection.SectionType:
		if id >= 0 && id < len(m.prs) {
			m.prs[id], cmd = m.prs[id].Update(msg)
		}
	case issuesection.SectionType:
		if id >= 0 && id < len(m.issues) {
			m.issues[id], cmd = m.issues[id].Update(msg)
		}
	}
	return cmd
}

func (m *Model) updateCurrentSection(msg tea.Msg) tea.Cmd {
	s := m.getCurrSection()
	if s == nil {
		return nil
	}
	return m.updateSection(s.GetId(), s.GetType(), msg)
}

func (m *Model) syncProgramContext() {
	for _, s := range m.prs {
		s.UpdateProgramContext(m.ctx)
	}
	for _, s := range m.issues {
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
	if s == nil || s.GetIsLoading() || s.GetError() != nil {
		return ""
	}
	total := s.GetTotalCount()
	shown := s.NumRows()
	if total > shown {
		return m.ctx.Styles.DimText.Render(fmt.Sprintf("showing %d of %d %s", shown, total, s.GetItemPlural()))
	}
	if total == 1 {
		return m.ctx.Styles.DimText.Render(fmt.Sprintf("1 %s", s.GetItemSingular()))
	}
	return m.ctx.Styles.DimText.Render(fmt.Sprintf("%d %s", total, s.GetItemPlural()))
}

// toSectioners adapts sections to the tab bar's minimal interface.
func toSectioners(sections []section.Section) []tabs.Sectioner {
	out := make([]tabs.Sectioner, len(sections))
	for i, s := range sections {
		out[i] = s
	}
	return out
}
