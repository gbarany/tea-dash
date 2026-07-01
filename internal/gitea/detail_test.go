package gitea

import (
	"testing"
	"time"

	sdk "code.gitea.io/sdk/gitea"

	"github.com/gbarany/tea-dash/internal/data"
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

// TestMapComments drives the pure comment mapper: author from Poster.UserName
// (json:"login"), nil Poster -> empty author, nil entries skipped, and an
// empty input mapping to a nil slice.
func TestMapComments(t *testing.T) {
	t.Run("empty and nil", func(t *testing.T) {
		if got := mapComments(nil); got != nil {
			t.Errorf("nil input should map to nil slice, got %v", got)
		}
		if got := mapComments([]*sdk.Comment{}); got != nil {
			t.Errorf("empty input should map to nil slice, got %v", got)
		}
	})

	t.Run("mapping and guards", func(t *testing.T) {
		created := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
		in := []*sdk.Comment{
			{Poster: &sdk.User{UserName: "alice"}, Body: "first", Created: created},
			nil,                              // skipped
			{Poster: nil, Body: "no poster"}, // empty author, kept
		}
		got := mapComments(in)
		if len(got) != 2 {
			t.Fatalf("expected 2 comments (nil skipped), got %d: %+v", len(got), got)
		}
		if got[0].Author != "alice" || got[0].Body != "first" || !got[0].CreatedAt.Equal(created) {
			t.Errorf("comment[0] mismatch: %+v", got[0])
		}
		if got[1].Author != "" || got[1].Body != "no poster" {
			t.Errorf("comment[1] should have empty author, got %+v", got[1])
		}
	})
}

// TestMapReviews drives the pure review mapper: author from Reviewer.UserName,
// state stringified, PENDING and empty (draft) states filtered out, nil
// Reviewer -> empty author, nil entries skipped.
func TestMapReviews(t *testing.T) {
	t.Run("empty and nil", func(t *testing.T) {
		if got := mapReviews(nil); got != nil {
			t.Errorf("nil input should map to nil slice, got %v", got)
		}
	})

	t.Run("state filtering and mapping", func(t *testing.T) {
		submitted := time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
		in := []*sdk.PullReview{
			{Reviewer: &sdk.User{UserName: "bob"}, State: sdk.ReviewStateApproved, Body: "lgtm", Submitted: submitted},
			{Reviewer: &sdk.User{UserName: "carol"}, State: sdk.ReviewStateRequestChanges, Body: "nope"},
			{Reviewer: &sdk.User{UserName: "dave"}, State: sdk.ReviewStateComment, Body: "nit"},
			{Reviewer: &sdk.User{UserName: "eve"}, State: sdk.ReviewStatePending, Body: "draft"},   // filtered
			{Reviewer: &sdk.User{UserName: "frank"}, State: sdk.ReviewStateUnknown, Body: "empty"}, // filtered
			nil, // skipped
			{Reviewer: nil, State: sdk.ReviewStateApproved, Body: "ghost"}, // empty author, kept
		}
		got := mapReviews(in)
		if len(got) != 4 {
			t.Fatalf("expected 4 reviews (PENDING/empty/nil dropped), got %d: %+v", len(got), got)
		}
		if got[0].Author != "bob" || got[0].State != data.ReviewStateApproved || got[0].Body != "lgtm" || !got[0].SubmittedAt.Equal(submitted) {
			t.Errorf("review[0] mismatch: %+v", got[0])
		}
		if got[1].State != data.ReviewStateRequestChanges || got[2].State != data.ReviewStateComment {
			t.Errorf("review states mismatch: %+v", got)
		}
		if got[3].Author != "" || got[3].State != data.ReviewStateApproved {
			t.Errorf("review[3] (nil reviewer) should have empty author, got %+v", got[3])
		}
	})
}

// TestMapCombinedStatus drives the pure CI mapper: state/SHA/total plus each
// check's fields, the TargetURL->URL fallback, nil-status guard, and skipping
// of nil per-status entries.
func TestMapCombinedStatus(t *testing.T) {
	t.Run("nil guard", func(t *testing.T) {
		got := mapCombinedStatus(nil)
		if got.State != "" || got.SHA != "" || got.Total != 0 || got.Checks != nil || got.HasCI() {
			t.Errorf("nil should map to zero CIStatus, got %+v", got)
		}
	})

	t.Run("full mapping with URL fallback", func(t *testing.T) {
		cs := &sdk.CombinedStatus{
			State:      sdk.StatusSuccess,
			SHA:        "abc123",
			TotalCount: 2,
			Statuses: []*sdk.Status{
				{Context: "build", State: sdk.StatusSuccess, Description: "ok", TargetURL: "http://ci/build"},
				nil, // skipped
				{Context: "lint", State: sdk.StatusFailure, Description: "bad", TargetURL: "", URL: "http://api/lint"},
			},
		}
		got := mapCombinedStatus(cs)
		if got.State != data.CIStateSuccess || got.SHA != "abc123" || got.Total != 2 {
			t.Errorf("combined status header mismatch: %+v", got)
		}
		if !got.HasCI() {
			t.Errorf("non-empty combined status should report HasCI: %+v", got)
		}
		if len(got.Checks) != 2 {
			t.Fatalf("expected 2 checks (nil skipped), got %d: %+v", len(got.Checks), got.Checks)
		}
		if got.Checks[0].Context != "build" || got.Checks[0].State != data.CheckStateSuccess ||
			got.Checks[0].Description != "ok" || got.Checks[0].TargetURL != "http://ci/build" {
			t.Errorf("check[0] mismatch: %+v", got.Checks[0])
		}
		if got.Checks[1].State != data.CheckStateFailure || got.Checks[1].TargetURL != "http://api/lint" {
			t.Errorf("check[1] should fall back to URL, got %+v", got.Checks[1])
		}
	})
}
