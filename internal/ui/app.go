// Package ui contains the Bubble Tea models that make up the tea-dash TUI.
package ui

import (
	stdctx "context"
	"fmt"
	"strings"
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
	"github.com/gbarany/tea-dash/internal/ui/components/header"
	"github.com/gbarany/tea-dash/internal/ui/components/helpoverlay"
	"github.com/gbarany/tea-dash/internal/ui/components/issuesection"
	"github.com/gbarany/tea-dash/internal/ui/components/notificationsection"
	"github.com/gbarany/tea-dash/internal/ui/components/palette"
	"github.com/gbarany/tea-dash/internal/ui/components/prview"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/components/sidebar"
	"github.com/gbarany/tea-dash/internal/ui/components/statusbar"
	"github.com/gbarany/tea-dash/internal/ui/components/tabs"
	"github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
	"github.com/gbarany/tea-dash/internal/ui/layout"
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

	// layout is every rectangle of the framed shell, recomputed by
	// syncMainContentDimensions on every resize/toggle that can change it.
	layout layout.Layout
	// previewFocused routes keys to the preview pane and swaps which panel
	// gets the focused border color. Task 4 wires the toggle (enter/tab,
	// esc); it is always false until then, so the list panel is always the
	// focused one.
	previewFocused bool

	// activeOverlay is which modal (if any) is currently showing —
	// overlayNone | overlayHelp | overlayPalette. While non-zero, Update
	// routes every KeyPressMsg to the overlay first (spec §4), ahead of
	// even the action prompt and search-focus interceptions below. Only
	// one overlay is ever open at a time — openHelpOverlay/openPalette
	// both close whichever is open before opening their own.
	activeOverlay overlayKind
	helpOverlay   helpoverlay.Model
	palette       palette.Model

	actionDispatcher func(actions.Intent) tea.Cmd
	actionPrompt     actionprompt.Model
	pendingAction    actions.Intent
	pendingQuit      bool
	actionFeedback   actionfeedback.Model
	copyToClipboard  func(string) error

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

	// zones is the mouse hit-testing registry (layout.Zones), rebuilt from
	// scratch exactly once per render — see rebuildZones's doc comment for
	// why it's a *pointer* field (View is a value-receiver method; a plain
	// Zones value field would suffer the exact same lost-mutation bug the
	// help overlay's viewport sizing had before it was fixed to size via a
	// pointer-receiver hook) and why registration lives in that one place.
	// handleMouseClick/handleMouseWheel only ever READ it.
	zones *layout.Zones

	// lastClickAt/lastClickRow implement double-click detection (spec §3):
	// two left clicks on the SAME list row within doubleClickWindow count
	// as a double-click. time.Now() (via clockNow, overridable by tests
	// through nowFn) is used because tea.MouseClickMsg carries no
	// timestamp; time.Time's monotonic reading (present on every value
	// time.Now() returns) makes the Sub() comparison safe even if the wall
	// clock is adjusted mid-session.
	lastClickAt  time.Time
	lastClickRow int
	nowFn        func() time.Time

	// pendingRowPalette is the Task 6 seam for spec §3's "right-click list
	// row -> command palette scoped to that row's actions": a right-click
	// on a list row records the clicked row's index here, and
	// openRowPaletteFromPending (called from the same handleZoneRightClick
	// call that sets it) reads and clears it in the same tick — see that
	// function's doc comment. -1 means none pending.
	pendingRowPalette int
}

type actionButton struct {
	Label   string
	Builtin string
}

// overlayKind is which modal (if any) currently owns all key input — see
// Model.activeOverlay.
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlayPalette
)

// paletteScope narrows which items app.go's paletteItems builds: paletteAll
// is the full ":"/"ctrl+p" palette (actions + view jumps + sections +
// custom commands); paletteRowActions is the right-click scope (spec §3:
// "row -> command palette scoped to that row's actions"), action items
// only.
type paletteScope int

const (
	paletteAll paletteScope = iota
	paletteRowActions
)

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

type reviewersLoadedMsg struct {
	intent    actions.Intent
	reviewers []data.User
	err       error
}

type mergeCapabilitiesLoadedMsg struct {
	intent       actions.Intent
	capabilities data.MergeCapabilities
	err          error
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

	// MockHost and InstanceHost feed the header's right-side host label
	// (spec §5). main.go sets MockHost ("demo.gitea.local") on the --mock
	// path and InstanceHost (the real instance URL's host) otherwise.
	MockHost     string
	InstanceHost string
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
	iconSet := icons.Unicode
	if cfg != nil {
		iconSet = icons.Parse(cfg.Theme.Icons)
	}
	ctx := &context.ProgramContext{
		Config:         cfg,
		Client:         client,
		User:           user,
		View:           view,
		PreviewOpen:    previewOpen,
		Styles:         context.StylesForConfig(cfg),
		Icons:          iconSet,
		CurrentRepo:    opts.CurrentRepo,
		SmartFiltering: opts.SmartFiltering,
		MockHost:       opts.MockHost,
		InstanceHost:   opts.InstanceHost,
	}
	ctx.StartTask = func(t context.Task) tea.Cmd {
		tasks[t.Id] = t
		return nil
	}

	keys := defaultKeyMap()
	keys.applyConfig(cfg)
	m := Model{
		ctx:               ctx,
		keys:              keys,
		tabs:              tabs.New(ctx),
		sidebar:           sidebar.New(ctx),
		helpOverlay:       helpoverlay.New(ctx),
		palette:           palette.New(ctx),
		tasks:             tasks,
		actionPrompt:      actionprompt.New(),
		actionFeedback:    actionfeedback.New(),
		copyToClipboard:   writeClipboard,
		pullDetails:       map[string]*data.PullDetail{},
		issueDetails:      map[string]*data.IssueDetail{},
		pullEnrichErr:     map[string]error{},
		issueEnrichErr:    map[string]error{},
		actionDetails:     map[string]*data.ActionRunDetail{},
		actionEnrichErr:   map[string]error{},
		zones:             &layout.Zones{},
		pendingRowPalette: -1,
	}
	// Build only the starting view's sections; the other slice stays nil until
	// the first switch (lazy build). setCurrentViewSections wires the tab bar.
	m.setCurrentViewSections(buildSections(view, ctx))
	// Compute an initial layout (even at the zero ScreenWidth/Height a
	// fresh Model starts with) so m.layout is never the Layout zero value —
	// View() branches on m.layout.TooSmall before the first WindowSizeMsg
	// ever arrives in some tests.
	m.syncMainContentDimensions()
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
	// An open overlay (help or the command palette) gets first routing
	// priority — ahead of even the action prompt / search-focus
	// interceptions below (spec §4: "overlay intercepts all keys while
	// open"). Non-key messages (resize, async fetches, spinner ticks, ...)
	// still flow through the normal switch beneath this, so the overlay
	// stays live/responsive to a resize while open.
	if key, ok := msg.(tea.KeyPressMsg); ok && m.activeOverlay != overlayNone {
		return m.updateOverlay(key)
	}
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
		return m, m.setError(fmt.Sprintf("Couldn't open browser: %v — copy: %s", msg.err, msg.url))

	case copyResultMsg:
		if msg.err != nil {
			return m, m.setError(fmt.Sprintf("Couldn't copy: %v", msg.err))
		}
		return m, m.setSuccess(fmt.Sprintf("Copied %s.", msg.value))

	case reviewersLoadedMsg:
		return m.handleReviewersLoaded(msg)

	case mergeCapabilitiesLoadedMsg:
		return m.handleMergeCapabilitiesLoaded(msg)

	case actions.ResultMsg:
		var feedbackCmd tea.Cmd
		m.actionFeedback, feedbackCmd = m.actionFeedback.Set(feedbackFromActionResult(msg))
		if msg.Status == actions.ResultSucceeded {
			m.clearPreviewCacheForAction(msg.Intent.Target)
			if s := m.getCurrSection(); s != nil &&
				s.GetId() == msg.Intent.Target.SectionID &&
				s.GetType() == msg.Intent.Target.SectionType {
				if m.ctx.PreviewOpen {
					m.syncSidebar()
				}
				return m, tea.Batch(feedbackCmd, s.FetchRows(), m.enrichCurrRow())
			}
		}
		return m, feedbackCmd

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
			return m, m.setError(fmt.Sprintf("Couldn't load PR checks: %v", msg.err))
		}
		return m.switchToPullChecks(msg.row, msg.detail.HeadRef, msg.detail.HeadSHA)

	case actionfeedback.ExpireMsg:
		// Delivered by the tea.Cmd a Success/Info/Cancel Set() returned;
		// Expire ignores it if a newer Set has since superseded that
		// generation (see actionfeedback.Model.Expire's doc comment).
		m.actionFeedback = m.actionFeedback.Expire(msg.Gen)
		return m, nil

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
		// Any key dismisses an Error toast (spec §6); Success/Info/Cancel
		// dismiss themselves via their own expiry tick instead — see
		// actionfeedback.Model.DismissError's doc comment.
		m.actionFeedback = m.actionFeedback.DismissError()
		if b, ok := m.matchCustomKeybinding(msg); ok {
			return m, m.startCustomCommand(b)
		}
		if b, ok := m.matchBuiltinKeybinding(msg); ok {
			if next, cmd, handled := m.handleBuiltinKeybinding(b); handled {
				return next, cmd
			}
		}
		// esc: universal dismiss cascade (spec §2's Global "esc" row).
		// Checked before preview-focused routing and the big switch below
		// so it always works, regardless of what's open.
		if key.Matches(msg, m.keys.Esc) {
			next, cmd := m.dismissTop()
			return next, cmd
		}
		// While the preview is focused, scroll/tab keys route to it first
		// (spec §2's "Preview (focused)" row: j/k/d/u/g/G scroll, [/] tabs)
		// — everything else (view jumps, tab/enter to unfocus, global
		// actions, ...) is left unhandled by sidebar.Update and falls
		// through to the normal routing below unchanged.
		if m.previewFocused {
			if next, cmd, handled := m.sidebar.Update(msg); handled {
				m.sidebar = next
				return m, cmd
			}
		}
		switch {
		case !m.scopedBuiltinOverridden("viewpulls") && key.Matches(msg, m.keys.ViewPulls):
			return m, m.switchToView(context.PullsView)
		case !m.scopedBuiltinOverridden("viewissues") && key.Matches(msg, m.keys.ViewIssues):
			return m, m.switchToView(context.IssuesView)
		case !m.scopedBuiltinOverridden("viewnotifications") && key.Matches(msg, m.keys.ViewNotifications):
			return m, m.switchToView(context.NotificationsView)
		case !m.scopedBuiltinOverridden("viewactions") && key.Matches(msg, m.keys.ViewActions):
			return m, m.switchToView(context.ActionsView)
		case !m.scopedBuiltinOverridden("viewbranches") && key.Matches(msg, m.keys.ViewBranches):
			return m, m.switchToView(context.BranchesView)
		// Branches view keeps enter meaning checkout (its rows have no
		// preview drill-in target of their own); FocusPreview's binding
		// falls back to tab there — this is the only view-specific enter
		// exception (spec §2's Branches footnote). Checked before the
		// generic FocusPreview case below so tab (which doesn't match
		// tea.KeyEnter) still falls through to toggle focus normally.
		case !m.scopedBuiltinOverridden("checkout") && m.ctx.View == context.BranchesView && msg.Code == tea.KeyEnter:
			return m, m.startAction(actions.KindSwitchBranch)
		case !m.scopedBuiltinOverridden("focuspreview") && key.Matches(msg, m.keys.FocusPreview) && m.previewVisible():
			m.previewFocused = !m.previewFocused
			return m, nil
		case !m.scopedBuiltinOverridden("up") && key.Matches(msg, m.keys.Up):
			return m.updateCurrentSectionWithPreview(tea.KeyPressMsg{Code: tea.KeyUp})
		case !m.scopedBuiltinOverridden("down") && key.Matches(msg, m.keys.Down):
			return m.updateCurrentSectionWithPreview(tea.KeyPressMsg{Code: tea.KeyDown})
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
		case !m.scopedBuiltinOverridden("logs") && m.ctx.View == context.ActionsView && key.Matches(msg, m.keys.ViewLogs):
			return m, m.startAction(actions.KindViewLogs)
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
		case !m.scopedBuiltinOverridden("requestReview") && !m.scopedBuiltinOverridden("requestReviewer") &&
			!m.scopedBuiltinOverridden("requestReviewers") && key.Matches(msg, m.keys.RequestReviewers):
			return m, m.startAction(actions.KindRequestReviewers)
		case !m.scopedBuiltinOverridden("removeReview") && !m.scopedBuiltinOverridden("removeReviewer") &&
			!m.scopedBuiltinOverridden("removeReviewers") && !m.scopedBuiltinOverridden("removeRequestedReviewers") &&
			key.Matches(msg, m.keys.RemoveReviewers):
			return m, m.startAction(actions.KindRemoveReviewers)
		case !m.scopedBuiltinOverridden("push") && m.ctx.View == context.BranchesView && key.Matches(msg, m.keys.PushBranch):
			return m, m.startAction(actions.KindPushBranch)
		case !m.scopedBuiltinOverridden("forcePush") && m.ctx.View == context.BranchesView && key.Matches(msg, m.keys.ForcePushBranch):
			return m, m.startAction(actions.KindForcePushBranch)
		case !m.scopedBuiltinOverridden("fastForward") && m.ctx.View == context.BranchesView && key.Matches(msg, m.keys.FastForwardBranch):
			return m, m.startAction(actions.KindFastForwardBranch)
		case !m.scopedBuiltinOverridden("delete") && m.ctx.View == context.BranchesView && key.Matches(msg, m.keys.DeleteBranch):
			return m, m.startAction(actions.KindDeleteBranch)
		case !m.scopedBuiltinOverridden("diff") && key.Matches(msg, m.keys.ExternalDiff):
			return m, m.startAction(actions.KindExternalDiff)
		case !m.scopedBuiltinOverridden("checkout") && key.Matches(msg, m.keys.Checkout):
			if m.ctx.View == context.BranchesView {
				return m, m.startAction(actions.KindSwitchBranch)
			}
			return m, m.startAction(actions.KindCheckout)
		case !m.scopedBuiltinOverridden("quit") && key.Matches(msg, m.keys.Quit):
			return m.quitOrConfirm()
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
			m.openHelpOverlay()
			return m, nil
		case !m.scopedBuiltinOverridden("palette") && key.Matches(msg, m.keys.Palette):
			return m, m.openPalette(paletteAll)
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
			m.previewFocused = false // nothing left to focus
			return m, nil
		case !m.scopedBuiltinOverridden("toggleSmartFiltering") && key.Matches(msg, m.keys.ToggleSmart):
			return m.toggleSmartFiltering()
		case !m.scopedBuiltinOverridden("expand") && key.Matches(msg, m.keys.Expand):
			if m.ctx.PreviewOpen {
				m.expanded = !m.expanded
				m.syncSidebar()
			}
			return m, nil
		}
	}

	// Fall-through: forward to the current section (row navigation, etc.). When
	// the preview is open, moving the cursor to a new row re-renders the pane and
	// lazily fetches that row's detail.
	return m.updateCurrentSectionWithPreview(msg)
}

// View renders the framed full-space shell (spec §1): a one-row header
// embedded in the top border, the list/preview panels as bordered boxes
// with their tabs embedded in their own top borders, and a one-row status
// bar embedded in the bottom border. Below the 40x10 floor it renders a
// centered "too small" notice instead (see layout.Compute).
func (m Model) View() tea.View {
	l := m.layout
	var content string
	if l.TooSmall {
		// Nothing is clickable below the floor (just a centered notice) —
		// clear the registry rather than leave stale zones from the last
		// normal-size render sitting around matching bogus coordinates.
		m.zones.Reset()
		content = tooSmallNotice(l.Full, m.ctx.Styles)
	} else {
		content = m.renderShell(l)
	}
	return tea.View{Content: content, AltScreen: true, MouseMode: tea.MouseModeCellMotion}
}

// renderShell builds every row of the non-TooSmall shell. It guarantees
// exactly l.Full.H rows of exactly l.Full.W cells each: every interior
// block (section body, sidebar, help/prompt overlay) is passed through
// fitBlock, and header/statusbar guarantee their own exact width.
func (m Model) renderShell(l layout.Layout) string {
	styles := m.ctx.Styles

	// rebuildZones is the ONE place mouse hit-testing zones are ever
	// written (see its doc comment) — every render recomputes them fresh
	// from this exact geometry before anything below reads from
	// m.tabs/m.sidebar, so the zones agree with what's about to be drawn.
	m.rebuildZones(l)

	host := m.ctx.MockHost
	if host == "" {
		host = m.ctx.InstanceHost
	}
	headerRow := header.View(l.Header.W, m.ctx.View, host, m.ctx.User, styles)

	showPreview := l.PreviewPanel.W > 0 && l.PreviewPanel.H > 0

	// A previewFocused=true with no visible preview panel to actually focus
	// (closed, or auto-collapsed by layout.Compute on a narrow terminal)
	// would otherwise leave the list panel drawn as unfocused/dim even
	// though it's the only panel on screen — previewVisible()/the
	// togglePreview handlers keep previewFocused false in that state, but
	// this is a second, cheap belt-and-braces check right where it's drawn.
	focused := m.previewFocused && showPreview
	listStyle := styles.BorderBlurred
	if !focused {
		listStyle = styles.BorderFocused
	}
	previewStyle := styles.BorderBlurred
	if focused {
		previewStyle = styles.BorderFocused
	}

	rows := make([]string, 0, l.ListPanel.H+2)
	rows = append(rows, headerRow)

	// An action prompt or an open overlay (help, the command palette)
	// replaces the list+preview area with one full-width panel — the
	// plan's Task 5 explicitly allows this simpler "full interior
	// replacement" over a true lipgloss.Place-centered floating modal, and
	// it renders cleanly here, so that's the choice made. Using the
	// combined width — instead of squeezing into the narrower list-only
	// interior — avoids word-wrap breaking a short multi-word phrase (e.g.
	// "R refresh all") across two lines, and gives the help overlay's
	// viewport a reasonably wide column for its two-column key/desc rows.
	if overlay, ok := m.overlayContent(); ok {
		fullPanel := overlayFullPanel(l)
		interior := overlayInterior(l)
		body := fitBlock(overlay, interior.W, interior.H)
		rows = append(rows, renderPanel(fullPanel, interior, m.tabs.View(), body, listStyle, listBorders(), true)...)
	} else {
		listBody := fitBlock(m.listInteriorContent(), l.ListInterior.W, l.ListInterior.H)
		// drawRightBorder=!showPreview: when the preview follows, its own
		// left border is the shared seam (see renderPanel's doc comment).
		listRows := renderPanel(l.ListPanel, l.ListInterior, m.tabs.View(), listBody, listStyle, listBorders(), !showPreview)
		if showPreview {
			previewBody := fitBlock(m.sidebar.View(), l.PreviewInterior.W, l.PreviewInterior.H)
			previewRows := renderPanel(l.PreviewPanel, l.PreviewInterior, m.sidebar.TabsBorderSegment(), previewBody, previewStyle, previewBorders(), true)
			for i := range listRows {
				rows = append(rows, listRows[i]+previewRows[i])
			}
		} else {
			rows = append(rows, listRows...)
		}
	}

	rows = append(rows, m.statusBarRow(l))
	return strings.Join(rows, "\n")
}

// rebuildZones repopulates m.zones (a pointer field — see its doc comment)
// with every mouse hit-testable region for the CURRENT frame, in Z-order
// (later Add calls win overlapping hits, per layout.Zones.Hit). This is the
// single place zones are ever written: handleMouseClick/handleMouseWheel
// (Update) only ever call m.zones.Hit, relying on it reflecting the most
// recently rendered frame — true in the live bubbletea loop, since View()
// always runs immediately after the Update that could have changed
// geometry (selecting a row, switching sections, data arriving, ...); a
// unit test that changes state without an intervening m.View() call is
// exercising a sequence that can't happen for real, so mouse tests call
// View() once to render before dispatching a mouse message, exactly
// mirroring that real ordering.
func (m Model) rebuildZones(l layout.Layout) {
	m.zones.Reset()
	m.zones.Add(layout.ZoneStatusBar, l.StatusBar, 0)

	host := m.ctx.MockHost
	if host == "" {
		host = m.ctx.InstanceHost
	}
	for _, lr := range header.Labels(l.Header.W, m.ctx.View, host, m.ctx.User, m.ctx.Styles) {
		m.zones.Add(layout.ZoneViewLabel,
			layout.Rect{X: lr.Start, Y: l.Header.Y, W: lr.End - lr.Start, H: 1},
			int(lr.View))
	}

	// The action prompt swallows all mouse input unconditionally (see
	// handleMouseClick) — no zones needed for that region. An open overlay
	// (help, the command palette) replaces list+preview with one
	// clickable region so spec §3's "click outside dismisses" has a
	// single rect to test against instead of "didn't hit any of several
	// other zones". The palette additionally layers a ZonePaletteItem zone
	// per visible row on top of that background region (same Z-order
	// trick as ZoneListRow over ZoneListBody below), so a click on an item
	// resolves to it instead of the generic "click inside overlay, no-op"
	// background — see handleMouseClick.
	if m.actionPrompt.Active() {
		return
	}
	if m.activeOverlay != overlayNone {
		fullPanel := overlayFullPanel(l)
		m.zones.Add(layout.ZoneOverlay, fullPanel, 0)
		if m.activeOverlay == overlayPalette {
			interior := overlayInterior(l)
			top := interior.Y + palette.HeaderRows
			items, _ := m.palette.Visible()
			for i := range items {
				y := top + i
				if y >= interior.Y+interior.H {
					break
				}
				m.zones.Add(layout.ZonePaletteItem, layout.Rect{X: interior.X, Y: y, W: interior.W, H: 1}, i)
			}
		}
		return
	}

	m.zones.Add(layout.ZoneListBody, l.ListInterior, 0)
	sectionTabsOriginX := l.ListPanel.X + 1
	for _, r := range m.tabs.Ranges() {
		m.zones.Add(layout.ZoneSectionTab,
			layout.Rect{X: sectionTabsOriginX + r.Start, Y: l.SectionTabsRow, W: r.End - r.Start, H: 1},
			r.Index)
	}
	if s := m.getCurrSection(); s != nil && !s.GetIsLoading() && s.GetError() == nil {
		top, height := l.ListRows.Y, l.ListRows.H
		for i := 0; i < s.NumRows() && i < height; i++ {
			m.zones.Add(layout.ZoneListRow,
				layout.Rect{X: l.ListRows.X, Y: top + i, W: l.ListRows.W, H: 1},
				i)
		}
	}

	showPreview := l.PreviewPanel.W > 0 && l.PreviewPanel.H > 0
	if !showPreview {
		return
	}
	m.zones.Add(layout.ZonePreviewBody, l.PreviewInterior, 0)
	previewTabsOriginX := l.PreviewPanel.X + 1
	for _, r := range m.sidebar.TabRanges() {
		m.zones.Add(layout.ZonePreviewTab,
			layout.Rect{X: previewTabsOriginX + r.Start, Y: l.PreviewTabsRow, W: r.End - r.Start, H: 1},
			r.Index)
	}
}

// overlayFullPanel is the single full-width/full-height rect an open
// overlay (or the action prompt) replaces the list+preview area with —
// shared by renderShell (which draws it) and rebuildZones (which registers
// ZoneOverlay over it), so the two never disagree about its bounds.
func overlayFullPanel(l layout.Layout) layout.Rect {
	return layout.Rect{X: 0, Y: l.ListPanel.Y, W: l.ListPanel.W + l.PreviewPanel.W, H: l.ListPanel.H}
}

// overlayInterior strips overlayFullPanel's own border to the content rect
// an overlay's View() is fitBlock'd into — the same rect rebuildZones
// anchors the palette's per-item ZonePaletteItem zones to (see there),
// since the geometry must agree exactly with what's actually drawn.
func overlayInterior(l layout.Layout) layout.Rect {
	fp := overlayFullPanel(l)
	return layout.Rect{X: fp.X + 1, Y: fp.Y + 1, W: fp.W - 2, H: fp.H - 2}
}

// overlayContent returns the content that should replace the list/preview
// area — the active action prompt, the open help overlay, or the open
// command palette — and whether any of those is currently showing.
func (m Model) overlayContent() (string, bool) {
	if m.actionPrompt.Active() {
		fullWidth := m.layout.ListPanel.W + m.layout.PreviewPanel.W - 2
		if fullWidth < 0 {
			fullWidth = 0
		}
		return m.actionPrompt.View(fullWidth), true
	}
	if m.activeOverlay == overlayHelp {
		// Sized to this same full-width/full-height interior by
		// resizeHelpOverlay (see its doc comment for why that has to
		// happen on the persisted model in syncMainContentDimensions,
		// not lazily here).
		return m.helpOverlay.View(), true
	}
	if m.activeOverlay == overlayPalette {
		// Sized by resizePalette, same reasoning as resizeHelpOverlay.
		return m.palette.View(), true
	}
	return "", false
}

// listInteriorContent is the list panel's normal (non-overlay) body.
func (m Model) listInteriorContent() string {
	if s := m.getCurrSection(); s != nil {
		return s.View()
	}
	return ""
}

// statusBarRow renders the bottom border/status row: left is transient
// action feedback, middle is section status counts, right is a short
// key-hint line ending in "? help · q quit" (spec §1). helpLine()'s compact
// form is still too long for a single status-bar segment shared with the
// other two — the full keymap renders in the help overlay instead (see
// overlayContent); statusHints is a separate, deliberately short line just
// for this row.
func (m Model) statusBarRow(l layout.Layout) string {
	return statusbar.View(l.StatusBar.W, m.statusLeftSegment(), m.statusLine(), m.statusHints(), m.ctx.Styles)
}

// statusHints is the status bar's right segment: short, global, and always
// ends in the help/palette/quit hint (spec §1's mockup: "? help · : palette
// · q quit").
func (m Model) statusHints() string {
	hints := []string{"p preview"}
	if m.ctx.CurrentRepo != "" {
		hints = append(hints, "t current repo")
	}
	hints = append(hints, fmt.Sprintf("%s help", keyHelp(m.keys.Help, "?")))
	hints = append(hints, fmt.Sprintf("%s palette", keyHelp(m.keys.Palette, ":")))
	hints = append(hints, fmt.Sprintf("%s quit", keyHelp(m.keys.Quit, "q")))
	return strings.Join(hints, " · ")
}

// statusLeftSegment renders the toast (Task 8's actionfeedback, which
// merged in the old separate `notice` field) pre-styled — statusbar.View
// renders it as-is rather than re-wrapping it in the status bar's base
// style, so its StatusToast*+icon coloring survives. Reads the configured
// theme.icons set from ctx (Task 9; previously hardcoded to icons.Unicode).
func (m Model) statusLeftSegment() string {
	if m.actionFeedback.Empty() {
		return ""
	}
	return m.actionFeedback.View(60, m.ctx.Styles, m.ctx.Icons)
}

// tooSmallNotice centers a "terminal too small" message in the full
// terminal rect, used below the 40x10 floor instead of the framed shell.
func tooSmallNotice(full layout.Rect, styles context.Styles) string {
	if full.H <= 0 {
		return ""
	}
	w := full.W
	if w < 0 {
		w = 0
	}
	if w == 0 {
		return strings.Repeat("\n", full.H-1)
	}
	msg := styles.ErrorText.Render(fmt.Sprintf("terminal too small (%dx%d, need 40x10)", full.W, full.H))
	return lipgloss.Place(w, full.H, lipgloss.Center, lipgloss.Center, msg)
}

// panelBorders is the four corner runes a bordered panel draws, which
// depend only on whether a panel to the right is also shown (this shell
// never has more than list+preview side by side).
type panelBorders struct {
	topLeft, topRight, bottomLeft, bottomRight string
}

// listBorders is the list panel's corners: "├" on the left always (it
// always starts at column 0, directly below the header's left run), "┤" on
// the right — only actually drawn when the list panel is alone (no
// preview): see renderPanel's drawRightBorder. When a preview follows,
// the seam between them is the preview's own left border (previewBorders'
// "┬"/"┴"/"│") instead of a second, redundant border column.
func listBorders() panelBorders {
	return panelBorders{topLeft: "├", topRight: "┤", bottomLeft: "├", bottomRight: "┤"}
}

// previewBorders is the preview panel's corners: always follows a list
// panel to its left ("┬"/"┴") and always closes the frame on the right
// ("┤"/"┤").
func previewBorders() panelBorders {
	return panelBorders{topLeft: "┬", topRight: "┤", bottomLeft: "┴", bottomRight: "┤"}
}

// renderPanel draws one full bordered panel — its own top border (carrying
// tabsSegment, embedded per spec §1), bordered interior rows, and its own
// bottom border — as exactly panel.H rows of exactly panel.W cells each.
// body must already be fitBlock'd to interior.W x interior.H.
//
// drawRightBorder controls the panel's own right edge (top/mid/bottom):
// when false, it's left as plain blank space instead of a border rune —
// used for the list panel when a preview panel follows immediately to its
// right, so the two panels share a SINGLE visible seam (the preview's own
// left border) rather than two adjacent border columns. layout.Compute's
// ListPanel/PreviewPanel rects are non-overlapping by contract (their own
// golden tests assert ListPanel.W+PreviewPanel.W==W with no shared
// column), so this reclaimed column is real width the list panel owns but
// doesn't use for content — ListInterior stays exactly what layout says,
// and the column renders as a 1-cell blank gap rather than extra content
// (avoiding a mismatch with MainContentWidth, which the table is sized to).
func renderPanel(panel, interior layout.Rect, tabsSegment, body string, style lipgloss.Style, b panelBorders, drawRightBorder bool) []string {
	rightTop, rightMid, rightBottom := style.Render(b.topRight), style.Render("│"), style.Render(b.bottomRight)
	if !drawRightBorder {
		rightTop, rightMid, rightBottom = " ", " ", " "
	}
	rows := make([]string, 0, panel.H)
	rows = append(rows, style.Render(b.topLeft)+embedInBorderRow(tabsSegment, interior.W, style)+rightTop)
	for _, line := range strings.Split(body, "\n") {
		rows = append(rows, style.Render("│")+line+rightMid)
	}
	rows = append(rows, style.Render(b.bottomLeft)+embedInBorderRow("", interior.W, style)+rightBottom)
	return rows
}

// embedInBorderRow renders a w-cell border row: a plain dash rule when
// segment is "" (no tabs to embed), otherwise segment (already styled —
// e.g. components/tabs or sidebar.TabsBorderSegment's embedding format)
// followed by dash fill out to w.
func embedInBorderRow(segment string, w int, style lipgloss.Style) string {
	if w <= 0 {
		return ""
	}
	if segment == "" {
		return style.Render(strings.Repeat("─", w))
	}
	segW := lipgloss.Width(segment)
	if segW >= w {
		return lipgloss.NewStyle().MaxWidth(w).Render(segment)
	}
	return segment + style.Render(strings.Repeat("─", w-segW))
}

// tabWidth is how many spaces fitBlock expands a literal tab character
// into before measuring/padding. A fixed expansion (rather than aligning to
// real tab stops, which would need to know the tab's starting column) is
// simple, deterministic, and enough to keep the interior's right border
// from floating.
const tabWidth = 4

// expandTabs replaces literal tabs with spaces before any width
// measurement: lipgloss.Width counts a tab as exactly 1 cell, but a real
// terminal expands it to the next tab stop — typically several columns —
// so a tab-carrying line's measured width undercounts its actual rendered
// width. Content that reaches fitBlock with raw tabs still in it (e.g.
// prview surfacing CI log lines verbatim, like
// "ok  \tgithub.com/x\t0.211s") would otherwise get padded short, leaving
// the panel's right border rune shifted left of where the terminal
// actually draws it once tabs expand.
func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth))
}

// fitBlock pads or truncates content to exactly w columns by h rows, so
// every panel interior contributes a fixed number of same-width rows to
// the frame regardless of what it renders (a loading spinner, an error
// block, a handful of table rows, or wrapped overlay text) — the shell's
// exact total row/column count depends on it.
func fitBlock(content string, w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	content = expandTabs(content)
	wrapped := lipgloss.NewStyle().Width(w).Render(content)
	lines := strings.Split(wrapped, "\n")
	out := make([]string, h)
	for i := 0; i < h; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out[i] = padOrTruncateLine(line, w)
	}
	return strings.Join(out, "\n")
}

func padOrTruncateLine(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = lipgloss.NewStyle().MaxWidth(w).Render(s)
	}
	if lw := lipgloss.Width(s); lw < w {
		s += strings.Repeat(" ", w-lw)
	}
	return s
}

// helpLine is the current view's compact key-hint text (helpLineShort).
// It used to have an expanded form too, toggled by showHelp and rendered
// as a one-line list-panel overlay — Task 5's help overlay (see
// overlayContent, helpoverlay.Model) replaces that entirely with a proper
// scrollable modal generated from keyMap.Groups(view), so only the compact
// form is left. Kept as its own function (rather than inlining
// helpLineShort's body here) because tests call it directly to sanity
// check per-view shortcut text.
func (m Model) helpLine() string {
	return m.helpLineShort()
}

func (m Model) helpLineShort() string {
	text := "↑/↓/j/k move · g/G first/last · h/l section · s view · / search · p preview · [/] tabs · r/R refresh"
	if m.ctx.CurrentRepo != "" {
		text += " · t current repo"
	}
	switch m.ctx.View {
	case context.ActionsView:
		text = "↑/↓/j/k move · g/G first/last · h/l section · s view · / search · p preview · [/] tabs · r refresh · R rerun · ! cancel"
		if m.ctx.CurrentRepo != "" {
			text += " · t current repo"
		}
	case context.NotificationsView:
		text += " · m read · u unread · M all read · b pin · B unpin"
	case context.BranchesView:
		text += " · C/space switch · P push · f/F sync · d delete"
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

// availableActions lists the builtin actions valid for view/row — the same
// per-view/state logic the deleted action-bar row used to render as
// clickable buttons. Extracted (rather than deleted with the action bar) so
// Task 7's command palette can reuse it as its item source.
func availableActions(view context.ViewType, row data.RowData) []actionButton {
	buttons := []actionButton{
		{Label: "Open", Builtin: "open"},
		{Label: "Refresh", Builtin: "refresh"},
	}
	switch view {
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
			actionButton{Label: "Logs", Builtin: "viewLogs"},
			actionButton{Label: "Rerun", Builtin: "rerun"},
			actionButton{Label: "Cancel", Builtin: "cancel"},
		)
	case context.BranchesView:
		buttons = []actionButton{
			{Label: "Refresh", Builtin: "refresh"},
			{Label: "Checkout", Builtin: "checkout"},
			{Label: "Fast-forward", Builtin: "fastForward"},
			{Label: "Push", Builtin: "push"},
			{Label: "Force push", Builtin: "forcePush"},
			{Label: "Delete", Builtin: "delete"},
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
			actionButton{Label: "Request review", Builtin: "requestReviewers"},
			actionButton{Label: "Remove reviewers", Builtin: "removeReviewers"},
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
		m.sidebar.SetTabs(sidebarTabs(prview.RenderPullTabs(r, m.pullDetails[key], w, m.expanded, m.ctx.Styles, m.ctx.Icons)))
		return
	case data.Issue:
		if err := m.issueEnrichErr[key]; err != nil {
			m.sidebar.SetContent(m.failedPreview(row, err))
			return
		}
		m.sidebar.SetTabs(sidebarTabs(prview.RenderIssueTabs(r, m.issueDetails[key], w, m.expanded, m.ctx.Styles, m.ctx.Icons)))
		return
	case data.Notification:
		rendered = prview.RenderNotification(r, w, m.ctx.Styles, m.ctx.Icons)
	case data.ActionRun:
		if err := m.actionEnrichErr[key]; err != nil {
			m.sidebar.SetContent(m.failedPreview(row, err))
			return
		}
		m.sidebar.SetTabs(sidebarTabs(prview.RenderActionTabs(r, m.actionDetails[key], w, m.ctx.Styles, m.ctx.Icons)))
		return
	default:
		m.sidebar.SetContent("")
		return
	}
	m.sidebar.SetContent(rendered)
}

func sidebarTabs(tabs []prview.Tab) []sidebar.Tab {
	out := make([]sidebar.Tab, 0, len(tabs))
	for _, t := range tabs {
		out = append(out, sidebar.Tab{Title: t.Title, Content: t.Content})
	}
	return out
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
		return m, m.setInfo("Select a pull request to watch checks.")
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
		return m, m.setError(fmt.Sprintf("Can't watch checks for invalid repo %q.", row.RepoNameWithOwner))
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
		return m, m.setError(fmt.Sprintf("Can't watch checks for invalid repo %q.", row.RepoNameWithOwner))
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
	var feedbackCmd tea.Cmd
	if branch == "" && sha == "" {
		feedbackCmd = m.setInfo(fmt.Sprintf("Showing Actions for %s; PR head was not available to narrow checks.", row.RepoNameWithOwner))
	} else {
		feedbackCmd = m.setInfo(fmt.Sprintf("Watching checks for %s#%d.", row.RepoNameWithOwner, row.Number))
	}
	if m.ctx.PreviewOpen {
		m.syncSidebar()
	}
	if s := m.getCurrSection(); s != nil {
		return m, tea.Batch(feedbackCmd, s.FetchRows())
	}
	return m, feedbackCmd
}

// switchView cycles pulls -> issues -> notifications -> actions -> branches, lazily
// building and fetching the target view's sections on first visit.
func (m *Model) switchView() tea.Cmd {
	var next context.ViewType
	switch m.ctx.View {
	case context.PullsView:
		next = context.IssuesView
	case context.IssuesView:
		next = context.NotificationsView
	case context.NotificationsView:
		next = context.ActionsView
	case context.ActionsView:
		next = context.BranchesView
	default:
		next = context.PullsView
	}
	return m.switchToView(next)
}

func (m *Model) switchToView(view context.ViewType) tea.Cmd {
	if m.ctx.View == view {
		return nil
	}
	m.ctx.View = view
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

func (m Model) selectCurrentSectionRow(i int) (Model, tea.Cmd) {
	s := m.getCurrSection()
	if s == nil || s.GetIsLoading() || s.GetError() != nil || s.NumRows() == 0 {
		return m, nil
	}
	before := m.selKey()
	s.SelectRow(i)
	moreCmd := s.MaybeFetchNextPage()
	if !m.ctx.PreviewOpen || m.selKey() == before {
		return m, moreCmd
	}
	m.syncSidebar()
	return m, tea.Batch(moreCmd, m.enrichCurrRow())
}

// doubleClickWindow is spec §3's double-click threshold: two left clicks on
// the same list row within this long count as one double-click.
const doubleClickWindow = 400 * time.Millisecond

// clockNow returns the current time for double-click timing, via nowFn when
// a test has set one (tea.MouseClickMsg carries no timestamp of its own, so
// there's nothing to read the real click time from) — real usage always
// gets time.Now().
func (m Model) clockNow() time.Time {
	if m.nowFn != nil {
		return m.nowFn()
	}
	return time.Now()
}

// handleMouseClick is the single zoneAt(x, y) dispatch every click resolves
// through (spec §3's table): an open overlay or the action prompt claims
// all mouse input first — action prompt: unconditionally swallowed,
// unchanged from its pre-Task-6 behavior; overlay: a click on a
// ZonePaletteItem runs that item (the palette's own click-to-run — see
// its doc comment on the "compute from the rect" vs. "one zone per item"
// choice), a click elsewhere inside the overlay's background
// (ZoneOverlay) is a no-op, and a click outside either dismisses.
// Otherwise the hit zone (if any) drives a left- or right-click handler.
func (m Model) handleMouseClick(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if m.actionPrompt.Active() {
		return m, nil
	}
	if m.activeOverlay != overlayNone {
		zone, ok := m.zones.Hit(msg.X, msg.Y)
		if ok && zone.Kind == layout.ZonePaletteItem {
			if item, itemOk := m.palette.ItemAtVisibleIndex(zone.Payload); itemOk {
				m.activeOverlay = overlayNone
				return m.dispatchPaletteItem(item)
			}
			return m, nil
		}
		if ok && zone.Kind == layout.ZoneOverlay {
			return m, nil
		}
		return m.dismissTop()
	}
	zone, ok := m.zones.Hit(msg.X, msg.Y)
	if !ok {
		// A left click that misses every registered zone entirely (e.g. a
		// border gap) must still reset the double-click state, for the same
		// reason handleZoneLeftClick resets it for a non-row zone HIT (T7
		// Step 0, T8 Step 0(a) from the T7 review): otherwise a row click,
		// then a miss, then the SAME row index again within
		// doubleClickWindow reads as a double-click purely by coincidence
		// of timing, since clickListRow only ever compares against the
		// most recent click's time/row.
		if msg.Button == tea.MouseLeft {
			m.lastClickAt = time.Time{}
		}
		return m, nil
	}
	switch msg.Button {
	case tea.MouseLeft:
		return m.handleZoneLeftClick(zone)
	case tea.MouseRight:
		return m.handleZoneRightClick(zone)
	default:
		return m, nil
	}
}

// handleZoneLeftClick dispatches a left click by zone kind (spec §3): view
// label switches view, section tab switches section, a list row selects
// (and double-click focuses/checks out — see clickListRow), a preview tab
// switches the sidebar's tab, and the preview body focuses it. Zones with
// no defined click behavior (list/preview body background, status bar) are
// simply not listed, falling through to the no-op default.
//
// Any click that ISN'T on a list row resets lastClickAt first (T7 Step 0,
// from the T6 review): without this, clicking a row, then a section tab,
// then the SAME row index again within doubleClickWindow would read as a
// double-click on that row purely by coincidence of timing and index,
// even though a tab switch happened in between — clickListRow only ever
// compares against the most recent click's time/row, so it can't tell the
// difference unless something else clears that state first.
func (m Model) handleZoneLeftClick(zone layout.Zone) (Model, tea.Cmd) {
	if zone.Kind != layout.ZoneListRow {
		m.lastClickAt = time.Time{}
	}
	switch zone.Kind {
	case layout.ZoneViewLabel:
		return m, m.switchToView(context.ViewType(zone.Payload))
	case layout.ZoneSectionTab:
		return m.switchSectionTo(zone.Payload)
	case layout.ZoneListRow:
		return m.clickListRow(zone.Payload)
	case layout.ZonePreviewTab:
		m.sidebar.SelectTab(zone.Payload)
		return m, nil
	case layout.ZonePreviewBody:
		return m.focusPreviewIfVisible()
	default:
		return m, nil
	}
}

// handleZoneRightClick implements spec §3's "right click list row -> command
// palette scoped to that row's actions": select the row (consistent with a
// left click), record the intent in pendingRowPalette, and immediately
// consume it via openRowPaletteFromPending — see that function's and the
// field's doc comments for why the set-then-immediately-read round trip
// still goes through the field rather than opening the palette directly.
func (m Model) handleZoneRightClick(zone layout.Zone) (Model, tea.Cmd) {
	if zone.Kind != layout.ZoneListRow {
		return m, nil
	}
	next, cmd := m.selectCurrentSectionRow(zone.Payload)
	next.pendingRowPalette = zone.Payload
	paletteCmd := next.openRowPaletteFromPending()
	return next, tea.Batch(cmd, paletteCmd)
}

// clickListRow selects the clicked row, then checks whether this click and
// the previous one form a double-click on the SAME row within
// doubleClickWindow (spec §3: "double left click list row -> focus preview,
// same as enter"). A detected double-click consumes lastClickAt (reset to
// the zero value) so a third quick click starts a fresh pair rather than
// immediately re-triggering.
func (m Model) clickListRow(row int) (Model, tea.Cmd) {
	next, cmd := m.selectCurrentSectionRow(row)
	now := m.clockNow()
	isDouble := !m.lastClickAt.IsZero() && row == m.lastClickRow && now.Sub(m.lastClickAt) < doubleClickWindow
	if isDouble {
		next.lastClickAt = time.Time{}
		clicked, dblCmd := next.doubleClickRow()
		return clicked, tea.Batch(cmd, dblCmd)
	}
	next.lastClickAt = now
	next.lastClickRow = row
	return next, cmd
}

// doubleClickRow is a double-click's action on the already-selected row:
// the same "focus preview" toggle enter performs (spec §3: "same as
// enter"), except in the Branches view, which keeps enter/double-click
// meaning checkout instead (T4's Branches exception — its rows have no
// preview drill-in target of their own).
func (m Model) doubleClickRow() (Model, tea.Cmd) {
	if m.ctx.View == context.BranchesView {
		return m, m.startAction(actions.KindSwitchBranch)
	}
	return m.focusPreviewIfVisible()
}

func (m Model) focusPreviewIfVisible() (Model, tea.Cmd) {
	if m.previewVisible() {
		m.previewFocused = !m.previewFocused
	}
	return m, nil
}

// handleMouseWheel is the wheel half of the zoneAt dispatch: over the help
// overlay it scrolls the overlay (reusing updateOverlay, the same routing
// keyboard j/k use, so overlay-scroll logic lives in exactly one place);
// over the palette it moves the selection DIRECTLY via
// palette.Model.MoveSelection rather than that same synthetic-j/k trick —
// plain "j"/"k" are ordinary filter characters in the palette (see its
// Update doc comment), so routing a wheel tick through updateOverlay's key
// path would type letters into the query instead of moving the cursor;
// over the list it moves the selection (existing behavior, via the same
// synthetic up/down key messages keyboard nav uses); over the preview it
// scrolls the preview's own viewport regardless of focus (spec §3) without
// touching list selection or previewFocused.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	zone, ok := m.zones.Hit(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	if m.activeOverlay == overlayPalette {
		if zone.Kind != layout.ZoneOverlay && zone.Kind != layout.ZonePaletteItem {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseWheelUp:
			m.palette = m.palette.MoveSelection(-1)
		case tea.MouseWheelDown:
			m.palette = m.palette.MoveSelection(1)
		}
		return m, nil
	}
	if m.activeOverlay != overlayNone {
		if zone.Kind != layout.ZoneOverlay {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseWheelUp:
			return m.updateOverlay(tea.KeyPressMsg{Code: 'k', Text: "k"})
		case tea.MouseWheelDown:
			return m.updateOverlay(tea.KeyPressMsg{Code: 'j', Text: "j"})
		default:
			return m, nil
		}
	}
	switch zone.Kind {
	case layout.ZoneListRow, layout.ZoneListBody:
		switch msg.Button {
		case tea.MouseWheelUp:
			return m.updateCurrentSectionWithPreview(tea.KeyPressMsg{Code: tea.KeyUp})
		case tea.MouseWheelDown:
			return m.updateCurrentSectionWithPreview(tea.KeyPressMsg{Code: tea.KeyDown})
		default:
			return m, nil
		}
	case layout.ZonePreviewBody, layout.ZonePreviewTab:
		var key tea.KeyPressMsg
		switch msg.Button {
		case tea.MouseWheelUp:
			key = tea.KeyPressMsg{Code: tea.KeyUp}
		case tea.MouseWheelDown:
			key = tea.KeyPressMsg{Code: tea.KeyDown}
		default:
			return m, nil
		}
		var cmd tea.Cmd
		m.sidebar, cmd, _ = m.sidebar.Update(key)
		return m, cmd
	default:
		return m, nil
	}
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
		return m, m.setInfo("No matching git remote detected for this Gitea instance.")
	}
	m.ctx.SmartFiltering = !m.ctx.SmartFiltering
	var feedbackCmd tea.Cmd
	if m.ctx.SmartFiltering {
		feedbackCmd = m.setInfo(fmt.Sprintf("Showing current repository: %s.", m.ctx.CurrentRepo))
	} else {
		feedbackCmd = m.setInfo("Showing all repositories.")
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
		return m, tea.Batch(feedbackCmd, m.refreshAllSections())
	default:
		m.syncProgramContext()
		return m, feedbackCmd
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
		return m.setInfo("Select a row before running an action.")
	}
	if err := validateActionTarget(kind, target); err != nil {
		return m.setError(err.Error())
	}
	if kind == actions.KindSwitchBranch {
		branch, ok := m.getCurrRowData().(localgit.Branch)
		if !ok || target.RowKind != actions.RowKindBranch {
			return m.setInfo("Switch branch is only available for local branches.")
		}
		if branch.Current {
			return m.setInfo(fmt.Sprintf("%s is already current in %s.", branch.Name, branch.Repository))
		}
	}
	if kind == actions.KindDeleteBranch {
		branch, ok := m.getCurrRowData().(localgit.Branch)
		if !ok || target.RowKind != actions.RowKindBranch {
			return m.setInfo("Delete branch is only available for local branches.")
		}
		if branch.Current {
			return m.setInfo(fmt.Sprintf("%s is current in %s; switch away before deleting it.", branch.Name, branch.Repository))
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
	if reviewerPickerAction(kind) && m.ctx.Client != nil {
		return tea.Batch(m.setStart("Loading reviewers..."), loadReviewersCmd(m.ctx.Client, intent))
	}
	if kind == actions.KindMerge && m.ctx.Client != nil {
		return tea.Batch(m.setStart("Loading merge options..."), loadMergeCapabilitiesCmd(m.ctx.Client, intent))
	}
	m.actionPrompt = m.actionPrompt.Focus(promptConfigForAction(kind, target))
	return nil
}

func (m *Model) startCustomCommand(binding config.Keybinding) tea.Cmd {
	target, ok := m.selectedActionTarget()
	if !ok {
		return m.setInfo("Select a row before running a custom command.")
	}
	intent := actions.Intent{
		Kind:    actions.KindCustomCommand,
		Target:  target,
		Command: binding.Command,
		Name:    binding.Name,
	}
	return m.dispatchActionIntent(intent)
}

// handleBuiltinKeybinding is one of three parallel builtin-name switches
// (T7/T10 review note — considered consolidating into one shared table,
// deferred: each switch's cases carry a different payload — this one runs
// the actual behavior, keys.go's rebindBuiltin writes a keyMap field, and
// keys.go's bindingForBuiltin reads one back for the palette's key hint —
// and several builtins are asymmetric across them (e.g. "quit"/"redraw"/
// "pageup" dispatch here with no keyMap field to rebind or read at all;
// "firstline"/"lastline" dispatch here and have a keyMap field for the help
// overlay's display but no rebindBuiltin case, since g/G aren't
// user-remappable today). A single shared table would need to model that
// asymmetry (optional keyMap field, optional behavior closure) rather than
// just merging three flat maps, so it stayed a documented three-way
// cross-reference instead of a mechanical merge. normalizeBuiltin is the
// one piece already shared (case/punctuation canonicalization); the alias
// groupings themselves (e.g. "opengithub"/"open"/"openbrowser") are
// independently listed in each switch and must be kept in sync by hand.
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
		next, cmd := m.quitOrConfirm()
		return next, cmd, true
	case "redraw":
		return m, tea.ClearScreen, true
	case "up":
		next, cmd := m.updateCurrentSectionWithPreview(tea.KeyPressMsg{Code: tea.KeyUp})
		return next, cmd, true
	case "down":
		next, cmd := m.updateCurrentSectionWithPreview(tea.KeyPressMsg{Code: tea.KeyDown})
		return next, cmd, true
	case "firstline":
		next, cmd := m.selectCurrentSectionRow(0)
		return next, cmd, true
	case "lastline":
		if s := m.getCurrSection(); s != nil {
			next, cmd := m.selectCurrentSectionRow(s.NumRows() - 1)
			return next, cmd, true
		}
		return m, nil, true
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
	case "viewissues":
		return m, m.switchToView(context.IssuesView), true
	case "viewprs", "viewpulls":
		return m, m.switchToView(context.PullsView), true
	case "viewnotifications", "viewinbox":
		return m, m.switchToView(context.NotificationsView), true
	case "viewactions", "viewci":
		return m, m.switchToView(context.ActionsView), true
	case "viewbranches":
		return m, m.switchToView(context.BranchesView), true
	case "switchview":
		return m, m.switchView(), true
	case "focuspreview", "togglefocus":
		if m.previewVisible() {
			m.previewFocused = !m.previewFocused
		}
		return m, nil, true
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
		m.previewFocused = false // nothing left to focus
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
	case "prevsidebartab", "previoussidebartab":
		// [/] only act while the preview is focused (spec §2) — matches
		// the default-key routing in Update (see previewVisible/previewFocused).
		if m.previewFocused {
			m.sidebar.PrevTab()
		}
		return m, nil, true
	case "nextsidebartab":
		if m.previewFocused {
			m.sidebar.NextTab()
		}
		return m, nil, true
	case "pageup", "scrollup":
		if m.ctx.PreviewOpen {
			var cmd tea.Cmd
			m.sidebar, cmd, _ = m.sidebar.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
			return m, cmd, true
		}
		return m, nil, true
	case "pagedown", "scrolldown":
		if m.ctx.PreviewOpen {
			var cmd tea.Cmd
			m.sidebar, cmd, _ = m.sidebar.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
			return m, cmd, true
		}
		return m, nil, true
	case "copyurl":
		return m, m.copySelectedURL(), true
	case "copynumber":
		return m, m.copySelectedNumber(), true
	case "help":
		m.openHelpOverlay()
		return m, nil, true
	case "palette", "commandpalette":
		return m, m.openPalette(paletteAll), true
	case "markasread", "markread", "markasdone", "markdone":
		next, cmd := m.markSelectedNotificationRead()
		return next, cmd, true
	case "markasunread", "markunread":
		next, cmd := m.markSelectedNotificationUnread()
		return next, cmd, true
	case "markallasread", "markallread", "markallasdone", "markalldone":
		next, cmd := m.markAllNotificationsRead()
		return next, cmd, true
	case "pin", "togglepin", "togglepinned", "togglebookmark":
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
	case "requestreview", "requestreviewer", "requestreviewers":
		return m, m.startAction(actions.KindRequestReviewers), true
	case "removereview", "removereviewer", "removereviewers", "removerequestedreviewers":
		return m, m.startAction(actions.KindRemoveReviewers), true
	case "diff":
		return m, m.startAction(actions.KindExternalDiff), true
	case "checkout":
		if m.ctx.View == context.BranchesView {
			return m, m.startAction(actions.KindSwitchBranch), true
		}
		return m, m.startAction(actions.KindCheckout), true
	case "push":
		return m, m.startAction(actions.KindPushBranch), true
	case "forcepush":
		return m, m.startAction(actions.KindForcePushBranch), true
	case "fastforward":
		return m, m.startAction(actions.KindFastForwardBranch), true
	case "delete":
		return m, m.startAction(actions.KindDeleteBranch), true
	case "rerun", "rerunrun":
		return m, m.startAction(actions.KindRerunRun), true
	case "cancel", "cancelrun":
		return m, m.startAction(actions.KindCancelRun), true
	case "logs", "viewlogs":
		return m, m.startAction(actions.KindViewLogs), true
	default:
		return m, m.setError(fmt.Sprintf("Unknown builtin keybinding %q.", binding.Builtin)), true
	}
}

func (m *Model) updateActionPrompt(msg tea.Msg) tea.Cmd {
	var result actionprompt.Result
	var cmd tea.Cmd
	m.actionPrompt, result, cmd = m.actionPrompt.Update(msg)
	if m.pendingQuit {
		if result.Canceled {
			m.pendingQuit = false
			return cmd
		}
		if result.Submitted {
			m.pendingQuit = false
			if cmd == nil {
				return tea.Quit
			}
			return tea.Batch(cmd, tea.Quit)
		}
		return cmd
	}
	if result.Canceled {
		m.pendingAction = actions.Intent{}
		var feedbackCmd tea.Cmd
		m.actionFeedback, feedbackCmd = m.actionFeedback.Set(actionfeedback.Cancel("Action cancelled."))
		return tea.Batch(cmd, feedbackCmd)
	}
	if !result.Submitted {
		return cmd
	}
	if m.pendingAction.Kind == actions.KindReview && m.pendingAction.Prompt.Value == "" && reviewPromptNeedsBody(result.Value) {
		m.pendingAction.Prompt.Value = result.Value
		m.pendingAction.Prompt.Label = result.Label
		m.actionPrompt = m.actionPrompt.Focus(reviewBodyPromptConfig(m.pendingAction.Target, result.Label))
		return cmd
	}
	if m.pendingAction.Kind == actions.KindMerge && m.pendingAction.Prompt.Value == "" && mergePromptNeedsMessage(result.Value) {
		m.pendingAction.Prompt.Value = result.Value
		m.pendingAction.Prompt.Label = result.Label
		m.actionPrompt = m.actionPrompt.Focus(mergeTitlePromptConfig(m.pendingAction.Target))
		return cmd
	}
	if m.pendingAction.Kind == actions.KindMerge && m.pendingAction.Prompt.Value != "" &&
		mergePromptNeedsMessage(m.pendingAction.Prompt.Value) && m.pendingAction.Prompt.Title == "" {
		m.pendingAction.Prompt.Title = result.Value
		m.actionPrompt = m.actionPrompt.Focus(mergeMessagePromptConfig(m.pendingAction.Target))
		return cmd
	}
	intent := m.pendingAction
	if intent.Kind == actions.KindReview && intent.Prompt.Value != "" && reviewPromptNeedsBody(intent.Prompt.Value) {
		intent.Prompt.Body = result.Value
	} else if intent.Kind == actions.KindMerge && intent.Prompt.Value != "" && mergePromptNeedsMessage(intent.Prompt.Value) {
		intent.Prompt.Body = result.Value
	} else {
		intent.Prompt.Value = result.Value
		intent.Prompt.Label = result.Label
	}
	m.pendingAction = actions.Intent{}
	if m.actionDispatcher == nil {
		return tea.Batch(cmd, m.setError("Action not wired yet."))
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

func (m *Model) handleReviewersLoaded(msg reviewersLoadedMsg) (Model, tea.Cmd) {
	if !reviewerPickerAction(msg.intent.Kind) {
		return *m, nil
	}
	if m.pendingAction.Kind != msg.intent.Kind || m.pendingAction.Target != msg.intent.Target {
		return *m, nil
	}
	m.pendingAction = msg.intent
	if msg.err != nil {
		cmd := m.setError(fmt.Sprintf("Couldn't load reviewers: %v. Enter usernames manually.", msg.err))
		m.actionPrompt = m.actionPrompt.Focus(promptConfigForAction(msg.intent.Kind, msg.intent.Target))
		return *m, cmd
	}
	if len(msg.reviewers) == 0 {
		cmd := m.setInfo("No requestable reviewers found. Enter usernames manually.")
		m.actionPrompt = m.actionPrompt.Focus(promptConfigForAction(msg.intent.Kind, msg.intent.Target))
		return *m, cmd
	}
	cfg := reviewerPickerPromptConfig(msg.intent.Kind, msg.intent.Target, msg.reviewers)
	if len(cfg.Options) == 0 {
		cmd := m.setInfo("No requestable reviewers found. Enter usernames manually.")
		m.actionPrompt = m.actionPrompt.Focus(promptConfigForAction(msg.intent.Kind, msg.intent.Target))
		return *m, cmd
	}
	// The "Loading reviewers…" in-flight toast (started in startAction) is
	// superseded by the picker opening, not by a new toast — Clear, not Set.
	m.clearFeedback()
	m.pendingAction.Prompt.Mode = actions.PromptMultiPicker
	m.actionPrompt = m.actionPrompt.Focus(cfg)
	return *m, nil
}

func (m *Model) handleMergeCapabilitiesLoaded(msg mergeCapabilitiesLoadedMsg) (Model, tea.Cmd) {
	if msg.intent.Kind != actions.KindMerge {
		return *m, nil
	}
	if m.pendingAction.Kind != msg.intent.Kind || m.pendingAction.Target != msg.intent.Target {
		return *m, nil
	}
	m.pendingAction = msg.intent
	caps := msg.capabilities
	var cmd tea.Cmd
	if msg.err != nil {
		cmd = m.setError(fmt.Sprintf("Couldn't load merge settings: %v. Showing default merge options.", msg.err))
		caps = data.DefaultMergeCapabilities()
	} else {
		// The "Loading merge options…" in-flight toast is superseded by the
		// picker opening, not by a new toast — Clear, not Set.
		m.clearFeedback()
	}
	m.actionPrompt = m.actionPrompt.Focus(promptConfigForActionWithMergeCapabilities(msg.intent.Kind, msg.intent.Target, caps))
	return *m, cmd
}

func reviewerPickerAction(kind actions.Kind) bool {
	return kind == actions.KindRequestReviewers || kind == actions.KindRemoveReviewers
}

func loadReviewersCmd(client *gitea.Client, intent actions.Intent) tea.Cmd {
	return func() tea.Msg {
		owner, repo, ok := strings.Cut(intent.Target.Repo, "/")
		if !ok || owner == "" || repo == "" {
			return reviewersLoadedMsg{intent: intent, err: fmt.Errorf("invalid repository %q", intent.Target.Repo)}
		}
		reviewers, err := client.ListReviewers(owner, repo)
		return reviewersLoadedMsg{intent: intent, reviewers: reviewers, err: err}
	}
}

func loadMergeCapabilitiesCmd(client *gitea.Client, intent actions.Intent) tea.Cmd {
	return func() tea.Msg {
		owner, repo, ok := data.SplitOwnerRepo(intent.Target.Repo)
		if !ok {
			return mergeCapabilitiesLoadedMsg{intent: intent, err: fmt.Errorf("invalid repository %q", intent.Target.Repo)}
		}
		caps, err := client.MergeCapabilities(owner, repo)
		return mergeCapabilitiesLoadedMsg{intent: intent, capabilities: caps, err: err}
	}
}

func reviewerPickerPromptConfig(kind actions.Kind, target actions.Target, reviewers []data.User) actionprompt.Config {
	cfg := promptConfigForAction(kind, target)
	cfg.Mode = actionprompt.ModeMultiPicker
	cfg.Placeholder = ""
	cfg.Options = make([]actionprompt.Option, 0, len(reviewers))
	for _, reviewer := range reviewers {
		if strings.TrimSpace(reviewer.Login) == "" {
			continue
		}
		cfg.Options = append(cfg.Options, actionprompt.Option{
			Label: reviewerOptionLabel(reviewer),
			Value: reviewer.Login,
		})
	}
	return cfg
}

func reviewerOptionLabel(user data.User) string {
	fullName := strings.TrimSpace(user.FullName)
	login := strings.TrimSpace(user.Login)
	if fullName == "" {
		return login
	}
	return fmt.Sprintf("%s (%s)", fullName, login)
}

func reviewPromptNeedsBody(value string) bool {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "approve", "approved":
		return false
	default:
		return true
	}
}

func mergePromptNeedsMessage(value string) bool {
	for _, part := range strings.Split(strings.ToLower(value), "+") {
		switch strings.TrimSpace(part) {
		case "message", "edit-message", "edit_message":
			return true
		}
	}
	return false
}

func reviewBodyPromptConfig(target actions.Target, label string) actionprompt.Config {
	if label == "" {
		label = "Review"
	}
	return actionprompt.Config{
		Mode:        actionprompt.ModeText,
		Title:       fmt.Sprintf("Review message #%d", target.Number),
		Message:     fmt.Sprintf("%s - %s", target.Repo, target.Title),
		Placeholder: fmt.Sprintf("%s message", label),
	}
}

func mergeTitlePromptConfig(target actions.Target) actionprompt.Config {
	return actionprompt.Config{
		Mode:        actionprompt.ModeText,
		Title:       fmt.Sprintf("Merge title #%d", target.Number),
		Message:     fmt.Sprintf("%s - %s", target.Repo, target.Title),
		Placeholder: "Merge title",
		Initial:     target.Title,
	}
}

func mergeMessagePromptConfig(target actions.Target) actionprompt.Config {
	return actionprompt.Config{
		Mode:        actionprompt.ModeText,
		Title:       fmt.Sprintf("Merge message #%d", target.Number),
		Message:     fmt.Sprintf("%s - %s", target.Repo, target.Title),
		Placeholder: "Merge message",
	}
}

// previewVisible reports whether the preview panel is actually rendered
// right now — open (not just toggled on) and not auto-collapsed by the
// terminal being too narrow (layout.Compute's PreviewCollapsed). Focus only
// makes sense when there's a visible panel to focus.
func (m Model) previewVisible() bool {
	return m.ctx.PreviewOpen && !m.layout.PreviewCollapsed
}

// dismissTop implements the universal esc cascade (spec §2's Global "esc"
// row): the first dismissible layer, most-nested first, closes; esc at the
// top level (nothing open) does nothing. The cascade order is overlay →
// action prompt → search → preview focus. In practice this whole function
// is UNREACHABLE while any overlay is open (esc while help or the palette
// is open never even reaches Update's KeyPressMsg switch that calls
// dismissTop — see updateOverlay, which intercepts and closes both
// overlays itself) — this entry is kept for when a future overlay wants to
// esc-cascade to an outer state instead of just closing, and for
// documentation. Action prompt and search are listed for the same reason:
// both already intercept esc themselves, before Update ever reaches this
// function (see the tea.KeyPressMsg + m.actionPrompt.Active() check at the
// top of Update, and the tea.KeyPressMsg + IsSearchFocused check right
// after it, which forward straight to the action prompt's / section's own
// esc handling).
func (m Model) dismissTop() (Model, tea.Cmd) {
	if m.activeOverlay != overlayNone {
		m.activeOverlay = overlayNone
		return m, nil
	}
	if m.actionPrompt.Active() {
		// Unreachable today (see doc comment above) — kept so this
		// function documents the full, real cascade order.
		return m, nil
	}
	if s := m.getCurrSection(); s != nil && s.IsSearchFocused() {
		// Unreachable today (see doc comment above).
		cmd := s.SetIsSearching(false)
		return m, cmd
	}
	if m.previewFocused {
		m.previewFocused = false
		return m, nil
	}
	return m, nil
}

// updateOverlay routes every key press to the active overlay while one is
// open (spec §4: "overlay intercepts all keys while open"). The palette is
// handled first and separately (updatePaletteOverlay) because — unlike the
// read-only help overlay — it owns a text input that must receive
// PRINTABLE keys as filter text, including "?" and "q": help's
// esc/q/?-closes-anything rule below would otherwise make the palette
// impossible to type a query into. The one exception is switching overlays
// directly: pressing the palette's own open key while help is open
// switches straight to the palette (spec: "only one overlay open at a
// time") rather than requiring esc first — there's no printable-key
// conflict for THIS direction since ":"/"ctrl+p" isn't a key help's
// viewport recognizes either way. The reverse (pressing "?" to jump from
// an open palette to help) is deliberately NOT wired: "?" must stay a
// literal filter character while the palette has focus.
func (m Model) updateOverlay(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.activeOverlay == overlayHelp && key.Matches(msg, m.keys.Palette) {
		m.activeOverlay = overlayNone
		return m, m.openPalette(paletteAll)
	}
	if m.activeOverlay == overlayPalette {
		return m.updatePaletteOverlay(msg)
	}
	// esc/q/? close the (non-palette) overlay; everything else goes to its
	// own scroll handling (j/k/d/u/g/G) and is swallowed regardless of
	// whether it recognized it — nothing falls through to the normal
	// routing below.
	if key.Matches(msg, m.keys.Esc) || key.Matches(msg, m.keys.Quit) || key.Matches(msg, m.keys.Help) {
		m.activeOverlay = overlayNone
		return m, nil
	}
	switch m.activeOverlay {
	case overlayHelp:
		var cmd tea.Cmd
		m.helpOverlay, cmd, _ = m.helpOverlay.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

// updatePaletteOverlay forwards a key press to the palette component and
// reacts to what it reports: EventDismiss closes the overlay (esc);
// EventRun closes it and dispatches the selected item (enter); anything
// else (typing, arrow navigation) just persists the palette's own state.
func (m Model) updatePaletteOverlay(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	var ev palette.Event
	m.palette, cmd, ev = m.palette.Update(msg)
	switch ev.Kind {
	case palette.EventDismiss:
		m.activeOverlay = overlayNone
		return m, cmd
	case palette.EventRun:
		m.activeOverlay = overlayNone
		next, dispatchCmd := m.dispatchPaletteItem(ev.Item)
		return next, tea.Batch(cmd, dispatchCmd)
	default:
		return m, cmd
	}
}

// openHelpOverlay opens the help overlay (spec §4), loading it fresh with
// the current view's keymap groups every time — so a config rebind or a
// view switch since the last time it was open is always reflected, per
// helpoverlay.Model.SetGroups's contract. Mirrors the old showHelp toggle's
// UX: pressing the help key again while it's already open closes it,
// same as updateOverlay's own esc/q/? handling (both are reachable —
// this one from the normal key switch below, before the overlay is open).
// Opening help while the palette is open closes the palette first (spec:
// "only one overlay open at a time").
func (m *Model) openHelpOverlay() {
	if m.activeOverlay == overlayHelp {
		m.activeOverlay = overlayNone
		return
	}
	m.helpOverlay.SetGroups(toHelpOverlayGroups(m.keys.Groups(m.ctx.View)))
	m.activeOverlay = overlayHelp
}

// openPalette (re)opens the command palette scoped by scope, loading it
// fresh with paletteItems(scope) every time — so it always reflects the
// current view/row/keymap, same reasoning as openHelpOverlay. Opening the
// palette while help is open closes help first (spec: "only one overlay
// open at a time"); a second press of the palette's own open key while
// it's ALREADY open does not toggle it closed the way help's does — that
// branch is normally unreachable anyway (updateOverlay routes every key to
// the palette's own Update first while it's open, so this function is only
// ever entered from the big key switch below, i.e. with no overlay open or
// with help open) and toggling here would be surprising UX for a text
// input (typing ":" again while filtering shouldn't close the box).
func (m *Model) openPalette(scope paletteScope) tea.Cmd {
	m.activeOverlay = overlayPalette
	return m.palette.Open(m.paletteItems(scope))
}

// openRowPaletteFromPending is the Task 6 seam's consumer: it reads and
// clears pendingRowPalette (see that field's doc comment) and, if a
// right-click was actually pending, opens the palette scoped to that row's
// actions. -1 (nothing pending — e.g. called speculatively) is a no-op.
func (m *Model) openRowPaletteFromPending() tea.Cmd {
	if m.pendingRowPalette < 0 {
		return nil
	}
	m.pendingRowPalette = -1
	return m.openPalette(paletteRowActions)
}

// paletteItems builds the command palette's item list for the current
// view/row: every builtin action valid right now (availableActions, the
// same validity rules the old action-bar row used), reusing each
// keyMap-derived key hint where one exists; "Go to <view>" for every view;
// "Section: <title>" per the current view's sections; and the user's
// custom commands (cfg.Keybindings entries with Command set, scoped to
// what's actually reachable from here via activeKeybindings). scope ==
// paletteRowActions (the right-click entry point) stops after the action
// items, per spec §3.
func (m Model) paletteItems(scope paletteScope) []palette.Item {
	row := m.getCurrRowData()
	items := make([]palette.Item, 0, 16)
	for _, b := range availableActions(m.ctx.View, row) {
		hint := ""
		if binding, ok := m.keys.bindingForBuiltin(b.Builtin); ok {
			hint = keyHelp(binding, "")
		}
		items = append(items, palette.Item{
			Kind:    palette.KindAction,
			Label:   b.Label,
			Builtin: b.Builtin,
			KeyHint: hint,
		})
	}
	if scope == paletteRowActions {
		return items
	}
	items = append(items,
		palette.Item{Kind: palette.KindView, Label: "Go to Pulls", Index: int(context.PullsView), KeyHint: keyHelp(m.keys.ViewPulls, "")},
		palette.Item{Kind: palette.KindView, Label: "Go to Issues", Index: int(context.IssuesView), KeyHint: keyHelp(m.keys.ViewIssues, "")},
		palette.Item{Kind: palette.KindView, Label: "Go to Inbox", Index: int(context.NotificationsView), KeyHint: keyHelp(m.keys.ViewNotifications, "")},
		palette.Item{Kind: palette.KindView, Label: "Go to CI", Index: int(context.ActionsView), KeyHint: keyHelp(m.keys.ViewActions, "")},
		palette.Item{Kind: palette.KindView, Label: "Go to Branches", Index: int(context.BranchesView), KeyHint: keyHelp(m.keys.ViewBranches, "")},
	)
	for i, s := range m.currentViewSections() {
		items = append(items, palette.Item{Kind: palette.KindSection, Label: "Section: " + s.GetTitle(), Index: i})
	}
	for _, b := range m.activeKeybindings() {
		if strings.TrimSpace(b.Command) == "" {
			continue
		}
		items = append(items, palette.Item{Kind: palette.KindCustom, Label: b.Name, Command: b.Command, KeyHint: b.Key})
	}
	return items
}

// dispatchPaletteItem runs item exactly the way its non-palette equivalent
// would: a KindAction item goes through handleBuiltinKeybinding (same as a
// keybinding match), so "Merge" in the palette opens the merge picker
// exactly like pressing "m"; KindView/KindSection go through
// switchToView/switchSectionTo; KindCustom goes through startCustomCommand.
// The palette adds no new action plumbing, per the spec.
func (m Model) dispatchPaletteItem(item palette.Item) (Model, tea.Cmd) {
	switch item.Kind {
	case palette.KindAction:
		if next, cmd, handled := m.handleBuiltinKeybinding(config.Keybinding{Builtin: item.Builtin}); handled {
			return next, cmd
		}
		return m, nil
	case palette.KindView:
		return m, m.switchToView(context.ViewType(item.Index))
	case palette.KindSection:
		return m.switchSectionTo(item.Index)
	case palette.KindCustom:
		return m, m.startCustomCommand(config.Keybinding{Command: item.Command, Name: item.Label})
	default:
		return m, nil
	}
}

// toHelpOverlayGroups adapts keys.go's []BindingGroup (package ui) to
// helpoverlay.Group — identical shape, different package, because
// helpoverlay can't import package ui (see its package doc comment for the
// import-cycle reasoning) and keyMap can't move there (it's the dispatch
// table for the rest of app.go).
func toHelpOverlayGroups(groups []BindingGroup) []helpoverlay.Group {
	out := make([]helpoverlay.Group, len(groups))
	for i, g := range groups {
		out[i] = helpoverlay.Group{Title: g.Title, Bindings: g.Bindings}
	}
	return out
}

func (m Model) quitOrConfirm() (Model, tea.Cmd) {
	if m.ctx != nil && m.ctx.Config != nil && m.ctx.Config.ConfirmQuitEnabled() {
		m.pendingQuit = true
		m.actionPrompt = m.actionPrompt.Focus(actionprompt.Config{
			Mode:    actionprompt.ModeConfirm,
			Title:   "Quit tea-dash",
			Message: "Quit tea-dash?",
		})
		return m, nil
	}
	return m, tea.Quit
}

func (m *Model) dispatchActionIntent(intent actions.Intent) tea.Cmd {
	if m.actionDispatcher == nil {
		return m.setError("Action not wired yet.")
	}
	startCmd := m.setStart(actionStartText(intent))
	return tea.Batch(startCmd, m.actionDispatcher(intent))
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
		if detail := m.pullDetails[m.selKey()]; detail != nil {
			sha = detail.HeadSHA
		}
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
	if target.RowKind == actions.RowKindPullRequest {
		if detail := m.pullDetails[m.selKey()]; detail != nil {
			target.HeadRefName = detail.HeadRef
			target.BaseRefName = detail.BaseRef
		}
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
	case actions.KindMerge, actions.KindUpdateBranch, actions.KindMarkReady, actions.KindMarkDraft, actions.KindReview, actions.KindRequestReviewers, actions.KindRemoveReviewers, actions.KindExternalDiff:
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
	case actions.KindRerunRun, actions.KindCancelRun, actions.KindViewLogs:
		if target.RowKind != actions.RowKindActionRun {
			return fmt.Errorf("%s is only available for action runs.", actionLabel(kind))
		}
	case actions.KindSwitchBranch, actions.KindPushBranch, actions.KindForcePushBranch, actions.KindFastForwardBranch, actions.KindDeleteBranch:
		if target.RowKind != actions.RowKindBranch {
			return fmt.Errorf("%s is only available for local branches.", actionLabel(kind))
		}
	default:
		return nil
	}
	return nil
}

func actionDispatchesDirectly(kind actions.Kind) bool {
	return kind == actions.KindRerunRun ||
		kind == actions.KindViewLogs ||
		kind == actions.KindSubscribe ||
		kind == actions.KindUnsubscribe ||
		kind == actions.KindExternalDiff
}

func promptModeForAction(kind actions.Kind) actions.PromptMode {
	switch kind {
	case actions.KindComment:
		return actions.PromptText
	case actions.KindAddLabel, actions.KindRemoveLabel, actions.KindSetMilestone, actions.KindRequestReviewers, actions.KindRemoveReviewers:
		return actions.PromptText
	case actions.KindMerge, actions.KindReview:
		return actions.PromptPicker
	default:
		return actions.PromptConfirm
	}
}

func promptConfigForAction(kind actions.Kind, target actions.Target) actionprompt.Config {
	return promptConfigForActionWithMergeCapabilities(kind, target, data.DefaultMergeCapabilities())
}

func promptConfigForActionWithMergeCapabilities(kind actions.Kind, target actions.Target, caps data.MergeCapabilities) actionprompt.Config {
	title := fmt.Sprintf("%s #%d", actionLabel(kind), target.Number)
	message := fmt.Sprintf("%s - %s", target.Repo, target.Title)
	if kind == actions.KindSwitchBranch || kind == actions.KindPushBranch || kind == actions.KindForcePushBranch || kind == actions.KindFastForwardBranch || kind == actions.KindDeleteBranch {
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
	case actions.KindRequestReviewers:
		return actionprompt.Config{
			Mode:        actionprompt.ModeText,
			Title:       title,
			Message:     message,
			Placeholder: "Reviewer usernames, comma-separated",
		}
	case actions.KindRemoveReviewers:
		return actionprompt.Config{
			Mode:        actionprompt.ModeText,
			Title:       title,
			Message:     message,
			Placeholder: "Reviewer usernames to remove, comma-separated",
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
			Options: mergePromptOptions(caps),
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

type mergePromptStyle struct {
	Label   string
	Style   data.MergeStyle
	Message bool
}

func mergePromptOptions(caps data.MergeCapabilities) []actionprompt.Option {
	styles := []mergePromptStyle{
		{Label: "Merge", Style: data.MergeStyleMerge, Message: true},
		{Label: "Squash", Style: data.MergeStyleSquash, Message: true},
		{Label: "Rebase", Style: data.MergeStyleRebase},
		{Label: "Rebase merge", Style: data.MergeStyleRebaseMerge, Message: true},
		{Label: "Fast-forward only", Style: data.MergeStyleFastForwardOnly},
	}
	supported := make([]mergePromptStyle, 0, len(styles))
	for _, style := range styles {
		if caps.SupportsStyle(style.Style) {
			supported = append(supported, style)
		}
	}
	if len(supported) == 0 {
		caps = data.DefaultMergeCapabilities()
		supported = styles
	}

	options := make([]actionprompt.Option, 0, 32)
	add := func(label string, style data.MergeStyle, suffix string) {
		options = append(options, actionprompt.Option{Label: label, Value: string(style) + suffix})
	}

	for _, style := range supported {
		add(style.Label, style.Style, "")
	}
	for _, style := range supported {
		if style.Message {
			add(style.Label+" with message", style.Style, "+message")
		}
	}
	for _, style := range supported {
		add(style.Label+" + delete branch", style.Style, "+delete")
	}
	if caps.ForceMerge {
		for _, style := range supported {
			add(style.Label+" + force merge", style.Style, "+force")
		}
	}
	if caps.AutoMerge {
		for _, style := range supported {
			add(style.Label+" when checks pass", style.Style, "+auto")
		}
		for _, style := range supported {
			add(style.Label+" + delete branch when checks pass", style.Style, "+delete+auto")
		}
	}
	if caps.ForceMerge {
		for _, style := range supported {
			add(style.Label+" + delete branch + force merge", style.Style, "+delete+force")
		}
	}
	return options
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
	case actions.KindRequestReviewers:
		return "Request reviewers"
	case actions.KindRemoveReviewers:
		return "Remove reviewers"
	case actions.KindExternalDiff:
		return "External diff"
	case actions.KindCheckout:
		return "Checkout"
	case actions.KindSwitchBranch:
		return "Switch branch"
	case actions.KindPushBranch:
		return "Push branch"
	case actions.KindForcePushBranch:
		return "Force-push branch"
	case actions.KindFastForwardBranch:
		return "Fast-forward branch"
	case actions.KindDeleteBranch:
		return "Delete branch"
	case actions.KindRerunRun:
		return "Rerun"
	case actions.KindCancelRun:
		return "Cancel run"
	case actions.KindViewLogs:
		return "View logs"
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
	if intent.Kind == actions.KindSwitchBranch || intent.Kind == actions.KindPushBranch || intent.Kind == actions.KindForcePushBranch || intent.Kind == actions.KindFastForwardBranch || intent.Kind == actions.KindDeleteBranch {
		return fmt.Sprintf("Starting %s for %s.", actionLabel(intent.Kind), intent.Target.Title)
	}
	return fmt.Sprintf("Starting %s for %s#%d.", actionLabel(intent.Kind), intent.Target.Repo, intent.Target.Number)
}

// setInfo/setError/setSuccess/setStart are the only way app.go should ever
// touch m.actionFeedback for a plain status message (feedbackFromActionResult
// is the other path, for actions.ResultMsg specifically) — they exist so no
// call site can forget to propagate the tea.Cmd Set returns for
// auto-expiring kinds (Task 8: a dropped cmd means a toast that never
// expires). Pointer receiver so they can be called as `m.setInfo(...)` from
// both *Model and (addressable) Model-valued call sites alike; every one
// returns the tea.Cmd the caller MUST return or tea.Batch into whatever it
// already returns.
//
// notice->toast Kind mapping (this sweep replaced the old `notice` string
// field entirely): validation/guidance ("select a row first", "X is only
// available for Y") and state-description ("showing current repository")
// messages are Info; genuine failures (an error value, "couldn't ...",
// "no client available", "invalid repo") are Error; a completed
// notification action (mark/pin/unpin) is Success; a long-running step
// before a prompt opens ("loading reviewers…") is Start. See the Task 8
// report for the handful of genuinely ambiguous calls.
func (m *Model) setInfo(text string) tea.Cmd {
	var cmd tea.Cmd
	m.actionFeedback, cmd = m.actionFeedback.Set(actionfeedback.Info(text))
	return cmd
}

func (m *Model) setError(text string) tea.Cmd {
	var cmd tea.Cmd
	m.actionFeedback, cmd = m.actionFeedback.Set(actionfeedback.Error(text))
	return cmd
}

func (m *Model) setSuccess(text string) tea.Cmd {
	var cmd tea.Cmd
	m.actionFeedback, cmd = m.actionFeedback.Set(actionfeedback.Success(text))
	return cmd
}

func (m *Model) setStart(text string) tea.Cmd {
	var cmd tea.Cmd
	m.actionFeedback, cmd = m.actionFeedback.Set(actionfeedback.Start(text))
	return cmd
}

// clearFeedback silently drops the current toast (no generation bump — see
// actionfeedback.Model.Clear) — used where a preceding "loading…"/Start
// toast is being superseded by a state change that isn't itself a new
// toast (e.g. a picker opening), not a real dismissal.
func (m *Model) clearFeedback() {
	m.actionFeedback = m.actionFeedback.Clear()
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
		return m, m.setInfo("Select a notification to mark read.")
	}
	if row.ID == 0 {
		return m, m.setError("Selected notification has no thread id.")
	}
	if m.ctx.Client == nil {
		return m, m.setError("No Gitea client available to mark notifications read.")
	}
	return m, markNotificationReadCmd(m.ctx.Client, m.currSectionId, row.ID)
}

func (m Model) markSelectedNotificationUnread() (Model, tea.Cmd) {
	row, ok := m.getCurrRowData().(data.Notification)
	if !ok {
		return m, m.setInfo("Select a notification to mark unread.")
	}
	if row.ID == 0 {
		return m, m.setError("Selected notification has no thread id.")
	}
	if m.ctx.Client == nil {
		return m, m.setError("No Gitea client available to mark notifications unread.")
	}
	return m, markNotificationUnreadCmd(m.ctx.Client, m.currSectionId, row.ID)
}

func (m Model) toggleSelectedNotificationPin() (Model, tea.Cmd) {
	row, ok := m.getCurrRowData().(data.Notification)
	if !ok {
		return m, m.setInfo("Select a notification to pin.")
	}
	if row.Pinned {
		return m.unpinNotification(row)
	}
	return m.pinNotification(row)
}

func (m Model) unpinSelectedNotification() (Model, tea.Cmd) {
	row, ok := m.getCurrRowData().(data.Notification)
	if !ok {
		return m, m.setInfo("Select a notification to unpin.")
	}
	return m.unpinNotification(row)
}

func (m Model) pinNotification(row data.Notification) (Model, tea.Cmd) {
	if row.ID == 0 {
		return m, m.setError("Selected notification has no thread id.")
	}
	if m.ctx.Client == nil {
		return m, m.setError("No Gitea client available to pin notifications.")
	}
	return m, markNotificationPinnedCmd(m.ctx.Client, m.currSectionId, row.ID)
}

func (m Model) unpinNotification(row data.Notification) (Model, tea.Cmd) {
	if row.ID == 0 {
		return m, m.setError("Selected notification has no thread id.")
	}
	if m.ctx.Client == nil {
		return m, m.setError("No Gitea client available to unpin notifications.")
	}
	return m, markNotificationUnpinnedCmd(m.ctx.Client, m.currSectionId, row.ID, row.Unread)
}

func (m Model) markAllNotificationsRead() (Model, tea.Cmd) {
	if m.ctx.View != context.NotificationsView {
		return m, m.setInfo("Switch to notifications to mark all read.")
	}
	if m.ctx.Client == nil {
		return m, m.setError("No Gitea client available to mark notifications read.")
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
		var feedbackCmd tea.Cmd
		switch {
		case msg.all:
			feedbackCmd = m.setError(fmt.Sprintf("Couldn't mark all notifications read: %v", msg.err))
		case msg.pinned:
			feedbackCmd = m.setError(fmt.Sprintf("Couldn't pin notification: %v", msg.err))
		case msg.unpinned:
			feedbackCmd = m.setError(fmt.Sprintf("Couldn't unpin notification: %v", msg.err))
		case msg.unread:
			feedbackCmd = m.setError(fmt.Sprintf("Couldn't mark notification unread: %v", msg.err))
		default:
			feedbackCmd = m.setError(fmt.Sprintf("Couldn't mark notification read: %v", msg.err))
		}
		return m, feedbackCmd
	}
	var feedbackCmd tea.Cmd
	switch {
	case msg.all:
		feedbackCmd = m.setSuccess("Marked all notifications read.")
	case msg.pinned:
		feedbackCmd = m.setSuccess("Pinned notification.")
	case msg.unpinned:
		feedbackCmd = m.setSuccess("Unpinned notification.")
	case msg.unread:
		feedbackCmd = m.setSuccess("Marked notification unread.")
	default:
		feedbackCmd = m.setSuccess("Marked notification read.")
	}
	if msg.sectionID < 0 || msg.sectionID >= len(m.notifications) {
		return m, feedbackCmd
	}
	return m, tea.Batch(feedbackCmd, m.notifications[msg.sectionID].FetchRows())
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

func (m Model) updateCurrentSectionWithPreview(msg tea.Msg) (Model, tea.Cmd) {
	before := m.selKey()
	cmd := m.updateCurrentSection(msg)
	if m.ctx.PreviewOpen && m.selKey() != before {
		m.syncSidebar()
		cmd = tea.Batch(cmd, m.enrichCurrRow())
	}
	return m, cmd
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

// syncMainContentDimensions recomputes the shell's layout.Layout from the
// current screen size and preview/search toggles and stores it on the
// model (m.layout). MainContentWidth/Height and PreviewWidth/Height keep
// feeding sections and the sidebar exactly as before — now sourced from the
// computed layout's interior rects instead of hand-rolled arithmetic.
func (m *Model) syncMainContentDimensions() {
	searchOpen := false
	if s := m.getCurrSection(); s != nil {
		searchOpen = s.IsSearchFocused()
	}

	// defaults.preview.width configures the preview's CONTENT width;
	// layout.Input.PreviewWidth wants the panel's TOTAL width including its
	// own two border columns.
	previewWidthTotal := 0
	if m.ctx.Config != nil {
		if configured := m.ctx.Config.Defaults.Preview.PreviewWidth(); configured > 0 {
			previewWidthTotal = configured + 2
		}
	}

	m.layout = layout.Compute(layout.Input{
		Width:        m.ctx.ScreenWidth,
		Height:       m.ctx.ScreenHeight,
		PreviewOpen:  m.ctx.PreviewOpen,
		PreviewWidth: previewWidthTotal,
		SectionCount: len(m.currentViewSections()),
		SearchOpen:   searchOpen,
	})

	m.ctx.MainContentWidth = m.layout.ListInterior.W
	m.ctx.MainContentHeight = m.layout.ListInterior.H
	m.ctx.PreviewWidth = m.layout.PreviewInterior.W
	m.ctx.PreviewHeight = m.layout.PreviewInterior.H
	m.resizeHelpOverlay()
	m.resizePalette()
}

// resizeHelpOverlay sizes the help overlay's viewport to the same
// full-width list+preview interior overlayContent() composites it into.
// This has to happen here — on the *pointer*-receiver model that
// Update actually persists — rather than lazily inside overlayContent()/
// View() (both value-receiver methods whose mutations are thrown away
// after each render): Update's own scroll handling (j/k/g/G/d/u, routed
// to helpoverlay.Model.Update) reads the viewport's OWN Height/Width to
// compute the new offset (GotoBottom's max offset, HalfPageDown's step
// size), not anything derived at render time. Sizing only at View() time
// left the persisted viewport stuck at its New() zero size forever: 'd'
// silently no-opped (half of a zero height is zero) and 'G' scrolled to
// an offset equal to the full line count (max(0, total-0)), rendering
// blank — both invisible in a quick glance at the rendered frame, since
// fitBlock pads/crops every render to the right final size regardless of
// what the viewport's internal window actually showed.
func (m *Model) resizeHelpOverlay() {
	w := m.layout.ListPanel.W + m.layout.PreviewPanel.W - 2
	if w < 0 {
		w = 0
	}
	h := m.layout.ListPanel.H - 2
	if h < 1 {
		h = 1
	}
	m.helpOverlay.SetSize(w, h)
}

// resizePalette sizes the palette's item list to the same full-width/
// full-height interior overlayContent() composites it into, for the same
// pointer-receiver-persistence reason as resizeHelpOverlay: the palette's
// own scroll bookkeeping (Visible/ensureVisible) reads its OWN height, not
// anything derived at render time.
func (m *Model) resizePalette() {
	w := m.layout.ListPanel.W + m.layout.PreviewPanel.W - 2
	if w < 0 {
		w = 0
	}
	h := m.layout.ListPanel.H - 2
	if h < 1 {
		h = 1
	}
	m.palette.SetSize(w, h)
}

// statusLine is the status bar's middle segment: plain (unstyled) text —
// statusbar.View applies styles.StatusBar once over the whole assembled row.
func (m Model) statusLine() string {
	s := m.getCurrSection()
	if s == nil || s.GetError() != nil {
		return ""
	}
	if s.GetIsLoading() {
		title := strings.TrimSpace(s.GetTitle())
		if title == "" {
			title = strings.TrimSpace(s.GetItemPlural())
		}
		return "Loading " + title + "…"
	}
	total := s.GetTotalCount()
	shown := s.NumRows()
	if total > shown {
		return fmt.Sprintf("showing %d of %d %s", shown, total, s.GetItemPlural())
	}
	if total == 1 {
		return fmt.Sprintf("1 %s", s.GetItemSingular())
	}
	return fmt.Sprintf("%d %s", total, s.GetItemPlural())
}

// toSectioners adapts sections to the tab bar's minimal interface.
func toSectioners(sections []section.Section) []tabs.Sectioner {
	out := make([]tabs.Sectioner, len(sections))
	for i, s := range sections {
		out[i] = s
	}
	return out
}
