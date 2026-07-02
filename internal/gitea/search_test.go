package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/auth"
	"github.com/gbarany/tea-dash/internal/config"
)

const searchJSON = `[
  {"number":7,"title":"Fix thing","state":"open",
   "html_url":"https://x/acme/widgets/pulls/7",
   "user":{"login":"me","full_name":"Me"},
   "labels":[{"name":"bug","color":"ff0000"}],
   "created_at":"2026-06-01T00:00:00Z","updated_at":"2026-06-02T00:00:00Z",
   "repository":{"full_name":"acme/widgets"},
   "pull_request":{"merged":false}}
]`

const issueJSON = `[
  {"number":42,"title":"Broken login","state":"open",
   "html_url":"https://x/acme/widgets/issues/42",
   "user":{"login":"me","full_name":"Me"},
   "labels":[{"name":"bug","color":"ff0000"}],
   "created_at":"2026-06-01T00:00:00Z","updated_at":"2026-06-02T00:00:00Z",
   "repository":{"full_name":"acme/widgets"}}
]`

func TestBuildSearchParams(t *testing.T) {
	tests := []struct {
		name        string
		filter      config.PrIssueFilter
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:   "me-scoped pulls",
			filter: config.PrIssueFilter{State: "open", Type: "pulls", CreatedBy: "@me"},
			// C1 guard: the me-scope MUST be created=true, never created_by.
			wantContain: []string{"type=pulls", "created=true", "state=open", "limit=50"},
			wantAbsent:  []string{"created_by"},
		},
		{
			name:        "labels are comma-joined",
			filter:      config.PrIssueFilter{Labels: []string{"a", "b"}},
			wantContain: []string{"labels=a%2Cb"},
		},
		{
			name:        "keyword query",
			filter:      config.PrIssueFilter{Q: "foo"},
			wantContain: []string{"q=foo"},
		},
		{
			name: "all me-scoped flags and scalar fields render",
			filter: config.PrIssueFilter{
				Milestone:       "v2",
				AssignedBy:      "@me",
				Mentioned:       "@me",
				ReviewRequested: "@me",
				Since:           "2026-01-01T00:00:00Z",
				Sort:            "recentupdate",
			},
			wantContain: []string{
				"milestones=v2",
				"assigned=true",
				"mentioned=true",
				"review_requested=true",
				"since=2026-01-01",
				"sort=recentupdate",
			},
		},
		{
			// Endpoint contract: a non-@me author emits NEITHER the me-scope
			// boolean NOR a per-repo created_by param (the search endpoint ignores
			// it). Config.Validate rejects this upstream; buildSearchParams just
			// must not smuggle it through.
			name:        "non-@me createdBy emits no author filter",
			filter:      config.PrIssueFilter{CreatedBy: "lunny"},
			wantAbsent:  []string{"created=true", "created_by"},
			wantContain: []string{"type="},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSearchParams(tt.filter, 0).Encode()
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("query %q missing %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("query %q must not contain %q", got, absent)
				}
			}
		})
	}
}

func TestSearchPulls(t *testing.T) {
	var gotQuery string
	srv := searchServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		// The page is capped, but the server reports the real total via header.
		w.Header().Set("X-Total-Count", "5")
		fmt.Fprint(w, searchJSON)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	prs, total, err := c.SearchPulls(context.Background(), config.PrIssueFilter{State: "open", CreatedBy: "@me"}, 0)
	if err != nil {
		t.Fatalf("SearchPulls: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(prs))
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5 (from X-Total-Count)", total)
	}
	pr := prs[0]
	if pr.Number != 7 || pr.RepoNameWithOwner != "acme/widgets" || pr.Author != "me" {
		t.Fatalf("mapped PR = %+v", pr)
	}

	// The me-scope MUST be the boolean `created=true` on the search endpoint,
	// and MUST NOT be the per-repo `created_by` param (which search ignores).
	// This is the C1 regression guard.
	for _, want := range []string{"type=pulls", "created=true"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
	if strings.Contains(gotQuery, "created_by") {
		t.Fatalf("query %q must not use created_by on the search endpoint", gotQuery)
	}
}

func TestSearchPullsLimitReachesQuery(t *testing.T) {
	var gotQuery string
	srv := searchServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, searchJSON)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// An explicit limit (as a section's Limit or per-view default supplies) must
	// reach the search endpoint, not the hardcoded default.
	if _, _, err := c.SearchPulls(context.Background(), config.PrIssueFilter{State: "open", CreatedBy: "@me"}, 100); err != nil {
		t.Fatalf("SearchPulls: %v", err)
	}
	if !strings.Contains(gotQuery, "limit=100") {
		t.Fatalf("query %q missing limit=100", gotQuery)
	}
}

func TestSearchPullsPageReachesQuery(t *testing.T) {
	var gotQuery string
	srv := searchServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("X-Total-Count", "75")
		fmt.Fprint(w, searchJSON)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if _, total, err := c.SearchPullsPage(context.Background(), config.PrIssueFilter{State: "open", CreatedBy: "@me"}, 25, 3); err != nil {
		t.Fatalf("SearchPullsPage: %v", err)
	} else if total != 75 {
		t.Fatalf("total = %d, want 75", total)
	}
	for _, want := range []string{"limit=25", "page=3"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestSearchIssues(t *testing.T) {
	var gotQuery string
	srv := searchServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, issueJSON)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	issues, _, err := c.SearchIssues(context.Background(), config.PrIssueFilter{State: "open", CreatedBy: "@me"}, 0)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(issues))
	}
	is := issues[0]
	if is.Number != 42 || is.Title != "Broken login" ||
		is.RepoNameWithOwner != "acme/widgets" || is.Author != "me" || is.State != "open" {
		t.Fatalf("mapped issue = %+v", is)
	}
	if len(is.Labels) != 1 || is.Labels[0].Name != "bug" {
		t.Fatalf("labels = %+v", is.Labels)
	}
	if !strings.Contains(gotQuery, "type=issues") {
		t.Fatalf("query %q missing %q", gotQuery, "type=issues")
	}
}

func TestListRepoIssuesUsesRepoEndpointAndPlainLoginFilters(t *testing.T) {
	var gotPath, gotQuery string
	srv := repoSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("X-Total-Count", "9")
		fmt.Fprint(w, issueJSON)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	issues, total, err := c.ListRepoIssuesPage(context.Background(), "acme/widgets", config.PrIssueFilter{
		State:      "closed",
		Labels:     []string{"bug", "urgent"},
		Milestone:  "v1",
		CreatedBy:  "alice",
		AssignedBy: "@me",
		Mentioned:  "bob",
		Q:          "login",
		Sort:       "recentupdate",
	}, 25, 3)
	if err != nil {
		t.Fatalf("ListRepoIssuesPage: %v", err)
	}
	if total != 9 || len(issues) != 1 {
		t.Fatalf("got total=%d len=%d, want total=9 len=1", total, len(issues))
	}
	if gotPath != "/api/v1/repos/acme/widgets/issues" {
		t.Fatalf("path = %q, want repo issues endpoint", gotPath)
	}
	for _, want := range []string{
		"type=issues",
		"state=closed",
		"labels=bug%2Curgent",
		"milestones=v1",
		"created_by=alice",
		"assigned_by=me",
		"mentioned_by=bob",
		"q=login",
		"page=3",
		"limit=25",
	} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
	if issues[0].RepoNameWithOwner != "acme/widgets" {
		t.Fatalf("repo = %q, want acme/widgets", issues[0].RepoNameWithOwner)
	}
}

func TestListRepoPullsUsesRepoIssueEndpointForFilterablePRRows(t *testing.T) {
	var gotPath, gotQuery string
	srv := repoSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("X-Total-Count", "3")
		fmt.Fprint(w, `[
		  {"number":8,"title":"Repo PR","state":"closed",
		   "html_url":"https://git.example/acme/widgets/pulls/8",
		   "user":{"login":"alice"},
		   "pull_request":{"merged":true},
		   "updated_at":"2026-06-02T00:00:00Z"}
		]`)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	prs, total, err := c.ListRepoPullsPage(context.Background(), "acme/widgets", config.PrIssueFilter{
		State:     "closed",
		CreatedBy: "alice",
	}, 10, 2)
	if err != nil {
		t.Fatalf("ListRepoPullsPage: %v", err)
	}
	if total != 3 || len(prs) != 1 {
		t.Fatalf("got total=%d len=%d, want total=3 len=1", total, len(prs))
	}
	if gotPath != "/api/v1/repos/acme/widgets/issues" {
		t.Fatalf("path = %q, want repo issues endpoint", gotPath)
	}
	for _, want := range []string{"type=pulls", "state=closed", "created_by=alice", "page=2", "limit=10"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
	if prs[0].Number != 8 || prs[0].RepoNameWithOwner != "acme/widgets" ||
		prs[0].Author != "alice" || prs[0].State != "merged" {
		t.Fatalf("mapped PR = %+v", prs[0])
	}
}

// searchServer builds a fake Gitea that serves the version/user probes plus a
// /repos/issues/search handler supplied by the caller.
func searchServer(t *testing.T, search http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	})
	mux.HandleFunc("/api/v1/repos/issues/search", search)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func repoSearchServer(t *testing.T, repoIssues http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	})
	mux.HandleFunc("/api/v1/repos/acme/widgets/issues", repoIssues)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSearchPullsNon2xx(t *testing.T) {
	srv := searchServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"unauthorized"}`)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	prs, total, err := c.SearchPulls(context.Background(), config.PrIssueFilter{State: "open", CreatedBy: "@me"}, 0)
	if err == nil {
		t.Fatal("expected an error on HTTP 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error %q should contain the status 401", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0 on error", total)
	}
	if prs != nil {
		t.Fatalf("prs = %+v, want nil on error", prs)
	}
}

func TestSearchPullsMapsMergedAndDraft(t *testing.T) {
	const body = `[
	  {"number":1,"title":"Merged one","state":"closed",
	   "pull_request":{"merged":true}},
	  {"number":2,"title":"Draft one","state":"open",
	   "pull_request":{"draft":true}}
	]`
	srv := searchServer(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, body)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	prs, _, err := c.SearchPulls(context.Background(), config.PrIssueFilter{State: "open", CreatedBy: "@me"}, 0)
	if err != nil {
		t.Fatalf("SearchPulls: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d PRs, want 2", len(prs))
	}
	if prs[0].State != "merged" {
		t.Fatalf("merged row State = %q, want %q", prs[0].State, "merged")
	}
	if !prs[1].Draft {
		t.Fatalf("draft row Draft = %v, want true", prs[1].Draft)
	}
}
