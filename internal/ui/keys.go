package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/gbarany/tea-dash/internal/config"
)

// keyMap defines the app-level key bindings. Row navigation is also forwarded
// to the underlying table widget so configurable builtins reuse table behavior.
type keyMap struct {
	Refresh           key.Binding
	RefreshAll        key.Binding
	Open              key.Binding
	Quit              key.Binding
	Up                key.Binding
	Down              key.Binding
	NextSection       key.Binding
	PrevSection       key.Binding
	SwitchView        key.Binding
	Search            key.Binding
	TogglePreview     key.Binding
	ToggleSmart       key.Binding
	ScrollUp          key.Binding
	ScrollDown        key.Binding
	PrevSidebarTab    key.Binding
	NextSidebarTab    key.Binding
	Expand            key.Binding
	Comment           key.Binding
	Assign            key.Binding
	Unassign          key.Binding
	Subscribe         key.Binding
	Unsubscribe       key.Binding
	AddLabel          key.Binding
	RemoveLabel       key.Binding
	Milestone         key.Binding
	Merge             key.Binding
	UpdateBranch      key.Binding
	MarkReady         key.Binding
	WatchChecks       key.Binding
	Close             key.Binding
	Reopen            key.Binding
	Review            key.Binding
	RequestReviewers  key.Binding
	ExternalDiff      key.Binding
	Checkout          key.Binding
	PushBranch        key.Binding
	ForcePushBranch   key.Binding
	FastForwardBranch key.Binding
	DeleteBranch      key.Binding
	RerunRun          key.Binding
	CancelRun         key.Binding
	ViewLogs          key.Binding
	CopyNumber        key.Binding
	CopyURL           key.Binding
	Help              key.Binding
	MarkRead          key.Binding
	MarkUnread        key.Binding
	MarkAllRead       key.Binding
	Pin               key.Binding
	Unpin             key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		RefreshAll: key.NewBinding(
			key.WithKeys("R", "ctrl+r"),
			key.WithHelp("R", "refresh all"),
		),
		Open: key.NewBinding(
			key.WithKeys("o", "enter"),
			key.WithHelp("o", "open in browser"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		NextSection: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("l", "next section"),
		),
		PrevSection: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h", "prev section"),
		),
		SwitchView: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "switch view"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		TogglePreview: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "preview"),
		),
		ToggleSmart: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "current repo"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "scroll preview up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "scroll preview down"),
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
		CopyNumber: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy number"),
		),
		CopyURL: key.NewBinding(
			key.WithKeys("Y"),
			key.WithHelp("Y", "copy URL"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
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
	case "viewissues", "viewprs", "switchview":
		k.SwitchView = binding(keyName, "switch view")
	case "search":
		k.Search = binding(keyName, "search")
	case "togglepreview":
		k.TogglePreview = binding(keyName, "preview")
	case "togglesmartfiltering", "togglesmartfilter", "currentrepo":
		k.ToggleSmart = binding(keyName, "current repo")
	case "pageup", "scrollup":
		k.ScrollUp = binding(keyName, "scroll preview up")
	case "pagedown", "scrolldown":
		k.ScrollDown = binding(keyName, "scroll preview down")
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

func normalizeBuiltin(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	return strings.ToLower(name)
}
