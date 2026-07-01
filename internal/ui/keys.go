package ui

import "charm.land/bubbles/v2/key"

// keyMap defines the app-level key bindings. Row navigation (↑/↓, j/k, page
// keys) is handled by the underlying table widget.
type keyMap struct {
	Refresh       key.Binding
	Open          key.Binding
	Quit          key.Binding
	NextSection   key.Binding
	PrevSection   key.Binding
	SwitchView    key.Binding
	Search        key.Binding
	TogglePreview key.Binding
	ScrollUp      key.Binding
	ScrollDown    key.Binding
	Expand        key.Binding
	Comment       key.Binding
	Merge         key.Binding
	Close         key.Binding
	Reopen        key.Binding
	Review        key.Binding
	ExternalDiff  key.Binding
	Checkout      key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
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
		Merge: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "merge"),
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
			key.WithKeys("d"),
			key.WithHelp("d", "external diff"),
		),
		Checkout: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "checkout"),
		),
	}
}
