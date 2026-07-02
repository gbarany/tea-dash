package ui

import (
	stdctx "context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/auth"
	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	localgit "github.com/gbarany/tea-dash/internal/git"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/actions"
	"github.com/gbarany/tea-dash/internal/ui/components/actionsection"
	"github.com/gbarany/tea-dash/internal/ui/components/branchsection"
	"github.com/gbarany/tea-dash/internal/ui/components/issuesection"
	"github.com/gbarany/tea-dash/internal/ui/components/notificationsection"
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
			Rows: prs, TotalCount: len(prs), TaskId: "t1",
		},
	}
}

func notificationFetchedMsg(rows []data.Notification) context.TaskFinishedMsg {
	return context.TaskFinishedMsg{
		SectionId:   0,
		SectionType: notificationsection.SectionType,
		TaskId:      "n1",
		Msg: notificationsection.SectionNotificationsFetchedMsg{
			Rows: rows, TotalCount: len(rows), TaskId: "n1",
		},
	}
}

func fetchedIssuesMsg(issues []data.Issue) context.TaskFinishedMsg {
	return context.TaskFinishedMsg{
		SectionId:   0,
		SectionType: issuesection.SectionType,
		TaskId:      "i1",
		Msg: issuesection.SectionIssuesFetchedMsg{
			Rows: issues, TotalCount: len(issues), TaskId: "i1",
		},
	}
}

func actionFetchedMsg(rows []data.ActionRun) context.TaskFinishedMsg {
	return context.TaskFinishedMsg{
		SectionId:   0,
		SectionType: actionsection.SectionType,
		TaskId:      "a1",
		Msg: actionsection.SectionActionsFetchedMsg{
			Rows: rows, TotalCount: len(rows), TaskId: "a1",
		},
	}
}

func branchFetchedMsg(rows []localgit.Branch) context.TaskFinishedMsg {
	return context.TaskFinishedMsg{
		SectionId:   0,
		SectionType: branchsection.SectionType,
		TaskId:      "b1",
		Msg: branchsection.SectionBranchesFetchedMsg{
			Rows: rows, TotalCount: len(rows), TaskId: "b1",
		},
	}
}

func TestModelRendersLoadedPulls(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"}) // close default preview for full-width table assertions
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 128, Title: "Add wiki CLI", RepoNameWithOwner: "gitea/tea",
		Author: "lunny", State: "open", UpdatedAt: time.Now().Add(-2 * time.Hour),
	}}))

	view := m.View().Content
	// M1: total == 1 renders the singular ("1 pull request", not "1 pull requests").
	for _, want := range []string{"#128", "Add wiki CLI", "gitea/tea", "@lunny", "1 pull request"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q\n---\n%s", want, view)
		}
	}
	if strings.Contains(view, "1 pull requests") {
		t.Fatalf("status line should be singular for total==1, got:\n%s", view)
	}
}

func TestViewEnablesMouseCellMotion(t *testing.T) {
	m := New(&config.Config{}, nil)
	view := m.View()
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("MouseMode = %v, want MouseModeCellMotion", view.MouseMode)
	}
}

func TestMouseClickSelectsVisibleRowAndRefreshesPreview(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{
		{Number: 1, Title: "First", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
		{Number: 2, Title: "Second", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
	}))
	firstKey := m.selKey()
	m = update(t, m, enrichedMsg{
		key:  firstKey,
		pull: &data.PullDetail{Body: "firstdetailtoken", BaseRef: "main", HeadRef: "first"},
	})

	click := tea.MouseClickMsg{X: 3, Y: m.tableDataStartY() + 1, Button: tea.MouseLeft}
	next, _ := m.Update(click)
	m = next.(Model)

	if got := m.getCurrRowData().GetNumber(); got != 2 {
		t.Fatalf("clicked row number = %d, want 2", got)
	}
	view := m.View().Content
	if strings.Contains(view, "firstdetailtoken") {
		t.Fatalf("click should refresh preview away from first row detail:\n%s", view)
	}
	if !strings.Contains(view, "Loading") {
		t.Fatalf("click should show the newly selected row's loading preview:\n%s", view)
	}
}

func TestMouseClickInPreviewDoesNotChangeSelection(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{
		{Number: 1, Title: "First", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
		{Number: 2, Title: "Second", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
	}))

	previewX := 2 + m.ctx.MainContentWidth + 2
	click := tea.MouseClickMsg{X: previewX, Y: m.tableDataStartY() + 1, Button: tea.MouseLeft}
	m = update(t, m, click)

	if got := m.getCurrRowData().GetNumber(); got != 1 {
		t.Fatalf("selection changed after preview click: row = %d, want 1", got)
	}
}

func TestMouseWheelMovesSelectionInList(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{
		{Number: 1, Title: "First", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
		{Number: 2, Title: "Second", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
		{Number: 3, Title: "Third", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
	}))

	firstKey := m.selKey()
	m = update(t, m, enrichedMsg{
		key:  firstKey,
		pull: &data.PullDetail{Body: "firstdetailtoken", BaseRef: "main", HeadRef: "first"},
	})

	listY := m.tableDataStartY()
	m = update(t, m, tea.MouseWheelMsg{X: 3, Y: listY, Button: tea.MouseWheelDown})
	if got := m.getCurrRowData().GetNumber(); got != 2 {
		t.Fatalf("wheel down selected row = %d, want 2", got)
	}
	view := m.View().Content
	if strings.Contains(view, "firstdetailtoken") || !strings.Contains(view, "Loading") {
		t.Fatalf("wheel down should refresh preview to the selected row loading state:\n%s", view)
	}
	m = update(t, m, tea.MouseWheelMsg{X: 3, Y: listY, Button: tea.MouseWheelDown})
	if got := m.getCurrRowData().GetNumber(); got != 3 {
		t.Fatalf("second wheel down selected row = %d, want 3", got)
	}
	m = update(t, m, tea.MouseWheelMsg{X: 3, Y: listY, Button: tea.MouseWheelUp})
	if got := m.getCurrRowData().GetNumber(); got != 2 {
		t.Fatalf("wheel up selected row = %d, want 2", got)
	}
}

func TestMouseWheelInPreviewDoesNotMoveListSelection(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{
		{Number: 1, Title: "First", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
		{Number: 2, Title: "Second", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
	}))

	previewX := 2 + m.ctx.MainContentWidth + 2
	m = update(t, m, tea.MouseWheelMsg{X: previewX, Y: m.tableDataStartY(), Button: tea.MouseWheelDown})
	if got := m.getCurrRowData().GetNumber(); got != 1 {
		t.Fatalf("preview wheel changed list selection: row = %d, want 1", got)
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

func TestSwitchViewCyclesToNotifications(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(Model)
	if m.ctx.View != context.NotificationsView {
		t.Fatalf("View = %v, want NotificationsView", m.ctx.View)
	}
	if len(m.notifications) == 0 {
		t.Fatal("expected notification sections to be built lazily on switch")
	}
	if cmd == nil {
		t.Fatal("expected a fetch command after switching to the notifications view")
	}
}

func TestSwitchViewCyclesToActionsBranchesAndBackToPulls(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"}) // issues
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"}) // notifications
	next, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(Model)
	if m.ctx.View != context.ActionsView {
		t.Fatalf("View = %v, want ActionsView", m.ctx.View)
	}
	if len(m.actions) == 0 {
		t.Fatal("expected action sections to be built lazily on switch")
	}
	if cmd == nil {
		t.Fatal("expected a fetch command after switching to the actions view")
	}

	next, cmd = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(Model)
	if m.ctx.View != context.BranchesView {
		t.Fatalf("View = %v, want BranchesView", m.ctx.View)
	}
	if len(m.branches) == 0 {
		t.Fatal("expected branch sections to be built lazily on switch")
	}
	if cmd == nil {
		t.Fatal("expected a fetch command after switching to the branches view")
	}

	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.ctx.View != context.PullsView {
		t.Fatalf("next switch after branches should wrap to pulls: View = %v", m.ctx.View)
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
	// 'h' at the first section clamps (stays 0).
	m = update(t, m, tea.KeyPressMsg{Code: 'h', Text: "h"})
	if m.currSectionId != 0 {
		t.Fatalf("'h' at id 0 should clamp: currSectionId = %d, want 0", m.currSectionId)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.currSectionId != 1 {
		t.Fatalf("after 'l' currSectionId = %d, want 1", m.currSectionId)
	}
	// 'l' at the last section clamps (stays 1).
	m = update(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.currSectionId != 1 {
		t.Fatalf("'l' at last id should clamp: currSectionId = %d, want 1", m.currSectionId)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'h', Text: "h"})
	if m.currSectionId != 0 {
		t.Fatalf("after 'h' currSectionId = %d, want 0", m.currSectionId)
	}
}

func TestDefaultPullSectionsIncludeClosedHistory(t *testing.T) {
	m := New(&config.Config{}, nil)
	if len(m.prs) != 2 {
		t.Fatalf("len(prs) = %d, want open + closed default sections", len(m.prs))
	}

	open := m.prs[0].(*pullsection.Model).Config
	if open.Title != "Open Pull Requests" || open.Filter.State != "open" || open.Filter.CreatedBy != "@me" {
		t.Fatalf("open default section = %+v, want title/open/@me", open)
	}

	closed := m.prs[1].(*pullsection.Model).Config
	if closed.Title != "Closed Pull Requests" || closed.Filter.State != "closed" || closed.Filter.CreatedBy != "@me" {
		t.Fatalf("closed default section = %+v, want title/closed/@me", closed)
	}

	m = update(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.currSectionId != 1 {
		t.Fatalf("after 'l' currSectionId = %d, want the closed PR section", m.currSectionId)
	}
}

func TestClosedPullSectionEmptyStateIsNotOpenSpecific(t *testing.T) {
	m := New(&config.Config{
		PRSections: []config.SectionConfig{{
			Title:  "Closed Pull Requests",
			Filter: config.PrIssueFilter{State: "closed", CreatedBy: "@me"},
		}},
	}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, fetchedMsg(nil))

	view := m.View().Content
	if strings.Contains(view, "No open pull requests") {
		t.Fatalf("closed PR section must not render an open-specific empty state:\n%s", view)
	}
	if !strings.Contains(view, "No pull requests match this filter") {
		t.Fatalf("closed PR section missing generic empty state:\n%s", view)
	}
}

func TestSlashFocusesSearch(t *testing.T) {
	m := New(&config.Config{}, nil)
	if s := m.getCurrSection(); s != nil && s.IsSearchFocused() {
		t.Fatal("search should not be focused before '/' is pressed")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(Model)
	s := m.getCurrSection()
	if s == nil {
		t.Fatal("expected a current section")
	}
	if !s.IsSearchFocused() {
		t.Fatal("'/' should focus the current section's search bar")
	}
	if cmd == nil {
		t.Fatal("'/' should return the search bar's focus command")
	}
}

func TestDefaultsViewStartsIssues(t *testing.T) {
	m := New(&config.Config{Defaults: config.Defaults{View: "issues"}}, nil)
	if m.ctx.View != context.IssuesView {
		t.Fatalf("View = %v, want IssuesView", m.ctx.View)
	}
	if len(m.issues) == 0 {
		t.Fatal("defaults.view=issues should build the issues sections at startup")
	}
	if len(m.prs) != 0 {
		t.Fatalf("len(prs) = %d, want 0 (pulls stay lazy when issues is the start view)", len(m.prs))
	}
}

func TestDefaultsViewStartsNotifications(t *testing.T) {
	m := New(&config.Config{Defaults: config.Defaults{View: "notifications"}}, nil)
	if m.ctx.View != context.NotificationsView {
		t.Fatalf("View = %v, want NotificationsView", m.ctx.View)
	}
	if len(m.notifications) == 0 {
		t.Fatal("defaults.view=notifications should build the notification sections at startup")
	}
	if len(m.prs) != 0 || len(m.issues) != 0 {
		t.Fatalf("prs=%d issues=%d, want inactive views lazy", len(m.prs), len(m.issues))
	}
}

func TestDefaultsViewStartsActions(t *testing.T) {
	m := New(&config.Config{
		Defaults: config.Defaults{View: "actions"},
		ActionsSections: []config.SectionConfig{{
			Title: "CI",
			Repo:  "gbarany/tea-dash",
		}},
	}, nil)
	if m.ctx.View != context.ActionsView {
		t.Fatalf("View = %v, want ActionsView", m.ctx.View)
	}
	if len(m.actions) == 0 {
		t.Fatal("defaults.view=actions should build the action sections at startup")
	}
	if len(m.prs) != 0 || len(m.issues) != 0 || len(m.notifications) != 0 {
		t.Fatalf("prs=%d issues=%d notifications=%d, want inactive views lazy",
			len(m.prs), len(m.issues), len(m.notifications))
	}
}

func TestDefaultsViewStartsBranches(t *testing.T) {
	m := New(&config.Config{Defaults: config.Defaults{View: "branches"}}, nil)
	if m.ctx.View != context.BranchesView {
		t.Fatalf("View = %v, want BranchesView", m.ctx.View)
	}
	if len(m.branches) == 0 {
		t.Fatal("defaults.view=branches should build the branch sections at startup")
	}
	if len(m.prs) != 0 || len(m.issues) != 0 || len(m.notifications) != 0 {
		t.Fatalf("prs=%d issues=%d notifications=%d, want inactive views lazy", len(m.prs), len(m.issues), len(m.notifications))
	}
}

// TestCrossViewFetchRoutesToOwnSlice verifies a late PR fetch that lands while
// the Issues view is active is still routed to the pulls slice by (id, type),
// not to whatever section is currently on screen.
func TestCrossViewFetchRoutesToOwnSlice(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	// Switch to the Issues view (pulls section 0 stays in m.prs, now off-screen).
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.ctx.View != context.IssuesView {
		t.Fatalf("View = %v, want IssuesView", m.ctx.View)
	}
	// A pull-request fetch result arrives for section 0 of the pulls view.
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: pullsection.SectionType, TaskId: "t1",
		Msg: pullsection.SectionPullRequestsFetchedMsg{
			Rows: []data.PullRequest{{
				Number: 5, Title: "Late PR", RepoNameWithOwner: "gitea/tea", Author: "me", State: "open",
			}},
			TotalCount: 1, TaskId: "t1",
		},
	})
	// Switch through Notifications and Branches back to the pulls view; the
	// fetch must have landed in m.prs rather than whatever view is on screen.
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.ctx.View != context.PullsView {
		t.Fatalf("View = %v, want PullsView", m.ctx.View)
	}
	if got := m.getCurrSection().NumRows(); got != 1 {
		t.Fatalf("pulls section NumRows = %d, want 1 (cross-view fetch must route to m.prs)", got)
	}
}

// TestSearchFocusDivertsCommandKeys verifies that while the search bar is
// focused, command keys ('q', 's') are typed into the box instead of quitting
// or switching views.
func TestSearchFocusDivertsCommandKeys(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.getCurrSection().IsSearchFocused() {
		t.Fatal("'/' should focus the search bar")
	}
	// 'q' must not quit; it is typed into the search box.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = next.(Model)
	if isQuitCmd(cmd) {
		t.Fatal("'q' while searching must not quit the program")
	}
	if got := m.getCurrSection().(*pullsection.Model).SearchBar.Value(); !strings.Contains(got, "q") {
		t.Fatalf("search value = %q, want it to contain the typed \"q\"", got)
	}
	// 's' must not switch views; it is typed into the search box too.
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.ctx.View != context.PullsView {
		t.Fatalf("'s' while searching must not switch views: View = %v", m.ctx.View)
	}
}

// TestShowingCountAndSingular exercises the two non-plural status-line branches:
// "showing X of Y" when the page is capped, and the singular for total==1.
func TestShowingCountAndSingular(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	// One row shown, server total 5 -> "showing 1 of 5 pull requests".
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: pullsection.SectionType, TaskId: "t1",
		Msg: pullsection.SectionPullRequestsFetchedMsg{
			Rows: []data.PullRequest{{
				Number: 1, Title: "One", RepoNameWithOwner: "gitea/tea", Author: "me", State: "open",
			}},
			TotalCount: 5, TaskId: "t1",
		},
	})
	if view := m.View().Content; !strings.Contains(view, "showing 1 of 5 pull requests") {
		t.Fatalf("status line missing \"showing 1 of 5 pull requests\":\n%s", view)
	}
	// total == shown == 1 -> singular "1 pull request".
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 2, Title: "Two", RepoNameWithOwner: "gitea/tea", Author: "me", State: "open",
	}}))
	view := m.View().Content
	if !strings.Contains(view, "1 pull request") || strings.Contains(view, "1 pull requests") {
		t.Fatalf("status line should read singular \"1 pull request\":\n%s", view)
	}
}

// TestModelRendersLoadedIssues mirrors TestShowingCountAndSingular but for the
// ISSUES view, guarding the issue-specific status-line wording: a copy-pasted
// "pull request" phrasing in issuesection.NewModel would fail here. It routes an
// issues fetch through the real Update path and asserts the rendered View().
func TestModelRendersLoadedIssues(t *testing.T) {
	m := New(&config.Config{Defaults: config.Defaults{View: "issues"}}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"}) // close default preview for full-width table assertions
	if m.ctx.View != context.IssuesView {
		t.Fatalf("View = %v, want IssuesView", m.ctx.View)
	}

	// One row shown, server total 5 -> "showing 1 of 5 issues" (plural wording).
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: issuesection.SectionType, TaskId: "t1",
		Msg: issuesection.SectionIssuesFetchedMsg{
			Rows: []data.Issue{{
				Number: 7, Title: "Fix flaky test", RepoNameWithOwner: "gitea/tea",
				Author: "me", State: "open", UpdatedAt: time.Now().Add(-time.Hour),
			}},
			TotalCount: 5, TaskId: "t1",
		},
	})
	view := m.View().Content
	for _, want := range []string{"#7", "Fix flaky test", "gitea/tea", "@me", "showing 1 of 5 issues"} {
		if !strings.Contains(view, want) {
			t.Fatalf("issues view is missing %q\n---\n%s", want, view)
		}
	}

	// total == shown == 1 -> singular "1 issue" (never "1 issues").
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: issuesection.SectionType, TaskId: "t2",
		Msg: issuesection.SectionIssuesFetchedMsg{
			Rows: []data.Issue{{
				Number: 8, Title: "Only one", RepoNameWithOwner: "gitea/tea",
				Author: "me", State: "open", UpdatedAt: time.Now().Add(-time.Hour),
			}},
			TotalCount: 1, TaskId: "t2",
		},
	})
	view = m.View().Content
	if !strings.Contains(view, "1 issue") || strings.Contains(view, "1 issues") {
		t.Fatalf("status line should read singular \"1 issue\":\n%s", view)
	}
}

func TestModelRendersLoadedNotifications(t *testing.T) {
	m := New(&config.Config{Defaults: config.Defaults{View: "notifications"}}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"}) // close preview for table assertions
	m = update(t, m, notificationFetchedMsg([]data.Notification{{
		ID: 12, Number: 42, SubjectTitle: "Review the new dashboard",
		SubjectType: "Pull", SubjectState: "open", RepoNameWithOwner: "gbarany/tea-dash",
		Unread: true, HTMLURL: "https://git.example/gbarany/tea-dash/pulls/42",
		UpdatedAt: time.Now().Add(-time.Hour),
	}}))

	view := m.View().Content
	for _, want := range []string{"#42", "Review the new dashboard", "gbarany/tea-dash", "pull", "unread", "1 notification"} {
		if !strings.Contains(view, want) {
			t.Fatalf("notifications view is missing %q\n---\n%s", want, view)
		}
	}
}

func TestMarkSelectedNotificationReadRefreshesNotifications(t *testing.T) {
	var hit bool
	client := newNotificationActionClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/notifications/threads/12" {
				http.NotFound(w, r)
				return
			}
			hit = true
			if r.Method != http.MethodPatch {
				t.Fatalf("method = %s, want PATCH", r.Method)
			}
			if got := r.URL.Query().Get("to-status"); got != "read" {
				t.Fatalf("to-status = %q, want read", got)
			}
			fmt.Fprint(w, `{"id":12,"unread":false}`)
		},
		nil,
	)
	m := New(&config.Config{Defaults: config.Defaults{View: "notifications"}}, client)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, notificationFetchedMsg([]data.Notification{{
		ID: 12, Number: 42, SubjectTitle: "Review the new dashboard",
		SubjectType: "Pull", SubjectState: "open", RepoNameWithOwner: "gbarany/tea-dash",
		Unread: true, HTMLURL: "https://git.example/gbarany/tea-dash/pulls/42",
	}}))

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if cmd == nil {
		t.Fatal("'m' on a notification should return a mark-read command")
	}
	m = next.(Model)
	msg := cmd()
	if !hit {
		t.Fatal("mark-read command did not call the notification thread endpoint")
	}
	next, refresh := m.Update(msg)
	m = next.(Model)
	if refresh == nil {
		t.Fatal("successful mark-read should refresh the notifications section")
	}
	if !strings.Contains(m.notice, "Marked notification read") {
		t.Fatalf("notice = %q, want mark-read confirmation", m.notice)
	}
}

func TestMarkSelectedNotificationUnreadRefreshesNotifications(t *testing.T) {
	var hit bool
	client := newNotificationActionClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/notifications/threads/12" {
				http.NotFound(w, r)
				return
			}
			hit = true
			if r.Method != http.MethodPatch {
				t.Fatalf("method = %s, want PATCH", r.Method)
			}
			if got := r.URL.Query().Get("to-status"); got != "unread" {
				t.Fatalf("to-status = %q, want unread", got)
			}
			fmt.Fprint(w, `{"id":12,"unread":true}`)
		},
		nil,
	)
	m := New(&config.Config{Defaults: config.Defaults{View: "notifications"}}, client)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, notificationFetchedMsg([]data.Notification{{
		ID: 12, Number: 42, SubjectTitle: "Review the new dashboard",
		SubjectType: "Pull", SubjectState: "open", RepoNameWithOwner: "gbarany/tea-dash",
		Unread: false, HTMLURL: "https://git.example/gbarany/tea-dash/pulls/42",
	}}))

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd == nil {
		t.Fatal("'u' on a notification should return a mark-unread command")
	}
	m = next.(Model)
	msg := cmd()
	if !hit {
		t.Fatal("mark-unread command did not call the notification thread endpoint")
	}
	next, refresh := m.Update(msg)
	m = next.(Model)
	if refresh == nil {
		t.Fatal("successful mark-unread should refresh the notifications section")
	}
	if !strings.Contains(m.notice, "Marked notification unread") {
		t.Fatalf("notice = %q, want mark-unread confirmation", m.notice)
	}
}

func TestMarkAllNotificationsReadRefreshesNotifications(t *testing.T) {
	var hit bool
	client := newNotificationActionClient(t,
		nil,
		func(w http.ResponseWriter, r *http.Request) {
			hit = true
			if r.Method != http.MethodPut {
				t.Fatalf("method = %s, want PUT", r.Method)
			}
			q := r.URL.Query()
			if got := q.Get("to-status"); got != "read" {
				t.Fatalf("to-status = %q, want read", got)
			}
			if got := q["status-types"]; len(got) != 1 || got[0] != "unread" {
				t.Fatalf("status-types = %v, want [unread]", got)
			}
			fmt.Fprint(w, `[]`)
		},
	)
	m := New(&config.Config{Defaults: config.Defaults{View: "notifications"}}, client)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, notificationFetchedMsg([]data.Notification{{
		ID: 12, Number: 42, SubjectTitle: "Review the new dashboard",
		SubjectType: "Pull", SubjectState: "open", RepoNameWithOwner: "gbarany/tea-dash",
		Unread: true, HTMLURL: "https://git.example/gbarany/tea-dash/pulls/42",
	}}))

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'M', Text: "M"})
	if cmd == nil {
		t.Fatal("'M' in notifications should return a mark-all-read command")
	}
	m = next.(Model)
	msg := cmd()
	if !hit {
		t.Fatal("mark-all command did not call the notifications endpoint")
	}
	next, refresh := m.Update(msg)
	m = next.(Model)
	if refresh == nil {
		t.Fatal("successful mark-all-read should refresh the notifications section")
	}
	if !strings.Contains(m.notice, "Marked all notifications read") {
		t.Fatalf("notice = %q, want mark-all confirmation", m.notice)
	}
}

func TestModelRendersLoadedActions(t *testing.T) {
	m := New(&config.Config{
		Defaults: config.Defaults{View: "actions"},
		ActionsSections: []config.SectionConfig{{
			Title: "CI",
			Repo:  "gbarany/tea-dash",
		}},
	}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"}) // close preview for table assertions
	m = update(t, m, actionFetchedMsg([]data.ActionRun{{
		ID: 101, RunNumber: 77, DisplayTitle: "CI passed", WorkflowName: "CI",
		RepoNameWithOwner: "gbarany/tea-dash", Actor: "octo", Event: "push",
		Status: "completed", Conclusion: "success", UpdatedAt: time.Now().Add(-time.Hour),
	}}))

	view := m.View().Content
	for _, want := range []string{"#77", "CI passed", "gbarany/tea-dash", "@octo push", "completed/success", "1 action run"} {
		if !strings.Contains(view, want) {
			t.Fatalf("actions view is missing %q\n---\n%s", want, view)
		}
	}
}

func TestModelRendersLoadedBranches(t *testing.T) {
	m := New(&config.Config{Defaults: config.Defaults{View: "branches"}}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"}) // close preview for table assertions
	m = update(t, m, branchFetchedMsg([]localgit.Branch{{
		Repository: "tea-dash", Name: "m5/repo-branches", Current: true,
		Upstream: "origin/m4/notifications", Ahead: 1, Commit: "abc1234",
		Subject: "Add branches view", UpdatedAt: time.Now().Add(-time.Hour),
	}}))

	view := m.View().Content
	for _, want := range []string{"m5/repo-branches", "tea-dash", "origin/m4/notifications", "current", "ahead 1", "1 branch"} {
		if !strings.Contains(view, want) {
			t.Fatalf("branches view is missing %q\n---\n%s", want, view)
		}
	}
}

func TestActionsPreviewRendersStaticSummary(t *testing.T) {
	m := New(&config.Config{
		Defaults: config.Defaults{View: "actions"},
		ActionsSections: []config.SectionConfig{{
			Title: "CI",
			Repo:  "gbarany/tea-dash",
		}},
	}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, actionFetchedMsg([]data.ActionRun{{
		ID: 101, RunNumber: 77, DisplayTitle: "CI passed", WorkflowName: "CI",
		RepoNameWithOwner: "gbarany/tea-dash", Actor: "octo", Event: "push",
		Status: "completed", Conclusion: "success", HeadBranch: "main", HeadSHA: "abc123",
		UpdatedAt: time.Now().Add(-time.Hour),
	}}))

	view := m.View().Content
	for _, want := range []string{"gbarany/tea-dash · #77", "CI passed", "completed/success", "main", "abc123", "push", "@octo"} {
		if !strings.Contains(view, want) {
			t.Fatalf("actions preview missing %q\n---\n%s", want, view)
		}
	}
}

func TestActionsPreviewFetchesAndCachesRunDetail(t *testing.T) {
	var runCalls, jobCalls int
	srv := actionDetailServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/repos/gbarany/tea-dash/actions/runs/101", func(w http.ResponseWriter, _ *http.Request) {
			runCalls++
			fmt.Fprint(w, `{
				"id": 101,
				"run_number": 77,
				"display_title": "CI passed",
				"name": "CI",
				"status": "completed",
				"conclusion": "success",
				"head_branch": "main",
				"head_sha": "abc123"
			}`)
		})
		mux.HandleFunc("/api/v1/repos/gbarany/tea-dash/actions/runs/101/jobs", func(w http.ResponseWriter, _ *http.Request) {
			jobCalls++
			fmt.Fprint(w, `{
				"jobs": [{
					"id": 201,
					"run_id": 101,
					"name": "build",
					"status": "completed",
					"conclusion": "success",
					"runner_name": "ubuntu-latest",
					"steps": [{
						"number": 1,
						"name": "checkout",
						"status": "completed",
						"conclusion": "success"
					}]
				}]
			}`)
		})
	})
	client, err := gitea.NewClient(stdctx.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	m := New(&config.Config{
		Defaults: config.Defaults{View: "actions"},
		ActionsSections: []config.SectionConfig{{
			Title: "CI",
			Repo:  "gbarany/tea-dash",
		}},
	}, client)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	next, cmd := m.Update(actionFetchedMsg([]data.ActionRun{{
		ID: 101, RunNumber: 77, DisplayTitle: "CI passed", WorkflowName: "CI",
		RepoNameWithOwner: "gbarany/tea-dash", Status: "completed", Conclusion: "success",
	}}))
	m = next.(Model)
	msg := runEnrichedCommand(t, cmd)
	if msg.err != nil {
		t.Fatalf("action detail fetch returned error: %v", msg.err)
	}
	if msg.sectionType != actionsection.SectionType || msg.action == nil {
		t.Fatalf("enrichedMsg = %+v, want action detail for action section", msg)
	}

	m = update(t, m, msg)
	key := m.selKey()
	if _, ok := m.actionDetails[key]; !ok {
		t.Fatalf("actionDetails should contain %q after enrichedMsg", key)
	}
	view := m.View().Content
	for _, want := range []string{"Jobs:", "build", "ubuntu-latest", "checkout"} {
		if !strings.Contains(view, want) {
			t.Fatalf("action detail preview missing %q:\n%s", want, view)
		}
	}
	if runCalls != 1 || jobCalls != 1 {
		t.Fatalf("detail endpoints called run=%d jobs=%d, want once each", runCalls, jobCalls)
	}
	if cmd := m.enrichCurrRow(); cmd != nil {
		t.Fatal("cached action detail should suppress another lazy fetch")
	}
}

// TestPreviewStartsOpenAndToggles verifies the preview pane is visible by
// default: the initial window size gives it dimensions and the composed View()
// renders the sidebar region. 'p' still toggles it closed and open again.
func TestPreviewStartsOpenAndToggles(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 7, Title: "Preview me", RepoNameWithOwner: "gitea/tea",
		Author: "me", State: "open",
	}}))
	if !m.ctx.PreviewOpen {
		t.Fatal("preview should start open")
	}
	if m.ctx.PreviewWidth <= 0 {
		t.Fatalf("preview width should be positive when open, got %d", m.ctx.PreviewWidth)
	}
	view := m.View().Content
	if !strings.Contains(view, "Loading") {
		t.Fatalf("default-open preview should render the sidebar region (Loading placeholder):\n%s", view)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"})
	if m.ctx.PreviewOpen {
		t.Fatal("'p' should close the preview")
	}
	if m.ctx.PreviewWidth != 0 {
		t.Fatalf("closed preview width should be 0, got %d", m.ctx.PreviewWidth)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"})
	if !m.ctx.PreviewOpen {
		t.Fatal("second 'p' should reopen the preview")
	}
}

// TestEnrichedMsgPopulatesSidebar verifies that, with the preview open, an
// enrichedMsg for the selected row's key is cached and the sidebar re-renders to
// show the fetched body (and no longer the loading placeholder).
func TestEnrichedMsgPopulatesSidebar(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 42, Title: "Add preview", RepoNameWithOwner: "gbarany/tea-dash",
		Author: "me", State: "open",
	}}))

	key := m.selKey()
	if key != "gbarany/tea-dash#42" {
		t.Fatalf("selKey = %q, want gbarany/tea-dash#42", key)
	}
	m = update(t, m, enrichedMsg{
		key:  key,
		pull: &data.PullDetail{Body: "uniquebodytoken", BaseRef: "main", HeadRef: "feature"},
	})
	if _, ok := m.pullDetails[key]; !ok {
		t.Fatalf("pullDetails should contain %q after enrichedMsg", key)
	}
	view := m.View().Content
	if !strings.Contains(view, "uniquebodytoken") {
		t.Fatalf("sidebar should reflect the enriched body:\n%s", view)
	}
	if strings.Contains(view, "Loading") {
		t.Fatalf("sidebar should no longer show Loading after enrichment:\n%s", view)
	}
}

func TestEnrichedMsgErrorRendersFailureAndCanRecover(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 9, Title: "Needs detail", RepoNameWithOwner: "gbarany/tea-dash",
		Author: "me", State: "open",
	}}))

	key := m.selKey()
	m = update(t, m, enrichedMsg{key: key, sectionType: pullsection.SectionType, err: errBoom})
	view := m.View().Content
	for _, want := range []string{"Failed to load preview", "boom", "Press r to retry"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failed preview missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Loading") {
		t.Fatalf("failed preview should not look like a perpetual loading state:\n%s", view)
	}

	m = update(t, m, enrichedMsg{
		key:  key,
		pull: &data.PullDetail{Body: "recoveredbodytoken", BaseRef: "main", HeadRef: "feature"},
	})
	view = m.View().Content
	if strings.Contains(view, "Failed to load preview") || !strings.Contains(view, "recoveredbodytoken") {
		t.Fatalf("successful retry should replace the failed preview:\n%s", view)
	}
	if _, ok := m.pullEnrichErr[key]; ok {
		t.Fatalf("pullEnrichErr should be cleared after a successful retry for %q", key)
	}
}

func TestRefreshClearsSelectedPreviewCache(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 11, Title: "Refresh detail", RepoNameWithOwner: "gbarany/tea-dash",
		Author: "me", State: "open",
	}}))

	key := m.selKey()
	m = update(t, m, enrichedMsg{
		key:  key,
		pull: &data.PullDetail{Body: "staledetailtoken", BaseRef: "main", HeadRef: "feature"},
	})
	m = update(t, m, enrichedMsg{key: key, sectionType: pullsection.SectionType, err: errBoom})

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("refresh after load should still trigger a fetch")
	}
	m = next.(Model)
	if _, ok := m.pullDetails[key]; ok {
		t.Fatalf("refresh should clear cached pull detail for %q", key)
	}
	if _, ok := m.pullEnrichErr[key]; ok {
		t.Fatalf("refresh should clear cached preview error for %q", key)
	}
	view := m.View().Content
	if strings.Contains(view, "staledetailtoken") || strings.Contains(view, "Failed to load preview") {
		t.Fatalf("refresh should replace stale preview content with a loading state:\n%s", view)
	}
	if !strings.Contains(view, "Loading") {
		t.Fatalf("refresh should leave the selected preview ready to retry detail loading:\n%s", view)
	}
}

func TestPreviewErrorsDoNotLeakBetweenPullsAndIssuesWithSameNumber(t *testing.T) {
	m := New(&config.Config{Defaults: config.Defaults{View: "issues"}}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: issuesection.SectionType, TaskId: "i1",
		Msg: issuesection.SectionIssuesFetchedMsg{
			Rows: []data.Issue{{
				Number: 7, Title: "Issue seven", RepoNameWithOwner: "gbarany/tea-dash",
				Author: "me", State: "open",
			}},
			TotalCount: 1, TaskId: "i1",
		},
	})

	key := m.selKey()
	m = update(t, m, enrichedMsg{
		key:   key,
		issue: &data.IssueDetail{Body: "issuebodytoken"},
	})

	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"}) // notifications
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"}) // actions
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"}) // branches
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"}) // pulls
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: pullsection.SectionType, TaskId: "p1",
		Msg: pullsection.SectionPullRequestsFetchedMsg{
			Rows: []data.PullRequest{{
				Number: 7, Title: "Pull seven", RepoNameWithOwner: "gbarany/tea-dash",
				Author: "me", State: "open",
			}},
			TotalCount: 1, TaskId: "p1",
		},
	})
	m = update(t, m, enrichedMsg{key: key, sectionType: pullsection.SectionType, err: errBoom})

	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"}) // issues
	view := m.View().Content
	if !strings.Contains(view, "issuebodytoken") {
		t.Fatalf("issue detail with the same repo/number should remain visible:\n%s", view)
	}
	if strings.Contains(view, "Failed to load preview") {
		t.Fatalf("pull preview error should not leak into issue preview:\n%s", view)
	}
}

func TestPreviewRefreshesWhenSwitchingSections(t *testing.T) {
	cfg := &config.Config{
		PRSections: []config.SectionConfig{
			{Title: "Mine", Filter: config.PrIssueFilter{CreatedBy: "@me"}},
			{Title: "Review", Filter: config.PrIssueFilter{ReviewRequested: "@me"}},
		},
	}
	m := New(cfg, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: pullsection.SectionType, TaskId: "p0",
		Msg: pullsection.SectionPullRequestsFetchedMsg{
			Rows: []data.PullRequest{{
				Number: 1, Title: "First row", RepoNameWithOwner: "gbarany/tea-dash",
				Author: "me", State: "open",
			}},
			TotalCount: 1, TaskId: "p0",
		},
	})
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 1, SectionType: pullsection.SectionType, TaskId: "p1",
		Msg: pullsection.SectionPullRequestsFetchedMsg{
			Rows: []data.PullRequest{{
				Number: 2, Title: "Second row", RepoNameWithOwner: "gbarany/tea-dash",
				Author: "me", State: "open",
			}},
			TotalCount: 1, TaskId: "p1",
		},
	})
	firstKey := m.selKey()
	m = update(t, m, enrichedMsg{
		key:  firstKey,
		pull: &data.PullDetail{Body: "firstdetailbody", BaseRef: "main", HeadRef: "one"},
	})
	if view := m.View().Content; !strings.Contains(view, "firstdetailbody") {
		t.Fatalf("preview should show first section detail before switching:\n%s", view)
	}

	m = update(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	view := m.View().Content
	if strings.Contains(view, "firstdetailbody") {
		t.Fatalf("section switch should not leave the old row detail in the preview:\n%s", view)
	}
	if !strings.Contains(view, "Second row") || !strings.Contains(view, "Loading") {
		t.Fatalf("section switch should render the newly selected row's loading preview:\n%s", view)
	}
}

func TestPreviewLayoutSmallTerminalDoesNotPanic(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 1, Height: 1})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 3, Title: "Tiny", RepoNameWithOwner: "gbarany/tea-dash",
		Author: "me", State: "open",
	}}))
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("tiny preview layout panicked: %v", r)
		}
	}()
	_ = m.View()
}

// TestPreviewToggleGatedWhileSearching verifies 'p' does not toggle the preview
// while the search bar is focused; it is typed into the box like any other key.
func TestPreviewToggleGatedWhileSearching(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.getCurrSection().IsSearchFocused() {
		t.Fatal("'/' should focus the search bar")
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"})
	if !m.ctx.PreviewOpen {
		t.Fatal("'p' while searching must not toggle the default-open preview")
	}
	if got := m.getCurrSection().(*pullsection.Model).SearchBar.Value(); !strings.Contains(got, "p") {
		t.Fatalf("search value = %q, want it to contain the typed \"p\"", got)
	}
}

func TestActiveActionPromptCapturesGlobalKeys(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 42, Title: "Prompt row", RepoNameWithOwner: "gbarany/tea-dash",
		Author: "me", State: "open",
	}}))

	m = update(t, m, tea.KeyPressMsg{Code: 'c', Text: "c"})
	if !m.actionPrompt.Active() {
		t.Fatal("'c' should open an action prompt")
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = next.(Model)
	if isQuitCmd(cmd) {
		t.Fatal("'q' while an action prompt is active must not quit")
	}
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.ctx.View != context.PullsView {
		t.Fatalf("'s' while an action prompt is active must not switch views: %v", m.ctx.View)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"})
	if !m.ctx.PreviewOpen {
		t.Fatal("'p' while an action prompt is active must not toggle the default-open preview")
	}
	m = update(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	if m.getCurrSection().IsSearchFocused() {
		t.Fatal("'/' while an action prompt is active must not focus search")
	}
}

func TestActionKeysDispatchExpectedIntents(t *testing.T) {
	tests := []struct {
		name        string
		key         tea.KeyPressMsg
		kind        actions.Kind
		beforeEnter []tea.KeyPressMsg
		wantPrompt  actions.Prompt
	}{
		{name: "comment", key: tea.KeyPressMsg{Code: 'c', Text: "c"}, kind: actions.KindComment},
		{
			name:        "merge",
			key:         tea.KeyPressMsg{Code: 'm', Text: "m"},
			kind:        actions.KindMerge,
			beforeEnter: []tea.KeyPressMsg{{Code: 'j', Text: "j"}},
			wantPrompt: actions.Prompt{
				Mode:  actions.PromptPicker,
				Value: "squash",
				Label: "Squash",
			},
		},
		{name: "close", key: tea.KeyPressMsg{Code: 'x', Text: "x"}, kind: actions.KindClose},
		{name: "reopen", key: tea.KeyPressMsg{Code: 'X', Text: "X"}, kind: actions.KindReopen},
		{name: "review", key: tea.KeyPressMsg{Code: 'v', Text: "v"}, kind: actions.KindReview},
		{name: "external diff", key: tea.KeyPressMsg{Code: 'd', Text: "d"}, kind: actions.KindExternalDiff},
		{name: "external diff ctrl-t alias", key: tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}, kind: actions.KindExternalDiff},
		{name: "checkout", key: tea.KeyPressMsg{Code: 'C', Text: "C"}, kind: actions.KindCheckout},
		{name: "checkout space alias", key: tea.KeyPressMsg{Code: ' ', Text: " "}, kind: actions.KindCheckout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []actions.Intent
			m := New(&config.Config{}, nil)
			m.actionDispatcher = func(intent actions.Intent) tea.Cmd {
				got = append(got, intent)
				return nil
			}
			m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
			m = update(t, m, fetchedMsg([]data.PullRequest{{
				Number: 42, Title: "Action row", RepoNameWithOwner: "gbarany/tea-dash",
				Author: "me", State: "open", HTMLURL: "https://example.test/gbarany/tea-dash/pulls/42",
			}}))

			m = update(t, m, tt.key)
			if !m.actionPrompt.Active() {
				t.Fatalf("%s key should open an action prompt", tt.name)
			}
			if tt.kind == actions.KindComment {
				m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
				m = update(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
			}
			for _, key := range tt.beforeEnter {
				m = update(t, m, key)
			}
			m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

			if len(got) != 1 {
				t.Fatalf("dispatcher calls = %d, want 1", len(got))
			}
			wantTarget := actions.Target{
				SectionID:   0,
				SectionType: pullsection.SectionType,
				RowKind:     actions.RowKindPullRequest,
				Repo:        "gbarany/tea-dash",
				Number:      42,
				Title:       "Action row",
				URL:         "https://example.test/gbarany/tea-dash/pulls/42",
			}
			if got[0].Kind != tt.kind || got[0].Target != wantTarget {
				t.Fatalf("intent = %+v, want kind %q target %+v", got[0], tt.kind, wantTarget)
			}
			if tt.kind == actions.KindComment && got[0].Prompt.Value != "ok" {
				t.Fatalf("comment prompt value = %q, want ok", got[0].Prompt.Value)
			}
			if tt.wantPrompt != (actions.Prompt{}) && got[0].Prompt != tt.wantPrompt {
				t.Fatalf("prompt = %+v, want %+v", got[0].Prompt, tt.wantPrompt)
			}
		})
	}
}

func TestHelpKeyTogglesFullHelp(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	if strings.Contains(m.View().Content, "R refresh all") {
		t.Fatal("full help should be hidden before '?' is pressed")
	}
	m = update(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	view := m.View().Content
	for _, want := range []string{"R refresh all", "y copy number", "Y copy URL", "C/space checkout"} {
		if !strings.Contains(view, want) {
			t.Fatalf("full help missing %q:\n%s", want, view)
		}
	}
	m = update(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if strings.Contains(m.View().Content, "R refresh all") {
		t.Fatal("second '?' should hide full help")
	}
}

func TestNotificationsHelpShowsUnreadShortcut(t *testing.T) {
	m := New(&config.Config{Defaults: config.Defaults{View: "notifications"}}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	if view := m.View().Content; !strings.Contains(view, "u unread") {
		t.Fatalf("compact notifications help missing unread shortcut:\n%s", view)
	}
	m = update(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if view := m.View().Content; !strings.Contains(view, "u mark unread") {
		t.Fatalf("full notifications help missing mark-unread shortcut:\n%s", view)
	}
}

func TestRefreshAllFetchesEveryCurrentSection(t *testing.T) {
	cfg := &config.Config{
		PRSections: []config.SectionConfig{
			{Title: "Mine", Filter: config.PrIssueFilter{CreatedBy: "@me"}},
			{Title: "Review", Filter: config.PrIssueFilter{ReviewRequested: "@me"}},
		},
	}
	m := New(cfg, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: pullsection.SectionType, TaskId: "p0",
		Msg: pullsection.SectionPullRequestsFetchedMsg{
			Rows: []data.PullRequest{{
				Number: 1, Title: "First row", RepoNameWithOwner: "gbarany/tea-dash",
				Author: "me", State: "open",
			}},
			TotalCount: 1, TaskId: "p0",
		},
	})
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 1, SectionType: pullsection.SectionType, TaskId: "p1",
		Msg: pullsection.SectionPullRequestsFetchedMsg{
			Rows: []data.PullRequest{{
				Number: 2, Title: "Second row", RepoNameWithOwner: "gbarany/tea-dash",
				Author: "me", State: "open",
			}},
			TotalCount: 1, TaskId: "p1",
		},
	})

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("'R' should batch refresh commands for all current sections")
	}
	if len(m.tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want both sections to register refresh tasks", len(m.tasks))
	}
}

func TestCopyHotkeysUseSelectedRow(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want string
	}{
		{name: "copy number", key: tea.KeyPressMsg{Code: 'y', Text: "y"}, want: "42"},
		{name: "copy url", key: tea.KeyPressMsg{Code: 'Y', Text: "Y"}, want: "https://example.test/gbarany/tea-dash/pulls/42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var copied string
			m := New(&config.Config{}, nil)
			m.copyToClipboard = func(s string) error {
				copied = s
				return nil
			}
			m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
			m = update(t, m, fetchedMsg([]data.PullRequest{{
				Number: 42, Title: "Copy row", RepoNameWithOwner: "gbarany/tea-dash",
				Author: "me", State: "open", HTMLURL: "https://example.test/gbarany/tea-dash/pulls/42",
			}}))

			next, cmd := m.Update(tt.key)
			m = next.(Model)
			if cmd == nil {
				t.Fatalf("%s should return a clipboard command", tt.name)
			}
			m = update(t, m, cmd())
			if copied != tt.want {
				t.Fatalf("copied = %q, want %q", copied, tt.want)
			}
			if !strings.Contains(m.View().Content, "Copied") {
				t.Fatalf("copy result should be visible in the status area:\n%s", m.View().Content)
			}
		})
	}
}

func TestGHDashFirstLastHotkeysMoveSelection(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{
		{Number: 1, Title: "First row", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
		{Number: 2, Title: "Second row", RepoNameWithOwner: "gbarany/tea-dash", Author: "me", State: "open"},
	}))

	m = update(t, m, tea.KeyPressMsg{Code: 'G', Text: "G"})
	if got := m.getCurrRowData().GetNumber(); got != 2 {
		t.Fatalf("'G' selected #%d, want #2", got)
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if got := m.getCurrRowData().GetNumber(); got != 1 {
		t.Fatalf("'g' selected #%d, want #1", got)
	}
}

func TestInvalidActionOnIssueShowsNotice(t *testing.T) {
	var dispatched bool
	m := New(&config.Config{Defaults: config.Defaults{View: "issues"}}, nil)
	m.actionDispatcher = func(actions.Intent) tea.Cmd {
		dispatched = true
		return nil
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedIssuesMsg([]data.Issue{{
		Number: 7, Title: "Issue row", RepoNameWithOwner: "gbarany/tea-dash",
		Author: "me", State: "open",
	}}))

	m = update(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if dispatched {
		t.Fatal("merge on an issue must not dispatch")
	}
	if m.actionPrompt.Active() {
		t.Fatal("merge on an issue must not open a prompt")
	}
	if !strings.Contains(m.notice, "pull requests") {
		t.Fatalf("invalid action notice = %q, want pull requests message", m.notice)
	}
	if view := m.View().Content; !strings.Contains(view, "pull requests") {
		t.Fatalf("invalid action notice should render in the view:\n%s", view)
	}
}

func TestNilActionDispatcherShowsNoticeOnSubmit(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 42, Title: "Action row", RepoNameWithOwner: "gbarany/tea-dash",
		Author: "me", State: "open",
	}}))

	m = update(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !m.actionPrompt.Active() {
		t.Fatal("'m' should open an action prompt")
	}
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.actionPrompt.Active() {
		t.Fatal("submitted prompt should close")
	}
	if !strings.Contains(m.notice, "Action not wired yet") {
		t.Fatalf("nil dispatcher notice = %q, want action-not-wired message", m.notice)
	}
	if view := m.View().Content; !strings.Contains(view, "Action not wired yet") {
		t.Fatalf("nil dispatcher notice should render in the view:\n%s", view)
	}
}

func TestSuccessfulActionRefreshesRowsAndClearsPreviewCache(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 42, Title: "Action row", RepoNameWithOwner: "gbarany/tea-dash",
		Author: "me", State: "open",
	}}))
	key := m.selKey()
	m = update(t, m, enrichedMsg{
		key:  key,
		pull: &data.PullDetail{Body: "staledetailtoken", BaseRef: "main", HeadRef: "feature"},
	})
	if _, ok := m.pullDetails[key]; !ok {
		t.Fatalf("test setup: expected cached pull detail for %q", key)
	}

	next, cmd := m.Update(actions.ResultMsg{
		Intent: actions.Intent{Kind: actions.KindClose, Target: actions.Target{
			SectionID: 0, SectionType: pullsection.SectionType, RowKind: actions.RowKindPullRequest,
			Repo: "gbarany/tea-dash", Number: 42,
		}},
		Status:  actions.ResultSucceeded,
		Message: "Closed gbarany/tea-dash#42.",
	})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("successful action should refresh the affected section")
	}
	if _, ok := m.pullDetails[key]; ok {
		t.Fatalf("successful action should clear cached pull detail for %q", key)
	}
	view := m.View().Content
	if strings.Contains(view, "staledetailtoken") || !strings.Contains(view, "Loading") {
		t.Fatalf("successful action should replace stale preview with loading state:\n%s", view)
	}
}

// isQuitCmd reports whether running cmd yields a tea.QuitMsg.
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func runEnrichedCommand(t *testing.T, cmd tea.Cmd) enrichedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected an enrichment command, got nil")
	}
	msg := cmd()
	if enriched, ok := msg.(enrichedMsg); ok {
		return enriched
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("enrichment command returned %T, want enrichedMsg or BatchMsg", msg)
	}
	for _, nested := range batch {
		if nested == nil {
			continue
		}
		if enriched, ok := nested().(enrichedMsg); ok {
			return enriched
		}
	}
	t.Fatal("enrichment batch did not contain an enrichedMsg")
	return enrichedMsg{}
}

func actionDetailServer(t *testing.T, register func(*http.ServeMux)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	})
	register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

var errBoom = errBoomType("boom")

type errBoomType string

func (e errBoomType) Error() string { return string(e) }

func newNotificationActionClient(
	t *testing.T,
	threadHandler http.HandlerFunc,
	notificationsHandler http.HandlerFunc,
) *gitea.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	})
	if threadHandler != nil {
		mux.HandleFunc("/api/v1/notifications/threads/12", threadHandler)
	}
	if notificationsHandler != nil {
		mux.HandleFunc("/api/v1/notifications", notificationsHandler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := gitea.NewClient(stdctx.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}
