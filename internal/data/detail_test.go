package data

import (
	"testing"
	"time"
)

// TestPullDetailZeroValue documents the zero value of the detail type: no
// panics, nil slices, empty embedded CIStatus.
func TestPullDetailZeroValue(t *testing.T) {
	var d PullDetail
	if d.Body != "" || d.BaseRef != "" || d.HeadRef != "" || d.HeadSHA != "" {
		t.Errorf("zero PullDetail should have empty strings, got %+v", d)
	}
	if d.Mergeable || d.Merged {
		t.Errorf("zero PullDetail bools should be false, got mergeable=%v merged=%v", d.Mergeable, d.Merged)
	}
	if d.Additions != 0 || d.Deletions != 0 || d.ChangedFiles != 0 {
		t.Errorf("zero PullDetail stats should be 0, got %+v", d)
	}
	if d.Comments != nil || d.Reviews != nil {
		t.Errorf("zero PullDetail slices should be nil, got comments=%v reviews=%v", d.Comments, d.Reviews)
	}
	if d.CI.State != "" || d.CI.SHA != "" || d.CI.Total != 0 || d.CI.Checks != nil {
		t.Errorf("zero PullDetail CI should be empty, got %+v", d.CI)
	}
}

// TestPullDetailConstruction verifies a fully-populated value round-trips
// through the struct as written.
func TestPullDetailConstruction(t *testing.T) {
	now := time.Now()
	d := PullDetail{
		Body:         "hello",
		BaseRef:      "main",
		HeadRef:      "feature",
		HeadSHA:      "deadbeef",
		Mergeable:    true,
		Merged:       false,
		Additions:    10,
		Deletions:    3,
		ChangedFiles: 2,
		Comments:     []Comment{{Author: "a", Body: "b", CreatedAt: now}},
		Reviews:      []Review{{Author: "r", State: "APPROVED", Body: "lgtm", SubmittedAt: now}},
		CI: CIStatus{
			State: "success",
			SHA:   "deadbeef",
			Total: 1,
			Checks: []Check{
				{Context: "build", State: "success", Description: "ok", TargetURL: "http://x"},
			},
		},
	}
	if d.Additions != 10 || d.Deletions != 3 || d.ChangedFiles != 2 {
		t.Errorf("stats did not round-trip: %+v", d)
	}
	if len(d.Comments) != 1 || d.Comments[0].Author != "a" {
		t.Errorf("comments did not round-trip: %+v", d.Comments)
	}
	if len(d.Reviews) != 1 || d.Reviews[0].State != "APPROVED" {
		t.Errorf("reviews did not round-trip: %+v", d.Reviews)
	}
	if d.CI.Total != 1 || len(d.CI.Checks) != 1 || d.CI.Checks[0].Context != "build" {
		t.Errorf("CI did not round-trip: %+v", d.CI)
	}
}

// TestIssueDetailZeroValue documents the issue detail zero value.
func TestIssueDetailZeroValue(t *testing.T) {
	var d IssueDetail
	if d.Body != "" {
		t.Errorf("zero IssueDetail body should be empty, got %q", d.Body)
	}
	if d.Comments != nil {
		t.Errorf("zero IssueDetail comments should be nil, got %v", d.Comments)
	}
}
