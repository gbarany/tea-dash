package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

// keyMap defines the app-level key bindings. Row navigation is also forwarded
// to the underlying table widget so configurable builtins reuse table behavior
// (as do "g"/"G" first/last row and "ctrl+d"/"ctrl+u" list half-page — bubbles'
// table.DefaultKeyMap already binds GotoTop/GotoBottom/HalfPageUp/HalfPageDown
// to exactly those keys, so once no tea-dash-level binding claims them first
// they fall through to the table for free; FirstLine/LastLine/HalfPageDown/
// HalfPageUp below exist for Groups()'s help metadata, not because
// tea-dash's own switch dispatches them).
//
// Every field here is meant to show up in exactly one Groups(view) entry — see
// the doc comment there.
type keyMap struct {
	// Views: jump directly to a view, or cycle through them.
	ViewPulls         key.Binding
	ViewIssues        key.Binding
	ViewNotifications key.Binding
	ViewActions       key.Binding
	ViewBranches      key.Binding
	SwitchView        key.Binding

	// Sections: previous/next tab within the current view.
	NextSection key.Binding
	PrevSection key.Binding

	// List: row navigation. Up/Down are real bindings tea-dash dispatches
	// itself (and reuses while the preview is focused, for line-scroll);
	// FirstLine/LastLine/HalfPageDown/HalfPageUp are display-only — see the
	// type doc comment (bubbles' table.DefaultKeyMap already binds
	// ctrl+d/ctrl+u for free, same as g/G).
	Up           key.Binding
	Down         key.Binding
	FirstLine    key.Binding
	LastLine     key.Binding
	HalfPageDown key.Binding
	HalfPageUp   key.Binding

	// Preview: focus toggle, focused-scroll (dispatched by
	// components/sidebar.Update, not tea-dash's own switch — display-only
	// here, same as FirstLine/LastLine), pane toggle, tab cycling (only
	// live while focused — see app.go), and body expand.
	FocusPreview    key.Binding
	PreviewHalfUp   key.Binding
	PreviewHalfDown key.Binding
	TogglePreview   key.Binding
	PrevSidebarTab  key.Binding
	NextSidebarTab  key.Binding
	Expand          key.Binding

	// Search.
	Search key.Binding

	// Overlays.
	Help    key.Binding
	Palette key.Binding

	// Global.
	Esc         key.Binding
	Open        key.Binding
	Refresh     key.Binding
	RefreshAll  key.Binding
	CopyNumber  key.Binding
	CopyURL     key.Binding
	ToggleSmart key.Binding
	Quit        key.Binding

	// PRs / Issues.
	Comment          key.Binding
	Assign           key.Binding
	Unassign         key.Binding
	Subscribe        key.Binding
	Unsubscribe      key.Binding
	AddLabel         key.Binding
	RemoveLabel      key.Binding
	Milestone        key.Binding
	Merge            key.Binding
	UpdateBranch     key.Binding
	MarkReady        key.Binding
	WatchChecks      key.Binding
	Close            key.Binding
	Reopen           key.Binding
	Review           key.Binding
	RequestReviewers key.Binding
	RemoveReviewers  key.Binding
	ExternalDiff     key.Binding
	Checkout         key.Binding

	// Branches.
	PushBranch        key.Binding
	ForcePushBranch   key.Binding
	FastForwardBranch key.Binding
	DeleteBranch      key.Binding

	// CI (Actions).
	RerunRun  key.Binding
	CancelRun key.Binding
	ViewLogs  key.Binding

	// Inbox (Notifications).
	MarkRead    key.Binding
	MarkUnread  key.Binding
	MarkAllRead key.Binding
	Pin         key.Binding
	Unpin       key.Binding
}

// BindingGroup is one titled cluster of key bindings, e.g. for the help
// overlay (Task 5) or a generated README table.
type BindingGroup struct {
	Title    string
	Bindings []key.Binding
}

// Groups returns every binding tea-dash dispatches, clustered per spec §2's
// table: the universal groups (Views/Sections/List/Preview/Search/Overlays/
// Global), then the current view's scoped group (PRs/Issues/Inbox/CI/
// Branches). Every keyMap field appears in exactly one binding across a
// single call's result (view-scoped fields like Checkout or Comment are
// shared BY NAME across multiple views' scoped groups, but only one
// view-scoped group — the current view's — is ever included per call, so
// there's no duplication within one Groups(view) result) — with one
// exception: the Global group has any binding whose key(s) are shadowed by
// the view-scoped group filtered out first (see suppressShadowed), since
// app.go's dispatch switch resolves those in the scoped group's favor and
// listing both would tell the user a shadowed key does something it
// can't reach in this view.
func (k keyMap) Groups(view context.ViewType) []BindingGroup {
	scoped := k.viewScopedGroup(view)
	groups := []BindingGroup{
		{Title: "Views", Bindings: []key.Binding{
			k.ViewPulls, k.ViewIssues, k.ViewNotifications, k.ViewActions, k.ViewBranches, k.SwitchView,
		}},
		{Title: "Sections", Bindings: []key.Binding{k.PrevSection, k.NextSection}},
		{Title: "List", Bindings: []key.Binding{k.Up, k.Down, k.FirstLine, k.LastLine, k.HalfPageDown, k.HalfPageUp}},
		{Title: "Preview", Bindings: []key.Binding{
			k.FocusPreview, k.PreviewHalfUp, k.PreviewHalfDown, k.PrevSidebarTab, k.NextSidebarTab, k.TogglePreview, k.Expand,
		}},
		{Title: "Search", Bindings: []key.Binding{k.Search}},
		{Title: "Overlays", Bindings: []key.Binding{k.Help, k.Palette}},
		{Title: "Global", Bindings: suppressShadowed([]key.Binding{
			k.Esc, k.Open, k.Refresh, k.RefreshAll, k.CopyNumber, k.CopyURL, k.ToggleSmart, k.Quit,
		}, scoped.Bindings)},
	}
	return append(groups, scoped)
}

// suppressShadowed drops any binding from bindings whose key(s) are also
// claimed by a binding in scopedBindings. Dispatch-time switch-statement
// ordering in app.go (e.g. the Actions view's RerunRun case, guarded by
// `m.ctx.View == context.ActionsView`, is checked — and so wins — before
// the general RefreshAll case for their shared "R" key) means a universal
// binding shadowed this way is genuinely unreachable in that view, so the
// help overlay listing it alongside the scoped binding that actually owns
// the key would be actively misleading (README's migration table: "in the
// CI view, R reruns instead ... there is no refresh-all default key in
// that view"). Written as general key-set suppression rather than a
// RefreshAll/RerunRun special case so any future Task 6+ collision gets
// the same treatment for free.
func suppressShadowed(bindings, scopedBindings []key.Binding) []key.Binding {
	shadowed := map[string]bool{}
	for _, b := range scopedBindings {
		for _, k := range b.Keys() {
			shadowed[k] = true
		}
	}
	out := make([]key.Binding, 0, len(bindings))
	for _, b := range bindings {
		claimedByScope := false
		for _, k := range b.Keys() {
			if shadowed[k] {
				claimedByScope = true
				break
			}
		}
		if !claimedByScope {
			out = append(out, b)
		}
	}
	return out
}

// viewScopedGroup is the one group of bindings specific to the current view
// (spec §2's PRs/Issues/Inbox/CI/Branches rows).
func (k keyMap) viewScopedGroup(view context.ViewType) BindingGroup {
	switch view {
	case context.IssuesView:
		return BindingGroup{Title: "Issues", Bindings: []key.Binding{
			k.Comment, k.Assign, k.Unassign, k.AddLabel, k.RemoveLabel, k.Milestone,
			k.Subscribe, k.Unsubscribe, k.Close, k.Reopen, k.Checkout,
		}}
	case context.NotificationsView:
		return BindingGroup{Title: "Inbox", Bindings: []key.Binding{
			k.MarkRead, k.MarkUnread, k.MarkAllRead, k.Pin, k.Unpin,
		}}
	case context.ActionsView:
		return BindingGroup{Title: "CI", Bindings: []key.Binding{k.RerunRun, k.CancelRun, k.ViewLogs}}
	case context.BranchesView:
		return BindingGroup{Title: "Branches", Bindings: []key.Binding{
			k.Checkout, k.PushBranch, k.ForcePushBranch, k.FastForwardBranch, k.DeleteBranch,
		}}
	default:
		return BindingGroup{Title: "PRs", Bindings: []key.Binding{
			k.Comment, k.Assign, k.Unassign, k.AddLabel, k.RemoveLabel, k.Merge, k.UpdateBranch,
			k.MarkReady, k.WatchChecks, k.Close, k.Reopen, k.Review, k.RequestReviewers,
			k.RemoveReviewers, k.ExternalDiff, k.Checkout,
		}}
	}
}

func defaultKeyMap() keyMap {
	return keyMap{
		ViewPulls: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "pulls"),
		),
		ViewIssues: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "issues"),
		),
		ViewNotifications: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "inbox"),
		),
		ViewActions: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "CI"),
		),
		ViewBranches: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "branches"),
		),
		SwitchView: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "cycle views"),
		),
		NextSection: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("l", "next section"),
		),
		PrevSection: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h", "prev section"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		FirstLine: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "first row"),
		),
		LastLine: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "last row"),
		),
		HalfPageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "list ½ page down"),
		),
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "list ½ page up"),
		),
		FocusPreview: key.NewBinding(
			key.WithKeys("enter", "tab"),
			key.WithHelp("enter/tab", "focus preview"),
		),
		PreviewHalfUp: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "preview ½ page up"),
		),
		PreviewHalfDown: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "preview ½ page down"),
		),
		TogglePreview: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "toggle preview"),
		),
		PrevSidebarTab: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "previous preview tab"),
		),
		NextSidebarTab: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next preview tab"),
		),
		Expand: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "expand"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Palette: key.NewBinding(
			key.WithKeys(":", "ctrl+p"),
			key.WithHelp(":/ctrl+p", "command palette"),
		),
		Esc: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "dismiss"),
		),
		Open: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open in browser"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		RefreshAll: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "refresh all"),
		),
		CopyNumber: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy number"),
		),
		CopyURL: key.NewBinding(
			key.WithKeys("Y"),
			key.WithHelp("Y", "copy URL"),
		),
		ToggleSmart: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "current repo"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Comment: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "comment"),
		),
		Assign: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "assign"),
		),
		Unassign: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "unassign"),
		),
		Subscribe: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "subscribe"),
		),
		Unsubscribe: key.NewBinding(
			key.WithKeys("B"),
			key.WithHelp("B", "unsubscribe"),
		),
		AddLabel: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "add label"),
		),
		RemoveLabel: key.NewBinding(
			key.WithKeys("U"),
			key.WithHelp("U", "remove label"),
		),
		Milestone: key.NewBinding(
			key.WithKeys("M"),
			key.WithHelp("M", "milestone"),
		),
		Merge: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "merge"),
		),
		UpdateBranch: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "update branch"),
		),
		MarkReady: key.NewBinding(
			key.WithKeys("W"),
			key.WithHelp("W", "ready"),
		),
		WatchChecks: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "checks"),
		),
		Close: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "close"),
		),
		Reopen: key.NewBinding(
			key.WithKeys("X"),
			key.WithHelp("X", "reopen"),
		),
		Review: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "review"),
		),
		RequestReviewers: key.NewBinding(
			key.WithKeys("@"),
			key.WithHelp("@", "request review"),
		),
		// "#" pairs with "@" (spec §2's PR row: "@/# request/remove
		// reviewers").
		RemoveReviewers: key.NewBinding(
			key.WithKeys("#"),
			key.WithHelp("#", "remove reviewer"),
		),
		ExternalDiff: key.NewBinding(
			key.WithKeys("d", "ctrl+t"),
			key.WithHelp("d/ctrl+t", "external diff"),
		),
		Checkout: key.NewBinding(
			key.WithKeys("C", " ", "space"),
			key.WithHelp("C/space", "checkout"),
		),
		PushBranch: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "push branch"),
		),
		ForcePushBranch: key.NewBinding(
			key.WithKeys("F"),
			key.WithHelp("F", "force push branch"),
		),
		FastForwardBranch: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "fast-forward branch"),
		),
		DeleteBranch: key.NewBinding(
			key.WithKeys("d", "backspace"),
			key.WithHelp("d/backspace", "delete branch"),
		),
		RerunRun: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "rerun"),
		),
		CancelRun: key.NewBinding(
			key.WithKeys("!"),
			key.WithHelp("!", "cancel run"),
		),
		ViewLogs: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "view logs"),
		),
		MarkRead: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "mark read"),
		),
		MarkUnread: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "mark unread"),
		),
		MarkAllRead: key.NewBinding(
			key.WithKeys("M"),
			key.WithHelp("M", "mark all read"),
		),
		Pin: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "pin"),
		),
		Unpin: key.NewBinding(
			key.WithKeys("B"),
			key.WithHelp("B", "unpin"),
		),
	}
}

func (k *keyMap) applyConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for _, b := range cfg.Keybindings.Universal {
		if strings.TrimSpace(b.Builtin) == "" {
			continue
		}
		k.rebindBuiltin(b.Builtin, b.Key)
	}
}

// rebindBuiltin is one of three parallel builtin-name switches — see
// app.go's handleBuiltinKeybinding doc comment for why they're a documented
// cross-reference rather than one merged table (this one writes a keyMap
// field; bindingForBuiltin below reads one back; handleBuiltinKeybinding
// runs the actual behavior).
func (k *keyMap) rebindBuiltin(name, keyName string) {
	keyName = strings.TrimSpace(keyName)
	if keyName == "" {
		return
	}
	switch normalizeBuiltin(name) {
	case "refresh":
		k.Refresh = binding(keyName, "refresh")
	case "refreshall":
		k.RefreshAll = binding(keyName, "refresh all")
	case "opengithub", "open", "openbrowser":
		k.Open = binding(keyName, "open in browser")
	case "quit":
		k.Quit = binding(keyName, "quit")
	case "up":
		k.Up = binding(keyName, "move up")
	case "down":
		k.Down = binding(keyName, "move down")
	case "nextsection":
		k.NextSection = binding(keyName, "next section")
	case "prevsection", "previoussection":
		k.PrevSection = binding(keyName, "prev section")
	case "viewpulls", "viewprs":
		k.ViewPulls = binding(keyName, "pulls")
	case "viewissues":
		k.ViewIssues = binding(keyName, "issues")
	case "viewnotifications", "viewinbox":
		k.ViewNotifications = binding(keyName, "inbox")
	case "viewactions", "viewci":
		k.ViewActions = binding(keyName, "CI")
	case "viewbranches":
		k.ViewBranches = binding(keyName, "branches")
	case "switchview":
		k.SwitchView = binding(keyName, "cycle views")
	case "focuspreview", "togglefocus":
		k.FocusPreview = binding(keyName, "focus preview")
	case "search":
		k.Search = binding(keyName, "search")
	case "togglepreview":
		k.TogglePreview = binding(keyName, "toggle preview")
	case "togglesmartfiltering", "togglesmartfilter", "currentrepo":
		k.ToggleSmart = binding(keyName, "current repo")
	case "prevsidebartab", "previoussidebartab":
		k.PrevSidebarTab = binding(keyName, "previous preview tab")
	case "nextsidebartab":
		k.NextSidebarTab = binding(keyName, "next preview tab")
	case "summaryviewmore", "expand":
		k.Expand = binding(keyName, "expand")
	case "comment":
		k.Comment = binding(keyName, "comment")
	case "assign":
		k.Assign = binding(keyName, "assign")
	case "unassign":
		k.Unassign = binding(keyName, "unassign")
	case "subscribe":
		k.Subscribe = binding(keyName, "subscribe")
	case "unsubscribe":
		k.Unsubscribe = binding(keyName, "unsubscribe")
	case "addlabel":
		k.AddLabel = binding(keyName, "add label")
	case "removelabel":
		k.RemoveLabel = binding(keyName, "remove label")
	case "milestone", "setmilestone":
		k.Milestone = binding(keyName, "milestone")
	case "merge":
		k.Merge = binding(keyName, "merge")
	case "update", "updatebranch":
		k.UpdateBranch = binding(keyName, "update branch")
	case "ready", "markready":
		k.MarkReady = binding(keyName, "ready")
	case "watch", "watchchecks", "checks":
		k.WatchChecks = binding(keyName, "checks")
	case "close":
		k.Close = binding(keyName, "close")
	case "reopen":
		k.Reopen = binding(keyName, "reopen")
	case "approve", "review":
		k.Review = binding(keyName, "review")
	case "requestreview", "requestreviewer", "requestreviewers":
		k.RequestReviewers = binding(keyName, "request review")
	case "removereview", "removereviewer", "removereviewers", "removerequestedreviewers":
		k.RemoveReviewers = binding(keyName, "remove reviewer")
	case "diff":
		k.ExternalDiff = binding(keyName, "external diff")
	case "checkout":
		k.Checkout = binding(keyName, "checkout")
	case "push":
		k.PushBranch = binding(keyName, "push branch")
	case "forcepush":
		k.ForcePushBranch = binding(keyName, "force push branch")
	case "fastforward":
		k.FastForwardBranch = binding(keyName, "fast-forward branch")
	case "delete":
		k.DeleteBranch = binding(keyName, "delete branch")
	case "rerun", "rerunrun":
		k.RerunRun = binding(keyName, "rerun")
	case "cancel", "cancelrun":
		k.CancelRun = binding(keyName, "cancel run")
	case "logs", "viewlogs":
		k.ViewLogs = binding(keyName, "view logs")
	case "copyurl":
		k.CopyURL = binding(keyName, "copy URL")
	case "copynumber":
		k.CopyNumber = binding(keyName, "copy number")
	case "help":
		k.Help = binding(keyName, "help")
	case "palette", "commandpalette":
		k.Palette = binding(keyName, "command palette")
	case "markasread", "markread":
		k.MarkRead = binding(keyName, "mark read")
	case "markasunread", "markunread":
		k.MarkUnread = binding(keyName, "mark unread")
	case "markallasread", "markallread":
		k.MarkAllRead = binding(keyName, "mark all read")
	case "pin", "togglepin", "togglepinned", "togglebookmark":
		k.Pin = binding(keyName, "pin")
	case "unpin":
		k.Unpin = binding(keyName, "unpin")
	}
}

func binding(keyName, help string) key.Binding {
	return key.NewBinding(key.WithKeys(keyName), key.WithHelp(keyName, help))
}

// bindingForBuiltin returns the keyMap field currently bound to a builtin
// action name, matched via the same alias grouping rebindBuiltin writes —
// this is its read-only mirror. Used by the command palette (app.go's
// paletteItems) to show each action item's current key hint; covers
// exactly the builtin names availableActions() can produce (open, refresh,
// and every view/row action — not every builtin rebindBuiltin recognizes,
// since e.g. "quit"/"redraw"/"search" never appear as palette action
// items).
func (k keyMap) bindingForBuiltin(name string) (key.Binding, bool) {
	switch normalizeBuiltin(name) {
	case "open", "opengithub", "openbrowser":
		return k.Open, true
	case "refresh":
		return k.Refresh, true
	case "refreshall":
		return k.RefreshAll, true
	case "markread", "markasread", "markasdone", "markdone":
		return k.MarkRead, true
	case "markunread", "markasunread":
		return k.MarkUnread, true
	case "markallread", "markallasread", "markallasdone", "markalldone":
		return k.MarkAllRead, true
	case "pin", "togglepin", "togglepinned", "togglebookmark":
		return k.Pin, true
	case "unpin":
		return k.Unpin, true
	case "viewlogs", "logs":
		return k.ViewLogs, true
	case "rerun", "rerunrun":
		return k.RerunRun, true
	case "cancel", "cancelrun":
		return k.CancelRun, true
	case "checkout":
		return k.Checkout, true
	case "fastforward":
		return k.FastForwardBranch, true
	case "push":
		return k.PushBranch, true
	case "forcepush":
		return k.ForcePushBranch, true
	case "delete":
		return k.DeleteBranch, true
	case "comment":
		return k.Comment, true
	case "subscribe":
		return k.Subscribe, true
	case "unsubscribe":
		return k.Unsubscribe, true
	case "setmilestone", "milestone":
		return k.Milestone, true
	case "close":
		return k.Close, true
	case "reopen":
		return k.Reopen, true
	case "ready", "markready":
		return k.MarkReady, true
	case "requestreviewers", "requestreview", "requestreviewer":
		return k.RequestReviewers, true
	case "removereviewers", "removereview", "removereviewer", "removerequestedreviewers":
		return k.RemoveReviewers, true
	case "watchchecks", "watch", "checks":
		return k.WatchChecks, true
	case "diff":
		return k.ExternalDiff, true
	case "merge":
		return k.Merge, true
	default:
		return key.Binding{}, false
	}
}

func normalizeBuiltin(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	return strings.ToLower(name)
}
