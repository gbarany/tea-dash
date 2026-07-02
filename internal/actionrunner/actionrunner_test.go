package actionrunner

import (
	"context"
	"errors"
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
	review.Prompt.Value = "request_changes"
	got = runDispatch(t, r, review)
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("review result = %+v", got)
	}
	if client.review.Event != data.PullReviewEventRequestChanges {
		t.Fatalf("review event = %q, want request-changes", client.review.Event)
	}
}

func TestDispatchExternalDiffRunsConfiguredPager(t *testing.T) {
	client := &fakeClient{diff: []byte("diff --git a/a b/a\n+hello\n")}
	shellRunner := &fakeShellRunner{}
	r := New(Options{
		Client:      client,
		Config:      &config.Config{Pager: config.Pager{Diff: "diffnav"}},
		CWD:         "/tmp/repo",
		ShellRunner: shellRunner,
	})

	got := runDispatch(t, r, pullIntent(uiactions.KindExternalDiff))
	if got.Status != uiactions.ResultSucceeded || got.Err != nil {
		t.Fatalf("diff result = %+v", got)
	}
	if shellRunner.command.Name == "" {
		t.Fatal("shell runner was not called")
	}
	if !reflect.DeepEqual(shellRunner.command.Args, []string{"-c", "diffnav"}) {
		t.Fatalf("shell args = %#v, want shell -c diffnav", shellRunner.command.Args)
	}
	if string(shellRunner.command.Stdin) != string(client.diff) || shellRunner.command.Dir != "/tmp/repo" {
		t.Fatalf("shell command = %+v", shellRunner.command)
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

func TestDispatchReturnsErrorResult(t *testing.T) {
	client := &fakeClient{err: errors.New("boom")}
	got := runDispatch(t, New(Options{Client: client}), pullIntent(uiactions.KindMerge))
	if got.Status != uiactions.ResultErrored || got.Err == nil || !strings.Contains(got.Err.Error(), "boom") {
		t.Fatalf("error result = %+v", got)
	}
}

type fakeClient struct {
	err         error
	commentBody string
	issueState  data.ItemState
	pullState   data.ItemState
	merge       data.MergeOptions
	review      data.PullReviewOptions
	diff        []byte
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

func (f *fakeClient) MergePullRequest(_, _ string, _ int64, opt data.MergeOptions) (bool, error) {
	f.merge = opt
	return f.err == nil, f.err
}

func (f *fakeClient) SubmitPullReview(_, _ string, _ int64, opt data.PullReviewOptions) (data.Review, error) {
	f.review = opt
	return data.Review{State: data.ReviewState(opt.Event)}, f.err
}

func (f *fakeClient) GetPullDiff(_, _ string, _ int64) ([]byte, error) {
	return f.diff, f.err
}

type fakeShellRunner struct {
	command shell.Command
	err     error
}

func (f *fakeShellRunner) Run(_ context.Context, cmd shell.Command) ([]byte, error) {
	f.command = cmd
	return nil, f.err
}

var _ tea.Cmd
