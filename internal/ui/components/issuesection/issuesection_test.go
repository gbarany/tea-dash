package issuesection

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
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func newModel(t *testing.T) *Model {
	t.Helper()
	ctx := &context.ProgramContext{Styles: context.DefaultStyles(), MainContentWidth: 100, MainContentHeight: 20}
	m := NewModel(0, ctx, config.SectionConfig{Title: "Issues"})
	return m
}

// newSearchableModel builds a model whose ctx has a StartTask so enter's
// refetch does not panic on a nil StartTask closure.
func newSearchableModel(t *testing.T) *Model {
	t.Helper()
	ctx := &context.ProgramContext{
		Styles: context.DefaultStyles(), MainContentWidth: 100, MainContentHeight: 20,
		StartTask: func(context.Task) tea.Cmd { return nil },
	}
	return NewModel(0, ctx, config.SectionConfig{Title: "Issues"})
}

func TestSearchEnterAppliesKeywordAndRefetches(t *testing.T) {
	m := newSearchableModel(t)
	m.SetIsSearching(true)
	for _, r := range "bug" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)
	if m.Config.Filter.Q != "bug" {
		t.Fatalf("enter should apply the keyword: Config.Filter.Q = %q, want %q", m.Config.Filter.Q, "bug")
	}
	if m.IsSearchFocused() {
		t.Fatal("enter should unfocus the search bar")
	}
	if cmd == nil {
		t.Fatal("enter should return a non-nil refetch command")
	}
}

func TestSearchEscRevertsAndUnfocuses(t *testing.T) {
	m := newModel(t)
	m.Config.Filter.Q = "applied"
	m.SetIsSearching(true)
	for _, r := range "typed" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(*Model)
	if m.IsSearchFocused() {
		t.Fatal("esc should unfocus the search bar")
	}
	if m.Config.Filter.Q != "applied" {
		t.Fatalf("esc should not change the applied keyword: Q = %q, want %q", m.Config.Filter.Q, "applied")
	}
	if got := m.SearchBar.Value(); got != "applied" {
		t.Fatalf("esc should revert the bar to the applied keyword: Value() = %q, want %q", got, "applied")
	}
	if cmd != nil {
		t.Fatalf("esc should return a nil command, got %v", cmd)
	}
}

func TestImplementsSection(t *testing.T) {
	var _ section.Section = (*Model)(nil)
}

func TestFetchedMsgBuildsRows(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("t1")
	next, _ := m.Update(SectionIssuesFetchedMsg{
		Rows: []data.Issue{{
			Number: 77, Title: "Bug X", RepoNameWithOwner: "acme/widgets",
			Author: "octo", State: "open", UpdatedAt: time.Now().Add(-2 * time.Hour),
		}},
		TotalCount: 1, TaskId: "t1",
	})
	m = next.(*Model)

	if m.GetTotalCount() != 1 || m.NumRows() != 1 {
		t.Fatalf("counts: total=%d rows=%d", m.GetTotalCount(), m.NumRows())
	}
	if m.GetCurrRow() == nil || m.GetCurrRow().GetNumber() != 77 {
		t.Fatalf("GetCurrRow = %+v", m.GetCurrRow())
	}
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	for _, want := range []string{"#77", "Bug X", "acme/widgets", "@octo", "open"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("row %q missing %q", joined, want)
		}
	}
}

func TestStaleFetchIgnored(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("t2") // an in-flight fetch t2 is expected
	next, _ := m.Update(SectionIssuesFetchedMsg{
		Rows: []data.Issue{{Number: 1}}, TotalCount: 1, TaskId: "t1", // stale
	})
	m = next.(*Model)
	if m.NumRows() != 0 {
		t.Fatalf("stale fetch was applied: rows=%d", m.NumRows())
	}
}

func TestGetCurrRowNilBeforeFetch(t *testing.T) {
	m := newModel(t)
	if m.GetCurrRow() != nil {
		t.Fatalf("GetCurrRow() = %+v, want nil before any fetch", m.GetCurrRow())
	}
}

func TestFetchRowsUsesConfiguredReposFanoutWhenSectionRepoBlank(t *testing.T) {
	var paths []string
	srv := issueFanoutServer(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("X-Total-Count", "1")
		switch r.URL.Path {
		case "/api/v1/repos/acme/widgets/issues":
			fmt.Fprint(w, `[{"number":1,"title":"Widgets issue","state":"open","updated_at":"2026-06-02T00:00:00Z","user":{"login":"alice"}}]`)
		case "/api/v1/repos/acme/api/issues":
			fmt.Fprint(w, `[{"number":2,"title":"API issue","state":"open","updated_at":"2026-06-03T00:00:00Z","user":{"login":"bob"}}]`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	client, err := gitea.NewClient(stdctx.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := &context.ProgramContext{
		Styles: context.DefaultStyles(), MainContentWidth: 100, MainContentHeight: 20,
		Config: &config.Config{
			Repos:    []string{"acme/widgets", "acme/api"},
			Defaults: config.Defaults{IssuesLimit: 10},
		},
		Client:    client,
		StartTask: func(context.Task) tea.Cmd { return nil },
	}
	m := NewModel(0, ctx, config.SectionConfig{Title: "All Issues"})

	msg := runIssueFetchCommand(t, m.FetchRows())
	payload := msg.Msg.(SectionIssuesFetchedMsg)
	if payload.Err != nil {
		t.Fatalf("FetchRows payload error: %v", payload.Err)
	}
	if payload.TotalCount != 2 || len(payload.Rows) != 2 {
		t.Fatalf("payload total=%d rows=%+v, want two fanned-out issues", payload.TotalCount, payload.Rows)
	}
	if payload.Rows[0].Title != "API issue" || payload.Rows[1].Title != "Widgets issue" {
		t.Fatalf("rows = %+v, want globally sorted by updated time", payload.Rows)
	}
	for _, want := range []string{"/api/v1/repos/acme/widgets/issues", "/api/v1/repos/acme/api/issues"} {
		if !containsFetchPathWith(paths, want, "type=issues") {
			t.Fatalf("paths = %v, missing %q with type=issues", paths, want)
		}
	}
}

func issueFanoutServer(t *testing.T, repoIssues http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	})
	mux.HandleFunc("/api/v1/repos/acme/widgets/issues", repoIssues)
	mux.HandleFunc("/api/v1/repos/acme/api/issues", repoIssues)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func runIssueFetchCommand(t *testing.T, cmd tea.Cmd) context.TaskFinishedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("FetchRows returned nil command")
	}
	msg := cmd()
	if finished, ok := msg.(context.TaskFinishedMsg); ok {
		return finished
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("FetchRows command returned %T, want TaskFinishedMsg or BatchMsg", msg)
	}
	for _, nested := range batch {
		if nested == nil {
			continue
		}
		got := nested()
		if finished, ok := got.(context.TaskFinishedMsg); ok {
			return finished
		}
	}
	t.Fatal("FetchRows batch did not contain a TaskFinishedMsg")
	return context.TaskFinishedMsg{}
}

func containsFetchPathWith(paths []string, wantPath, wantQuery string) bool {
	for _, got := range paths {
		if strings.Contains(got, wantPath) && strings.Contains(got, wantQuery) {
			return true
		}
	}
	return false
}
