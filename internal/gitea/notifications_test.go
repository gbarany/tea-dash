package gitea

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/auth"
)

const notificationJSON = `[
  {
    "id": 12,
    "repository": {"full_name": "acme/widgets"},
    "subject": {
      "title": "Fix the dashboard",
      "url": "https://git.example/api/v1/repos/acme/widgets/pulls/42",
      "html_url": "https://git.example/acme/widgets/pulls/42",
      "latest_comment_html_url": "https://git.example/acme/widgets/pulls/42#comment-9",
      "type": "Pull",
      "state": "open"
    },
    "unread": true,
    "pinned": false,
    "updated_at": "2026-07-02T09:00:00Z",
    "url": "https://git.example/api/v1/notifications/threads/12"
  }
]`

func TestListNotificationsMapsThreads(t *testing.T) {
	var gotQuery string
	srv := notificationServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, notificationJSON)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	rows, total, err := c.ListNotifications(context.Background(), 25)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got total=%d len=%d, want one notification", total, len(rows))
	}
	n := rows[0]
	if n.ID != 12 || n.Number != 42 || n.SubjectTitle != "Fix the dashboard" ||
		n.SubjectType != "Pull" || n.SubjectState != "open" || n.RepoNameWithOwner != "acme/widgets" ||
		!n.Unread || n.Pinned || n.HTMLURL != "https://git.example/acme/widgets/pulls/42" ||
		n.LatestCommentURL != "https://git.example/acme/widgets/pulls/42#comment-9" {
		t.Fatalf("mapped notification = %+v", n)
	}
	wantUpdated := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	if !n.UpdatedAt.Equal(wantUpdated) {
		t.Fatalf("UpdatedAt = %s, want %s", n.UpdatedAt, wantUpdated)
	}
	if !strings.Contains(gotQuery, "limit=25") {
		t.Fatalf("query %q missing limit=25", gotQuery)
	}
	if !strings.Contains(gotQuery, "status-types=unread") {
		t.Fatalf("query %q missing unread status filter", gotQuery)
	}
}

func TestMarkNotificationReadUsesThreadEndpoint(t *testing.T) {
	var hit bool
	srv := notificationServer(t, nil, notificationRoute{
		pattern: "/api/v1/notifications/threads/12",
		handler: func(w http.ResponseWriter, r *http.Request) {
			hit = true
			if r.Method != http.MethodPatch {
				t.Fatalf("method = %s, want PATCH", r.Method)
			}
			if got := r.URL.Query().Get("to-status"); got != "read" {
				t.Fatalf("to-status = %q, want read", got)
			}
			assertEmptyBody(t, r)
			fmt.Fprint(w, `{"id":12,"unread":false}`)
		},
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.MarkNotificationRead(context.Background(), 12); err != nil {
		t.Fatalf("MarkNotificationRead: %v", err)
	}
	if !hit {
		t.Fatal("notification thread endpoint was not called")
	}
}

func TestMarkNotificationUnreadUsesThreadEndpoint(t *testing.T) {
	var hit bool
	srv := notificationServer(t, nil, notificationRoute{
		pattern: "/api/v1/notifications/threads/12",
		handler: func(w http.ResponseWriter, r *http.Request) {
			hit = true
			if r.Method != http.MethodPatch {
				t.Fatalf("method = %s, want PATCH", r.Method)
			}
			if got := r.URL.Query().Get("to-status"); got != "unread" {
				t.Fatalf("to-status = %q, want unread", got)
			}
			assertEmptyBody(t, r)
			fmt.Fprint(w, `{"id":12,"unread":true}`)
		},
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.MarkNotificationUnread(context.Background(), 12); err != nil {
		t.Fatalf("MarkNotificationUnread: %v", err)
	}
	if !hit {
		t.Fatal("notification thread endpoint was not called")
	}
}

func TestMarkAllNotificationsReadUsesNotificationsEndpoint(t *testing.T) {
	var hit bool
	srv := notificationServer(t, func(w http.ResponseWriter, r *http.Request) {
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
		assertEmptyBody(t, r)
		fmt.Fprint(w, `[]`)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.MarkAllNotificationsRead(context.Background()); err != nil {
		t.Fatalf("MarkAllNotificationsRead: %v", err)
	}
	if !hit {
		t.Fatal("notifications endpoint was not called")
	}
}

func TestMarkNotificationReadReturnsActionableError(t *testing.T) {
	srv := notificationServer(t, nil, notificationRoute{
		pattern: "/api/v1/notifications/threads/12",
		handler: func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "patch denied", http.StatusInternalServerError)
		},
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = c.MarkNotificationRead(context.Background(), 12)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "marking notification 12 read") ||
		!strings.Contains(msg, "500") ||
		!strings.Contains(msg, "patch denied") {
		t.Fatalf("error %q should include operation, status, and server body", msg)
	}
}

func TestMarkAllNotificationsReadReturnsActionableError(t *testing.T) {
	srv := notificationServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bulk denied", http.StatusBadGateway)
	})

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = c.MarkAllNotificationsRead(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "marking all unread notifications read") ||
		!strings.Contains(msg, "502") ||
		!strings.Contains(msg, "bulk denied") {
		t.Fatalf("error %q should include operation, status, and server body", msg)
	}
}

type notificationRoute struct {
	pattern string
	handler http.HandlerFunc
}

func notificationServer(t *testing.T, notifications http.HandlerFunc, routes ...notificationRoute) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	})
	if notifications != nil {
		mux.HandleFunc("/api/v1/notifications", notifications)
	}
	for _, route := range routes {
		mux.HandleFunc(route.pattern, route.handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func assertEmptyBody(t *testing.T, r *http.Request) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading request body: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("body = %q, want empty", body)
	}
}
