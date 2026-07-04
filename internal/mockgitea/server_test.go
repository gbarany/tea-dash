package mockgitea

import (
	"context"
	"testing"

	"github.com/gbarany/tea-dash/internal/auth"
	"github.com/gbarany/tea-dash/internal/gitea"
)

// newTestClient boots a server over the store and connects the REAL client.
func newTestClient(t *testing.T, s *Store) *gitea.Client {
	t.Helper()
	srv := NewServer(s)
	t.Cleanup(srv.Close)
	c, err := gitea.NewClient(context.Background(), auth.Config{URL: srv.URL(), Token: "mock-token"})
	if err != nil {
		t.Fatalf("NewClient against mock: %v", err)
	}
	return c
}

func TestClientConnectsAndResolvesMe(t *testing.T) {
	c := newTestClient(t, NewStore())
	if c.Me() != "gabor" {
		t.Fatalf("Me() = %q, want gabor", c.Me())
	}
}
