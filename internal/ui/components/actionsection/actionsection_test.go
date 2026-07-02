package actionsection

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
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

func newModel(t *testing.T) *Model {
	t.Helper()
	ctx := &appctx.ProgramContext{Styles: appctx.DefaultStyles(), MainContentWidth: 100, MainContentHeight: 20}
	return NewModel(0, ctx, config.SectionConfig{Title: "CI", Repo: "acme/widgets"})
}

func newFetchModel(t *testing.T, client *gitea.Client, cfg config.SectionConfig) *Model {
	t.Helper()
	ctx := &appctx.ProgramContext{
		Styles: appctx.DefaultStyles(), MainContentWidth: 100, MainContentHeight: 20,
		Config: &config.Config{Defaults: config.Defaults{ActionsLimit: 25}},
		Client: client,
		StartTask: func(appctx.Task) tea.Cmd {
			return nil
		},
	}
	return NewModel(0, ctx, cfg)
}

func TestImplementsSection(t *testing.T) {
	var _ section.Section = (*Model)(nil)
}

func TestFetchedMsgBuildsRows(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("a1")
	next, _ := m.Update(SectionActionsFetchedMsg{
		Rows: []data.ActionRun{{
			ID: 101, RunNumber: 12, DisplayTitle: "Fix checkout flakes",
			RepoNameWithOwner: "acme/widgets", Actor: "octo", Event: "push",
			Status: "completed", Conclusion: "success", UpdatedAt: time.Now().Add(-time.Hour),
		}},
		TotalCount: 1, TaskId: "a1",
	})
	m = next.(*Model)

	if m.GetTotalCount() != 1 || m.NumRows() != 1 {
		t.Fatalf("counts: total=%d rows=%d", m.GetTotalCount(), m.NumRows())
	}
	if m.GetCurrRow() == nil || m.GetCurrRow().GetNumber() != 12 {
		t.Fatalf("GetCurrRow = %+v", m.GetCurrRow())
	}
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	for _, want := range []string{"#12", "Fix checkout flakes", "acme/widgets", "@octo push", "completed/success"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("row %q missing %q", joined, want)
		}
	}
}

func TestFetchRowsNoRepoReturnsEmptyResult(t *testing.T) {
	m := newFetchModel(t, nil, config.SectionConfig{Title: "Actions"})

	msg := runFetchCommand(t, m.FetchRows())
	payload := msg.Msg.(SectionActionsFetchedMsg)
	if payload.Err != nil {
		t.Fatalf("blank repo should not error: %v", payload.Err)
	}
	if len(payload.Rows) != 0 || payload.TotalCount != 0 {
		t.Fatalf("blank repo payload = rows %d total %d, want empty", len(payload.Rows), payload.TotalCount)
	}
}

func TestFetchRowsNilClientReturnsErrorInsteadOfPanicking(t *testing.T) {
	m := newFetchModel(t, nil, config.SectionConfig{Title: "CI", Repo: "acme/widgets"})

	msg := runFetchCommand(t, m.FetchRows())
	payload := msg.Msg.(SectionActionsFetchedMsg)
	if payload.Err == nil || !strings.Contains(payload.Err.Error(), "Gitea client") {
		t.Fatalf("nil client payload error = %v, want Gitea client error", payload.Err)
	}
}

func TestFetchRowsUsesRepoFiltersAndLimit(t *testing.T) {
	var gotQuery string
	srv := actionServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs", func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			fmt.Fprint(w, `{
				"total_count": 1,
				"workflow_runs": [{
					"id": 101,
					"run_number": 12,
					"display_title": "Fix checkout flakes",
					"event": "push",
					"status": "completed",
					"conclusion": "success",
					"head_branch": "main",
					"head_sha": "abc123",
					"actor": {"login": "octo"},
					"updated_at": "2026-07-02T08:05:00Z"
				}]
			}`)
		})
	})
	client, err := gitea.NewClient(stdctx.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	m := newFetchModel(t, client, config.SectionConfig{
		Title: "CI",
		Repo:  "acme/widgets",
		Limit: 10,
		Filter: config.PrIssueFilter{
			Status:  "completed",
			Branch:  "main",
			Event:   "push",
			Actor:   "octo",
			HeadSHA: "abc123",
		},
	})

	msg := runFetchCommand(t, m.FetchRows())
	payload := msg.Msg.(SectionActionsFetchedMsg)
	if payload.Err != nil {
		t.Fatalf("FetchRows payload error: %v", payload.Err)
	}
	if payload.TotalCount != 1 || len(payload.Rows) != 1 || payload.Rows[0].RepoNameWithOwner != "acme/widgets" {
		t.Fatalf("payload = total %d rows %+v", payload.TotalCount, payload.Rows)
	}
	for _, want := range []string{
		"status=completed",
		"branch=main",
		"event=push",
		"actor=octo",
		"head_sha=abc123",
		"limit=10",
	} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestFetchRowsDefaultsLimitTo50(t *testing.T) {
	var gotQuery string
	srv := actionServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs", func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			fmt.Fprint(w, `[]`)
		})
	})
	client, err := gitea.NewClient(stdctx.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := &appctx.ProgramContext{
		Styles: appctx.DefaultStyles(), MainContentWidth: 100, MainContentHeight: 20,
		Config: &config.Config{},
		Client: client,
		StartTask: func(appctx.Task) tea.Cmd {
			return nil
		},
	}
	m := NewModel(0, ctx, config.SectionConfig{Title: "CI", Repo: "acme/widgets"})

	msg := runFetchCommand(t, m.FetchRows())
	payload := msg.Msg.(SectionActionsFetchedMsg)
	if payload.Err != nil {
		t.Fatalf("FetchRows payload error: %v", payload.Err)
	}
	if !strings.Contains(gotQuery, "limit=50") {
		t.Fatalf("query %q missing default limit=50", gotQuery)
	}
}

func runFetchCommand(t *testing.T, cmd tea.Cmd) appctx.TaskFinishedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("FetchRows returned nil command")
	}
	msg := cmd()
	if finished, ok := msg.(appctx.TaskFinishedMsg); ok {
		return finished
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("FetchRows command returned %T, want TaskFinishedMsg or BatchMsg", msg)
	}
	for _, nested := range batch {
		got := nested()
		if finished, ok := got.(appctx.TaskFinishedMsg); ok {
			return finished
		}
	}
	t.Fatal("FetchRows batch did not contain a TaskFinishedMsg")
	return appctx.TaskFinishedMsg{}
}

func actionServer(t *testing.T, register func(*http.ServeMux)) *httptest.Server {
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
