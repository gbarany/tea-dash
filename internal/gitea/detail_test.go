package gitea

import (
	"testing"

	sdk "code.gitea.io/sdk/gitea"
)

func intp(n int) *int { return &n }

// TestMapPullDetail drives the pure mapping helper without a live server,
// covering the *int nil-guards (stats absent) and the *PRBranchInfo guards.
func TestMapPullDetail(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		if got := mapPullDetail(nil); got.Body != "" || got.Additions != 0 {
			t.Errorf("nil should map to zero value, got %+v", got)
		}
	})

	t.Run("full", func(t *testing.T) {
		pr := &sdk.PullRequest{
			Body:         "body text",
			Mergeable:    true,
			HasMerged:    true,
			Base:         &sdk.PRBranchInfo{Ref: "main"},
			Head:         &sdk.PRBranchInfo{Ref: "feature", Sha: "abc123"},
			Additions:    intp(42),
			Deletions:    intp(7),
			ChangedFiles: intp(3),
		}
		got := mapPullDetail(pr)
		if got.Body != "body text" {
			t.Errorf("Body = %q, want %q", got.Body, "body text")
		}
		if got.BaseRef != "main" || got.HeadRef != "feature" || got.HeadSHA != "abc123" {
			t.Errorf("refs mismatch: %+v", got)
		}
		if !got.Mergeable || !got.Merged {
			t.Errorf("mergeable=%v merged=%v, want both true", got.Mergeable, got.Merged)
		}
		if got.Additions != 42 || got.Deletions != 7 || got.ChangedFiles != 3 {
			t.Errorf("stats mismatch: %+v", got)
		}
	})

	t.Run("nil stats and branches", func(t *testing.T) {
		pr := &sdk.PullRequest{Body: "b"} // Additions/Deletions/ChangedFiles nil; Base/Head nil
		got := mapPullDetail(pr)
		if got.Additions != 0 || got.Deletions != 0 || got.ChangedFiles != 0 {
			t.Errorf("nil stats should map to 0, got %+v", got)
		}
		if got.BaseRef != "" || got.HeadRef != "" || got.HeadSHA != "" {
			t.Errorf("nil branches should map to empty refs, got %+v", got)
		}
	})
}

// TestMapIssueDetail drives the issue mapping helper.
func TestMapIssueDetail(t *testing.T) {
	if got := mapIssueDetail(nil); got.Body != "" {
		t.Errorf("nil should map to zero value, got %+v", got)
	}
	iss := &sdk.Issue{Body: "issue body"}
	if got := mapIssueDetail(iss); got.Body != "issue body" {
		t.Errorf("Body = %q, want %q", got.Body, "issue body")
	}
}
