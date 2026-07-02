package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/gbarany/tea-dash/internal/config"
)

// keyMap defines the app-level key bindings. Row navigation (↑/↓, j/k, page
// keys) is handled by the underlying table widget.
type keyMap struct {
	Refresh       key.Binding
	RefreshAll    key.Binding
	Open          key.Binding
	Quit          key.Binding
	NextSection   key.Binding
	PrevSection   key.Binding
	SwitchView    key.Binding
	Search        key.Binding
	TogglePreview key.Binding
	ToggleSmart   key.Binding
	ScrollUp      key.Binding
	ScrollDown    key.Binding
	Expand        key.Binding
	Comment       key.Binding
	Assign        key.Binding
	Unassign      key.Binding
	AddLabel      key.Binding
	RemoveLabel   key.Binding
	Merge         key.Binding
	UpdateBranch  key.Binding
	Close         key.Binding
	Reopen        key.Binding
	Review        key.Binding
	ExternalDiff  key.Binding
	Checkout      key.Binding
	RerunRun      key.Binding
	CancelRun     key.Binding
	CopyNumber    key.Binding
	CopyURL       key.Binding
	Help          key.Binding
	MarkRead      key.Binding
	MarkUnread    key.Binding
	MarkAllRead   key.Binding
	Pin           key.Binding
	Unpin         key.Binding
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
		AddLabel: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "add label"),
		),
		RemoveLabel: key.NewBinding(
			key.WithKeys("U"),
			key.WithHelp("U", "remove label"),
		),
		Merge: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "merge"),
		),
		UpdateBranch: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "update branch"),
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
		ExternalDiff: key.NewBinding(
			key.WithKeys("d", "ctrl+t"),
			key.WithHelp("d/ctrl+t", "external diff"),
		),
		Checkout: key.NewBinding(
			key.WithKeys("C", " ", "space"),
			key.WithHelp("C/space", "checkout"),
		),
		RerunRun: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "rerun"),
		),
		CancelRun: key.NewBinding(
			key.WithKeys("!"),
			key.WithHelp("!", "cancel run"),
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
	case "summaryviewmore", "expand":
		k.Expand = binding(keyName, "expand")
	case "comment":
		k.Comment = binding(keyName, "comment")
	case "assign":
		k.Assign = binding(keyName, "assign")
	case "unassign":
		k.Unassign = binding(keyName, "unassign")
	case "addlabel":
		k.AddLabel = binding(keyName, "add label")
	case "removelabel":
		k.RemoveLabel = binding(keyName, "remove label")
	case "merge":
		k.Merge = binding(keyName, "merge")
	case "update", "updatebranch":
		k.UpdateBranch = binding(keyName, "update branch")
	case "close":
		k.Close = binding(keyName, "close")
	case "reopen":
		k.Reopen = binding(keyName, "reopen")
	case "approve", "review":
		k.Review = binding(keyName, "review")
	case "diff":
		k.ExternalDiff = binding(keyName, "external diff")
	case "checkout":
		k.Checkout = binding(keyName, "checkout")
	case "rerun", "rerunrun":
		k.RerunRun = binding(keyName, "rerun")
	case "cancel", "cancelrun":
		k.CancelRun = binding(keyName, "cancel run")
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
	case "pin", "togglepin", "togglepinned":
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
