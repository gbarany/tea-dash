package mockgitea

import (
	"context"
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
		State: "open", Author: s.Me(), Updated: now})
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
