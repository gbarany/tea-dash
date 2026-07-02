package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/auth"
)

const actionRunsJSON = `{
  "total_count": 12,
  "workflow_runs": [
    {
      "id": 101,
      "run_number": 12,
      "run_attempt": 2,
      "name": "CI",
      "display_title": "Fix checkout flakes",
      "event": "push",
      "status": "in_progress",
      "conclusion": "",
      "head_branch": "main",
      "head_sha": "abc123",
      "html_url": "https://git.example/acme/widgets/actions/runs/101",
      "actor": {"login": "octo"},
      "created_at": "2026-07-02T08:00:00Z",
      "updated_at": "2026-07-02T08:05:00Z",
      "run_started_at": "2026-07-02T08:01:00Z"
    }
  ]
}`

func TestListActionRunsMapsWrappedResponseAndFilters(t *testing.T) {
	var gotQuery string
	srv := actionServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs", func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			if r.Header.Get("Authorization") != "token t" {
				t.Fatalf("Authorization header = %q, want token auth", r.Header.Get("Authorization"))
			}
			fmt.Fprint(w, actionRunsJSON)
		})
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	runs, total, err := c.ListActionRuns(context.Background(), "acme", "widgets", ActionRunListOptions{
		Event:   "push",
		Branch:  "main",
		Status:  "in_progress",
		Actor:   "octo",
		HeadSHA: "abc123",
		Limit:   25,
	})
	if err != nil {
		t.Fatalf("ListActionRuns: %v", err)
	}
	if total != 12 || len(runs) != 1 {
		t.Fatalf("got total=%d len=%d, want total 12 and one run", total, len(runs))
	}
	run := runs[0]
	if run.ID != 101 || run.RunNumber != 12 || run.RunAttempt != 2 ||
		run.DisplayTitle != "Fix checkout flakes" || run.WorkflowName != "CI" ||
		run.Event != "push" || run.Status != "in_progress" || run.Conclusion != "" ||
		run.HeadBranch != "main" || run.HeadSHA != "abc123" || run.Actor != "octo" ||
		run.RepoNameWithOwner != "acme/widgets" || run.HTMLURL != "https://git.example/acme/widgets/actions/runs/101" {
		t.Fatalf("mapped run = %+v", run)
	}
	wantStarted := time.Date(2026, 7, 2, 8, 1, 0, 0, time.UTC)
	wantUpdated := time.Date(2026, 7, 2, 8, 5, 0, 0, time.UTC)
	if !run.StartedAt.Equal(wantStarted) || !run.UpdatedAt.Equal(wantUpdated) {
		t.Fatalf("run times = started %s updated %s", run.StartedAt, run.UpdatedAt)
	}
	for _, want := range []string{
		"event=push",
		"branch=main",
		"status=in_progress",
		"actor=octo",
		"head_sha=abc123",
		"limit=25",
	} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestListActionRunsMapsArrayResponse(t *testing.T) {
	srv := actionServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `[
			  {
			    "id": 102,
			    "name": "Nightly",
			    "status": "success",
			    "repository": {"full_name": "acme/widgets"},
			    "html_url": "https://git.example/acme/widgets/actions/runs/102"
			  }
			]`)
		})
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	runs, total, err := c.ListActionRuns(context.Background(), "acme", "widgets", ActionRunListOptions{})
	if err != nil {
		t.Fatalf("ListActionRuns: %v", err)
	}
	if total != 1 || len(runs) != 1 {
		t.Fatalf("got total=%d len=%d, want one array-decoded run", total, len(runs))
	}
	if runs[0].ID != 102 || runs[0].WorkflowName != "Nightly" || runs[0].Status != "success" {
		t.Fatalf("mapped run = %+v", runs[0])
	}
}

func TestGetActionRunMapsSingleRun(t *testing.T) {
	srv := actionServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs/101", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{
			  "id": 101,
			  "run_number": 12,
			  "name": "CI",
			  "status": "failure",
			  "conclusion": "failure",
			  "html_url": "https://git.example/acme/widgets/actions/runs/101"
			}`)
		})
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	run, err := c.GetActionRun(context.Background(), "acme", "widgets", 101)
	if err != nil {
		t.Fatalf("GetActionRun: %v", err)
	}
	if run.ID != 101 || run.RunNumber != 12 || run.WorkflowName != "CI" ||
		run.Status != "failure" || run.Conclusion != "failure" || run.RepoNameWithOwner != "acme/widgets" {
		t.Fatalf("mapped run = %+v", run)
	}
}

func TestListActionJobsMapsWrappedResponse(t *testing.T) {
	srv := actionServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs/101/jobs", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{
			  "total_count": 1,
			  "jobs": [
			    {
			      "id": 201,
			      "run_id": 101,
			      "name": "build",
			      "status": "success",
			      "conclusion": "success",
			      "runner_name": "ubuntu-latest",
			      "html_url": "https://git.example/acme/widgets/actions/runs/101/jobs/201",
			      "started_at": "2026-07-02T08:01:00Z",
			      "completed_at": "2026-07-02T08:04:00Z",
			      "steps": [
			        {
			          "number": 1,
			          "name": "checkout",
			          "status": "success",
			          "conclusion": "success",
			          "started_at": "2026-07-02T08:01:00Z",
			          "completed_at": "2026-07-02T08:02:00Z"
			        }
			      ]
			    }
			  ]
			}`)
		})
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	jobs, err := c.ListActionJobs(context.Background(), "acme", "widgets", 101)
	if err != nil {
		t.Fatalf("ListActionJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	job := jobs[0]
	if job.ID != 201 || job.RunID != 101 || job.Name != "build" ||
		job.Status != "success" || job.Conclusion != "success" ||
		job.RunnerName != "ubuntu-latest" || job.RepoNameWithOwner != "acme/widgets" ||
		job.HTMLURL != "https://git.example/acme/widgets/actions/runs/101/jobs/201" {
		t.Fatalf("mapped job = %+v", job)
	}
	if len(job.Steps) != 1 || job.Steps[0].Number != 1 || job.Steps[0].Name != "checkout" ||
		job.Steps[0].Status != "success" || job.Steps[0].Conclusion != "success" {
		t.Fatalf("mapped steps = %+v", job.Steps)
	}
}

func TestRerunActionRunPostsRunControlEndpoint(t *testing.T) {
	called := false
	srv := actionServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs/101/rerun", func(w http.ResponseWriter, r *http.Request) {
			called = true
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			if r.Header.Get("Authorization") != "token t" {
				t.Fatalf("Authorization header = %q, want token auth", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.RerunActionRun(context.Background(), "acme", "widgets", 101); err != nil {
		t.Fatalf("RerunActionRun: %v", err)
	}
	if !called {
		t.Fatal("rerun endpoint was not called")
	}
}

func TestCancelActionRunPostsRunControlEndpoint(t *testing.T) {
	called := false
	srv := actionServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v1/repos/acme/widgets/actions/runs/101/cancel", func(w http.ResponseWriter, r *http.Request) {
			called = true
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			w.WriteHeader(http.StatusAccepted)
		})
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.CancelActionRun(context.Background(), "acme", "widgets", 101); err != nil {
		t.Fatalf("CancelActionRun: %v", err)
	}
	if !called {
		t.Fatal("cancel endpoint was not called")
	}
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
