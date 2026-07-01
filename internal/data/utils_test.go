package data

import (
	"testing"
	"time"
)

func TestPullRequestImplementsRowData(t *testing.T) {
	var _ RowData = PullRequest{}

	pr := PullRequest{
		Number:            7,
		Title:             "Fix thing",
		RepoNameWithOwner: "acme/widgets",
		HTMLURL:           "https://x/acme/widgets/pulls/7",
		UpdatedAt:         time.Unix(1000, 0),
	}
	var rd RowData = pr
	if rd.GetNumber() != 7 || rd.GetTitle() != "Fix thing" ||
		rd.GetRepoNameWithOwner() != "acme/widgets" || rd.GetUrl() != pr.HTMLURL ||
		!rd.GetUpdatedAt().Equal(pr.UpdatedAt) {
		t.Fatalf("RowData accessors wrong: %+v", rd)
	}
}
