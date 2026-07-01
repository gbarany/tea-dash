package prview

import (
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/data"
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
	out := RenderPull(samplePull(), nil, 60, false)
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
	out := RenderPull(samplePull(), detail, 60, false)
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
	folded := RenderPull(samplePull(), detail, 60, false)
	expanded := RenderPull(samplePull(), detail, 60, true)

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
	out := RenderPull(row, nil, 60, false)
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
	loading := RenderIssue(row, nil, 60, false)
	for _, want := range []string{"Loading", "#7", "Something broke", "CLOSED"} {
		if !strings.Contains(loading, want) {
			t.Fatalf("issue loading preview missing %q:\n%s", want, loading)
		}
	}

	detail := &data.IssueDetail{Body: "issue body token-abc here"}
	loaded := RenderIssue(row, detail, 60, false)
	if strings.Contains(loaded, "Loading") {
		t.Fatalf("loaded issue preview still shows Loading:\n%s", loaded)
	}
	if !strings.Contains(loaded, "token-abc") {
		t.Fatalf("loaded issue preview missing body token:\n%s", loaded)
	}
}
