package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gbarany/tea-dash/internal/auth"
)

// fakeGitea serves the minimal endpoints NewClient touches: the version probe
// (hit at construction) and the current-user lookup.
func fakeGitea(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me","full_name":"Me"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestNewClientResolvesMe(t *testing.T) {
	srv := fakeGitea(t)
	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Me() != "me" {
		t.Fatalf("Me() = %q, want %q", c.Me(), "me")
	}
}
