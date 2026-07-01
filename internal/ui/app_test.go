package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func fetchedMsg(prs []data.PullRequest) context.TaskFinishedMsg {
	return context.TaskFinishedMsg{
		SectionId:   0,
		SectionType: pullsection.SectionType,
		TaskId:      "t1",
		Msg: pullsection.SectionPullRequestsFetchedMsg{
			Prs: prs, TotalCount: len(prs), TaskId: "t1",
		},
	}
}

func TestModelRendersLoadedPulls(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 128, Title: "Add wiki CLI", RepoNameWithOwner: "gitea/tea",
		Author: "lunny", State: "open", UpdatedAt: time.Now().Add(-2 * time.Hour),
	}}))

	view := m.View().Content
	for _, want := range []string{"#128", "Add wiki CLI", "gitea/tea", "@lunny", "1 pull requests"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q\n---\n%s", want, view)
		}
	}
}

func TestModelRendersError(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: pullsection.SectionType, TaskId: "t1",
		Msg: pullsection.SectionPullRequestsFetchedMsg{TaskId: "t1", Err: errBoom},
	})

	view := m.View().Content
	if !strings.Contains(view, "Error") || !strings.Contains(view, "boom") {
		t.Fatalf("expected an error view, got:\n%s", view)
	}
}

func TestQuitKeyStopsProgram(t *testing.T) {
	m := New(&config.Config{}, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected a quit command, got nil")
	}
}

func TestUnknownSectionIsNoOp(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 1, Title: "One", RepoNameWithOwner: "gitea/tea", Author: "me", State: "open",
	}}))

	s := m.getCurrSection()
	wantRows, wantTotal := s.NumRows(), s.GetTotalCount()

	// Correct id, WRONG type: the compound (id && type) guard must reject this
	// without touching the section, and return no command.
	next, cmd := m.Update(context.TaskFinishedMsg{SectionId: 0, SectionType: "nope", TaskId: "x"})
	m = next.(Model)
	if cmd != nil {
		t.Fatalf("expected nil cmd for a type-mismatched TaskFinishedMsg, got %v", cmd)
	}
	s = m.getCurrSection()
	if s.NumRows() != wantRows || s.GetTotalCount() != wantTotal {
		t.Fatalf("section changed: rows=%d (want %d), total=%d (want %d)",
			s.NumRows(), wantRows, s.GetTotalCount(), wantTotal)
	}
}

func TestOpenKeyWithNoRowsIsNoOp(t *testing.T) {
	m := New(&config.Config{}, nil)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("open with no rows panicked: %v", r)
		}
	}()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		t.Fatalf("expected nil cmd when there are no rows, got %v", cmd)
	}
}

func TestRefreshGatedWhileLoading(t *testing.T) {
	m := New(&config.Config{}, nil)
	// A fresh model starts in the loading state, so refresh is a no-op.
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"}); cmd != nil {
		t.Fatalf("refresh while loading should be a no-op, got %v", cmd)
	}
	// Once a fetch result lands (loading=false), refresh triggers a new fetch.
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, fetchedMsg(nil))
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"}); cmd == nil {
		t.Fatal("refresh after load should trigger a fetch, got nil cmd")
	}
}

func TestFetchRowsRegistersTask(t *testing.T) {
	m := New(&config.Config{}, nil)
	// Exercise the real StartTask closure wired in New: FetchRows registers the
	// task synchronously (the nil-client fetch closure is not run here).
	_ = m.getCurrSection().FetchRows()
	if len(m.tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1 after FetchRows", len(m.tasks))
	}
}

func TestSwitchViewBuildsIssues(t *testing.T) {
	m := New(&config.Config{}, nil)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(Model)
	if m.ctx.View != context.IssuesView {
		t.Fatalf("View = %v, want IssuesView", m.ctx.View)
	}
	if len(m.issues) == 0 {
		t.Fatal("expected issues sections to be built lazily on switch")
	}
	if cmd == nil {
		t.Fatal("expected a fetch command after switching to the issues view")
	}
}

func TestSectionSwitchWithTwoSections(t *testing.T) {
	cfg := &config.Config{
		PRSections: []config.SectionConfig{
			{Title: "A", Filter: config.PrIssueFilter{State: "open", CreatedBy: "@me"}},
			{Title: "B", Filter: config.PrIssueFilter{State: "open"}},
		},
	}
	m := New(cfg, nil)
	if len(m.prs) != 2 {
		t.Fatalf("len(prs) = %d, want 2 (config-driven sections)", len(m.prs))
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.currSectionId != 1 {
		t.Fatalf("after 'l' currSectionId = %d, want 1", m.currSectionId)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'h', Text: "h"})
	if m.currSectionId != 0 {
		t.Fatalf("after 'h' currSectionId = %d, want 0", m.currSectionId)
	}
}

var errBoom = errBoomType("boom")

type errBoomType string

func (e errBoomType) Error() string { return string(e) }
