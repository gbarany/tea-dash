package teacli

import (
	"strings"
	"testing"
)

func TestRepoPullsEndpoint(t *testing.T) {
	tests := []struct {
		name               string
		owner, repo, state string
		want               string
	}{
		{
			name:  "defaults to open",
			owner: "gitea", repo: "tea", state: "",
			want: "/repos/gitea/tea/pulls?state=open",
		},
		{
			name:  "explicit closed",
			owner: "gitea", repo: "tea", state: "closed",
			want: "/repos/gitea/tea/pulls?state=closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RepoPullsEndpoint(tt.owner, tt.repo, tt.state)
			if got != tt.want {
				t.Fatalf("RepoPullsEndpoint(%q, %q, %q) = %q, want %q",
					tt.owner, tt.repo, tt.state, got, tt.want)
			}
		})
	}
}

func TestParseAPIError(t *testing.T) {
	// Real error envelope returned by `tea api` when not logged in.
	if err := parseAPIError([]byte(`{"message":"Only signed in user is allowed to call APIs."}`)); err == nil {
		t.Fatal("expected an error for a Gitea error envelope")
	} else if !strings.Contains(err.Error(), "Only signed in user") {
		t.Fatalf("error message not surfaced: %v", err)
	}

	// Not errors: a list response, an object without "message", and empty.
	for _, body := range []string{`[{"number":1}]`, `{"id":1,"title":"x"}`, ``, `  `} {
		if err := parseAPIError([]byte(body)); err != nil {
			t.Fatalf("parseAPIError(%q) = %v, want nil", body, err)
		}
	}
}

func TestClientBinaryDefault(t *testing.T) {
	c := New()
	if got := c.binary(); got != DefaultBinary {
		t.Fatalf("binary() = %q, want %q", got, DefaultBinary)
	}

	c.Binary = ""
	if got := c.binary(); got != DefaultBinary {
		t.Fatalf("binary() with empty Binary = %q, want %q", got, DefaultBinary)
	}
}
