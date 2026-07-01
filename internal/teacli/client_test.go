package teacli

import "testing"

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
