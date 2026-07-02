// Package ui contains the Bubble Tea models that make up the tea-dash TUI.
package ui

import (
	stdctx "context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/issuesection"
	"github.com/gbarany/tea-dash/internal/ui/components/notificationsection"
	"github.com/gbarany/tea-dash/internal/ui/components/prview"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/components/sidebar"
	"github.com/gbarany/tea-dash/internal/ui/components/tabs"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Model is the root tea-dash model: a set of sections rendered as tabs. Each
// view (pulls, issues, notifications) owns its own section slice; inactive
// views stay nil until first switch (lazy build).
type Model struct {
	ctx           *context.ProgramContext
	keys          keyMap
	tabs          tabs.Model
	sidebar       sidebar.Model
	tasks         map[string]context.Task
	currSectionId int
	prs           []section.Section
	issues        []section.Section
	notifications []section.Section
	notice        string // transient status message (e.g. browser-open failure)

	// pullDetails and issueDetails memoize fetched detail views keyed by
	// "owner/repo#num". The map is chosen by the row's kind (PR vs issue), so
	// syncSidebar reads the right one without any runtime type assertion.
	pullDetails  map[string]*data.PullDetail
	issueDetails map[string]*data.IssueDetail
	// pullEnrichErr and issueEnrichErr record the last failed detail fetch per
	// key so the preview can show an error block (instead of a perpetual
	// "Loading…") while keeping the row retryable. They are kept separate, just
	// like the detail maps, because PRs and issues in the same repo can share a
	// number ("owner/repo#num").
	pullEnrichErr  map[string]error
	issueEnrichErr map[string]error
	// expanded controls whether the preview shows the full body or the folded
	// (read-more) form. Reset to false each time the preview is (re)opened.
	expanded bool
}

// openFailedMsg reports that opening a URL in the browser failed, so the UI can
// surface the error (and the URL, to copy) instead of failing silently.
type openFailedMsg struct {
	url string
	err error
}

// enrichedMsg carries the result of a lazy detail fetch back to the root, keyed
// by the "owner/repo#num" it was requested for so a stale result (the user moved
// on) is still cached under the right key rather than shown against the wrong row.
type enrichedMsg struct {
	key         string
	sectionType string
	pull        *data.PullDetail
	issue       *data.IssueDetail
	err         error
}

type notificationActionMsg struct {
	sectionID int
	all       bool
	err       error
}

// New builds the root model. client may be nil in tests (only FetchRows uses it).
func New(cfg *config.Config, client *gitea.Client) Model {
	tasks := map[string]context.Task{}
	user := ""
	if client != nil {
		user = client.Me()
	}
	view := context.PullsView
	if cfg != nil {
		switch cfg.Defaults.View {
		case "issues":
			view = context.IssuesView
		case "notifications":
			view = context.NotificationsView
		}
	}
	ctx := &context.ProgramContext{
		Config:      cfg,
		Client:      client,
		User:        user,
		View:        view,
		PreviewOpen: true,
		Styles:      context.DefaultStyles(),
	}
	ctx.StartTask = func(t context.Task) tea.Cmd {
		tasks[t.Id] = t
		return nil
	}

	m := Model{
		ctx:            ctx,
		keys:           defaultKeyMap(),
		tabs:           tabs.New(ctx),
		sidebar:        sidebar.New(ctx),
		tasks:          tasks,
		pullDetails:    map[string]*data.PullDetail{},
		issueDetails:   map[string]*data.IssueDetail{},
		pullEnrichErr:  map[string]error{},
		issueEnrichErr: map[string]error{},
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
		switch view {
		case context.IssuesView:
			sections[i] = issuesection.NewModel(i, ctx, cfg)
		case context.NotificationsView:
			sections[i] = notificationsection.NewModel(i, ctx, cfg)
		default:
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
		cmd := m.updateSection(msg.SectionId, msg.SectionType, msg.Msg)
		if m.ctx.PreviewOpen {
			m.syncSidebar()
			cmd = tea.Batch(cmd, m.enrichCurrRow())
		}
		return m, cmd

	case openFailedMsg:
		m.notice = fmt.Sprintf("Couldn't open browser: %v — copy: %s", msg.err, msg.url)
		return m, nil

	case enrichedMsg:
		// Cache the fetched detail (keyed by the row it was requested for) and
		// re-render the current preview, which picks up the new detail when the
		// selection still points at that row.
		if msg.err != nil {
			m.setEnrichErr(msg.sectionType, msg.key, msg.err)
		} else {
			if msg.pull != nil {
				delete(m.pullEnrichErr, msg.key)
				m.pullDetails[msg.key] = msg.pull
			}
			if msg.issue != nil {
				delete(m.issueEnrichErr, msg.key)
				m.issueDetails[msg.key] = msg.issue
			}
		}
		m.syncSidebar()
		return m, nil

	case notificationActionMsg:
		return m.handleNotificationAction(msg)

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
				if m.ctx.PreviewOpen {
					m.clearSelectedPreviewCache()
					m.syncSidebar()
				}
				return m, s.FetchRows()
			}
			return m, nil
		case key.Matches(msg, m.keys.Open):
			return m, m.openSelected()
		case key.Matches(msg, m.keys.MarkRead):
			return m.markSelectedNotificationRead()
		case key.Matches(msg, m.keys.MarkAllRead):
			return m.markAllNotificationsRead()
		case key.Matches(msg, m.keys.NextSection):
			if last := len(m.currentViewSections()) - 1; m.currSectionId < last {
				m.currSectionId++
			}
			m.tabs.SetCurrSectionId(m.currSectionId)
			if m.ctx.PreviewOpen {
				m.syncSidebar()
				return m, m.enrichCurrRow()
			}
			return m, nil
		case key.Matches(msg, m.keys.PrevSection):
			if m.currSectionId > 0 {
				m.currSectionId--
			}
			m.tabs.SetCurrSectionId(m.currSectionId)
			if m.ctx.PreviewOpen {
				m.syncSidebar()
				return m, m.enrichCurrRow()
			}
			return m, nil
		case key.Matches(msg, m.keys.SwitchView):
			cmd := m.switchView()
			return m, cmd
		case key.Matches(msg, m.keys.Search):
			if s := m.getCurrSection(); s != nil {
				return m, s.SetIsSearching(true)
			}
			return m, nil
		case key.Matches(msg, m.keys.TogglePreview):
			m.ctx.PreviewOpen = !m.ctx.PreviewOpen
			m.syncMainContentDimensions()
			m.syncProgramContext()
			if m.ctx.PreviewOpen {
				m.expanded = false
				m.syncSidebar()
				return m, m.enrichCurrRow()
			}
			return m, nil
		case key.Matches(msg, m.keys.Expand):
			if m.ctx.PreviewOpen {
				m.expanded = !m.expanded
				m.syncSidebar()
			}
			return m, nil
		case (key.Matches(msg, m.keys.ScrollUp) || key.Matches(msg, m.keys.ScrollDown)) && m.ctx.PreviewOpen:
			var cmd tea.Cmd
			m.sidebar, cmd = m.sidebar.Update(msg)
			return m, cmd
		}
	}

	// Fall-through: forward to the current section (row navigation, etc.). When
	// the preview is open, moving the cursor to a new row re-renders the pane and
	// lazily fetches that row's detail.
	before := m.selKey()
	cmd := m.updateCurrentSection(msg)
	if m.ctx.PreviewOpen && m.selKey() != before {
		m.syncSidebar()
		cmd = tea.Batch(cmd, m.enrichCurrRow())
	}
	return m, cmd
}

// View composes the same shell as before: title, (tab bar), section body,
// status line, help.
func (m Model) View() tea.View {
	subtitle := "  my pull requests"
	switch m.ctx.View {
	case context.IssuesView:
		subtitle = "  my issues"
	case context.NotificationsView:
		subtitle = "  notifications"
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
	if m.ctx.PreviewOpen {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, m.sidebar.View())
	}
	parts = append(parts, body, status,
		helpStyle.Render(m.helpText()))

	content := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return tea.View{Content: content, AltScreen: true}
}

// currentViewSections returns the section slice for the active view.
func (m *Model) currentViewSections() []section.Section {
	switch m.ctx.View {
	case context.NotificationsView:
		return m.notifications
	case context.IssuesView:
		return m.issues
	default:
		return m.prs
	}
}

// setCurrentViewSections stores s under the active view and rewires the tab bar.
func (m *Model) setCurrentViewSections(s []section.Section) {
	switch m.ctx.View {
	case context.NotificationsView:
		m.notifications = s
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

// selKey is the detail-cache/enrich key for the selected row ("owner/repo#num"),
// or "" when there is no selection or the repo is not a well-formed owner/name.
func (m Model) selKey() string {
	row := m.getCurrRowData()
	if row == nil {
		return ""
	}
	owner, name, ok := data.SplitOwnerRepo(row.GetRepoNameWithOwner())
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s/%s#%d", owner, name, row.GetNumber())
}

// enrichCurrRow lazily fetches the selected row's detail. It returns nil (no
// work) when there is no selection, the detail is already cached, or there is
// no client (tests); otherwise it returns a command that fetches in the
// background and reports back via enrichedMsg. Whether the row is a pull request
// or an issue is decided by the current section's type. A prior error is not a
// cached detail, so refresh/navigation can retry the same row.
func (m *Model) enrichCurrRow() tea.Cmd {
	key := m.selKey()
	if key == "" {
		return nil
	}
	if s := m.getCurrSection(); s != nil {
		switch s.GetType() {
		case pullsection.SectionType:
			if _, ok := m.pullDetails[key]; ok {
				return nil
			}
		case issuesection.SectionType:
			if _, ok := m.issueDetails[key]; ok {
				return nil
			}
		default:
			return nil
		}
	}
	row := m.getCurrRowData()
	owner, name, ok := data.SplitOwnerRepo(row.GetRepoNameWithOwner())
	if !ok {
		return nil
	}
	client := m.ctx.Client
	if client == nil {
		return nil
	}
	index := row.GetNumber()
	isPR := false
	if s := m.getCurrSection(); s != nil {
		isPR = s.GetType() == pullsection.SectionType
	}
	return func() tea.Msg {
		if isPR {
			d, err := client.GetPullDetail(owner, name, index)
			if err != nil {
				return enrichedMsg{key: key, sectionType: pullsection.SectionType, err: err}
			}
			return enrichedMsg{key: key, sectionType: pullsection.SectionType, pull: &d}
		}
		d, err := client.GetIssueDetail(owner, name, index)
		if err != nil {
			return enrichedMsg{key: key, sectionType: issuesection.SectionType, err: err}
		}
		return enrichedMsg{key: key, sectionType: issuesection.SectionType, issue: &d}
	}
}

// syncSidebar renders the current row (with its cached detail, if any) into the
// preview pane. The concrete row type selects the pull vs issue renderer; a
// missing/other-typed cache entry renders as the "loading" placeholder.
func (m *Model) syncSidebar() {
	row := m.getCurrRowData()
	if row == nil {
		m.sidebar.SetContent("")
		return
	}
	key := m.selKey()
	w := m.ctx.PreviewWidth
	var rendered string
	switch r := row.(type) {
	case data.PullRequest:
		if err := m.pullEnrichErr[key]; err != nil {
			m.sidebar.SetContent(m.failedPreview(row, err))
			return
		}
		rendered = prview.RenderPull(r, m.pullDetails[key], w, m.expanded)
	case data.Issue:
		if err := m.issueEnrichErr[key]; err != nil {
			m.sidebar.SetContent(m.failedPreview(row, err))
			return
		}
		rendered = prview.RenderIssue(r, m.issueDetails[key], w, m.expanded)
	case data.Notification:
		rendered = prview.RenderNotification(r, w)
	}
	m.sidebar.SetContent(rendered)
}

func (m *Model) setEnrichErr(sectionType, key string, err error) {
	switch sectionType {
	case pullsection.SectionType:
		m.pullEnrichErr[key] = err
	case issuesection.SectionType:
		m.issueEnrichErr[key] = err
	default:
		if s := m.getCurrSection(); s != nil {
			m.setEnrichErr(s.GetType(), key, err)
		}
	}
}

func (m *Model) clearSelectedPreviewCache() {
	key := m.selKey()
	if key == "" {
		return
	}
	if s := m.getCurrSection(); s != nil {
		switch s.GetType() {
		case pullsection.SectionType:
			delete(m.pullDetails, key)
			delete(m.pullEnrichErr, key)
		case issuesection.SectionType:
			delete(m.issueDetails, key)
			delete(m.issueEnrichErr, key)
		}
	}
}

// switchView cycles pulls -> issues -> notifications, lazily building and
// fetching the target view's sections on first visit.
func (m *Model) switchView() tea.Cmd {
	switch m.ctx.View {
	case context.PullsView:
		m.ctx.View = context.IssuesView
	case context.IssuesView:
		m.ctx.View = context.NotificationsView
	default:
		m.ctx.View = context.PullsView
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
	if m.ctx.PreviewOpen {
		m.syncSidebar()
		cmds = append(cmds, m.enrichCurrRow())
	}
	return tea.Batch(cmds...)
}

func (m Model) failedPreview(row data.RowData, err error) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("%s · #%d", row.GetRepoNameWithOwner(), row.GetNumber()),
		row.GetTitle(),
		"",
		m.ctx.Styles.ErrorText.Render("Failed to load preview."),
		fmt.Sprintf("%v", err),
		"",
		m.ctx.Styles.DimText.Render("Press r to retry."),
	)
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

func (m Model) markSelectedNotificationRead() (Model, tea.Cmd) {
	row, ok := m.getCurrRowData().(data.Notification)
	if !ok {
		m.notice = "Select a notification to mark read."
		return m, nil
	}
	if row.ID == 0 {
		m.notice = "Selected notification has no thread id."
		return m, nil
	}
	if m.ctx.Client == nil {
		m.notice = "No Gitea client available to mark notifications read."
		return m, nil
	}
	return m, markNotificationReadCmd(m.ctx.Client, m.currSectionId, row.ID)
}

func (m Model) markAllNotificationsRead() (Model, tea.Cmd) {
	if m.ctx.View != context.NotificationsView {
		m.notice = "Switch to notifications to mark all read."
		return m, nil
	}
	if m.ctx.Client == nil {
		m.notice = "No Gitea client available to mark notifications read."
		return m, nil
	}
	return m, markAllNotificationsReadCmd(m.ctx.Client, m.currSectionId)
}

func markNotificationReadCmd(client *gitea.Client, sectionID int, threadID int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 30*time.Second)
		defer cancel()
		return notificationActionMsg{
			sectionID: sectionID,
			err:       client.MarkNotificationRead(ctx, threadID),
		}
	}
}

func markAllNotificationsReadCmd(client *gitea.Client, sectionID int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 30*time.Second)
		defer cancel()
		return notificationActionMsg{
			sectionID: sectionID,
			all:       true,
			err:       client.MarkAllNotificationsRead(ctx),
		}
	}
}

func (m Model) handleNotificationAction(msg notificationActionMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		if msg.all {
			m.notice = fmt.Sprintf("Couldn't mark all notifications read: %v", msg.err)
		} else {
			m.notice = fmt.Sprintf("Couldn't mark notification read: %v", msg.err)
		}
		return m, nil
	}
	if msg.all {
		m.notice = "Marked all notifications read."
	} else {
		m.notice = "Marked notification read."
	}
	if msg.sectionID < 0 || msg.sectionID >= len(m.notifications) {
		return m, nil
	}
	return m, m.notifications[msg.sectionID].FetchRows()
}

func (m Model) helpText() string {
	text := "↑/↓ move · h/l section · s view · / search · p preview · e expand · r refresh · o open"
	if m.ctx.View == context.NotificationsView {
		text += " · m read · M all read"
	}
	return text + " · q quit"
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
	case notificationsection.SectionType:
		if id >= 0 && id < len(m.notifications) {
			m.notifications[id], cmd = m.notifications[id].Update(msg)
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
	for _, s := range m.notifications {
		s.UpdateProgramContext(m.ctx)
	}
	m.tabs.UpdateProgramContext(m.ctx)
	m.sidebar.UpdateProgramContext(m.ctx)
}

// syncMainContentDimensions splits the content area between the section table and
// the preview pane. When the preview is open the screen is divided in two (the
// preview capped at 80 columns, with a 2-column gutter between the panes); when
// it is closed the table gets the full width and the preview collapses to zero.
func (m *Model) syncMainContentDimensions() {
	h := m.ctx.ScreenHeight - 6
	if h < 3 {
		h = 3
	}
	m.ctx.MainContentHeight = h

	if m.ctx.PreviewOpen {
		pw := (m.ctx.ScreenWidth - 4) / 2
		if pw > 80 {
			pw = 80
		}
		if pw < 0 {
			pw = 0
		}
		m.ctx.PreviewWidth = pw
		mw := m.ctx.ScreenWidth - 4 - pw - 2
		if mw < 0 {
			mw = 0
		}
		m.ctx.MainContentWidth = mw
		m.ctx.PreviewHeight = m.ctx.MainContentHeight
		return
	}
	m.ctx.PreviewWidth = 0
	m.ctx.PreviewHeight = 0
	m.ctx.MainContentWidth = m.ctx.ScreenWidth - 4
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
