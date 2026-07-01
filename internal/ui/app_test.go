package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/teacli"
)

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestModelRendersLoadedPulls(t *testing.T) {
	m := New(&config.Config{Repos: []string{"gitea/tea"}})
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, pullsLoadedMsg{items: []pullItem{
		{repo: "gitea/tea", pr: teacli.PullRequest{
			Number:    128,
			Title:     "Add wiki CLI",
			State:     "open",
			Poster:    &teacli.User{Login: "lunny"},
			UpdatedAt: time.Now().Add(-2 * time.Hour),
		}},
	}})

	view := m.View()
	for _, want := range []string{"#128", "Add wiki CLI", "gitea/tea", "@lunny", "1 pull requests"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q\n---\n%s", want, view)
		}
	}
}

func TestModelRendersError(t *testing.T) {
	m := New(&config.Config{})
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = update(t, m, errMsg{err: errors.New("boom")})

	view := m.View()
	if !strings.Contains(view, "Error") || !strings.Contains(view, "boom") {
		t.Fatalf("expected an error view, got:\n%s", view)
	}
}

func TestQuitKeyStopsProgram(t *testing.T) {
	m := New(&config.Config{})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected a quit command, got nil")
	}
}
