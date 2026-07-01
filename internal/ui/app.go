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
	"github.com/gbarany/tea-dash/internal/ui/actions"
	"github.com/gbarany/tea-dash/internal/ui/components/actionfeedback"
	"github.com/gbarany/tea-dash/internal/ui/components/actionprompt"
	"github.com/gbarany/tea-dash/internal/ui/components/issuesection"
	"github.com/gbarany/tea-dash/internal/ui/components/prview"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/components/sidebar"
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
	sidebar       sidebar.Model
	tasks         map[string]context.Task
	currSectionId int
	prs           []section.Section
	issues        []section.Section
	notice        string // transient status message (e.g. browser-open failure)

	actionDispatcher func(actions.Intent) tea.Cmd
	actionPrompt     actionprompt.Model
	pendingAction    actions.Intent
	actionFeedback   actionfeedback.Model

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
		ctx:            ctx,
		keys:           defaultKeyMap(),
		tabs:           tabs.New(ctx),
		sidebar:        sidebar.New(ctx),
		tasks:          tasks,
		actionPrompt:   actionprompt.New(),
		actionFeedback: actionfeedback.New(),
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

// SetActionDispatcher wires the frontend action-intent seam. The dispatcher is
// responsible for all backend or local-git side effects.
func (m *Model) SetActionDispatcher(dispatcher func(actions.Intent) tea.Cmd) {
	m.actionDispatcher = dispatcher
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
	if _, ok := msg.(tea.KeyPressMsg); ok && m.actionPrompt.Active() {
		return m, m.updateActionPrompt(msg)
	}
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

	case actions.ResultMsg:
		m.notice = ""
		m.actionFeedback = m.actionFeedback.Set(feedbackFromActionResult(msg))
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
		case key.Matches(msg, m.keys.Comment):
			return m, m.startAction(actions.KindComment)
		case key.Matches(msg, m.keys.Merge):
			return m, m.startAction(actions.KindMerge)
		case key.Matches(msg, m.keys.Close):
			return m, m.startAction(actions.KindClose)
		case key.Matches(msg, m.keys.Reopen):
			return m, m.startAction(actions.KindReopen)
		case key.Matches(msg, m.keys.Review):
			return m, m.startAction(actions.KindReview)
		case key.Matches(msg, m.keys.ExternalDiff):
			return m, m.startAction(actions.KindExternalDiff)
		case key.Matches(msg, m.keys.Checkout):
			return m, m.startAction(actions.KindCheckout)
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
	if m.ctx.View == context.IssuesView {
		subtitle = "  my issues"
	}
	title := titleStyle.Render("tea-dash") + m.ctx.Styles.DimText.Render(subtitle)

	parts := []string{title}
	if tv := m.tabs.View(); tv != "" {
		parts = append(parts, tv)
	}
	status := m.statusLine()
	if m.actionPrompt.Active() {
		status = m.actionPrompt.View(m.ctx.ScreenWidth - 4)
	} else if m.notice != "" {
		status = m.ctx.Styles.ErrorText.Render(m.notice)
	} else if !m.actionFeedback.Empty() {
		status = m.actionFeedback.View(m.ctx.ScreenWidth - 4)
	}
	body := ""
	if s := m.getCurrSection(); s != nil {
		body = s.View()
	}
	if m.ctx.PreviewOpen {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, m.sidebar.View())
	}
	parts = append(parts, body, status,
		helpStyle.Render("↑/↓ move · h/l section · s view · / search · p preview · c comment · m merge · x close · q quit"))

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

func (m *Model) startAction(kind actions.Kind) tea.Cmd {
	target, ok := m.selectedActionTarget()
	if !ok {
		m.notice = "Select a row before running an action."
		return nil
	}
	if actionRequiresPullRequest(kind) && target.RowKind != actions.RowKindPullRequest {
		m.notice = fmt.Sprintf("%s is only available for pull requests.", actionLabel(kind))
		return nil
	}
	intent := actions.Intent{
		Kind:   kind,
		Target: target,
		Prompt: actions.Prompt{Mode: promptModeForAction(kind)},
	}
	m.pendingAction = intent
	m.actionPrompt = m.actionPrompt.Focus(promptConfigForAction(kind, target))
	return nil
}

func (m *Model) updateActionPrompt(msg tea.Msg) tea.Cmd {
	var result actionprompt.Result
	var cmd tea.Cmd
	m.actionPrompt, result, cmd = m.actionPrompt.Update(msg)
	if result.Canceled {
		m.pendingAction = actions.Intent{}
		m.actionFeedback = m.actionFeedback.Set(actionfeedback.Cancel("Action cancelled."))
		return cmd
	}
	if !result.Submitted {
		return cmd
	}
	intent := m.pendingAction
	intent.Prompt.Value = result.Value
	intent.Prompt.Label = result.Label
	m.pendingAction = actions.Intent{}
	if m.actionDispatcher == nil {
		m.notice = "Action not wired yet."
		return cmd
	}
	m.actionFeedback = m.actionFeedback.Set(actionfeedback.Start(actionStartText(intent)))
	dispatchCmd := m.actionDispatcher(intent)
	if cmd == nil {
		return dispatchCmd
	}
	if dispatchCmd == nil {
		return cmd
	}
	return tea.Batch(cmd, dispatchCmd)
}

func (m Model) selectedActionTarget() (actions.Target, bool) {
	s := m.getCurrSection()
	row := m.getCurrRowData()
	if s == nil || row == nil {
		return actions.Target{}, false
	}
	rowKind := actions.RowKind(s.GetType())
	switch row.(type) {
	case data.PullRequest:
		rowKind = actions.RowKindPullRequest
	case data.Issue:
		rowKind = actions.RowKindIssue
	}
	return actions.Target{
		SectionID:   s.GetId(),
		SectionType: s.GetType(),
		RowKind:     rowKind,
		Repo:        row.GetRepoNameWithOwner(),
		Number:      row.GetNumber(),
		Title:       row.GetTitle(),
		URL:         row.GetURL(),
	}, true
}

func actionRequiresPullRequest(kind actions.Kind) bool {
	switch kind {
	case actions.KindMerge, actions.KindReview, actions.KindExternalDiff, actions.KindCheckout:
		return true
	default:
		return false
	}
}

func promptModeForAction(kind actions.Kind) actions.PromptMode {
	switch kind {
	case actions.KindComment:
		return actions.PromptText
	case actions.KindReview:
		return actions.PromptPicker
	default:
		return actions.PromptConfirm
	}
}

func promptConfigForAction(kind actions.Kind, target actions.Target) actionprompt.Config {
	title := fmt.Sprintf("%s #%d", actionLabel(kind), target.Number)
	message := fmt.Sprintf("%s - %s", target.Repo, target.Title)
	switch kind {
	case actions.KindComment:
		return actionprompt.Config{
			Mode:        actionprompt.ModeText,
			Title:       title,
			Message:     message,
			Placeholder: "Write a comment",
		}
	case actions.KindReview:
		return actionprompt.Config{
			Mode:    actionprompt.ModePicker,
			Title:   title,
			Message: message,
			Options: []actionprompt.Option{
				{Label: "Comment", Value: "comment"},
				{Label: "Approve", Value: "approve"},
				{Label: "Request changes", Value: "request_changes"},
			},
		}
	default:
		return actionprompt.Config{
			Mode:    actionprompt.ModeConfirm,
			Title:   title,
			Message: message,
		}
	}
}

func actionLabel(kind actions.Kind) string {
	switch kind {
	case actions.KindComment:
		return "Comment"
	case actions.KindMerge:
		return "Merge"
	case actions.KindClose:
		return "Close"
	case actions.KindReopen:
		return "Reopen"
	case actions.KindReview:
		return "Review"
	case actions.KindExternalDiff:
		return "External diff"
	case actions.KindCheckout:
		return "Checkout"
	default:
		return string(kind)
	}
}

func actionStartText(intent actions.Intent) string {
	return fmt.Sprintf("Starting %s for %s#%d.", actionLabel(intent.Kind), intent.Target.Repo, intent.Target.Number)
}

func feedbackFromActionResult(msg actions.ResultMsg) actionfeedback.Message {
	text := msg.Message
	if text == "" && msg.Err != nil {
		text = msg.Err.Error()
	}
	if text == "" {
		text = actionStartText(msg.Intent)
	}
	switch msg.Status {
	case actions.ResultSucceeded:
		return actionfeedback.Success(text)
	case actions.ResultErrored:
		return actionfeedback.Error(text)
	case actions.ResultCanceled:
		return actionfeedback.Cancel(text)
	default:
		return actionfeedback.Start(text)
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
