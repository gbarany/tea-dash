package actions

import "testing"

func TestKindConstants(t *testing.T) {
	tests := map[Kind]string{
		KindComment:      "comment",
		KindAddLabel:     "add_label",
		KindRemoveLabel:  "remove_label",
		KindSetMilestone: "set_milestone",
		KindMerge:        "merge",
		KindUpdateBranch: "update_branch",
		KindMarkReady:    "mark_ready",
		KindMarkDraft:    "mark_draft",
		KindClose:        "close",
		KindReopen:       "reopen",
		KindReview:       "review",
		KindExternalDiff: "external_diff",
		KindCheckout:     "checkout",
		KindRerunRun:     "rerun_run",
		KindCancelRun:    "cancel_run",
	}
	for got, want := range tests {
		if string(got) != want {
			t.Fatalf("kind constant = %q, want %q", got, want)
		}
	}
}

func TestIntentCarriesTargetAndPrompt(t *testing.T) {
	intent := Intent{
		Kind: KindComment,
		Target: Target{
			SectionID:   2,
			SectionType: "pr",
			RowKind:     RowKindPullRequest,
			Repo:        "gbarany/tea-dash",
			Number:      42,
			RunID:       101,
			Title:       "Add action UI",
			URL:         "https://example.test/pr/42",
		},
		Prompt: Prompt{Mode: PromptText, Value: "ship it"},
	}
	if intent.Kind != KindComment || intent.Target.Repo != "gbarany/tea-dash" ||
		intent.Target.RunID != 101 || intent.Prompt.Value != "ship it" {
		t.Fatalf("intent did not retain values: %+v", intent)
	}
}
