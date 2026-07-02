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

func notificationServer(t *testing.T, notifications http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	})
	mux.HandleFunc("/api/v1/notifications", notifications)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}
