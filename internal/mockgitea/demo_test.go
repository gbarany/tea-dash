package mockgitea

import (
	"context"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/config"
)

func TestDemoDataShape(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	s := DemoData(now)
	if s.Me().Login != "gabor" {
		t.Fatal("me must be gabor")
	}
	for _, repo := range []string{"teahouse/kettle", "teahouse/steep", "teahouse/infra"} {
		if s.RepoByFullName(repo) == nil {
			t.Fatalf("missing repo %s", repo)
		}
	}
	if n := len(s.AllPulls()); n < 12 {
		t.Fatalf("want >=12 PRs, got %d", n)
	}
	if n := len(s.AllIssues()); n < 10 {
		t.Fatalf("want >=10 issues, got %d", n)
	}
	if n := len(s.Notifications()); n < 8 {
		t.Fatalf("want >=8 notifications, got %d", n)
	}
	// every view's default demo sections must be non-empty
	c := newTestClient(t, s)
	rows, _, err := c.SearchPullsPage(context.Background(),
		config.PrIssueFilter{State: "open", CreatedBy: "@me"}, 30, 1)
	if err != nil || len(rows) < 3 {
		t.Fatalf("gabor needs >=3 open PRs for the demo: %v %d", err, len(rows))
	}
	review, _, err := c.SearchPullsPage(context.Background(),
		config.PrIssueFilter{State: "open", ReviewRequested: "@me"}, 30, 1)
	if err != nil || len(review) < 2 {
		t.Fatalf("gabor needs >=2 review requests: %v %d", err, len(review))
	}
}

func TestDemoDataDeterministic(t *testing.T) {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	a, b := DemoData(now), DemoData(now)
	if len(a.AllPulls()) != len(b.AllPulls()) || a.AllPulls()[0].Title != b.AllPulls()[0].Title {
		t.Fatal("DemoData must be deterministic for a fixed now")
	}
}
