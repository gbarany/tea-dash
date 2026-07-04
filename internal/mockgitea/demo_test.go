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

// TestDemoDataNoSharedIssueNumberBetweenPullAndIssue guards the invariant
// AddComment's doc comment calls out: Gitea numbers pulls and issues out of
// one shared per-repo index space, so a given (repo, number) must match at
// most one row across all pulls and issues in that repo. If a future edit to
// DemoData accidentally reuses a number across a pull and an issue in the
// same repo, AddComment would (harmlessly, but wrongly) bump both — this
// pins the fixture invariant rather than just AddComment's behavior.
func TestDemoDataNoSharedIssueNumberBetweenPullAndIssue(t *testing.T) {
	s := DemoData(time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC))
	for _, repo := range []string{"teahouse/kettle", "teahouse/steep", "teahouse/infra"} {
		seen := map[int64]string{}
		for _, p := range s.Pulls(repo) {
			if prev, ok := seen[p.Number]; ok {
				t.Fatalf("%s: number %d reused (%s, then pull %q)", repo, p.Number, prev, p.Title)
			}
			seen[p.Number] = "pull " + p.Title
		}
		for _, i := range s.Issues(repo) {
			if prev, ok := seen[i.Number]; ok {
				t.Fatalf("%s: number %d reused (%s, then issue %q)", repo, i.Number, prev, i.Title)
			}
			seen[i.Number] = "issue " + i.Title
		}
	}
}

// TestDemoDataNoPinnedAndUnreadNotification guards the constraint noted on
// demoBuilder.notifications: Pinned and Unread are independent booleans on
// the store's Notification type, but MarkAllNotificationsRead sweeps every
// row's Unread unconditionally, which only matches real single-NotifyStatus
// -enum Gitea semantics (see notificationEffectiveStatus) when no row is
// both pinned and unread.
func TestDemoDataNoPinnedAndUnreadNotification(t *testing.T) {
	s := DemoData(time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC))
	for _, n := range s.Notifications() {
		if n.Pinned && n.Unread {
			t.Fatalf("notification %d (%q) is both pinned and unread", n.ID, n.Title)
		}
	}
}

func TestDemoConfigValidates(t *testing.T) {
	cfg := DemoConfig("/tmp/kettle")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("demo config must validate: %v", err)
	}
	if len(cfg.PRSections) < 3 || len(cfg.IssuesSections) < 2 {
		t.Fatalf("demo config too thin: %+v", cfg)
	}
}
