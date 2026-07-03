package mockgitea

import (
	"strings"
	"testing"
	"time"
)

func detailStore(now time.Time) *Store {
	s := searchStore(now)
	p := s.Pull("teahouse/kettle", 1)
	p.HeadRef, p.HeadSHA, p.BaseRef = "fix/login", "abc123", "main"
	p.Diff = "diff --git a/login.go b/login.go\n+fixed\n"
	p.Additions, p.Deletions, p.ChangedFiles = 42, 7, 3
	p.Statuses = []*CommitStatus{{Status: "success", Context: "ci/build"}}
	p.Reviews = []*Review{{ID: 9, State: "APPROVED", Reviewer: &User{Login: "mei"}, Created: now}}
	s.AddComment("teahouse/kettle", 1, "mei", "looks good")
	s.AddLabelDef("teahouse/kettle", &Label{ID: 11, Name: "bug", Color: "ee0000"})
	s.AddMilestoneDef("teahouse/kettle", &Milestone{ID: 21, Title: "v1.0", State: "open"})
	return s
}

func TestGetPullDetail(t *testing.T) {
	c := newTestClient(t, detailStore(time.Now()))
	d, err := c.GetPullDetail("teahouse", "kettle", 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.HeadRef != "fix/login" || d.HeadSHA != "abc123" {
		t.Fatalf("head not mapped: %+v", d)
	}
	if len(d.Comments) != 1 || len(d.Reviews) != 1 {
		t.Fatalf("want 1 comment + 1 review, got %+v", d)
	}
	if len(d.CI.Checks) == 0 {
		t.Fatalf("want combined-status checks, got %+v", d.CI)
	}
	if d.Additions != 42 || d.Deletions != 7 || d.ChangedFiles != 3 {
		t.Fatalf("diff stats not mapped: %+v", d)
	}
}

func TestGetIssueDetailAndDiffAndReviewers(t *testing.T) {
	c := newTestClient(t, detailStore(time.Now()))
	if _, err := c.GetIssueDetail("teahouse", "kettle", 4); err != nil {
		t.Fatal(err)
	}
	diff, err := c.GetPullDiff("teahouse", "kettle", 1)
	if err != nil || !strings.Contains(string(diff), "login.go") {
		t.Fatalf("diff: %v %q", err, diff)
	}
	users, err := c.ListReviewers("teahouse", "kettle")
	if err != nil || len(users) == 0 {
		t.Fatalf("reviewers: %v %v", err, users)
	}
	if _, err := c.MergeCapabilities("teahouse", "kettle"); err != nil {
		t.Fatal(err)
	}
}

// TestGetPullDetailUnknownNumberIs404WithPathInBody is a durable regression
// test for the loud-404 invariant (server.go's routes doc comment): an
// unknown resource must fail with a body naming the offending method+path,
// not a bare/empty 404, so route drift surfaces immediately in any test that
// exercises it — even through the real SDK's error-message extraction, which
// unwraps the JSON body's "message" field into the returned error's text.
func TestGetPullDetailUnknownNumberIs404WithPathInBody(t *testing.T) {
	c := newTestClient(t, detailStore(time.Now()))
	_, err := c.GetPullDetail("teahouse", "kettle", 999)
	if err == nil {
		t.Fatal("want error for unknown pull number")
	}
	msg := err.Error()
	if !strings.Contains(msg, "GET") || !strings.Contains(msg, "/api/v1/repos/teahouse/kettle/pulls/999") {
		t.Fatalf("error %q should name the method and path, got %q", msg, msg)
	}
}

// TestWorstStatusPrecedence pins worstStatus' severity order: error outranks
// failure, which outranks pending, which outranks success, which outranks
// warning (the least severe — a passed-but-flagged check, not a broken one).
func TestWorstStatusPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		statuses []*CommitStatus
		want     string
	}{
		{"empty", nil, "success"},
		{"success and failure -> failure", []*CommitStatus{{Status: "success"}, {Status: "failure"}}, "failure"},
		{"pending and success -> pending", []*CommitStatus{{Status: "pending"}, {Status: "success"}}, "pending"},
		{"error outranks failure", []*CommitStatus{{Status: "failure"}, {Status: "error"}}, "error"},
		{"warning is least severe", []*CommitStatus{{Status: "warning"}, {Status: "success"}}, "success"},
		{"nil entries skipped", []*CommitStatus{nil, {Status: "pending"}, nil}, "pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worstStatus(tt.statuses); got != tt.want {
				t.Errorf("worstStatus(%+v) = %q, want %q", tt.statuses, got, tt.want)
			}
		})
	}
}
