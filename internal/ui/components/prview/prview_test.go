package prview

import (
	"strings"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/data"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

func samplePull() data.PullRequest {
	return data.PullRequest{
		Number:            42,
		Title:             "Add preview pane",
		RepoNameWithOwner: "gbarany/tea-dash",
		State:             "open",
	}
}

// longBody builds a Markdown bullet list that renders to well over foldLines.
func longBody(items int) string {
	rows := make([]string, items)
	for i := range rows {
		rows[i] = "- a distinct list item describing preview line behavior"
	}
	return strings.Join(rows, "\n")
}

// TestRenderPullLoading verifies the nil-detail placeholder shows "Loading…"
// alongside the locator and title.
func TestRenderPullLoading(t *testing.T) {
	out := RenderPull(samplePull(), nil, 60, false, appctx.DefaultStyles(), icons.Unicode)
	for _, want := range []string{"Loading", "#42", "Add preview pane", "gbarany/tea-dash"} {
		if !strings.Contains(out, want) {
			t.Fatalf("loading preview missing %q:\n%s", want, out)
		}
	}
}

// TestRenderPullWithDetail verifies a loaded body is rendered (and the loading
// placeholder is gone), including base/head and diffstat lines.
func TestRenderPullWithDetail(t *testing.T) {
	detail := &data.PullDetail{
		Body:         "This body has a **unique-token-xyz** phrase.",
		BaseRef:      "main",
		HeadRef:      "feature",
		Additions:    10,
		Deletions:    3,
		ChangedFiles: 2,
	}
	out := RenderPull(samplePull(), detail, 60, false, appctx.DefaultStyles(), icons.Unicode)
	if strings.Contains(out, "Loading") {
		t.Fatalf("loaded preview still shows Loading:\n%s", out)
	}
	for _, want := range []string{"unique-token-xyz", "main", "feature", "+10 -3, 2 files"} {
		if !strings.Contains(out, want) {
			t.Fatalf("loaded preview missing %q:\n%s", want, out)
		}
	}
}

// TestRenderPullFoldVsExpanded verifies a long body folds (shorter + hint) when
// not expanded and shows in full when expanded.
func TestRenderPullFoldVsExpanded(t *testing.T) {
	detail := &data.PullDetail{Body: longBody(40)}
	folded := RenderPull(samplePull(), detail, 60, false, appctx.DefaultStyles(), icons.Unicode)
	expanded := RenderPull(samplePull(), detail, 60, true, appctx.DefaultStyles(), icons.Unicode)

	if len(expanded) <= len(folded) {
		t.Fatalf("expanded (%d) should be longer than folded (%d)", len(expanded), len(folded))
	}
	if !strings.Contains(folded, "read more") {
		t.Fatalf("folded long body missing read-more hint:\n%s", folded)
	}
	if strings.Contains(expanded, "read more") {
		t.Fatalf("expanded body should not show read-more hint:\n%s", expanded)
	}
}

// TestRenderPullDraft verifies a draft row shows the DRAFT pill instead of a
// state pill.
func TestRenderPullDraft(t *testing.T) {
	row := samplePull()
	row.Draft = true
	out := RenderPull(row, nil, 60, false, appctx.DefaultStyles(), icons.Unicode)
	if !strings.Contains(out, "DRAFT") {
		t.Fatalf("draft preview missing DRAFT pill:\n%s", out)
	}
}

// TestRenderIssue verifies the issue variant renders locator, title, an
// uppercase state, and a loaded body.
func TestRenderIssue(t *testing.T) {
	row := data.Issue{
		Number:            7,
		Title:             "Something broke",
		RepoNameWithOwner: "gbarany/tea-dash",
		State:             "closed",
	}
	loading := RenderIssue(row, nil, 60, false, appctx.DefaultStyles(), icons.Unicode)
	for _, want := range []string{"Loading", "#7", "Something broke", "CLOSED"} {
		if !strings.Contains(loading, want) {
			t.Fatalf("issue loading preview missing %q:\n%s", want, loading)
		}
	}

	detail := &data.IssueDetail{Body: "issue body token-abc here"}
	loaded := RenderIssue(row, detail, 60, false, appctx.DefaultStyles(), icons.Unicode)
	if strings.Contains(loaded, "Loading") {
		t.Fatalf("loaded issue preview still shows Loading:\n%s", loaded)
	}
	if !strings.Contains(loaded, "token-abc") {
		t.Fatalf("loaded issue preview missing body token:\n%s", loaded)
	}
}

// TestRenderPullCommentsCIReviews verifies the appended CI, reviews, and
// comments sections all render with their contexts, badges, headers, authors,
// and bodies.
func TestRenderPullCommentsCIReviews(t *testing.T) {
	now := time.Now()
	detail := &data.PullDetail{
		Body: "pr body",
		CI: data.CIStatus{
			State: data.CIStateFailure,
			Checks: []data.Check{
				{Context: "build-linux", State: data.CheckStateSuccess, Description: "passed"},
				{Context: "test-race", State: data.CheckStateFailure, Description: "1 test failed"},
			},
		},
		Reviews: []data.Review{
			{Author: "octocat", State: data.ReviewStateApproved, SubmittedAt: now},
		},
		Comments: []data.Comment{
			{Author: "alice", Body: "first comment body zeta", CreatedAt: now.Add(-2 * time.Hour)},
			{Author: "bob", Body: "second comment body omega", CreatedAt: now.Add(-30 * time.Minute)},
		},
	}
	out := RenderPull(samplePull(), detail, 60, false, appctx.DefaultStyles(), icons.Unicode)

	wants := []string{
		// CI block
		"Checks:", "build-linux", "test-race",
		// Reviews block
		"Reviews:", "APPROVED", "octocat",
		// Comments block
		"2 comments", "alice", "first comment body zeta", "bob", "second comment body omega",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("PR preview missing %q:\n%s", want, out)
		}
	}
}

func TestRenderPullTabsSeparatesOverviewChecksReviewsAndComments(t *testing.T) {
	detail := &data.PullDetail{
		Body:    "overview-tab-token",
		BaseRef: "main",
		HeadRef: "feature",
		CI: data.CIStatus{
			State: data.CIStateFailure,
			Checks: []data.Check{{
				Context: "checks-tab-token",
				State:   data.CheckStateFailure,
			}},
		},
		Reviews: []data.Review{{
			Author: "reviews-tab-token",
			State:  data.ReviewStateApproved,
		}},
		Comments: []data.Comment{{
			Author: "comments-author-token",
			Body:   "comments-tab-token",
		}},
	}

	tabs := RenderPullTabs(samplePull(), detail, 60, false, appctx.DefaultStyles(), icons.Unicode)
	if len(tabs) != 4 {
		t.Fatalf("len(RenderPullTabs) = %d, want 4 tabs: %+v", len(tabs), tabs)
	}
	wantTitles := []string{"Overview", "Checks", "Reviews", "Comments"}
	for i, want := range wantTitles {
		if tabs[i].Title != want {
			t.Fatalf("tab %d title = %q, want %q", i, tabs[i].Title, want)
		}
	}
	if !strings.Contains(tabs[0].Content, "overview-tab-token") ||
		strings.Contains(tabs[0].Content, "checks-tab-token") ||
		strings.Contains(tabs[0].Content, "reviews-tab-token") ||
		strings.Contains(tabs[0].Content, "comments-tab-token") {
		t.Fatalf("overview tab should contain only overview detail:\n%s", tabs[0].Content)
	}
	if !strings.Contains(tabs[1].Content, "checks-tab-token") || strings.Contains(tabs[1].Content, "overview-tab-token") {
		t.Fatalf("checks tab should contain checks only:\n%s", tabs[1].Content)
	}
	if !strings.Contains(tabs[2].Content, "reviews-tab-token") || strings.Contains(tabs[2].Content, "checks-tab-token") {
		t.Fatalf("reviews tab should contain reviews only:\n%s", tabs[2].Content)
	}
	if !strings.Contains(tabs[3].Content, "comments-tab-token") || strings.Contains(tabs[3].Content, "reviews-tab-token") {
		t.Fatalf("comments tab should contain comments only:\n%s", tabs[3].Content)
	}
}

func TestRenderIssueTabsSeparatesOverviewAndComments(t *testing.T) {
	row := data.Issue{
		Number:            7,
		Title:             "Something broke",
		RepoNameWithOwner: "gbarany/tea-dash",
		State:             "open",
	}
	tabs := RenderIssueTabs(row, &data.IssueDetail{
		Body: "issue-overview-token",
		Comments: []data.Comment{{
			Author: "issue-commenter-token",
			Body:   "issue-comment-token",
		}},
	}, 60, false, appctx.DefaultStyles(), icons.Unicode)

	if len(tabs) != 2 {
		t.Fatalf("len(RenderIssueTabs) = %d, want 2 tabs: %+v", len(tabs), tabs)
	}
	if tabs[0].Title != "Overview" || tabs[1].Title != "Comments" {
		t.Fatalf("issue tab titles = %+v, want Overview/Comments", tabs)
	}
	if !strings.Contains(tabs[0].Content, "issue-overview-token") ||
		strings.Contains(tabs[0].Content, "issue-comment-token") {
		t.Fatalf("issue overview tab should not include comments:\n%s", tabs[0].Content)
	}
	if !strings.Contains(tabs[1].Content, "issue-comment-token") ||
		strings.Contains(tabs[1].Content, "issue-overview-token") {
		t.Fatalf("issue comments tab should not include overview body:\n%s", tabs[1].Content)
	}
}

// TestRenderIssueSingleComment verifies the issue comments block uses the
// singular "1 comment" header and shows the comment body.
func TestRenderIssueSingleComment(t *testing.T) {
	row := data.Issue{
		Number:            7,
		Title:             "Something broke",
		RepoNameWithOwner: "gbarany/tea-dash",
		State:             "open",
	}
	detail := &data.IssueDetail{
		Body: "issue body",
		Comments: []data.Comment{
			{Author: "carol", Body: "only comment here delta", CreatedAt: time.Now()},
		},
	}
	out := RenderIssue(row, detail, 60, false, appctx.DefaultStyles(), icons.Unicode)

	if !strings.Contains(out, "1 comment") {
		t.Fatalf("issue preview missing singular \"1 comment\":\n%s", out)
	}
	if strings.Contains(out, "1 comments") {
		t.Fatalf("issue preview should use singular header, got plural:\n%s", out)
	}
	if !strings.Contains(out, "only comment here delta") {
		t.Fatalf("issue preview missing comment body:\n%s", out)
	}
	if !strings.Contains(out, "carol") {
		t.Fatalf("issue preview missing comment author:\n%s", out)
	}
}

func TestRenderActionWithDetailShowsJobsAndSteps(t *testing.T) {
	row := data.ActionRun{
		ID:                101,
		RunNumber:         77,
		DisplayTitle:      "CI passed",
		WorkflowName:      "CI",
		RepoNameWithOwner: "gbarany/tea-dash",
		Status:            "completed",
		Conclusion:        "success",
	}
	detail := &data.ActionRunDetail{
		Run: row,
		Jobs: []data.ActionJob{{
			ID:         201,
			RunID:      101,
			Name:       "build",
			Status:     "completed",
			Conclusion: "success",
			RunnerName: "ubuntu-latest",
			Steps: []data.ActionStep{{
				Number:     1,
				Name:       "checkout",
				Status:     "completed",
				Conclusion: "success",
			}, {
				Number:     2,
				Name:       "go test",
				Status:     "completed",
				Conclusion: "failure",
			}},
		}},
	}

	out := RenderAction(row, detail, 80, appctx.DefaultStyles(), icons.Unicode)
	for _, want := range []string{
		"gbarany/tea-dash · #77",
		"CI passed",
		"Jobs:",
		"build",
		"ubuntu-latest",
		"checkout",
		"go test",
		"completed/success",
		"completed/failure",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("action detail preview missing %q:\n%s", want, out)
		}
	}
}

// TestRenderNilDetailNoComments verifies a nil detail keeps the Loading
// placeholder and renders no comments/CI sections.
func TestRenderNilDetailNoComments(t *testing.T) {
	out := RenderPull(samplePull(), nil, 60, false, appctx.DefaultStyles(), icons.Unicode)
	if !strings.Contains(out, "Loading") {
		t.Fatalf("nil-detail preview should show Loading:\n%s", out)
	}
	for _, absent := range []string{"comment", "Checks:", "Reviews:"} {
		if strings.Contains(out, absent) {
			t.Fatalf("nil-detail preview should not contain %q:\n%s", absent, out)
		}
	}
}
