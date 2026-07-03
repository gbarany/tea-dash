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
