// Package ui contains the Bubble Tea models that make up the tea-dash TUI.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// section identifies a top-level dashboard view.
type section int

const (
	sectionPulls section = iota
	sectionIssues
	sectionNotifications
	sectionCount
)

func (s section) name() string {
	switch s {
	case sectionPulls:
		return "Pull Requests"
	case sectionIssues:
		return "Issues"
	case sectionNotifications:
		return "Notifications"
	default:
		return "Unknown"
	}
}

// teaCommand returns the `tea` invocation this section will eventually be
// backed by. It is shown as a placeholder until the data layer is wired up.
func (s section) teaCommand() string {
	switch s {
	case sectionPulls:
		return "tea pulls list -o json"
	case sectionIssues:
		return "tea issues list -o json"
	case sectionNotifications:
		return "tea notifications list -o json"
	default:
		return "tea"
	}
}

// Model is the root Bubble Tea model for tea-dash.
type Model struct {
	keys    keyMap
	section section
	width   int
	height  int
	ready   bool
}

// New returns an initialised root model.
func New() Model {
	return Model{
		keys:    defaultKeyMap(),
		section: sectionPulls,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.NextSection):
			m.section = (m.section + 1) % sectionCount
		case key.Matches(msg, m.keys.PrevSection):
			m.section = (m.section + sectionCount - 1) % sectionCount
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "Starting tea-dash…"
	}

	tabs := make([]string, 0, sectionCount)
	for s := section(0); s < sectionCount; s++ {
		style := inactiveTabStyle
		if s == m.section {
			style = activeTabStyle
		}
		tabs = append(tabs, style.Render(s.name()))
	}

	header := lipgloss.JoinHorizontal(
		lipgloss.Bottom,
		titleStyle.Render("tea-dash"),
		"  ",
		strings.Join(tabs, dividerStyle.Render(" · ")),
	)

	body := bodyStyle.Render(fmt.Sprintf(
		"%s\n\nNot wired up yet — this view will render the output of:\n\n  %s",
		m.section.name(),
		m.section.teaCommand(),
	))

	help := helpStyle.Render("tab/⇧tab: switch section · r: refresh · ?: help · q: quit")

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, help))
}
