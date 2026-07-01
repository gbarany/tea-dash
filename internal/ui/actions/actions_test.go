package actions

import "testing"

func TestKindConstants(t *testing.T) {
	tests := map[Kind]string{
		KindComment:      "comment",
		KindMerge:        "merge",
		KindClose:        "close",
		KindReopen:       "reopen",
		KindReview:       "review",
		KindExternalDiff: "external_diff",
		KindCheckout:     "checkout",
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
			Title:       "Add action UI",
			URL:         "https://example.test/pr/42",
		},
		Prompt: Prompt{Mode: PromptText, Value: "ship it"},
	}
	if intent.Kind != KindComment || intent.Target.Repo != "gbarany/tea-dash" || intent.Prompt.Value != "ship it" {
		t.Fatalf("intent did not retain values: %+v", intent)
	}
}
