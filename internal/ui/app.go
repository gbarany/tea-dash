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
	localgit "github.com/gbarany/tea-dash/internal/git"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/actions"
	"github.com/gbarany/tea-dash/internal/ui/components/actionfeedback"
	"github.com/gbarany/tea-dash/internal/ui/components/actionprompt"
	"github.com/gbarany/tea-dash/internal/ui/components/actionsection"
	"github.com/gbarany/tea-dash/internal/ui/components/branchsection"
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
	actions       []section.Section
	branches      []section.Section
	notice        string // transient status message (e.g. browser-open failure)

	actionDispatcher func(actions.Intent) tea.Cmd
	actionPrompt     actionprompt.Model
	pendingAction    actions.Intent
	actionFeedback   actionfeedback.Model
	copyToClipboard  func(string) error
	showHelp         bool

	// Detail maps memoize fetched preview detail keyed by "owner/repo#num".
	// syncSidebar reads the map matching the selected row kind.
	pullDetails   map[string]*data.PullDetail
	issueDetails  map[string]*data.IssueDetail
	actionDetails map[string]*data.ActionRunDetail
	// Enrich error maps record the last failed detail fetch per key so the
	// preview can show an error block while keeping the row retryable. They are
	// kept separate, just like the detail maps, because row kinds can share a
	// number in the same repo ("owner/repo#num").
	pullEnrichErr   map[string]error
	issueEnrichErr  map[string]error
	actionEnrichErr map[string]error
	// expanded controls whether the preview shows the full body or the folded
	// (read-more) form. Reset to false each time the preview is (re)opened.
	expanded bool
}

type actionButton struct {
	Label   string
	Builtin string
}

// openFailedMsg reports that opening a URL in the browser failed, so the UI can
// surface the error (and the URL, to copy) instead of failing silently.
type openFailedMsg struct {
	url string
	err error
}

type copyResultMsg struct {
	value string
	err   error
}

// autoRefreshMsg is emitted by the optional background refetch timer.
type autoRefreshMsg struct{}

// enrichedMsg carries the result of a lazy detail fetch back to the root, keyed
// by the "owner/repo#num" it was requested for so a stale result (the user moved
// on) is still cached under the right key rather than shown against the wrong row.
type enrichedMsg struct {
	key         string
	sectionType string
	pull        *data.PullDetail
	issue       *data.IssueDetail
	action      *data.ActionRunDetail
	err         error
}

type notificationActionMsg struct {
	sectionID int
	all       bool
	unread    bool
	pinned    bool
	unpinned  bool
	err       error
}

// Options are runtime-only model settings resolved before the TUI starts.
type Options struct {
	CurrentRepo    string
	SmartFiltering bool
}

type watchChecksMsg struct {
	row    data.PullRequest
	detail data.PullDetail
	err    error
}

// New builds the root model. client may be nil in tests (only FetchRows uses it).
func New(cfg *config.Config, client *gitea.Client) Model {
	return NewWithOptions(cfg, client, Options{SmartFiltering: cfg.SmartFilteringEnabled()})
}

// NewWithOptions builds the root model with runtime-only options such as the
// current git repository detected from cwd.
func NewWithOptions(cfg *config.Config, client *gitea.Client, opts Options) Model {
	tasks := map[string]context.Task{}
	user := ""
	if client != nil {
		user = client.Me()
	}
	view := context.PullsView
	previewOpen := true
	if cfg != nil {
		switch cfg.Defaults.View {
		case "issues":
			view = context.IssuesView
		case "notifications":
			view = context.NotificationsView
		case "actions":
			view = context.ActionsView
		case "branches":
			view = context.BranchesView
		}
		previewOpen = cfg.Defaults.Preview.PreviewOpen()
	}
	ctx := &context.ProgramContext{
		Config:         cfg,
		Client:         client,
		User:           user,
		View:           view,
		PreviewOpen:    previewOpen,
		Styles:         context.StylesForConfig(cfg),
		CurrentRepo:    opts.CurrentRepo,
		SmartFiltering: opts.SmartFiltering,
	}
	ctx.StartTask = func(t context.Task) tea.Cmd {
		tasks[t.Id] = t
		return nil
	}

	keys := defaultKeyMap()
	keys.applyConfig(cfg)
	m := Model{
		ctx:             ctx,
		keys:            keys,
		tabs:            tabs.New(ctx),
		sidebar:         sidebar.New(ctx),
		tasks:           tasks,
		actionPrompt:    actionprompt.New(),
		actionFeedback:  actionfeedback.New(),
		copyToClipboard: writeClipboard,
		pullDetails:     map[string]*data.PullDetail{},
		issueDetails:    map[string]*data.IssueDetail{},
		pullEnrichErr:   map[string]error{},
		issueEnrichErr:  map[string]error{},
		actionDetails:   map[string]*data.ActionRunDetail{},
		actionEnrichErr: map[string]error{},
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
		switch view {
		case context.IssuesView:
			sections[i] = issuesection.NewModel(i, ctx, cfg)
		case context.NotificationsView:
			sections[i] = notificationsection.NewModel(i, ctx, cfg)
		case context.ActionsView:
			sections[i] = actionsection.NewModel(i, ctx, cfg)
		case context.BranchesView:
			sections[i] = branchsection.NewModel(i, ctx, cfg)
		default:
			sections[i] = pullsection.NewModel(i, ctx, cfg)
		}
	}
	return sections
}

func (m Model) matchCustomKeybinding(msg tea.KeyPressMsg) (config.Keybinding, bool) {
	if m.ctx == nil || m.ctx.Config == nil {
		return config.Keybinding{}, false
	}
	for _, b := range m.activeKeybindings() {
		if b.Command == "" {
			continue
		}
		if key.Matches(msg, key.NewBinding(key.WithKeys(b.Key))) {
			return b, true
		}
	}
	return config.Keybinding{}, false
}

func (m Model) matchBuiltinKeybinding(msg tea.KeyPressMsg) (config.Keybinding, bool) {
	if m.ctx == nil || m.ctx.Config == nil {
		return config.Keybinding{}, false
	}
	for _, b := range m.activeKeybindings() {
		if b.Builtin == "" {
			continue
		}
		if key.Matches(msg, key.NewBinding(key.WithKeys(b.Key))) {
			return b, true
		}
	}
	return config.Keybinding{}, false
}

func (m Model) activeKeybindings() []config.Keybinding {
	if m.ctx == nil || m.ctx.Config == nil {
		return nil
	}
	k := m.ctx.Config.Keybindings
	out := make([]config.Keybinding, 0, len(k.Universal)+4)
	out = append(out, k.Universal...)
	switch m.ctx.View {
	case context.IssuesView:
		out = append(out, k.Issues...)
	case context.NotificationsView:
		out = append(out, k.Notifications...)
	case context.ActionsView:
		out = append(out, k.Actions...)
	case context.BranchesView:
		out = append(out, k.Branches...)
	default:
		out = append(out, k.PRs...)
	}
	return out
}

func (m Model) scopedBuiltinOverridden(name string) bool {
	want := normalizeBuiltin(name)
	for _, b := range m.scopedKeybindings() {
		if b.Builtin != "" && normalizeBuiltin(b.Builtin) == want {
			return true
		}
	}
	return false
}

func (m Model) scopedKeybindings() []config.Keybinding {
	if m.ctx == nil || m.ctx.Config == nil {
		return nil
	}
	k := m.ctx.Config.Keybindings
	switch m.ctx.View {
	case context.IssuesView:
		return k.Issues
	case context.NotificationsView:
		return k.Notifications
	case context.ActionsView:
		return k.Actions
	case context.BranchesView:
		return k.Branches
	default:
		return k.PRs
	}
}

// Init starts the initial fetch for every section in the current view.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, s := range m.currentViewSections() {
		cmds = append(cmds, s.FetchRows())
	}
	cmds = append(cmds, m.autoRefreshCmd())
	return tea.Batch(cmds...)
}

func (m Model) autoRefreshInterval() time.Duration {
	if m.ctx == nil || m.ctx.Config == nil {
		return 0
	}
	return m.ctx.Config.Defaults.RefetchInterval()
}

func (m Model) autoRefreshCmd() tea.Cmd {
	interval := m.autoRefreshInterval()
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return autoRefreshMsg{}
	})
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

	case copyResultMsg:
		if msg.err != nil {
			m.notice = fmt.Sprintf("Couldn't copy: %v", msg.err)
		} else {
			m.notice = fmt.Sprintf("Copied %s.", msg.value)
		}
		return m, nil

	case actions.ResultMsg:
		m.notice = ""
		m.actionFeedback = m.actionFeedback.Set(feedbackFromActionResult(msg))
		if msg.Status == actions.ResultSucceeded {
			m.clearPreviewCacheForAction(msg.Intent.Target)
			if s := m.getCurrSection(); s != nil &&
				s.GetId() == msg.Intent.Target.SectionID &&
				s.GetType() == msg.Intent.Target.SectionType {
				if m.ctx.PreviewOpen {
					m.syncSidebar()
				}
				return m, tea.Batch(s.FetchRows(), m.enrichCurrRow())
			}
		}
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
			if msg.action != nil {
				delete(m.actionEnrichErr, msg.key)
				m.actionDetails[msg.key] = msg.action
			}
		}
		m.syncSidebar()
		return m, nil

	case notificationActionMsg:
		return m.handleNotificationAction(msg)

	case autoRefreshMsg:
		return m.handleAutoRefresh()

	case watchChecksMsg:
		if msg.err != nil {
			m.notice = fmt.Sprintf("Couldn't load PR checks: %v", msg.err)
			return m, nil
		}
		return m.switchToPullChecks(msg.row, msg.detail.HeadRef, msg.detail.HeadSHA)

	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)

	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)

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
		if b, ok := m.matchCustomKeybinding(msg); ok {
			return m, m.startCustomCommand(b)
		}
		if b, ok := m.matchBuiltinKeybinding(msg); ok {
			if next, cmd, handled := m.handleBuiltinKeybinding(b); handled {
				return next, cmd
			}
		}
		switch {
		case !m.scopedBuiltinOverridden("markRead") && m.ctx.View == context.NotificationsView && key.Matches(msg, m.keys.MarkRead):
			return m.markSelectedNotificationRead()
		case !m.scopedBuiltinOverridden("markUnread") && m.ctx.View == context.NotificationsView && key.Matches(msg, m.keys.MarkUnread):
			return m.markSelectedNotificationUnread()
		case !m.scopedBuiltinOverridden("markAllRead") && m.ctx.View == context.NotificationsView && key.Matches(msg, m.keys.MarkAllRead):
			return m.markAllNotificationsRead()
		case !m.scopedBuiltinOverridden("togglePin") && m.ctx.View == context.NotificationsView && key.Matches(msg, m.keys.Pin):
			return m.toggleSelectedNotificationPin()
		case !m.scopedBuiltinOverridden("unpin") && m.ctx.View == context.NotificationsView && key.Matches(msg, m.keys.Unpin):
			return m.unpinSelectedNotification()
		case !m.scopedBuiltinOverridden("rerun") && m.ctx.View == context.ActionsView && key.Matches(msg, m.keys.RerunRun):
			return m, m.startAction(actions.KindRerunRun)
		case !m.scopedBuiltinOverridden("cancel") && m.ctx.View == context.ActionsView && key.Matches(msg, m.keys.CancelRun):
			return m, m.startAction(actions.KindCancelRun)
		case !m.scopedBuiltinOverridden("comment") && key.Matches(msg, m.keys.Comment):
			return m, m.startAction(actions.KindComment)
		case !m.scopedBuiltinOverridden("assign") && key.Matches(msg, m.keys.Assign):
			return m, m.startAction(actions.KindAssign)
		case !m.scopedBuiltinOverridden("unassign") && key.Matches(msg, m.keys.Unassign):
			return m, m.startAction(actions.KindUnassign)
		case !m.scopedBuiltinOverridden("subscribe") && m.ctx.View == context.IssuesView && key.Matches(msg, m.keys.Subscribe):
			return m, m.startAction(actions.KindSubscribe)
		case !m.scopedBuiltinOverridden("unsubscribe") && m.ctx.View == context.IssuesView && key.Matches(msg, m.keys.Unsubscribe):
			return m, m.startAction(actions.KindUnsubscribe)
		case !m.scopedBuiltinOverridden("addlabel") && key.Matches(msg, m.keys.AddLabel):
			return m, m.startAction(actions.KindAddLabel)
		case !m.scopedBuiltinOverridden("removelabel") && key.Matches(msg, m.keys.RemoveLabel):
			return m, m.startAction(actions.KindRemoveLabel)
		case !m.scopedBuiltinOverridden("setMilestone") && m.ctx.View == context.IssuesView && key.Matches(msg, m.keys.Milestone):
			return m, m.startAction(actions.KindSetMilestone)
		case !m.scopedBuiltinOverridden("merge") && key.Matches(msg, m.keys.Merge):
			return m, m.startAction(actions.KindMerge)
		case !m.scopedBuiltinOverridden("update") && key.Matches(msg, m.keys.UpdateBranch):
			return m, m.startAction(actions.KindUpdateBranch)
		case !m.scopedBuiltinOverridden("ready") && key.Matches(msg, m.keys.MarkReady):
			return m, m.startAction(actions.KindMarkReady)
		case !m.scopedBuiltinOverridden("watch") && !m.scopedBuiltinOverridden("watchChecks") &&
			!m.scopedBuiltinOverridden("checks") && key.Matches(msg, m.keys.WatchChecks):
			return m.watchSelectedPullChecks()
		case !m.scopedBuiltinOverridden("close") && key.Matches(msg, m.keys.Close):
			return m, m.startAction(actions.KindClose)
		case !m.scopedBuiltinOverridden("reopen") && key.Matches(msg, m.keys.Reopen):
			return m, m.startAction(actions.KindReopen)
		case !m.scopedBuiltinOverridden("review") && key.Matches(msg, m.keys.Review):
			return m, m.startAction(actions.KindReview)
		case !m.scopedBuiltinOverridden("diff") && key.Matches(msg, m.keys.ExternalDiff):
			return m, m.startAction(actions.KindExternalDiff)
		case !m.scopedBuiltinOverridden("checkout") && key.Matches(msg, m.keys.Checkout):
			if m.ctx.View == context.BranchesView {
				return m, m.startAction(actions.KindSwitchBranch)
			}
			return m, m.startAction(actions.KindCheckout)
		case !m.scopedBuiltinOverridden("quit") && key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case !m.scopedBuiltinOverridden("refresh") && key.Matches(msg, m.keys.Refresh):
			if s := m.getCurrSection(); s != nil && !s.GetIsLoading() {
				if m.ctx.PreviewOpen {
					m.clearSelectedPreviewCache()
					m.syncSidebar()
				}
				return m, s.FetchRows()
			}
			return m, nil
		case !m.scopedBuiltinOverridden("refreshAll") && key.Matches(msg, m.keys.RefreshAll):
			return m, m.refreshAllSections()
		case !m.scopedBuiltinOverridden("open") && key.Matches(msg, m.keys.Open):
			return m, m.openSelected()
		case !m.scopedBuiltinOverridden("copyNumber") && key.Matches(msg, m.keys.CopyNumber):
			return m, m.copySelectedNumber()
		case !m.scopedBuiltinOverridden("copyurl") && key.Matches(msg, m.keys.CopyURL):
			return m, m.copySelectedURL()
		case !m.scopedBuiltinOverridden("help") && key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			return m, nil
		case !m.scopedBuiltinOverridden("nextSection") && key.Matches(msg, m.keys.NextSection):
			if last := len(m.currentViewSections()) - 1; m.currSectionId < last {
				m.currSectionId++
			}
			m.tabs.SetCurrSectionId(m.currSectionId)
			if m.ctx.PreviewOpen {
				m.syncSidebar()
				return m, m.enrichCurrRow()
			}
			return m, nil
		case !m.scopedBuiltinOverridden("prevSection") && key.Matches(msg, m.keys.PrevSection):
			if m.currSectionId > 0 {
				m.currSectionId--
			}
			m.tabs.SetCurrSectionId(m.currSectionId)
			if m.ctx.PreviewOpen {
				m.syncSidebar()
				return m, m.enrichCurrRow()
			}
			return m, nil
		case !m.scopedBuiltinOverridden("switchView") && key.Matches(msg, m.keys.SwitchView):
			cmd := m.switchView()
			return m, cmd
		case !m.scopedBuiltinOverridden("search") && key.Matches(msg, m.keys.Search):
			if s := m.getCurrSection(); s != nil {
				return m, s.SetIsSearching(true)
			}
			return m, nil
		case !m.scopedBuiltinOverridden("togglePreview") && key.Matches(msg, m.keys.TogglePreview):
			m.ctx.PreviewOpen = !m.ctx.PreviewOpen
			m.syncMainContentDimensions()
			m.syncProgramContext()
			if m.ctx.PreviewOpen {
				m.expanded = false
				m.syncSidebar()
				return m, m.enrichCurrRow()
			}
			return m, nil
		case !m.scopedBuiltinOverridden("toggleSmartFiltering") && key.Matches(msg, m.keys.ToggleSmart):
			return m.toggleSmartFiltering()
		case !m.scopedBuiltinOverridden("expand") && key.Matches(msg, m.keys.Expand):
			if m.ctx.PreviewOpen {
				m.expanded = !m.expanded
				m.syncSidebar()
			}
			return m, nil
		case (!m.scopedBuiltinOverridden("scrollUp") && key.Matches(msg, m.keys.ScrollUp) ||
			!m.scopedBuiltinOverridden("scrollDown") && key.Matches(msg, m.keys.ScrollDown)) && m.ctx.PreviewOpen:
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
	case context.ActionsView:
		subtitle = "  actions"
	case context.BranchesView:
		subtitle = "  local branches"
	}
	title := m.ctx.Styles.Title.Render("tea-dash") + m.ctx.Styles.DimText.Render(subtitle)

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
	parts = append(parts, body)
	if bar := m.actionBarView(); bar != "" {
		parts = append(parts, bar)
	}
	parts = append(parts, status, m.ctx.Styles.HelpText.Render(m.helpLine()))

	content := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return tea.View{Content: content, AltScreen: true, MouseMode: tea.MouseModeCellMotion}
}

func (m Model) helpLine() string {
	if m.showHelp {
		text := "↑/↓/j/k move · g/G first/last · h/l section · s view · / search · p preview · e expand · ctrl+u/d scroll · r refresh · R refresh all · o/enter open · y copy number · Y copy URL"
		if m.ctx.CurrentRepo != "" {
			text += " · t current repo"
		}
		switch m.ctx.View {
		case context.ActionsView:
			text = "↑/↓/j/k move · g/G first/last · h/l section · s view · / search · p preview · e expand · ctrl+u/d scroll · r refresh · ctrl+r refresh all · R rerun · ! cancel run · o/enter open · y copy number · Y copy URL"
			if m.ctx.CurrentRepo != "" {
				text += " · t current repo"
			}
		case context.NotificationsView:
			text += " · m mark read · u mark unread · M mark all read · b pin/unpin · B unpin"
		case context.BranchesView:
			text += " · C/space switch"
		case context.IssuesView:
			text += " · c comment · a/A assign/unassign · L/U labels · M milestone · b/B subscribe/unsubscribe · x/X close/reopen"
		default:
			text += " · c comment · a/A assign/unassign · L/U labels · m merge · u update · W ready · w checks · x/X close/reopen · v review · d/ctrl+t diff · C/space checkout"
		}
		return text + " · q quit"
	}
	text := "↑/↓/j/k move · g/G first/last · h/l section · s view · / search · p preview · r/R refresh"
	if m.ctx.CurrentRepo != "" {
		text += " · t current repo"
	}
	switch m.ctx.View {
	case context.ActionsView:
		text = "↑/↓/j/k move · g/G first/last · h/l section · s view · / search · p preview · r refresh · R rerun · ! cancel"
		if m.ctx.CurrentRepo != "" {
			text += " · t current repo"
		}
	case context.NotificationsView:
		text += " · m read · u unread · M all read · b pin · B unpin"
	case context.BranchesView:
		text += " · C/space switch"
	case context.IssuesView:
		text += " · c comment · a/A assign · L/U labels · M milestone · b/B subscribe · x/X close/reopen"
	default:
		text += " · c comment · a/A assign · L/U labels · m merge · u update · W ready · w checks · d/ctrl+t diff · C/space checkout"
	}
	return text + fmt.Sprintf(" · %s help · %s quit", keyHelp(m.keys.Help, "?"), keyHelp(m.keys.Quit, "q"))
}

func keyHelp(binding key.Binding, fallback string) string {
	if h := binding.Help(); h.Key != "" {
		return h.Key
	}
	keys := binding.Keys()
	if len(keys) > 0 {
		return keys[0]
	}
	return fallback
}

func (m Model) actionButtons() []actionButton {
	buttons := []actionButton{
		{Label: "Open", Builtin: "open"},
		{Label: "Refresh", Builtin: "refresh"},
	}
	row := m.getCurrRowData()
	switch m.ctx.View {
	case context.NotificationsView:
		if n, ok := row.(data.Notification); ok {
			if n.Unread {
				buttons = append(buttons, actionButton{Label: "Mark read", Builtin: "markRead"})
			} else {
				buttons = append(buttons, actionButton{Label: "Mark unread", Builtin: "markUnread"})
			}
			if n.Pinned {
				buttons = append(buttons, actionButton{Label: "Unpin", Builtin: "unpin"})
			} else {
				buttons = append(buttons, actionButton{Label: "Pin", Builtin: "togglePin"})
			}
		}
		buttons = append(buttons, actionButton{Label: "All read", Builtin: "markAllRead"})
	case context.ActionsView:
		buttons = append(buttons,
			actionButton{Label: "Rerun", Builtin: "rerun"},
			actionButton{Label: "Cancel", Builtin: "cancel"},
		)
	case context.BranchesView:
		buttons = []actionButton{
			{Label: "Refresh", Builtin: "refresh"},
			{Label: "Checkout", Builtin: "checkout"},
		}
	case context.IssuesView:
		buttons = append(buttons,
			actionButton{Label: "Comment", Builtin: "comment"},
			actionButton{Label: "Checkout", Builtin: "checkout"},
			actionButton{Label: "Subscribe", Builtin: "subscribe"},
			actionButton{Label: "Unsubscribe", Builtin: "unsubscribe"},
			actionButton{Label: "Milestone", Builtin: "setMilestone"},
		)
		switch rowState(row) {
		case "closed":
			buttons = append(buttons, actionButton{Label: "Reopen", Builtin: "reopen"})
		default:
			buttons = append(buttons, actionButton{Label: "Close", Builtin: "close"})
		}
	default:
		if pr, ok := row.(data.PullRequest); ok && pr.Draft {
			buttons = append(buttons, actionButton{Label: "Ready", Builtin: "ready"})
		}
		buttons = append(buttons,
			actionButton{Label: "Comment", Builtin: "comment"},
			actionButton{Label: "Checks", Builtin: "watchChecks"},
			actionButton{Label: "Diff", Builtin: "diff"},
			actionButton{Label: "Checkout", Builtin: "checkout"},
		)
		switch rowState(row) {
		case "closed":
			buttons = append(buttons, actionButton{Label: "Reopen", Builtin: "reopen"})
		case "merged":
			// A merged PR cannot be closed or reopened through the normal issue
			// state transition, so keep only non-state-changing actions visible.
		default:
			buttons = append(buttons,
				actionButton{Label: "Merge", Builtin: "merge"},
				actionButton{Label: "Close", Builtin: "close"},
			)
		}
	}
	return buttons
}

func rowState(row data.RowData) string {
	switch r := row.(type) {
	case data.PullRequest:
		return r.State
	case data.Issue:
		return r.State
	case data.Notification:
		return r.SubjectState
	default:
		return ""
	}
}

func (m Model) actionBarView() string {
	if m.actionPrompt.Active() {
		return ""
	}
	buttons := m.actionButtons()
	if len(buttons) == 0 {
		return ""
	}
	rendered := make([]string, 0, len(buttons)*2-1)
	for i, b := range buttons {
		if i > 0 {
			rendered = append(rendered, " ")
		}
		rendered = append(rendered, m.renderActionButton(b))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, rendered...)
}

func (m Model) renderActionButton(b actionButton) string {
	return m.ctx.Styles.ActionButton.Render("[" + b.Label + "]")
}

func (m Model) actionBarY() int {
	y := 1 // appStyle top padding
	y++    // title
	if len(m.currentViewSections()) > 1 {
		y++ // tabs
	}
	y += m.ctx.MainContentHeight
	return y
}

func (m Model) actionButtonAt(x int) (actionButton, bool) {
	rel := x - 2 // appStyle left padding
	if rel < 0 || m.actionPrompt.Active() {
		return actionButton{}, false
	}
	pos := 0
	for _, b := range m.actionButtons() {
		w := lipgloss.Width(m.renderActionButton(b))
		if rel >= pos && rel < pos+w {
			return b, true
		}
		pos += w + 1
	}
	return actionButton{}, false
}

// currentViewSections returns the section slice for the active view.
func (m *Model) currentViewSections() []section.Section {
	switch m.ctx.View {
	case context.ActionsView:
		return m.actions
	case context.BranchesView:
		return m.branches
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
	case context.ActionsView:
		m.actions = s
	case context.BranchesView:
		m.branches = s
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
	sectionType := ""
	if s := m.getCurrSection(); s != nil {
		sectionType = s.GetType()
		switch s.GetType() {
		case pullsection.SectionType:
			if _, ok := m.pullDetails[key]; ok {
				return nil
			}
		case issuesection.SectionType:
			if _, ok := m.issueDetails[key]; ok {
				return nil
			}
		case actionsection.SectionType:
			if _, ok := m.actionDetails[key]; ok {
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
	switch sectionType {
	case pullsection.SectionType:
		index := row.GetNumber()
		return func() tea.Msg {
			d, err := client.GetPullDetail(owner, name, index)
			if err != nil {
				return enrichedMsg{key: key, sectionType: pullsection.SectionType, err: err}
			}
			return enrichedMsg{key: key, sectionType: pullsection.SectionType, pull: &d}
		}
	case issuesection.SectionType:
		index := row.GetNumber()
		return func() tea.Msg {
			d, err := client.GetIssueDetail(owner, name, index)
			if err != nil {
				return enrichedMsg{key: key, sectionType: issuesection.SectionType, err: err}
			}
			return enrichedMsg{key: key, sectionType: issuesection.SectionType, issue: &d}
		}
	case actionsection.SectionType:
		run, ok := row.(data.ActionRun)
		if !ok {
			return nil
		}
		runID := run.ID
		if runID == 0 {
			runID = run.GetNumber()
		}
		return func() tea.Msg {
			d, err := client.GetActionRun(stdctx.Background(), owner, name, runID)
			if err != nil {
				return enrichedMsg{key: key, sectionType: actionsection.SectionType, err: err}
			}
			jobs, err := client.ListActionJobs(stdctx.Background(), owner, name, runID)
			if err != nil {
				return enrichedMsg{key: key, sectionType: actionsection.SectionType, err: err}
			}
			return enrichedMsg{
				key:         key,
				sectionType: actionsection.SectionType,
				action:      &data.ActionRunDetail{Run: d, Jobs: jobs},
			}
		}
	default:
		return nil
	}
}

// syncSidebar renders the current row (with its cached detail, if any) into the
// preview pane. The concrete row type selects the renderer; a missing cache
// entry renders that row kind's loading placeholder.
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
	case data.ActionRun:
		if err := m.actionEnrichErr[key]; err != nil {
			m.sidebar.SetContent(m.failedPreview(row, err))
			return
		}
		rendered = prview.RenderAction(r, m.actionDetails[key], w)
	default:
		m.sidebar.SetContent("")
		return
	}
	m.sidebar.SetContent(rendered)
}

func (m *Model) setEnrichErr(sectionType, key string, err error) {
	switch sectionType {
	case pullsection.SectionType:
		m.pullEnrichErr[key] = err
	case issuesection.SectionType:
		m.issueEnrichErr[key] = err
	case actionsection.SectionType:
		m.actionEnrichErr[key] = err
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
		case actionsection.SectionType:
			delete(m.actionDetails, key)
			delete(m.actionEnrichErr, key)
		}
	}
}

func (m *Model) clearPreviewCacheForAction(target actions.Target) {
	if target.Repo == "" || target.Number <= 0 {
		return
	}
	key := fmt.Sprintf("%s#%d", target.Repo, target.Number)
	switch target.RowKind {
	case actions.RowKindPullRequest:
		delete(m.pullDetails, key)
		delete(m.pullEnrichErr, key)
	case actions.RowKindIssue:
		delete(m.issueDetails, key)
		delete(m.issueEnrichErr, key)
	case actions.RowKindActionRun:
		delete(m.actionDetails, key)
		delete(m.actionEnrichErr, key)
	}
}

func (m Model) watchSelectedPullChecks() (Model, tea.Cmd) {
	row, ok := m.getCurrRowData().(data.PullRequest)
	if !ok {
		m.notice = "Select a pull request to watch checks."
		return m, nil
	}
	branch, sha := pullCheckHead(row, m.pullDetails[m.selKey()])
	if branch != "" || sha != "" {
		return m.switchToPullChecks(row, branch, sha)
	}
	if m.ctx.Client == nil {
		return m.switchToPullChecks(row, "", "")
	}
	owner, repo, ok := data.SplitOwnerRepo(row.RepoNameWithOwner)
	if !ok {
		m.notice = fmt.Sprintf("Can't watch checks for invalid repo %q.", row.RepoNameWithOwner)
		return m, nil
	}
	return m, func() tea.Msg {
		detail, err := m.ctx.Client.GetPullDetail(owner, repo, row.Number)
		return watchChecksMsg{row: row, detail: detail, err: err}
	}
}

func pullCheckHead(row data.PullRequest, detail *data.PullDetail) (branch, sha string) {
	branch = row.HeadRef
	sha = row.HeadSHA
	if detail != nil {
		if branch == "" {
			branch = detail.HeadRef
		}
		if sha == "" {
			sha = detail.HeadSHA
		}
	}
	return branch, sha
}

func (m Model) switchToPullChecks(row data.PullRequest, branch, sha string) (Model, tea.Cmd) {
	if _, _, ok := data.SplitOwnerRepo(row.RepoNameWithOwner); !ok {
		m.notice = fmt.Sprintf("Can't watch checks for invalid repo %q.", row.RepoNameWithOwner)
		return m, nil
	}
	m.ctx.View = context.ActionsView
	m.currSectionId = 0
	m.syncMainContentDimensions()
	cfg := config.SectionConfig{
		Title: fmt.Sprintf("Checks for #%d", row.Number),
		Repo:  row.RepoNameWithOwner,
		Filter: config.PrIssueFilter{
			Branch:  branch,
			HeadSHA: sha,
		},
	}
	m.actionDetails = map[string]*data.ActionRunDetail{}
	m.actionEnrichErr = map[string]error{}
	m.setCurrentViewSections([]section.Section{actionsection.NewModel(0, m.ctx, cfg)})
	m.tabs.SetCurrSectionId(0)
	m.syncProgramContext()
	if branch == "" && sha == "" {
		m.notice = fmt.Sprintf("Showing Actions for %s; PR head was not available to narrow checks.", row.RepoNameWithOwner)
	} else {
		m.notice = fmt.Sprintf("Watching checks for %s#%d.", row.RepoNameWithOwner, row.Number)
	}
	if m.ctx.PreviewOpen {
		m.syncSidebar()
	}
	if s := m.getCurrSection(); s != nil {
		return m, s.FetchRows()
	}
	return m, nil
}

// switchView cycles pulls -> issues -> notifications -> actions -> branches, lazily
// building and fetching the target view's sections on first visit.
func (m *Model) switchView() tea.Cmd {
	switch m.ctx.View {
	case context.PullsView:
		m.ctx.View = context.IssuesView
	case context.IssuesView:
		m.ctx.View = context.NotificationsView
	case context.NotificationsView:
		m.ctx.View = context.ActionsView
	case context.ActionsView:
		m.ctx.View = context.BranchesView
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

func (m Model) tableDataStartY() int {
	y := 1 // appStyle top padding
	y++    // title
	if len(m.currentViewSections()) > 1 {
		y++ // tabs
	}
	if s := m.getCurrSection(); s != nil && s.IsSearchFocused() {
		y++ // search bar
	}
	y++ // table header
	return y
}

func (m Model) tabBarY() int {
	if len(m.currentViewSections()) < 2 {
		return -1
	}
	return 2 // appStyle top padding + title line
}

func (m Model) switchSectionTo(id int) (Model, tea.Cmd) {
	if id < 0 || id >= len(m.currentViewSections()) || id == m.currSectionId {
		return m, nil
	}
	m.currSectionId = id
	m.tabs.SetCurrSectionId(id)
	if m.ctx.PreviewOpen {
		m.syncSidebar()
		return m, m.enrichCurrRow()
	}
	return m, nil
}

func (m Model) inMainListPane(x, y int) bool {
	if x < 2 || y < m.tableDataStartY() {
		return false
	}
	width := m.ctx.MainContentWidth
	if width <= 0 {
		width = m.ctx.ScreenWidth - 4
	}
	return x < 2+width
}

func (m Model) rowIndexAtY(y int) (int, bool) {
	s := m.getCurrSection()
	if s == nil || s.GetIsLoading() || s.GetError() != nil {
		return 0, false
	}
	i := y - m.tableDataStartY()
	if i < 0 || i >= s.NumRows() {
		return 0, false
	}
	return i, true
}

func (m Model) selectRowFromMouse(x, y int) (Model, tea.Cmd) {
	if !m.inMainListPane(x, y) {
		return m, nil
	}
	i, ok := m.rowIndexAtY(y)
	if !ok {
		return m, nil
	}
	s := m.getCurrSection()
	before := m.selKey()
	s.SelectRow(i)
	moreCmd := s.MaybeFetchNextPage()
	if !m.ctx.PreviewOpen || m.selKey() == before {
		return m, moreCmd
	}
	m.syncSidebar()
	return m, tea.Batch(moreCmd, m.enrichCurrRow())
}

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	if m.actionPrompt.Active() {
		return m, nil
	}
	if msg.Y == m.tabBarY() {
		if id, ok := m.tabs.TabAt(msg.X - 2); ok {
			return m.switchSectionTo(id)
		}
	}
	if msg.Y == m.actionBarY() {
		if button, ok := m.actionButtonAt(msg.X); ok {
			return m.handleActionButton(button)
		}
	}
	return m.selectRowFromMouse(msg.X, msg.Y)
}

func (m Model) handleActionButton(button actionButton) (Model, tea.Cmd) {
	next, cmd, ok := m.handleBuiltinKeybinding(config.Keybinding{Builtin: button.Builtin})
	if ok {
		return next, cmd
	}
	m.notice = fmt.Sprintf("Action button %q is not wired.", button.Label)
	return m, nil
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	if !m.inMainListPane(msg.X, msg.Y) {
		return m, nil
	}
	before := m.selKey()
	var cmd tea.Cmd
	switch msg.Button {
	case tea.MouseWheelUp:
		cmd = m.updateCurrentSection(tea.KeyPressMsg{Code: tea.KeyUp})
	case tea.MouseWheelDown:
		cmd = m.updateCurrentSection(tea.KeyPressMsg{Code: tea.KeyDown})
	default:
		return m, nil
	}
	if m.ctx.PreviewOpen && m.selKey() != before {
		m.syncSidebar()
		cmd = tea.Batch(cmd, m.enrichCurrRow())
	}
	return m, cmd
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

func (m Model) copySelectedNumber() tea.Cmd {
	row := m.getCurrRowData()
	if row == nil {
		return nil
	}
	return m.copyValue(fmt.Sprintf("%d", row.GetNumber()))
}

func (m Model) copySelectedURL() tea.Cmd {
	row := m.getCurrRowData()
	if row == nil || row.GetURL() == "" {
		return nil
	}
	return m.copyValue(row.GetURL())
}

func (m Model) copyValue(value string) tea.Cmd {
	copyFn := m.copyToClipboard
	if copyFn == nil {
		copyFn = writeClipboard
	}
	return func() tea.Msg {
		return copyResultMsg{value: value, err: copyFn(value)}
	}
}

func (m *Model) refreshAllSections() tea.Cmd {
	var cmds []tea.Cmd
	switch m.ctx.View {
	case context.IssuesView:
		m.issueDetails = map[string]*data.IssueDetail{}
		m.issueEnrichErr = map[string]error{}
	case context.ActionsView:
		m.actionDetails = map[string]*data.ActionRunDetail{}
		m.actionEnrichErr = map[string]error{}
	default:
		m.pullDetails = map[string]*data.PullDetail{}
		m.pullEnrichErr = map[string]error{}
	}
	for _, s := range m.currentViewSections() {
		cmds = append(cmds, s.FetchRows())
	}
	if m.ctx.PreviewOpen {
		m.syncSidebar()
	}
	return tea.Batch(cmds...)
}

func (m Model) toggleSmartFiltering() (Model, tea.Cmd) {
	if m.ctx.CurrentRepo == "" {
		m.notice = "No matching git remote detected for this Gitea instance."
		return m, nil
	}
	m.ctx.SmartFiltering = !m.ctx.SmartFiltering
	if m.ctx.SmartFiltering {
		m.notice = fmt.Sprintf("Showing current repository: %s.", m.ctx.CurrentRepo)
	} else {
		m.notice = "Showing all repositories."
	}
	m.pullDetails = map[string]*data.PullDetail{}
	m.pullEnrichErr = map[string]error{}
	m.issueDetails = map[string]*data.IssueDetail{}
	m.issueEnrichErr = map[string]error{}
	m.prs = nil
	m.issues = nil
	switch m.ctx.View {
	case context.PullsView, context.IssuesView:
		m.currSectionId = 0
		m.setCurrentViewSections(buildSections(m.ctx.View, m.ctx))
		m.tabs.SetCurrSectionId(0)
		m.syncProgramContext()
		if m.ctx.PreviewOpen {
			m.syncSidebar()
		}
		return m, m.refreshAllSections()
	default:
		m.syncProgramContext()
		return m, nil
	}
}

func (m Model) handleAutoRefresh() (Model, tea.Cmd) {
	if m.autoRefreshInterval() <= 0 {
		return m, nil
	}
	cmds := []tea.Cmd{m.autoRefreshCmd()}
	if m.actionPrompt.Active() {
		return m, tea.Batch(cmds...)
	}
	if s := m.getCurrSection(); s != nil && s.IsSearchFocused() {
		return m, tea.Batch(cmds...)
	}
	cmds = append(cmds, m.refreshAllSections())
	return m, tea.Batch(cmds...)
}

func (m *Model) startAction(kind actions.Kind) tea.Cmd {
	target, ok := m.selectedActionTarget()
	if !ok {
		m.notice = "Select a row before running an action."
		return nil
	}
	if err := validateActionTarget(kind, target); err != nil {
		m.notice = err.Error()
		return nil
	}
	if kind == actions.KindSwitchBranch {
		branch, ok := m.getCurrRowData().(localgit.Branch)
		if !ok || target.RowKind != actions.RowKindBranch {
			m.notice = "Switch branch is only available for local branches."
			return nil
		}
		if branch.Current {
			m.notice = fmt.Sprintf("%s is already current in %s.", branch.Name, branch.Repository)
			return nil
		}
	}
	intent := actions.Intent{
		Kind:   kind,
		Target: target,
		Prompt: actions.Prompt{Mode: promptModeForAction(kind)},
	}
	if actionDispatchesDirectly(kind) {
		return m.dispatchActionIntent(intent)
	}
	m.pendingAction = intent
	m.actionPrompt = m.actionPrompt.Focus(promptConfigForAction(kind, target))
	return nil
}

func (m *Model) startCustomCommand(binding config.Keybinding) tea.Cmd {
	target, ok := m.selectedActionTarget()
	if !ok {
		m.notice = "Select a row before running a custom command."
		return nil
	}
	intent := actions.Intent{
		Kind:    actions.KindCustomCommand,
		Target:  target,
		Command: binding.Command,
		Name:    binding.Name,
	}
	return m.dispatchActionIntent(intent)
}

func (m Model) handleBuiltinKeybinding(binding config.Keybinding) (Model, tea.Cmd, bool) {
	switch normalizeBuiltin(binding.Builtin) {
	case "refresh":
		if s := m.getCurrSection(); s != nil && !s.GetIsLoading() {
			if m.ctx.PreviewOpen {
				m.clearSelectedPreviewCache()
				m.syncSidebar()
			}
			return m, s.FetchRows(), true
		}
		return m, nil, true
	case "refreshall":
		return m, m.refreshAllSections(), true
	case "opengithub", "open", "openbrowser":
		return m, m.openSelected(), true
	case "quit":
		return m, tea.Quit, true
	case "redraw":
		return m, tea.ClearScreen, true
	case "nextsection":
		if last := len(m.currentViewSections()) - 1; m.currSectionId < last {
			m.currSectionId++
		}
		m.tabs.SetCurrSectionId(m.currSectionId)
		if m.ctx.PreviewOpen {
			m.syncSidebar()
			return m, m.enrichCurrRow(), true
		}
		return m, nil, true
	case "prevsection", "previoussection":
		if m.currSectionId > 0 {
			m.currSectionId--
		}
		m.tabs.SetCurrSectionId(m.currSectionId)
		if m.ctx.PreviewOpen {
			m.syncSidebar()
			return m, m.enrichCurrRow(), true
		}
		return m, nil, true
	case "viewissues", "viewprs", "switchview":
		return m, m.switchView(), true
	case "search":
		if s := m.getCurrSection(); s != nil {
			return m, s.SetIsSearching(true), true
		}
		return m, nil, true
	case "togglepreview":
		m.ctx.PreviewOpen = !m.ctx.PreviewOpen
		m.syncMainContentDimensions()
		m.syncProgramContext()
		if m.ctx.PreviewOpen {
			m.expanded = false
			m.syncSidebar()
			return m, m.enrichCurrRow(), true
		}
		return m, nil, true
	case "togglesmartfiltering", "togglesmartfilter", "currentrepo":
		next, cmd := m.toggleSmartFiltering()
		return next, cmd, true
	case "summaryviewmore", "expand":
		if m.ctx.PreviewOpen {
			m.expanded = !m.expanded
			m.syncSidebar()
		}
		return m, nil, true
	case "pageup", "scrollup":
		if m.ctx.PreviewOpen {
			var cmd tea.Cmd
			m.sidebar, cmd = m.sidebar.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
			return m, cmd, true
		}
		return m, nil, true
	case "pagedown", "scrolldown":
		if m.ctx.PreviewOpen {
			var cmd tea.Cmd
			m.sidebar, cmd = m.sidebar.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
			return m, cmd, true
		}
		return m, nil, true
	case "copyurl":
		return m, m.copySelectedURL(), true
	case "copynumber":
		return m, m.copySelectedNumber(), true
	case "help":
		m.showHelp = !m.showHelp
		return m, nil, true
	case "markasread", "markread", "markasdone", "markdone":
		next, cmd := m.markSelectedNotificationRead()
		return next, cmd, true
	case "markasunread", "markunread":
		next, cmd := m.markSelectedNotificationUnread()
		return next, cmd, true
	case "markallasread", "markallread", "markallasdone", "markalldone":
		next, cmd := m.markAllNotificationsRead()
		return next, cmd, true
	case "pin", "togglepin", "togglepinned":
		next, cmd := m.toggleSelectedNotificationPin()
		return next, cmd, true
	case "unpin":
		next, cmd := m.unpinSelectedNotification()
		return next, cmd, true
	case "comment":
		return m, m.startAction(actions.KindComment), true
	case "assign":
		return m, m.startAction(actions.KindAssign), true
	case "unassign":
		return m, m.startAction(actions.KindUnassign), true
	case "subscribe":
		return m, m.startAction(actions.KindSubscribe), true
	case "unsubscribe":
		return m, m.startAction(actions.KindUnsubscribe), true
	case "addlabel":
		return m, m.startAction(actions.KindAddLabel), true
	case "removelabel":
		return m, m.startAction(actions.KindRemoveLabel), true
	case "milestone", "setmilestone":
		return m, m.startAction(actions.KindSetMilestone), true
	case "merge":
		return m, m.startAction(actions.KindMerge), true
	case "update", "updatebranch":
		return m, m.startAction(actions.KindUpdateBranch), true
	case "ready", "markready":
		return m, m.startAction(actions.KindMarkReady), true
	case "draft", "markdraft":
		return m, m.startAction(actions.KindMarkDraft), true
	case "watch", "watchchecks", "checks":
		next, cmd := m.watchSelectedPullChecks()
		return next, cmd, true
	case "close":
		return m, m.startAction(actions.KindClose), true
	case "reopen":
		return m, m.startAction(actions.KindReopen), true
	case "approve", "review":
		return m, m.startAction(actions.KindReview), true
	case "diff":
		return m, m.startAction(actions.KindExternalDiff), true
	case "checkout":
		if m.ctx.View == context.BranchesView {
			return m, m.startAction(actions.KindSwitchBranch), true
		}
		return m, m.startAction(actions.KindCheckout), true
	case "rerun", "rerunrun":
		return m, m.startAction(actions.KindRerunRun), true
	case "cancel", "cancelrun":
		return m, m.startAction(actions.KindCancelRun), true
	default:
		m.notice = fmt.Sprintf("Unknown builtin keybinding %q.", binding.Builtin)
		return m, nil, true
	}
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
	dispatchCmd := m.dispatchActionIntent(intent)
	if cmd == nil {
		return dispatchCmd
	}
	if dispatchCmd == nil {
		return cmd
	}
	return tea.Batch(cmd, dispatchCmd)
}

func (m *Model) dispatchActionIntent(intent actions.Intent) tea.Cmd {
	if m.actionDispatcher == nil {
		m.notice = "Action not wired yet."
		return nil
	}
	m.actionFeedback = m.actionFeedback.Set(actionfeedback.Start(actionStartText(intent)))
	return m.actionDispatcher(intent)
}

func (m Model) selectedActionTarget() (actions.Target, bool) {
	s := m.getCurrSection()
	row := m.getCurrRowData()
	if s == nil || row == nil {
		return actions.Target{}, false
	}
	rowKind := actions.RowKind(s.GetType())
	runID := int64(0)
	author := ""
	sha := ""
	switch r := row.(type) {
	case data.PullRequest:
		rowKind = actions.RowKindPullRequest
		author = r.Author
	case data.Issue:
		rowKind = actions.RowKindIssue
		author = r.Author
	case localgit.Branch:
		rowKind = actions.RowKindBranch
	case data.ActionRun:
		rowKind = actions.RowKindActionRun
		runID = r.ID
		author = r.Actor
		sha = r.HeadSHA
	}
	target := actions.Target{
		SectionID:   s.GetId(),
		SectionType: s.GetType(),
		RowKind:     rowKind,
		Repo:        row.GetRepoNameWithOwner(),
		Number:      row.GetNumber(),
		RunID:       runID,
		Title:       row.GetTitle(),
		URL:         row.GetURL(),
		Author:      author,
		SHA:         sha,
	}
	if branch, ok := row.(localgit.Branch); ok {
		target.RepositoryPath = branch.RepositoryPath
	} else if m.ctx.Config != nil {
		if repoPath, ok, err := config.MatchRepoPath(row.GetRepoNameWithOwner(), m.ctx.Config.RepoPaths); err == nil && ok {
			target.RepositoryPath = repoPath
		}
	}
	return target, true
}

func validateActionTarget(kind actions.Kind, target actions.Target) error {
	switch kind {
	case actions.KindMerge, actions.KindUpdateBranch, actions.KindMarkReady, actions.KindMarkDraft, actions.KindReview, actions.KindExternalDiff:
		if target.RowKind != actions.RowKindPullRequest {
			return fmt.Errorf("%s is only available for pull requests.", actionLabel(kind))
		}
	case actions.KindCheckout:
		if target.RowKind != actions.RowKindPullRequest && target.RowKind != actions.RowKindIssue {
			return fmt.Errorf("%s is only available for pull requests and issues.", actionLabel(kind))
		}
	case actions.KindComment, actions.KindAssign, actions.KindUnassign, actions.KindAddLabel, actions.KindRemoveLabel, actions.KindClose, actions.KindReopen:
		if target.RowKind != actions.RowKindPullRequest && target.RowKind != actions.RowKindIssue {
			return fmt.Errorf("%s is only available for pull requests and issues.", actionLabel(kind))
		}
	case actions.KindSetMilestone, actions.KindSubscribe, actions.KindUnsubscribe:
		if target.RowKind != actions.RowKindIssue {
			return fmt.Errorf("%s is only available for issues.", actionLabel(kind))
		}
	case actions.KindRerunRun, actions.KindCancelRun:
		if target.RowKind != actions.RowKindActionRun {
			return fmt.Errorf("%s is only available for action runs.", actionLabel(kind))
		}
	default:
		return nil
	}
	return nil
}

func actionDispatchesDirectly(kind actions.Kind) bool {
	return kind == actions.KindRerunRun || kind == actions.KindSubscribe || kind == actions.KindUnsubscribe
}

func promptModeForAction(kind actions.Kind) actions.PromptMode {
	switch kind {
	case actions.KindComment:
		return actions.PromptText
	case actions.KindAddLabel, actions.KindRemoveLabel, actions.KindSetMilestone:
		return actions.PromptText
	case actions.KindMerge, actions.KindReview:
		return actions.PromptPicker
	default:
		return actions.PromptConfirm
	}
}

func promptConfigForAction(kind actions.Kind, target actions.Target) actionprompt.Config {
	title := fmt.Sprintf("%s #%d", actionLabel(kind), target.Number)
	message := fmt.Sprintf("%s - %s", target.Repo, target.Title)
	if kind == actions.KindSwitchBranch {
		title = actionLabel(kind)
	}
	switch kind {
	case actions.KindComment:
		return actionprompt.Config{
			Mode:        actionprompt.ModeText,
			Title:       title,
			Message:     message,
			Placeholder: "Write a comment",
		}
	case actions.KindAddLabel:
		return actionprompt.Config{
			Mode:        actionprompt.ModeText,
			Title:       title,
			Message:     message,
			Placeholder: "Label names, comma-separated",
		}
	case actions.KindRemoveLabel:
		return actionprompt.Config{
			Mode:        actionprompt.ModeText,
			Title:       title,
			Message:     message,
			Placeholder: "Label names to remove, comma-separated",
		}
	case actions.KindSetMilestone:
		return actionprompt.Config{
			Mode:        actionprompt.ModeText,
			Title:       title,
			Message:     message,
			Placeholder: "Milestone title",
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
	case actions.KindMerge:
		return actionprompt.Config{
			Mode:    actionprompt.ModePicker,
			Title:   title,
			Message: message,
			Options: []actionprompt.Option{
				{Label: "Merge", Value: string(data.MergeStyleMerge)},
				{Label: "Squash", Value: string(data.MergeStyleSquash)},
				{Label: "Rebase", Value: string(data.MergeStyleRebase)},
				{Label: "Rebase merge", Value: string(data.MergeStyleRebaseMerge)},
				{Label: "Fast-forward only", Value: string(data.MergeStyleFastForwardOnly)},
			},
		}
	case actions.KindCancelRun:
		return actionprompt.Config{
			Mode:    actionprompt.ModeConfirm,
			Title:   title,
			Message: fmt.Sprintf("Cancel %s run #%d - %s", target.Repo, target.Number, target.Title),
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
	case actions.KindAssign:
		return "Assign"
	case actions.KindUnassign:
		return "Unassign"
	case actions.KindSubscribe:
		return "Subscribe"
	case actions.KindUnsubscribe:
		return "Unsubscribe"
	case actions.KindAddLabel:
		return "Add label"
	case actions.KindRemoveLabel:
		return "Remove label"
	case actions.KindSetMilestone:
		return "Set milestone"
	case actions.KindMerge:
		return "Merge"
	case actions.KindUpdateBranch:
		return "Update branch"
	case actions.KindMarkReady:
		return "Mark ready"
	case actions.KindMarkDraft:
		return "Mark draft"
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
	case actions.KindSwitchBranch:
		return "Switch branch"
	case actions.KindRerunRun:
		return "Rerun"
	case actions.KindCancelRun:
		return "Cancel run"
	case actions.KindCustomCommand:
		return "Custom command"
	default:
		return string(kind)
	}
}

func actionStartText(intent actions.Intent) string {
	if intent.Kind == actions.KindCustomCommand {
		label := intent.Name
		if label == "" {
			label = "custom command"
		}
		return fmt.Sprintf("Starting %s for %s.", label, intent.Target.Title)
	}
	if intent.Kind == actions.KindSwitchBranch {
		return fmt.Sprintf("Starting %s for %s.", actionLabel(intent.Kind), intent.Target.Title)
	}
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

func (m Model) markSelectedNotificationUnread() (Model, tea.Cmd) {
	row, ok := m.getCurrRowData().(data.Notification)
	if !ok {
		m.notice = "Select a notification to mark unread."
		return m, nil
	}
	if row.ID == 0 {
		m.notice = "Selected notification has no thread id."
		return m, nil
	}
	if m.ctx.Client == nil {
		m.notice = "No Gitea client available to mark notifications unread."
		return m, nil
	}
	return m, markNotificationUnreadCmd(m.ctx.Client, m.currSectionId, row.ID)
}

func (m Model) toggleSelectedNotificationPin() (Model, tea.Cmd) {
	row, ok := m.getCurrRowData().(data.Notification)
	if !ok {
		m.notice = "Select a notification to pin."
		return m, nil
	}
	if row.Pinned {
		return m.unpinNotification(row)
	}
	return m.pinNotification(row)
}

func (m Model) unpinSelectedNotification() (Model, tea.Cmd) {
	row, ok := m.getCurrRowData().(data.Notification)
	if !ok {
		m.notice = "Select a notification to unpin."
		return m, nil
	}
	return m.unpinNotification(row)
}

func (m Model) pinNotification(row data.Notification) (Model, tea.Cmd) {
	if row.ID == 0 {
		m.notice = "Selected notification has no thread id."
		return m, nil
	}
	if m.ctx.Client == nil {
		m.notice = "No Gitea client available to pin notifications."
		return m, nil
	}
	return m, markNotificationPinnedCmd(m.ctx.Client, m.currSectionId, row.ID)
}

func (m Model) unpinNotification(row data.Notification) (Model, tea.Cmd) {
	if row.ID == 0 {
		m.notice = "Selected notification has no thread id."
		return m, nil
	}
	if m.ctx.Client == nil {
		m.notice = "No Gitea client available to unpin notifications."
		return m, nil
	}
	return m, markNotificationUnpinnedCmd(m.ctx.Client, m.currSectionId, row.ID, row.Unread)
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

func markNotificationUnreadCmd(client *gitea.Client, sectionID int, threadID int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 30*time.Second)
		defer cancel()
		return notificationActionMsg{
			sectionID: sectionID,
			unread:    true,
			err:       client.MarkNotificationUnread(ctx, threadID),
		}
	}
}

func markNotificationPinnedCmd(client *gitea.Client, sectionID int, threadID int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 30*time.Second)
		defer cancel()
		return notificationActionMsg{
			sectionID: sectionID,
			pinned:    true,
			err:       client.PinNotification(ctx, threadID),
		}
	}
}

func markNotificationUnpinnedCmd(client *gitea.Client, sectionID int, threadID int64, unread bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 30*time.Second)
		defer cancel()
		return notificationActionMsg{
			sectionID: sectionID,
			unread:    unread,
			unpinned:  true,
			err:       client.UnpinNotification(ctx, threadID, unread),
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
		} else if msg.pinned {
			m.notice = fmt.Sprintf("Couldn't pin notification: %v", msg.err)
		} else if msg.unpinned {
			m.notice = fmt.Sprintf("Couldn't unpin notification: %v", msg.err)
		} else if msg.unread {
			m.notice = fmt.Sprintf("Couldn't mark notification unread: %v", msg.err)
		} else {
			m.notice = fmt.Sprintf("Couldn't mark notification read: %v", msg.err)
		}
		return m, nil
	}
	if msg.all {
		m.notice = "Marked all notifications read."
	} else if msg.pinned {
		m.notice = "Pinned notification."
	} else if msg.unpinned {
		m.notice = "Unpinned notification."
	} else if msg.unread {
		m.notice = "Marked notification unread."
	} else {
		m.notice = "Marked notification read."
	}
	if msg.sectionID < 0 || msg.sectionID >= len(m.notifications) {
		return m, nil
	}
	return m, m.notifications[msg.sectionID].FetchRows()
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
	case actionsection.SectionType:
		if id >= 0 && id < len(m.actions) {
			m.actions[id], cmd = m.actions[id].Update(msg)
		}
	case branchsection.SectionType:
		if id >= 0 && id < len(m.branches) {
			m.branches[id], cmd = m.branches[id].Update(msg)
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
	for _, s := range m.actions {
		s.UpdateProgramContext(m.ctx)
	}
	for _, s := range m.branches {
		s.UpdateProgramContext(m.ctx)
	}
	m.tabs.UpdateProgramContext(m.ctx)
	m.sidebar.UpdateProgramContext(m.ctx)
}

// syncMainContentDimensions splits the content area between the section table
// and the preview pane. When the preview is open it uses defaults.preview.width
// when configured, otherwise the previous automatic half-width layout capped at
// 80 columns. When closed, the table gets the full width.
func (m *Model) syncMainContentDimensions() {
	h := m.ctx.ScreenHeight - 7
	if h < 3 {
		h = 3
	}
	m.ctx.MainContentHeight = h

	if m.ctx.PreviewOpen {
		pw := m.configuredPreviewWidth()
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

func (m *Model) configuredPreviewWidth() int {
	available := m.ctx.ScreenWidth - 4
	if available < 0 {
		available = 0
	}
	if available == 0 {
		return 0
	}

	configured := 0
	if m.ctx.Config != nil {
		configured = m.ctx.Config.Defaults.Preview.PreviewWidth()
	}
	pw := available / 2
	if configured > 0 {
		pw = configured
	}

	maxPreview := available - 2 // reserve the gutter; the table may shrink to zero.
	if maxPreview < 0 {
		maxPreview = 0
	}
	if pw > maxPreview {
		pw = maxPreview
	}
	if configured == 0 && pw > 80 {
		pw = 80
	}
	if pw < 0 {
		pw = 0
	}
	return pw
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
