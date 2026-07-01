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

func TestGetPullDiffReturnsBytes(t *testing.T) {
	const want = "diff --git a/file.txt b/file.txt\n+hello\n"
	c := newDiffTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/acme/widgets/pulls/7.diff" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token t" {
			t.Fatalf("Authorization = %q, want token t", got)
		}
		fmt.Fprint(w, want)
	}))

	got, err := c.GetPullDiff("acme", "widgets", 7)
	if err != nil {
		t.Fatalf("GetPullDiff: %v", err)
	}
	if string(got) != want {
		t.Fatalf("GetPullDiff = %q, want %q", string(got), want)
	}
}

func TestGetPullDiffWrapsErrors(t *testing.T) {
	c := newDiffTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))

	_, err := c.GetPullDiff("acme", "widgets", 7)
	if err == nil {
		t.Fatal("GetPullDiff expected an error")
	}
	if !strings.Contains(err.Error(), "get pull diff acme/widgets#7") {
		t.Fatalf("GetPullDiff error = %v, want wrapped pull context", err)
	}
}

func newDiffTestClient(t *testing.T, diffHandler http.Handler) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me","full_name":"Me"}`)
	})
	mux.Handle("/api/v1/repos/acme/widgets/pulls/7.diff", diffHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}
