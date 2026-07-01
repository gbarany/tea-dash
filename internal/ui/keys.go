package ui

import "charm.land/bubbles/v2/key"

// keyMap defines the app-level key bindings. Row navigation (↑/↓, j/k, page
// keys) is handled by the underlying table widget.
type keyMap struct {
	Refresh key.Binding
	Open    key.Binding
	Quit    key.Binding
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
	}
}
