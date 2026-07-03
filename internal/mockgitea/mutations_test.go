package mockgitea

import (
	"context"
	"net/http"
	"strings"
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

// TestSubscribeIssueTwiceSurfacesAlreadySubscribedError locks in the SDK's
// inverted-looking status contract for AddIssueSubscription: a redundant
// re-subscribe gets HTTP 200 from the mock (no state change happened), which
// the SDK itself turns into an "already subscribed" error — the caller sees
// a failure, not a silent no-op, on the second call.
func TestSubscribeIssueTwiceSurfacesAlreadySubscribedError(t *testing.T) {
	s := detailStore(time.Now())
	c := newTestClient(t, s)
	if err := c.SubscribeIssue("teahouse", "kettle", 4); err != nil {
		t.Fatal(err)
	}
	err := c.SubscribeIssue("teahouse", "kettle", 4)
	if err == nil || !strings.Contains(err.Error(), "already subscribed") {
		t.Fatalf("redundant re-subscribe error = %v, want \"already subscribed\"", err)
	}
}

// TestEditIssueAssigneesOmittedVsExplicitEmpty pins the nilable-slice
// convention handleEditIssue/handleEditPull rely on: a PATCH body with no
// "assignees" key at all must leave the current assignee list untouched,
// while an explicit "assignees": [] must clear it. Both look similar at the
// call site (neither one sets a *new* assignee), so this only distinguishes
// them by decoding straight off the wire, which is what the bug would break.
func TestEditIssueAssigneesOmittedVsExplicitEmpty(t *testing.T) {
	s := detailStore(time.Now())
	srv := NewServer(s)
	defer srv.Close()

	patch := func(body string) *http.Response {
		req, err := http.NewRequest(http.MethodPatch,
			srv.URL()+"/api/v1/repos/teahouse/kettle/issues/4", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// Precondition from the searchStore fixture: issue #4 starts assigned to
	// gabor (me).
	if got := s.Issue("teahouse/kettle", 4).Assignees; len(got) != 1 {
		t.Fatalf("fixture precondition: want 1 assignee, got %+v", got)
	}

	if resp := patch(`{"state":"closed"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("state-only patch status = %d, want 200", resp.StatusCode)
	}
	if got := s.Issue("teahouse/kettle", 4).Assignees; len(got) != 1 {
		t.Fatalf("assignees after omitted-key patch = %+v, want untouched (1)", got)
	}

	if resp := patch(`{"assignees":[]}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("explicit-empty patch status = %d, want 200", resp.StatusCode)
	}
	if got := s.Issue("teahouse/kettle", 4).Assignees; len(got) != 0 {
		t.Fatalf("assignees after explicit-empty patch = %+v, want cleared", got)
	}
}

// TestAddIssueLabelsBadIDIs400UnknownItemIs404 pins the two distinct failure
// modes handleAddIssueLabels' pre-check split exists to separate: a label ID
// that doesn't resolve on an otherwise-known item is a client error in an
// otherwise well-formed body (400), while targeting a nonexistent item at
// all is a missing resource (404) — regardless of whether the label ID in
// that second request happens to be valid.
func TestAddIssueLabelsBadIDIs400UnknownItemIs404(t *testing.T) {
	s := detailStore(time.Now())
	srv := NewServer(s)
	defer srv.Close()

	post := func(path, body string) *http.Response {
		resp, err := http.Post(srv.URL()+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	if resp := post("/api/v1/repos/teahouse/kettle/issues/4/labels", `{"labels":[99999]}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad label id on known item: status = %d, want 400", resp.StatusCode)
	}
	if resp := post("/api/v1/repos/teahouse/kettle/issues/9999/labels", `{"labels":[11]}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("valid label id on unknown item: status = %d, want 404", resp.StatusCode)
	}
}
