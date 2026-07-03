// Package actionprompt implements the small blocking prompt used by root-level
// action keys.
package actionprompt

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Mode selects the prompt interaction.
type Mode string

const (
	ModeConfirm     Mode = "confirm"
	ModeText        Mode = "text"
	ModePicker      Mode = "picker"
	ModeMultiPicker Mode = "multi-picker"
)

// Option is a selectable picker entry.
type Option struct {
	Label string
	Value string
}

// Config describes a prompt instance.
type Config struct {
	Mode        Mode
	Title       string
	Message     string
	Placeholder string
	Options     []Option
	Initial     string
}

// Result reports whether a prompt completed and what it submitted.
type Result struct {
	Submitted bool
	Canceled  bool
	Value     string
	Label     string
}

// Model owns the active prompt state.
type Model struct {
	active        bool
	cfg           Config
	input         textinput.Model
	selected      int
	multiSelected map[int]bool
}

func New() Model {
	return Model{input: newInput(Config{})}
}

// Focus opens the prompt with cfg and initializes its local state.
func (m Model) Focus(cfg Config) Model {
	if cfg.Mode == "" {
		cfg.Mode = ModeConfirm
	}
	m.active = true
	m.cfg = cfg
	m.input = newInput(cfg)
	m.selected = 0
	m.multiSelected = map[int]bool{}
	return m
}

func (m Model) Active() bool { return m.active }

func (m Model) Value() string {
	if !m.active && m.cfg.Mode != ModeText {
		return ""
	}
	return m.input.Value()
}

func (m Model) Update(msg tea.Msg) (Model, Result, tea.Cmd) {
	if !m.active {
		return m, Result{}, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, Result{}, nil
	}
	switch key.String() {
	case "esc", "ctrl+c":
		m.active = false
		return m, Result{Canceled: true}, nil
	case "enter":
		return m.submit(), m.result(), nil
	}

	switch m.cfg.Mode {
	case ModeText:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, Result{}, cmd
	case ModePicker, ModeMultiPicker:
		switch key.String() {
		case "j", "down", "right":
			if m.selected < len(m.cfg.Options)-1 {
				m.selected++
			}
		case "k", "up", "left":
			if m.selected > 0 {
				m.selected--
			}
		case " ", "space":
			if m.cfg.Mode == ModeMultiPicker && len(m.cfg.Options) > 0 {
				if m.multiSelected[m.selected] {
					delete(m.multiSelected, m.selected)
				} else {
					m.multiSelected[m.selected] = true
				}
			}
		}
	case ModeConfirm:
		switch strings.ToLower(key.String()) {
		case "y":
			return m.submit(), m.result(), nil
		case "n":
			m.active = false
			return m, Result{Canceled: true}, nil
		}
	}
	return m, Result{}, nil
}

func (m Model) View(width int) string {
	if !m.active {
		return ""
	}
	var lines []string
	if m.cfg.Title != "" {
		lines = append(lines, "Action: "+m.cfg.Title)
	}
	if m.cfg.Message != "" {
		lines = append(lines, m.cfg.Message)
	}
	switch m.cfg.Mode {
	case ModeText:
		value := m.input.Value()
		if value == "" && m.cfg.Placeholder != "" {
			value = m.cfg.Placeholder
		}
		lines = append(lines, "> "+value)
	case ModePicker, ModeMultiPicker:
		if len(m.cfg.Options) == 0 {
			lines = append(lines, "No choices available")
		}
		for i, option := range m.cfg.Options {
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			if m.cfg.Mode == ModeMultiPicker {
				mark := "[ ] "
				if m.multiSelected[i] {
					mark = "[x] "
				}
				lines = append(lines, prefix+mark+option.Label)
				continue
			}
			lines = append(lines, prefix+option.Label)
		}
	default:
		lines = append(lines, "enter: confirm | esc: cancel")
	}
	if m.cfg.Mode == ModeMultiPicker {
		lines = append(lines, "space: toggle | enter: submit | esc: cancel")
	} else if m.cfg.Mode != ModeConfirm {
		lines = append(lines, "enter: submit | esc: cancel")
	}
	for i, line := range lines {
		lines[i] = fit(line, width)
	}
	return strings.Join(lines, "\n")
}

func newInput(cfg Config) textinput.Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = cfg.Placeholder
	ti.SetValue(cfg.Initial)
	ti.Focus()
	ti.CursorEnd()
	return ti
}

func (m Model) submit() Model {
	m.active = false
	return m
}

func (m Model) result() Result {
	switch m.cfg.Mode {
	case ModeText:
		return Result{Submitted: true, Value: m.input.Value()}
	case ModePicker:
		if len(m.cfg.Options) == 0 {
			return Result{Submitted: true}
		}
		option := m.cfg.Options[m.selected]
		return Result{Submitted: true, Value: option.Value, Label: option.Label}
	case ModeMultiPicker:
		var values []string
		var labels []string
		for i, option := range m.cfg.Options {
			if !m.multiSelected[i] {
				continue
			}
			values = append(values, option.Value)
			labels = append(labels, option.Label)
		}
		return Result{Submitted: true, Value: strings.Join(values, ","), Label: strings.Join(labels, ", ")}
	default:
		return Result{Submitted: true, Value: "confirm"}
	}
}

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}
