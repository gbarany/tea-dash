package mockgitea

import (
	"context"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
)

func TestMutationsRoundTrip(t *testing.T) {
	s := detailStore(time.Now())
	c := newTestClient(t, s)

	if _, err := c.AddComment("teahouse", "kettle", 1, "ship it"); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Comments("teahouse/kettle", 1)); got != 2 {
		t.Fatalf("comments = %d, want 2", got)
	}
	if err := c.SetIssueState("teahouse", "kettle", 4, data.ItemStateClosed); err != nil {
		t.Fatal(err)
	}
	if s.Issue("teahouse/kettle", 4).State != "closed" {
		t.Fatal("issue not closed")
	}
	if err := c.AddLabels("teahouse", "kettle", 4, []string{"bug"}); err != nil {
		t.Fatal(err)
	}
	if labels := s.Issue("teahouse/kettle", 4).Labels; len(labels) != 1 || labels[0].Name != "bug" {
		t.Fatalf("labels after AddLabels = %+v, want [bug]", labels)
	}
	if err := c.RemoveLabels("teahouse", "kettle", 4, []string{"bug"}); err != nil {
		t.Fatal(err)
	}
	if labels := s.Issue("teahouse/kettle", 4).Labels; len(labels) != 0 {
		t.Fatalf("labels after RemoveLabels = %+v, want none", labels)
	}
	if err := c.SetIssueMilestone("teahouse", "kettle", 4, "v1.0"); err != nil {
		t.Fatal(err)
	}
	if m := s.Issue("teahouse/kettle", 4).Milestone; m == nil || m.Title != "v1.0" {
		t.Fatalf("milestone after SetIssueMilestone = %+v, want v1.0", m)
	}
	if err := c.AssignIssueToMe("teahouse", "kettle", 4); err != nil {
		t.Fatal(err)
	}
	if err := c.SubscribeIssue("teahouse", "kettle", 4); err != nil {
		t.Fatal(err)
	}
	if !s.Issue("teahouse/kettle", 4).Subscribers[c.Me()] {
		t.Fatalf("subscribers after SubscribeIssue = %+v, want %s subscribed", s.Issue("teahouse/kettle", 4).Subscribers, c.Me())
	}
	if err := c.RequestPullReviewers("teahouse", "kettle", 1, []string{"mei"}); err != nil {
		t.Fatal(err)
	}
	if rv := s.Pull("teahouse/kettle", 1).Reviewers; len(rv) != 1 || rv[0].Login != "mei" {
		t.Fatalf("reviewers after RequestPullReviewers = %+v, want [mei]", rv)
	}
	if _, err := c.SubmitPullReview("teahouse", "kettle", 1,
		data.PullReviewOptions{Event: data.PullReviewEventApprove, Body: "lgtm"}); err != nil {
		t.Fatal(err)
	}
	if reviews := s.Pull("teahouse/kettle", 1).Reviews; len(reviews) != 2 || reviews[1].Body != "lgtm" || reviews[1].State != "APPROVED" {
		t.Fatalf("reviews after SubmitPullReview = %+v, want 2 with the new one appended", reviews)
	}
	if _, err := c.MarkPullDraft("teahouse", "kettle", 1); err != nil {
		t.Fatal(err)
	}
	if p := s.Pull("teahouse/kettle", 1); !p.Draft {
		t.Fatalf("pull after MarkPullDraft = %+v, want Draft=true", p)
	}
	if err := c.UpdatePullRequest("teahouse", "kettle", 1); err != nil {
		t.Fatal(err)
	}
}

func TestMergeRemovesFromOpenSearch(t *testing.T) {
	s := detailStore(time.Now())
	c := newTestClient(t, s)
	merged, err := c.MergePullRequest("teahouse", "kettle", 1, data.MergeOptions{Style: data.MergeStyleSquash})
	if err != nil || !merged {
		t.Fatalf("merge: %v merged=%v", err, merged)
	}
	rows, _, err := c.SearchPullsPage(context.Background(),
		config.PrIssueFilter{State: "open", CreatedBy: "@me"}, 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Number == 1 {
			t.Fatal("merged PR still in open search")
		}
	}
}
