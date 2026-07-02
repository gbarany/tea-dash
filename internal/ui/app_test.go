package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	localgit "github.com/gbarany/tea-dash/internal/git"
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

func TestSwitchViewCyclesToBranches(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"}) // issues
	m = update(t, m, tea.KeyPressMsg{Code: 's', Text: "s"}) // notifications
	next, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
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

// isQuitCmd reports whether running cmd yields a tea.QuitMsg.
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

var errBoom = errBoomType("boom")

type errBoomType string

func (e errBoomType) Error() string { return string(e) }
