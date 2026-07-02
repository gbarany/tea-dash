package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/auth"
	"github.com/gbarany/tea-dash/internal/data"
)

func mutationClient(t *testing.T, mutate http.HandlerFunc) *Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me","full_name":"Me"}`)
	})
	mux.HandleFunc("/", mutate)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func decodeMutationBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return body
}

func TestAddCommentPostsBodyAndMapsResponse(t *testing.T) {
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/acme/widgets/issues/7/comments" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body := decodeMutationBody(t, r)
		if body["body"] != "hello world" {
			t.Fatalf("body = %#v", body)
		}
		fmt.Fprint(w, `{"id":10,"body":"hello world","created_at":"2026-07-01T10:00:00Z","user":{"login":"alice"}}`)
	})

	got, err := c.AddComment("acme", "widgets", 7, "hello world")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	wantCreated := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	if got.Author != "alice" || got.Body != "hello world" || !got.CreatedAt.Equal(wantCreated) {
		t.Fatalf("comment = %+v", got)
	}
}

func TestSetIssueStatePatchesState(t *testing.T) {
	var states []string
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/acme/widgets/issues/42" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body := decodeMutationBody(t, r)
		state, _ := body["state"].(string)
		states = append(states, state)
		fmt.Fprintf(w, `{"number":42,"state":%q}`, state)
	})

	if err := c.SetIssueState("acme", "widgets", 42, data.ItemStateClosed); err != nil {
		t.Fatalf("close issue: %v", err)
	}
	if err := c.SetIssueState("acme", "widgets", 42, data.ItemStateOpen); err != nil {
		t.Fatalf("reopen issue: %v", err)
	}
	if len(states) != 2 || states[0] != "closed" || states[1] != "open" {
		t.Fatalf("states = %v, want [closed open]", states)
	}
}

func TestSetPullStatePatchesPullState(t *testing.T) {
	var states []string
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/acme/widgets/pulls/43" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body := decodeMutationBody(t, r)
		state, _ := body["state"].(string)
		states = append(states, state)
		fmt.Fprintf(w, `{"number":43,"state":%q}`, state)
	})

	if err := c.SetPullState("acme", "widgets", 43, data.ItemStateClosed); err != nil {
		t.Fatalf("close pull: %v", err)
	}
	if err := c.SetPullState("acme", "widgets", 43, data.ItemStateOpen); err != nil {
		t.Fatalf("reopen pull: %v", err)
	}
	if len(states) != 2 || states[0] != "closed" || states[1] != "open" {
		t.Fatalf("states = %v, want [closed open]", states)
	}
}

func TestAssignAndUnassignIssuePreservesOtherAssignees(t *testing.T) {
	var patched [][]string
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/acme/widgets/issues/42" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			fmt.Fprint(w, `{"number":42,"assignees":[{"login":"alice"}]}`)
		case http.MethodPatch:
			body := decodeMutationBody(t, r)
			raw, ok := body["assignees"].([]any)
			if !ok {
				t.Fatalf("assignees body = %#v", body)
			}
			got := make([]string, 0, len(raw))
			for _, v := range raw {
				got = append(got, v.(string))
			}
			patched = append(patched, got)
			fmt.Fprint(w, `{"number":42}`)
		default:
			t.Fatalf("method = %s", r.Method)
		}
	})

	if err := c.AssignIssueToMe("acme", "widgets", 42); err != nil {
		t.Fatalf("AssignIssueToMe: %v", err)
	}
	if len(patched) != 1 || strings.Join(patched[0], ",") != "alice,me" {
		t.Fatalf("assign patch = %#v, want alice,me", patched)
	}
}

func TestUnassignPullPreservesOtherAssignees(t *testing.T) {
	var patched []string
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/acme/widgets/pulls/43" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			fmt.Fprint(w, `{"number":43,"assignees":[{"login":"me"},{"login":"alice"}]}`)
		case http.MethodPatch:
			body := decodeMutationBody(t, r)
			raw, ok := body["assignees"].([]any)
			if !ok {
				t.Fatalf("assignees body = %#v", body)
			}
			patched = patched[:0]
			for _, v := range raw {
				patched = append(patched, v.(string))
			}
			fmt.Fprint(w, `{"number":43}`)
		default:
			t.Fatalf("method = %s", r.Method)
		}
	})

	if err := c.UnassignPullFromMe("acme", "widgets", 43); err != nil {
		t.Fatalf("UnassignPullFromMe: %v", err)
	}
	if len(patched) != 1 || patched[0] != "alice" {
		t.Fatalf("unassign patch = %#v, want alice only", patched)
	}
}

func TestAddLabelsResolvesNamesAndPostsIDs(t *testing.T) {
	var posted []int64
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/acme/widgets/labels":
			fmt.Fprint(w, `[{"id":1,"name":"bug"},{"id":2,"name":"urgent"}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/acme/widgets/issues/42/labels":
			body := decodeMutationBody(t, r)
			raw, ok := body["labels"].([]any)
			if !ok {
				t.Fatalf("labels body = %#v", body)
			}
			for _, v := range raw {
				posted = append(posted, int64(v.(float64)))
			}
			fmt.Fprint(w, `[{"id":1,"name":"bug"},{"id":2,"name":"urgent"}]`)
		default:
			t.Fatalf("%s %s", r.Method, r.URL.String())
		}
	})

	if err := c.AddLabels("acme", "widgets", 42, []string{"bug", "urgent"}); err != nil {
		t.Fatalf("AddLabels: %v", err)
	}
	if len(posted) != 2 || posted[0] != 1 || posted[1] != 2 {
		t.Fatalf("posted label IDs = %v, want [1 2]", posted)
	}
}

func TestRemoveLabelsResolvesNamesAndDeletesIDs(t *testing.T) {
	var deleted []string
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/acme/widgets/labels":
			fmt.Fprint(w, `[{"id":1,"name":"bug"},{"id":2,"name":"stale"}]`)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/repos/acme/widgets/issues/42/labels/"):
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/api/v1/repos/acme/widgets/issues/42/labels/"))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("%s %s", r.Method, r.URL.String())
		}
	})

	if err := c.RemoveLabels("acme", "widgets", 42, []string{"bug", "stale"}); err != nil {
		t.Fatalf("RemoveLabels: %v", err)
	}
	if strings.Join(deleted, ",") != "1,2" {
		t.Fatalf("deleted label IDs = %v, want [1 2]", deleted)
	}
}

func TestAddLabelsUnknownNameErrors(t *testing.T) {
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/repos/acme/widgets/labels" {
			t.Fatalf("%s %s", r.Method, r.URL.String())
		}
		fmt.Fprint(w, `[{"id":1,"name":"bug"}]`)
	})

	err := c.AddLabels("acme", "widgets", 42, []string{"missing"})
	if err == nil || !strings.Contains(err.Error(), `unknown label "missing"`) {
		t.Fatalf("AddLabels unknown error = %v", err)
	}
}

func TestMergePullRequestPostsOptionsAndReturnsMerged(t *testing.T) {
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/acme/widgets/pulls/44/merge" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body := decodeMutationBody(t, r)
		checks := map[string]any{
			"Do":                        "squash",
			"MergeTitleField":           "merge title",
			"MergeMessageField":         "merge message",
			"delete_branch_after_merge": true,
			"force_merge":               true,
			"head_commit_id":            "abc123",
		}
		for key, want := range checks {
			if body[key] != want {
				t.Fatalf("%s = %#v, want %#v in body %#v", key, body[key], want, body)
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	merged, err := c.MergePullRequest("acme", "widgets", 44, data.MergeOptions{
		Style:        data.MergeStyleSquash,
		Title:        "merge title",
		Message:      "merge message",
		DeleteBranch: true,
		ForceMerge:   true,
		HeadCommitID: "abc123",
	})
	if err != nil {
		t.Fatalf("MergePullRequest: %v", err)
	}
	if !merged {
		t.Fatal("merged = false, want true")
	}
}

func TestSubmitPullReviewPostsEventAndMapsResponse(t *testing.T) {
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/acme/widgets/pulls/45/reviews" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body := decodeMutationBody(t, r)
		if body["event"] != "REQUEST_CHANGES" || body["body"] != "needs work" {
			t.Fatalf("body = %#v", body)
		}
		fmt.Fprint(w, `{"id":2,"state":"REQUEST_CHANGES","body":"needs work","submitted_at":"2026-07-01T11:00:00Z","user":{"login":"reviewer"}}`)
	})

	got, err := c.SubmitPullReview("acme", "widgets", 45, data.PullReviewOptions{
		Event: data.PullReviewEventRequestChanges,
		Body:  "needs work",
	})
	if err != nil {
		t.Fatalf("SubmitPullReview: %v", err)
	}
	wantSubmitted := time.Date(2026, 7, 1, 11, 0, 0, 0, time.UTC)
	if got.Author != "reviewer" || got.State != data.ReviewStateRequestChanges || got.Body != "needs work" || !got.SubmittedAt.Equal(wantSubmitted) {
		t.Fatalf("review = %+v", got)
	}
}

func TestAddCommentWrapsServerError(t *testing.T) {
	c := mutationClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/acme/widgets/issues/7/comments" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	_, err := c.AddComment("acme", "widgets", 7, "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "add comment acme/widgets#7") || !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %q", err)
	}
}
