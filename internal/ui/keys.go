package ui

import "github.com/charmbracelet/bubbles/key"

// keyMap defines the key bindings for the root model.
type keyMap struct {
	NextSection key.Binding
	PrevSection key.Binding
	Refresh     key.Binding
	Help        key.Binding
	Quit        key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		NextSection: key.NewBinding(
			key.WithKeys("tab", "l", "right"),
			key.WithHelp("tab", "next section"),
		),
		PrevSection: key.NewBinding(
			key.WithKeys("shift+tab", "h", "left"),
			key.WithHelp("shift+tab", "previous section"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}
