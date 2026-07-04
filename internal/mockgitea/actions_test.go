package mockgitea

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/gitea"
)

func actionsStore(now time.Time) *Store {
	s := searchStore(now)
	s.AddRun(&ActionRun{ID: 100, RepoFullName: "teahouse/kettle", DisplayTitle: "CI",
		WorkflowName: "ci.yml", Status: "success", Event: "push",
		HeadBranch: "main", HeadSHA: "abc123", Actor: s.Me(), Updated: now,
		Jobs: []*ActionJob{{ID: 1001, Name: "build", Status: "success",
			Logs: "compiling kettle...\nok\n"}}})
	return s
}

func TestActionsRoundTrip(t *testing.T) {
	c := newTestClient(t, actionsStore(time.Now()))
	ctx := context.Background()
	runs, total, err := c.ListActionRuns(ctx, "teahouse", "kettle", gitea.ActionRunListOptions{})
	if err != nil || total != 1 || len(runs) != 1 {
		t.Fatalf("list: %v total=%d runs=%+v", err, total, runs)
	}
	jobs, err := c.ListActionJobs(ctx, "teahouse", "kettle", 100)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs: %v %+v", err, jobs)
	}
	logs, err := c.GetActionJobLogs(ctx, "teahouse", "kettle", 1001)
	if err != nil || !strings.Contains(string(logs), "compiling") {
		t.Fatalf("logs: %v %q", err, logs)
	}
	if err := c.RerunActionRun(ctx, "teahouse", "kettle", 100); err != nil {
		t.Fatal(err)
	}
	run, err := c.GetActionRun(ctx, "teahouse", "kettle", 100)
	if err != nil || run.Status != "running" {
		t.Fatalf("after rerun want running, got %+v err=%v", run, err)
	}
	if err := c.CancelActionRun(ctx, "teahouse", "kettle", 100); err != nil {
		t.Fatal(err)
	}
}

// TestListActionRunsFiltersByBranchAndHeadSHA seeds a second run on a
// different branch/sha and verifies Branch/HeadSHA each narrow the list to
// just the matching run — the watch-checks flow depends on both filters.
func TestListActionRunsFiltersByBranchAndHeadSHA(t *testing.T) {
	now := time.Now()
	s := actionsStore(now)
	s.AddRun(&ActionRun{ID: 200, RepoFullName: "teahouse/kettle", DisplayTitle: "steamer",
		WorkflowName: "ci.yml", Status: "success", Event: "push",
		HeadBranch: "feature/steamer", HeadSHA: "def456", Actor: s.Me(), Updated: now})

	c := newTestClient(t, s)
	ctx := context.Background()

	byBranch, total, err := c.ListActionRuns(ctx, "teahouse", "kettle", gitea.ActionRunListOptions{Branch: "feature/steamer"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(byBranch) != 1 || byBranch[0].ID != 200 {
		t.Fatalf("branch filter: total=%d runs=%+v, want only run 200", total, byBranch)
	}

	bySHA, total, err := c.ListActionRuns(ctx, "teahouse", "kettle", gitea.ActionRunListOptions{HeadSHA: "abc123"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(bySHA) != 1 || bySHA[0].ID != 100 {
		t.Fatalf("head_sha filter: total=%d runs=%+v, want only run 100", total, bySHA)
	}
}
