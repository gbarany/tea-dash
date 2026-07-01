package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/auth"
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

func TestSearchMyPulls(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	})
	mux.HandleFunc("/api/v1/repos/issues/search", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		// The page is capped, but the server reports the real total via header.
		w.Header().Set("X-Total-Count", "5")
		fmt.Fprint(w, searchJSON)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	prs, total, err := c.SearchMyPulls(context.Background(), "open")
	if err != nil {
		t.Fatalf("SearchMyPulls: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(prs))
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5 (from X-Total-Count)", total)
	}
	pr := prs[0]
	if pr.Number != 7 || pr.Title != "Fix thing" ||
		pr.RepoNameWithOwner != "acme/widgets" || pr.Author != "me" || pr.State != "open" {
		t.Fatalf("mapped PR = %+v", pr)
	}
	if len(pr.Labels) != 1 || pr.Labels[0].Name != "bug" {
		t.Fatalf("labels = %+v", pr.Labels)
	}

	// The me-scope MUST be the boolean `created=true` on the search endpoint,
	// and MUST NOT be the per-repo `created_by` param (which search ignores).
	// This is the C1 regression guard.
	for _, want := range []string{"type=pulls", "created=true", "state=open"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
	if strings.Contains(gotQuery, "created_by") {
		t.Fatalf("query %q must not use created_by on the search endpoint", gotQuery)
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

func TestSearchMyPullsNon2xx(t *testing.T) {
	srv := searchServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"unauthorized"}`)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	prs, total, err := c.SearchMyPulls(context.Background(), "open")
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

func TestSearchMyPullsMapsMergedAndDraft(t *testing.T) {
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

	prs, _, err := c.SearchMyPulls(context.Background(), "open")
	if err != nil {
		t.Fatalf("SearchMyPulls: %v", err)
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
