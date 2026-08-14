package mockgitea

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/config"
)

func searchStore(now time.Time) *Store {
	s := NewStore()
	mei := &User{ID: 2, Login: "mei"}
	s.AddUser(mei)
	s.AddRepo(&Repo{FullName: "teahouse/kettle", Name: "kettle", Owner: &User{Login: "teahouse"}})
	s.AddPull(&Pull{Number: 1, RepoFullName: "teahouse/kettle", Title: "fix: login flow",
		State: "open", Mergeable: true, Author: s.Me(), Updated: now})
	s.AddPull(&Pull{Number: 2, RepoFullName: "teahouse/kettle", Title: "feat: rate limits",
		State: "open", Author: mei, Reviewers: []*User{s.Me()}, Updated: now.Add(-time.Hour)})
	s.AddPull(&Pull{Number: 3, RepoFullName: "teahouse/kettle", Title: "old fix",
		State: "closed", Author: s.Me(), Updated: now.Add(-48 * time.Hour)})
	s.AddIssue(&Issue{Number: 4, RepoFullName: "teahouse/kettle", Title: "bug: kettle whistles",
		State: "open", Author: mei, Assignees: []*User{s.Me()}, Updated: now})
	return s
}

func TestSearchPullsCreatedByMe(t *testing.T) {
	c := newTestClient(t, searchStore(time.Now()))
	rows, total, err := c.SearchPullsPage(context.Background(),
		config.PrIssueFilter{State: "open", CreatedBy: "@me"}, 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0].Number != 1 {
		t.Fatalf("want PR #1 only, got total=%d rows=%+v", total, rows)
	}
}

func TestSearchPullsReviewRequested(t *testing.T) {
	c := newTestClient(t, searchStore(time.Now()))
	rows, _, err := c.SearchPullsPage(context.Background(),
		config.PrIssueFilter{State: "open", ReviewRequested: "@me"}, 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Number != 2 {
		t.Fatalf("want PR #2, got %+v", rows)
	}
}

func TestSearchIssuesAssigned(t *testing.T) {
	c := newTestClient(t, searchStore(time.Now()))
	rows, _, err := c.SearchIssuesPage(context.Background(),
		config.PrIssueFilter{State: "open", AssignedBy: "@me"}, 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Number != 4 {
		t.Fatalf("want issue #4, got %+v", rows)
	}
}

// TestFilterPullsStableTiesByID guards filterPulls' use of sort.SliceStable:
// two pulls with an identical Updated timestamp must keep their input
// (ID-ascending) relative order rather than being reordered arbitrarily.
func TestFilterPullsStableTiesByID(t *testing.T) {
	now := time.Now()
	me := &User{Login: "gabor"}
	pulls := []*Pull{
		{ID: 101, Number: 1, Title: "a", State: "open", Author: me, Updated: now},
		{ID: 102, Number: 2, Title: "b", State: "open", Author: me, Updated: now},
	}
	got := filterPulls(pulls, url.Values{"state": {"open"}}, "gabor")
	if len(got) != 2 || got[0].ID != 101 || got[1].ID != 102 {
		t.Fatalf("want stable ID-ascending order for equal Updated, got %+v", got)
	}
}

func TestSearchPullsLabelsMatchAny(t *testing.T) {
	now := time.Now()
	s := NewStore()
	me := s.Me()
	s.AddRepo(&Repo{FullName: "teahouse/kettle", Name: "kettle", Owner: &User{Login: "teahouse"}})
	bug := &Label{Name: "bug"}
	urgent := &Label{Name: "urgent"}
	s.AddPull(&Pull{Number: 1, RepoFullName: "teahouse/kettle", Title: "only bug",
		State: "open", Author: me, Labels: []*Label{bug}, Updated: now})
	s.AddPull(&Pull{Number: 2, RepoFullName: "teahouse/kettle", Title: "only urgent",
		State: "open", Author: me, Labels: []*Label{urgent}, Updated: now})
	s.AddPull(&Pull{Number: 3, RepoFullName: "teahouse/kettle", Title: "both",
		State: "open", Author: me, Labels: []*Label{bug, urgent}, Updated: now})
	s.AddPull(&Pull{Number: 4, RepoFullName: "teahouse/kettle", Title: "none",
		State: "open", Author: me, Updated: now})

	c := newTestClient(t, s)
	rows, total, err := c.SearchPullsPage(context.Background(),
		config.PrIssueFilter{State: "open", Labels: []string{"bug", "urgent"}}, 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("want any-of match for bug|urgent (3 rows), got total=%d rows=%+v", total, rows)
	}
	got := map[int64]bool{}
	for _, r := range rows {
		got[r.Number] = true
	}
	for _, n := range []int64{1, 2, 3} {
		if !got[n] {
			t.Fatalf("missing PR #%d in any-of label match: %+v", n, rows)
		}
	}
	if got[4] {
		t.Fatalf("unlabeled PR #4 should not match: %+v", rows)
	}
}

func TestRepoScopedPullsPagination(t *testing.T) {
	c := newTestClient(t, searchStore(time.Now()))
	rows, total, err := c.ListRepoPullsPage(context.Background(), "teahouse/kettle",
		config.PrIssueFilter{State: "all"}, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(rows) != 2 {
		t.Fatalf("want total 3 page of 2, got total=%d len=%d", total, len(rows))
	}
}
