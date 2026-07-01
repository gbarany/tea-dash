package data

import "testing"

func TestMutationDomainConstants(t *testing.T) {
	if ItemKindPull != "pull" || ItemKindIssue != "issue" {
		t.Fatalf("item kinds = %q/%q, want pull/issue", ItemKindPull, ItemKindIssue)
	}
	if ItemStateOpen != "open" || ItemStateClosed != "closed" {
		t.Fatalf("item states = %q/%q, want open/closed", ItemStateOpen, ItemStateClosed)
	}
	if MergeStyleMerge != "merge" ||
		MergeStyleRebase != "rebase" ||
		MergeStyleRebaseMerge != "rebase-merge" ||
		MergeStyleSquash != "squash" ||
		MergeStyleFastForwardOnly != "fast-forward-only" {
		t.Fatalf("merge style constants changed")
	}
	if PullReviewEventApprove != "approve" ||
		PullReviewEventRequestChanges != "request-changes" ||
		PullReviewEventComment != "comment" {
		t.Fatalf("review event constants changed")
	}
}
