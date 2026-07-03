package actionrunner

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	localgit "github.com/gbarany/tea-dash/internal/git"
	"github.com/gbarany/tea-dash/internal/shell"
	uiactions "github.com/gbarany/tea-dash/internal/ui/actions"
)

func runDispatch(t *testing.T, r Runner, intent uiactions.Intent) uiactions.ResultMsg {
	t.Helper()
	cmd := r.Dispatch(intent)
	if cmd == nil {
		t.Fatal("Dispatch returned nil")
	}
	msg := cmd()
	got, ok := msg.(uiactions.ResultMsg)
	if !ok {
		t.Fatalf("Dispatch msg = %T, want actions.ResultMsg", msg)
	}
	return got
}

func pullIntent(kind uiactions.Kind) uiactions.Intent {
	return uiactions.Intent{
		Kind: kind,
		Target: uiactions.Target{
			RowKind: uiactions.RowKindPullRequest,
			Repo:    "acme/widgets",
			Number:  7,
			Title:   "PR title",
		},
	}
}

func issueIntent(kind uiactions.Kind) uiactions.Intent {
	in := pullIntent(kind)
	in.Target.RowKind = uiactions.RowKindIssue
	return in
}

func branchIntent(kind uiactions.Kind) uiactions.Intent {
	return uiactions.Intent{
		Kind: kind,
		Target: uiactions.Target{
			RowKind:        uiactions.RowKindBranch,
			Repo:           "tea-dash",
			RepositoryPath: "/src/tea-dash",
			Title:          "feature/local-ops",
		},
	}
}

func actionRunIntent(kind uiactions.Kind) uiactions.Intent {
	return uiactions.Intent{
		Kind: kind,
		Target: uiactions.Target{
			RowKind: uiactions.RowKindActionRun,
			Repo:    "acme/widgets",
			Number:  77,
			RunID:   101,
			Title:   "CI",
		},
	}
}

func TestDispatchCommentAndClose(t *testing.T) {
	client := &fakeClient{}
	r := New(Options{Client: client})

	comment := pullIntent(uiactions.KindComment)
	comment.Prompt.Value = "hello"
	got := runDispatch(t, r, comment)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("comment result = %+v", got)
	}
	if client.commentBody != "hello" {
		t.Fatalf("commentBody = %q, want hello", client.commentBody)
	}

	got = runDispatch(t, r, issueIntent(uiactions.KindClose))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("close issue result = %+v", got)
	}
	if client.issueState != data.ItemStateClosed {
		t.Fatalf("issueState = %q, want closed", client.issueState)
	}

	got = runDispatch(t, r, pullIntent(uiactions.KindReopen))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("reopen pull result = %+v", got)
	}
	if client.pullState != data.ItemStateOpen {
		t.Fatalf("pullState = %q, want open", client.pullState)
	}
}

func TestDispatchAssignAndUnassign(t *testing.T) {
	client := &fakeClient{}
	r := New(Options{Client: client})

	got := runDispatch(t, r, pullIntent(uiactions.KindAssign))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("assign pull result = %+v", got)
	}
	if client.assignPull != 7 {
		t.Fatalf("assignPull = %d, want 7", client.assignPull)
	}

	got = runDispatch(t, r, issueIntent(uiactions.KindUnassign))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("unassign issue result = %+v", got)
	}
	if client.unassignIssue != 7 {
		t.Fatalf("unassignIssue = %d, want 7", client.unassignIssue)
	}
}

func TestDispatchIssueSubscribeAndUnsubscribe(t *testing.T) {
	client := &fakeClient{}
	r := New(Options{Client: client})

	got := runDispatch(t, r, issueIntent(uiactions.KindSubscribe))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("subscribe issue result = %+v", got)
	}
	if client.subscribeIssue != 7 {
		t.Fatalf("subscribeIssue = %d, want 7", client.subscribeIssue)
	}
	if !strings.Contains(got.Message, "Subscribed to acme/widgets#7") {
		t.Fatalf("subscribe message = %q", got.Message)
	}

	got = runDispatch(t, r, issueIntent(uiactions.KindUnsubscribe))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("unsubscribe issue result = %+v", got)
	}
	if client.unsubscribeIssue != 7 {
		t.Fatalf("unsubscribeIssue = %d, want 7", client.unsubscribeIssue)
	}
	if !strings.Contains(got.Message, "Unsubscribed from acme/widgets#7") {
		t.Fatalf("unsubscribe message = %q", got.Message)
	}
}

func TestDispatchIssueSubscriptionRejectsUnsupportedRows(t *testing.T) {
	got := runDispatch(t, New(Options{Client: &fakeClient{}}), pullIntent(uiactions.KindSubscribe))
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "only available for issues") {
		t.Fatalf("subscribe pull result = %+v", got)
	}
}

func TestDispatchAssignRejectsUnsupportedRows(t *testing.T) {
	intent := branchIntent(uiactions.KindAssign)
	intent.Target.Repo = "acme/widgets"
	got := runDispatch(t, New(Options{Client: &fakeClient{}}), intent)
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "only available for pull requests and issues") {
		t.Fatalf("assign branch result = %+v", got)
	}
}

func TestDispatchAddAndRemoveLabels(t *testing.T) {
	client := &fakeClient{}
	r := New(Options{Client: client})

	add := pullIntent(uiactions.KindAddLabel)
	add.Prompt.Value = "bug, urgent"
	got := runDispatch(t, r, add)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("add label result = %+v", got)
	}
	if !reflect.DeepEqual(client.addLabels, []string{"bug", "urgent"}) || client.labelIndex != 7 {
		t.Fatalf("add labels = %v index=%d, want [bug urgent] index 7", client.addLabels, client.labelIndex)
	}

	remove := issueIntent(uiactions.KindRemoveLabel)
	remove.Prompt.Value = "stale"
	got = runDispatch(t, r, remove)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("remove label result = %+v", got)
	}
	if !reflect.DeepEqual(client.removeLabels, []string{"stale"}) || client.labelIndex != 7 {
		t.Fatalf("remove labels = %v index=%d, want [stale] index 7", client.removeLabels, client.labelIndex)
	}
}

func TestDispatchLabelsRejectEmptyInput(t *testing.T) {
	intent := pullIntent(uiactions.KindAddLabel)
	intent.Prompt.Value = " , "
	got := runDispatch(t, New(Options{Client: &fakeClient{}}), intent)
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "label names cannot be empty") {
		t.Fatalf("empty label result = %+v", got)
	}
}

func TestDispatchLabelsRejectUnsupportedRows(t *testing.T) {
	intent := branchIntent(uiactions.KindAddLabel)
	intent.Target.Repo = "acme/widgets"
	intent.Prompt.Value = "bug"
	got := runDispatch(t, New(Options{Client: &fakeClient{}}), intent)
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "only available for pull requests and issues") {
		t.Fatalf("label branch result = %+v", got)
	}
}

func TestDispatchSetIssueMilestone(t *testing.T) {
	client := &fakeClient{}
	intent := issueIntent(uiactions.KindSetMilestone)
	intent.Prompt.Value = "v1.2"

	got := runDispatch(t, New(Options{Client: client}), intent)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("set milestone result = %+v", got)
	}
	if client.milestoneIndex != 7 || client.milestoneTitle != "v1.2" {
		t.Fatalf("milestone index=%d title=%q, want #7 v1.2", client.milestoneIndex, client.milestoneTitle)
	}
	if !strings.Contains(got.Message, `Set milestone "v1.2" on acme/widgets#7`) {
		t.Fatalf("message = %q, want set milestone confirmation", got.Message)
	}
}

func TestDispatchSetMilestoneRejectsEmptyAndUnsupportedRows(t *testing.T) {
	empty := issueIntent(uiactions.KindSetMilestone)
	empty.Prompt.Value = "  "
	got := runDispatch(t, New(Options{Client: &fakeClient{}}), empty)
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "milestone title cannot be empty") {
		t.Fatalf("empty milestone result = %+v", got)
	}

	pull := pullIntent(uiactions.KindSetMilestone)
	pull.Prompt.Value = "v1.2"
	got = runDispatch(t, New(Options{Client: &fakeClient{}}), pull)
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "only available for issues") {
		t.Fatalf("pull milestone result = %+v", got)
	}
}

func TestDispatchMergeAndReview(t *testing.T) {
	client := &fakeClient{}
	r := New(Options{Client: client})

	got := runDispatch(t, r, pullIntent(uiactions.KindMerge))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("merge result = %+v", got)
	}
	if client.merge.Style != data.MergeStyleMerge {
		t.Fatalf("merge style = %q, want merge", client.merge.Style)
	}

	squash := pullIntent(uiactions.KindMerge)
	squash.Prompt.Value = "squash"
	got = runDispatch(t, r, squash)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("squash merge result = %+v", got)
	}
	if client.merge.Style != data.MergeStyleSquash {
		t.Fatalf("merge style = %q, want squash", client.merge.Style)
	}

	review := pullIntent(uiactions.KindReview)
	review.Prompt.Value = "approve"
	got = runDispatch(t, r, review)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("review result = %+v", got)
	}
	if client.review.Event != data.PullReviewEventApprove {
		t.Fatalf("review event = %q, want approve", client.review.Event)
	}
}

func TestDispatchReviewRequestChangesPassesBody(t *testing.T) {
	client := &fakeClient{}
	intent := pullIntent(uiactions.KindReview)
	intent.Prompt.Value = "request_changes"
	intent.Prompt.Body = "Needs tests before merge."

	got := runDispatch(t, New(Options{Client: client}), intent)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("review result = %+v", got)
	}
	if client.review.Event != data.PullReviewEventRequestChanges || client.review.Body != "Needs tests before merge." {
		t.Fatalf("review = %+v, want request-changes with body", client.review)
	}
}

func TestDispatchReviewRequestChangesRejectsEmptyBody(t *testing.T) {
	intent := pullIntent(uiactions.KindReview)
	intent.Prompt.Value = "request_changes"
	intent.Prompt.Body = "  "

	got := runDispatch(t, New(Options{Client: &fakeClient{}}), intent)
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "review body cannot be empty") {
		t.Fatalf("empty review body result = %+v", got)
	}
}

func TestDispatchRequestReviewersParsesCommaSeparatedReviewers(t *testing.T) {
	client := &fakeClient{}
	r := New(Options{Client: client})
	intent := pullIntent(uiactions.KindRequestReviewers)
	intent.Prompt.Value = " alice, bob, alice "

	got := runDispatch(t, r, intent)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("request reviewers result = %+v", got)
	}
	if client.reviewRequestIndex != 7 {
		t.Fatalf("reviewRequestIndex = %d, want 7", client.reviewRequestIndex)
	}
	if !reflect.DeepEqual(client.requestedReviewers, []string{"alice", "bob"}) {
		t.Fatalf("requestedReviewers = %#v, want alice/bob", client.requestedReviewers)
	}
	if !strings.Contains(got.Message, "Requested review from alice, bob on acme/widgets#7") {
		t.Fatalf("message = %q, want reviewer confirmation", got.Message)
	}
}

func TestDispatchRequestReviewersRejectsEmptyInput(t *testing.T) {
	intent := pullIntent(uiactions.KindRequestReviewers)
	intent.Prompt.Value = " , "

	got := runDispatch(t, New(Options{Client: &fakeClient{}}), intent)
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "reviewer usernames cannot be empty") {
		t.Fatalf("empty request reviewers result = %+v", got)
	}
}

func TestDispatchUpdatePullRequest(t *testing.T) {
	client := &fakeClient{}
	got := runDispatch(t, New(Options{Client: client}), pullIntent(uiactions.KindUpdateBranch))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("update branch result = %+v", got)
	}
	if client.updatePull != 7 {
		t.Fatalf("updatePull = %d, want 7", client.updatePull)
	}
	if !strings.Contains(got.Message, "Updated acme/widgets#7") {
		t.Fatalf("message = %q, want update confirmation", got.Message)
	}
}

func TestDispatchMarkPullReady(t *testing.T) {
	client := &fakeClient{markReadyChanged: true}
	got := runDispatch(t, New(Options{Client: client}), pullIntent(uiactions.KindMarkReady))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("mark ready result = %+v", got)
	}
	if client.markReady != 7 {
		t.Fatalf("markReady = %d, want 7", client.markReady)
	}
	if !strings.Contains(got.Message, "Marked acme/widgets#7 ready for review") {
		t.Fatalf("message = %q, want ready confirmation", got.Message)
	}
}

func TestDispatchMarkPullDraft(t *testing.T) {
	client := &fakeClient{markDraftChanged: true}
	got := runDispatch(t, New(Options{Client: client}), pullIntent(uiactions.KindMarkDraft))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("mark draft result = %+v", got)
	}
	if client.markDraft != 7 {
		t.Fatalf("markDraft = %d, want 7", client.markDraft)
	}
	if !strings.Contains(got.Message, "Marked acme/widgets#7 draft") {
		t.Fatalf("message = %q, want draft confirmation", got.Message)
	}
}

func TestDispatchExternalDiffRunsConfiguredPager(t *testing.T) {
	client := &fakeClient{diff: []byte("diff --git a/a b/a\n+hello\n")}
	execProcess := &fakeExecProcess{}
	r := New(Options{
		Client:      client,
		Config:      &config.Config{Pager: config.Pager{Diff: "diffnav"}},
		CWD:         "/tmp/repo",
		ExecProcess: execProcess.Run,
	})

	got := runDispatch(t, r, pullIntent(uiactions.KindExternalDiff))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("diff result = %+v", got)
	}
	if execProcess.cmd == nil {
		t.Fatal("exec process was not called")
	}
	if !reflect.DeepEqual(execProcess.cmd.Args[1:], []string{"-c", "diffnav"}) {
		t.Fatalf("shell args = %#v, want shell -c diffnav", execProcess.cmd.Args)
	}
	stdin, err := io.ReadAll(execProcess.cmd.Stdin)
	if err != nil {
		t.Fatalf("read pager stdin: %v", err)
	}
	if string(stdin) != string(client.diff) || execProcess.cmd.Dir != "/tmp/repo" {
		t.Fatalf("exec command stdin=%q dir=%q, want diff bytes in /tmp/repo", string(stdin), execProcess.cmd.Dir)
	}
}

func TestDispatchActionLogsOpensFirstJobLogInPager(t *testing.T) {
	client := &fakeClient{logBytes: []byte("job log token\n")}
	execProcess := &fakeExecProcess{}
	r := New(Options{
		Client:      client,
		Config:      &config.Config{Pager: config.Pager{Diff: "less -R"}},
		CWD:         "/tmp/repo",
		ExecProcess: execProcess.Run,
	})

	got := runDispatch(t, r, actionRunIntent(uiactions.KindViewLogs))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("logs result = %+v", got)
	}
	if client.listJobsRunID != 101 || client.logJobID != 201 {
		t.Fatalf("listJobsRunID=%d logJobID=%d, want run 101 job 201", client.listJobsRunID, client.logJobID)
	}
	if execProcess.cmd == nil {
		t.Fatal("exec process was not called")
	}
	if !reflect.DeepEqual(execProcess.cmd.Args[1:], []string{"-c", "less -R"}) {
		t.Fatalf("shell args = %#v, want shell -c less -R", execProcess.cmd.Args)
	}
	stdin, err := io.ReadAll(execProcess.cmd.Stdin)
	if err != nil {
		t.Fatalf("read pager stdin: %v", err)
	}
	if string(stdin) != "job log token\n" || execProcess.cmd.Dir != "/tmp/repo" {
		t.Fatalf("exec command stdin=%q dir=%q, want logs in /tmp/repo", string(stdin), execProcess.cmd.Dir)
	}
	if !strings.Contains(got.Message, "Viewed logs for acme/widgets run #77 job build") {
		t.Fatalf("message = %q, want logs confirmation", got.Message)
	}
}

func TestDispatchActionLogsErrorsWhenRunHasNoJobs(t *testing.T) {
	client := &fakeClient{jobs: []data.ActionJob{}}
	got := runDispatch(t, New(Options{Client: client}), actionRunIntent(uiactions.KindViewLogs))
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "no jobs found") {
		t.Fatalf("logs result = %+v, want no-jobs error", got)
	}
}

func TestDispatchCustomCommandRendersSelectedRowTemplate(t *testing.T) {
	execProcess := &fakeExecProcess{}
	r := New(Options{
		Config:      &config.Config{},
		InstanceURL: "https://git.example",
		CWD:         "/fallback",
		ExecProcess: execProcess.Run,
	})
	intent := pullIntent(uiactions.KindCustomCommand)
	intent.Command = "cd {{.RepoPath}} && echo {{.RepoName}} {{.PrNumber}}/{{.PrIndex}} {{.Title}} {{.Author}} {{.HeadRefName}} {{.BaseRefName}} {{.InstanceURL}} {{.Url}}"
	intent.Name = "lazygit"
	intent.Target.RepositoryPath = "/src/widgets"
	intent.Target.URL = "https://git.example/acme/widgets/pulls/7"
	intent.Target.Author = "alice"
	intent.Target.HeadRefName = "feature/ref-fields"
	intent.Target.BaseRefName = "main"

	got := runDispatch(t, r, intent)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("custom command result = %+v", got)
	}
	if execProcess.cmd == nil {
		t.Fatal("exec process was not called")
	}
	if len(execProcess.cmd.Args) != 3 || execProcess.cmd.Args[1] != "-c" {
		t.Fatalf("shell args = %#v, want shell -c", execProcess.cmd.Args)
	}
	want := "cd /src/widgets && echo acme/widgets 7/7 PR title alice feature/ref-fields main https://git.example https://git.example/acme/widgets/pulls/7"
	if execProcess.cmd.Args[2] != want {
		t.Fatalf("rendered command = %q, want %q", execProcess.cmd.Args[2], want)
	}
	if execProcess.cmd.Dir != "/src/widgets" {
		t.Fatalf("command dir = %q, want selected repo path", execProcess.cmd.Dir)
	}
	if !strings.Contains(got.Message, "lazygit") {
		t.Fatalf("custom command message = %q", got.Message)
	}
}

func TestDispatchCustomCommandMissingVariableDoesNotRunShell(t *testing.T) {
	execProcess := &fakeExecProcess{}
	r := New(Options{ExecProcess: execProcess.Run})
	intent := pullIntent(uiactions.KindCustomCommand)
	intent.Command = "echo {{.NoSuchField}}"

	got := runDispatch(t, r, intent)
	if got.Status != uiactions.ResultErrored || got.Err == nil ||
		!strings.Contains(got.Err.Error(), "render custom command template") {
		t.Fatalf("custom command missing variable result = %+v", got)
	}
	if execProcess.cmd != nil {
		t.Fatalf("exec should not run for a missing template variable, ran %+v", execProcess.cmd)
	}
}

func TestDispatchCheckoutPassesConfig(t *testing.T) {
	var gotOpts localgit.CheckoutOptions
	r := New(Options{
		Client:      &fakeClient{},
		Config:      &config.Config{RepoPaths: map[string]string{"acme/widgets": "/src/widgets"}, Git: config.Git{Remote: "upstream", PRBranchTemplate: "review-{{.PrIndex}}"}},
		InstanceURL: "https://git.example",
		CWD:         "/cwd",
		Checkout: func(_ context.Context, opts localgit.CheckoutOptions) (localgit.CheckoutPlan, error) {
			gotOpts = opts
			return localgit.CheckoutPlan{RepoPath: "/src/widgets", Branch: "review-7"}, nil
		},
	})

	got := runDispatch(t, r, pullIntent(uiactions.KindCheckout))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("checkout result = %+v", got)
	}
	if gotOpts.RepoName != "acme/widgets" || gotOpts.PrIndex != 7 || gotOpts.CWD != "/cwd" ||
		gotOpts.InstanceURL != "https://git.example" || gotOpts.Remote != "upstream" ||
		gotOpts.BranchTemplate != "review-{{.PrIndex}}" {
		t.Fatalf("checkout opts = %+v", gotOpts)
	}
}

func TestDispatchCheckoutIssuePassesConfig(t *testing.T) {
	var gotOpts localgit.IssueCheckoutOptions
	r := New(Options{
		Client:      &fakeClient{},
		Config:      &config.Config{RepoPaths: map[string]string{"acme/widgets": "/src/widgets"}, Git: config.Git{IssueBranchTemplate: "issue/{{.IssueIndex}}"}},
		InstanceURL: "https://git.example",
		CWD:         "/cwd",
		IssueCheckout: func(_ context.Context, opts localgit.IssueCheckoutOptions) (localgit.IssueCheckoutPlan, error) {
			gotOpts = opts
			return localgit.IssueCheckoutPlan{RepoPath: "/src/widgets", Branch: "issue/7"}, nil
		},
	})

	got := runDispatch(t, r, issueIntent(uiactions.KindCheckout))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("issue checkout result = %+v", got)
	}
	if gotOpts.RepoName != "acme/widgets" || gotOpts.IssueIndex != 7 || gotOpts.CWD != "/cwd" ||
		gotOpts.InstanceURL != "https://git.example" ||
		gotOpts.BranchTemplate != "issue/{{.IssueIndex}}" {
		t.Fatalf("issue checkout opts = %+v", gotOpts)
	}
	if !strings.Contains(got.Message, "Checked out issue branch issue/7") {
		t.Fatalf("message = %q, want issue checkout confirmation", got.Message)
	}
}

func TestDispatchSwitchBranchDoesNotRequireClient(t *testing.T) {
	var gotOpts localgit.SwitchBranchOptions
	r := New(Options{
		BranchSwitch: func(_ context.Context, opts localgit.SwitchBranchOptions) (localgit.SwitchBranchResult, error) {
			gotOpts = opts
			return localgit.SwitchBranchResult{RepoPath: opts.RepoPath, Branch: opts.Branch}, nil
		},
	})

	got := runDispatch(t, r, branchIntent(uiactions.KindSwitchBranch))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("switch branch result = %+v", got)
	}
	if gotOpts.RepoPath != "/src/tea-dash" || gotOpts.Branch != "feature/local-ops" {
		t.Fatalf("switch branch opts = %+v", gotOpts)
	}
	if !strings.Contains(got.Message, "Switched to feature/local-ops") {
		t.Fatalf("switch branch message = %q", got.Message)
	}
}

func TestDispatchBranchPushAndDeleteDoNotRequireClient(t *testing.T) {
	t.Run("push", func(t *testing.T) {
		var gotOpts localgit.PushBranchOptions
		r := New(Options{
			BranchPush: func(_ context.Context, opts localgit.PushBranchOptions) (localgit.PushBranchResult, error) {
				gotOpts = opts
				return localgit.PushBranchResult{RepoPath: opts.RepoPath, Branch: opts.Branch, Remote: opts.Remote}, nil
			},
		})

		got := runDispatch(t, r, branchIntent(uiactions.KindPushBranch))
		if got.Status != uiactions.ResultSucceeded || got.Err != nil {
			t.Fatalf("push branch result = %+v", got)
		}
		if gotOpts.RepoPath != "/src/tea-dash" || gotOpts.Branch != "feature/local-ops" || gotOpts.Remote != "origin" {
			t.Fatalf("push branch opts = %+v", gotOpts)
		}
		if !strings.Contains(got.Message, "Pushed feature/local-ops") {
			t.Fatalf("push branch message = %q", got.Message)
		}
	})

	t.Run("delete", func(t *testing.T) {
		var gotOpts localgit.DeleteBranchOptions
		r := New(Options{
			BranchDelete: func(_ context.Context, opts localgit.DeleteBranchOptions) (localgit.DeleteBranchResult, error) {
				gotOpts = opts
				return localgit.DeleteBranchResult{RepoPath: opts.RepoPath, Branch: opts.Branch}, nil
			},
		})

		got := runDispatch(t, r, branchIntent(uiactions.KindDeleteBranch))
		if got.Status != uiactions.ResultSucceeded || got.Err != nil {
			t.Fatalf("delete branch result = %+v", got)
		}
		if gotOpts.RepoPath != "/src/tea-dash" || gotOpts.Branch != "feature/local-ops" {
			t.Fatalf("delete branch opts = %+v", gotOpts)
		}
		if !strings.Contains(got.Message, "Deleted feature/local-ops") {
			t.Fatalf("delete branch message = %q", got.Message)
		}
	})
}

func TestDispatchBranchSyncActionsDoNotRequireClient(t *testing.T) {
	t.Run("fast forward", func(t *testing.T) {
		var gotOpts localgit.FastForwardBranchOptions
		r := New(Options{
			BranchFastForward: func(_ context.Context, opts localgit.FastForwardBranchOptions) (localgit.FastForwardBranchResult, error) {
				gotOpts = opts
				return localgit.FastForwardBranchResult{RepoPath: opts.RepoPath, Branch: opts.Branch, Remote: "origin", Upstream: "origin/feature/local-ops"}, nil
			},
		})

		got := runDispatch(t, r, branchIntent(uiactions.KindFastForwardBranch))
		if got.Status != uiactions.ResultSucceeded || got.Err != nil {
			t.Fatalf("fast-forward branch result = %+v", got)
		}
		if gotOpts.RepoPath != "/src/tea-dash" || gotOpts.Branch != "feature/local-ops" {
			t.Fatalf("fast-forward branch opts = %+v", gotOpts)
		}
		if !strings.Contains(got.Message, "Fast-forwarded feature/local-ops") {
			t.Fatalf("fast-forward branch message = %q", got.Message)
		}
	})

	t.Run("force push", func(t *testing.T) {
		var gotOpts localgit.ForcePushBranchOptions
		r := New(Options{
			BranchForcePush: func(_ context.Context, opts localgit.ForcePushBranchOptions) (localgit.ForcePushBranchResult, error) {
				gotOpts = opts
				return localgit.ForcePushBranchResult{RepoPath: opts.RepoPath, Branch: opts.Branch, Remote: "origin"}, nil
			},
		})

		got := runDispatch(t, r, branchIntent(uiactions.KindForcePushBranch))
		if got.Status != uiactions.ResultSucceeded || got.Err != nil {
			t.Fatalf("force-push branch result = %+v", got)
		}
		if gotOpts.RepoPath != "/src/tea-dash" || gotOpts.Branch != "feature/local-ops" || gotOpts.Remote != "" {
			t.Fatalf("force-push branch opts = %+v", gotOpts)
		}
		if !strings.Contains(got.Message, "Force-pushed feature/local-ops") {
			t.Fatalf("force-push branch message = %q", got.Message)
		}
	})
}

func TestDispatchActionRunControlsUseRunID(t *testing.T) {
	client := &fakeClient{}
	r := New(Options{Client: client})

	got := runDispatch(t, r, actionRunIntent(uiactions.KindRerunRun))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("rerun result = %+v", got)
	}
	if client.rerunRunID != 101 {
		t.Fatalf("rerunRunID = %d, want 101", client.rerunRunID)
	}

	got = runDispatch(t, r, actionRunIntent(uiactions.KindCancelRun))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("cancel result = %+v", got)
	}
	if client.cancelRunID != 101 {
		t.Fatalf("cancelRunID = %d, want 101", client.cancelRunID)
	}
}

func TestDispatchReturnsErrorResult(t *testing.T) {
	client := &fakeClient{err: errors.New("boom")}
	got := runDispatch(t, New(Options{Client: client}), pullIntent(uiactions.KindMerge))
	if got.Status != uiactions.ResultErrored || got.Err == nil || !strings.Contains(got.Err.Error(), "boom") {
		t.Fatalf("error result = %+v", got)
	}
}

type fakeClient struct {
	err                error
	commentBody        string
	issueState         data.ItemState
	pullState          data.ItemState
	merge              data.MergeOptions
	review             data.PullReviewOptions
	updatePull         int64
	markReady          int64
	markDraft          int64
	diff               []byte
	jobs               []data.ActionJob
	listJobsRunID      int64
	logJobID           int64
	logBytes           []byte
	rerunRunID         int64
	cancelRunID        int64
	assignPull         int64
	assignIssue        int64
	unassignPull       int64
	unassignIssue      int64
	labelIndex         int64
	addLabels          []string
	removeLabels       []string
	milestoneIndex     int64
	milestoneTitle     string
	reviewRequestIndex int64
	requestedReviewers []string
	subscribeIssue     int64
	unsubscribeIssue   int64
	markReadyChanged   bool
	markDraftChanged   bool
}

func (f *fakeClient) AddComment(_, _ string, _ int64, body string) (data.Comment, error) {
	f.commentBody = body
	return data.Comment{Body: body}, f.err
}

func (f *fakeClient) SetIssueState(_, _ string, _ int64, state data.ItemState) error {
	f.issueState = state
	return f.err
}

func (f *fakeClient) SetPullState(_, _ string, _ int64, state data.ItemState) error {
	f.pullState = state
	return f.err
}

func (f *fakeClient) AssignPullToMe(_, _ string, index int64) error {
	f.assignPull = index
	return f.err
}

func (f *fakeClient) UnassignPullFromMe(_, _ string, index int64) error {
	f.unassignPull = index
	return f.err
}

func (f *fakeClient) AssignIssueToMe(_, _ string, index int64) error {
	f.assignIssue = index
	return f.err
}

func (f *fakeClient) UnassignIssueFromMe(_, _ string, index int64) error {
	f.unassignIssue = index
	return f.err
}

func (f *fakeClient) SubscribeIssue(_, _ string, index int64) error {
	f.subscribeIssue = index
	return f.err
}

func (f *fakeClient) UnsubscribeIssue(_, _ string, index int64) error {
	f.unsubscribeIssue = index
	return f.err
}

func (f *fakeClient) AddLabels(_, _ string, index int64, names []string) error {
	f.labelIndex = index
	f.addLabels = append([]string(nil), names...)
	return f.err
}

func (f *fakeClient) RemoveLabels(_, _ string, index int64, names []string) error {
	f.labelIndex = index
	f.removeLabels = append([]string(nil), names...)
	return f.err
}

func (f *fakeClient) SetIssueMilestone(_, _ string, index int64, title string) error {
	f.milestoneIndex = index
	f.milestoneTitle = title
	return f.err
}

func (f *fakeClient) MergePullRequest(_, _ string, _ int64, opt data.MergeOptions) (bool, error) {
	f.merge = opt
	return f.err == nil, f.err
}

func (f *fakeClient) SubmitPullReview(_, _ string, _ int64, opt data.PullReviewOptions) (data.Review, error) {
	f.review = opt
	return data.Review{State: data.ReviewState(opt.Event)}, f.err
}

func (f *fakeClient) RequestPullReviewers(_, _ string, index int64, reviewers []string) error {
	f.reviewRequestIndex = index
	f.requestedReviewers = append([]string(nil), reviewers...)
	return f.err
}

func (f *fakeClient) UpdatePullRequest(_, _ string, index int64) error {
	f.updatePull = index
	return f.err
}

func (f *fakeClient) MarkPullReady(_, _ string, index int64) (bool, error) {
	f.markReady = index
	return f.markReadyChanged, f.err
}

func (f *fakeClient) MarkPullDraft(_, _ string, index int64) (bool, error) {
	f.markDraft = index
	return f.markDraftChanged, f.err
}

func (f *fakeClient) GetPullDiff(_, _ string, _ int64) ([]byte, error) {
	return f.diff, f.err
}

func (f *fakeClient) ListActionJobs(_ context.Context, _, _ string, runID int64) ([]data.ActionJob, error) {
	f.listJobsRunID = runID
	if f.jobs != nil {
		return f.jobs, f.err
	}
	return []data.ActionJob{{ID: 201, RunID: runID, Name: "build"}}, f.err
}

func (f *fakeClient) GetActionJobLogs(_ context.Context, _, _ string, jobID int64) ([]byte, error) {
	f.logJobID = jobID
	return f.logBytes, f.err
}

func (f *fakeClient) RerunActionRun(_ context.Context, _, _ string, runID int64) error {
	f.rerunRunID = runID
	return f.err
}

func (f *fakeClient) CancelActionRun(_ context.Context, _, _ string, runID int64) error {
	f.cancelRunID = runID
	return f.err
}

type fakeShellRunner struct {
	command shell.Command
	err     error
}

func (f *fakeShellRunner) Run(_ context.Context, cmd shell.Command) ([]byte, error) {
	f.command = cmd
	return nil, f.err
}

type fakeExecProcess struct {
	cmd *exec.Cmd
	err error
}

func (f *fakeExecProcess) Run(cmd *exec.Cmd, cb tea.ExecCallback) tea.Cmd {
	f.cmd = cmd
	return func() tea.Msg {
		return cb(f.err)
	}
}

var _ tea.Cmd
