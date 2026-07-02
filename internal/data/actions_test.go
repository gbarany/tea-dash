package data

import (
	"testing"
	"time"
)

func TestActionRunSatisfiesRowData(t *testing.T) {
	started := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 7, 2, 8, 5, 0, 0, time.UTC)
	run := ActionRun{
		ID:                101,
		RunNumber:         12,
		DisplayTitle:      "Fix checkout flakes",
		WorkflowName:      "CI",
		RepoNameWithOwner: "acme/widgets",
		HTMLURL:           "https://git.example/acme/widgets/actions/runs/101",
		StartedAt:         started,
		UpdatedAt:         updated,
	}

	var row RowData = run
	if row.GetRepoNameWithOwner() != "acme/widgets" || row.GetTitle() != "Fix checkout flakes" ||
		row.GetNumber() != 12 || row.GetURL() != run.HTMLURL || !row.GetUpdatedAt().Equal(updated) {
		t.Fatalf("RowData projection mismatch: %+v", row)
	}
}

func TestActionRunRowDataFallbacks(t *testing.T) {
	started := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	run := ActionRun{
		ID:                101,
		WorkflowName:      "CI",
		RepoNameWithOwner: "acme/widgets",
		StartedAt:         started,
	}

	var row RowData = run
	if row.GetTitle() != "CI" {
		t.Fatalf("GetTitle() = %q, want workflow name fallback", row.GetTitle())
	}
	if row.GetNumber() != 101 {
		t.Fatalf("GetNumber() = %d, want ID fallback", row.GetNumber())
	}
	if !row.GetUpdatedAt().Equal(started) {
		t.Fatalf("GetUpdatedAt() = %s, want StartedAt fallback %s", row.GetUpdatedAt(), started)
	}
}

func TestActionJobCarriesSteps(t *testing.T) {
	started := time.Date(2026, 7, 2, 8, 1, 0, 0, time.UTC)
	completed := time.Date(2026, 7, 2, 8, 4, 0, 0, time.UTC)
	job := ActionJob{
		ID:          201,
		RunID:       101,
		Name:        "build",
		Status:      "success",
		Conclusion:  "success",
		RunnerName:  "ubuntu-latest",
		StartedAt:   started,
		CompletedAt: completed,
		Steps: []ActionStep{
			{
				Number:      1,
				Name:        "checkout",
				Status:      "success",
				Conclusion:  "success",
				StartedAt:   started,
				CompletedAt: completed,
			},
		},
	}

	if job.ID != 201 || job.RunID != 101 || len(job.Steps) != 1 {
		t.Fatalf("job fields mismatch: %+v", job)
	}
	if job.Steps[0].Number != 1 || job.Steps[0].Name != "checkout" {
		t.Fatalf("step fields mismatch: %+v", job.Steps[0])
	}
}
