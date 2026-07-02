package data

import (
	"testing"
	"time"
)

func TestNotificationSatisfiesRowData(t *testing.T) {
	updated := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	n := Notification{
		ID:                12,
		SubjectTitle:      "Fix the dashboard",
		RepoNameWithOwner: "gbarany/tea-dash",
		Number:            42,
		HTMLURL:           "https://git.example/gbarany/tea-dash/pulls/42",
		UpdatedAt:         updated,
	}

	var row RowData = n
	if row.GetRepoNameWithOwner() != "gbarany/tea-dash" || row.GetTitle() != "Fix the dashboard" ||
		row.GetNumber() != 42 || row.GetURL() != n.HTMLURL || !row.GetUpdatedAt().Equal(updated) {
		t.Fatalf("RowData projection mismatch: %+v", row)
	}
}
