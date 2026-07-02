// Package actionrunner executes UI action intents against Gitea, local git, and
// configured shell commands.
package actionrunner

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	localgit "github.com/gbarany/tea-dash/internal/git"
	"github.com/gbarany/tea-dash/internal/shell"
	uiactions "github.com/gbarany/tea-dash/internal/ui/actions"
)

const actionTimeout = 2 * time.Minute

// Client is the subset of the Gitea client used by action dispatch.
type Client interface {
	AddComment(owner, repo string, index int64, body string) (data.Comment, error)
	SetIssueState(owner, repo string, index int64, state data.ItemState) error
	SetPullState(owner, repo string, index int64, state data.ItemState) error
	MergePullRequest(owner, repo string, index int64, opt data.MergeOptions) (bool, error)
	SubmitPullReview(owner, repo string, index int64, opt data.PullReviewOptions) (data.Review, error)
	GetPullDiff(owner, repo string, index int64) ([]byte, error)
	RerunActionRun(ctx context.Context, owner, repo string, runID int64) error
	CancelActionRun(ctx context.Context, owner, repo string, runID int64) error
}

// CheckoutFunc runs or fakes a local PR checkout.
type CheckoutFunc func(context.Context, localgit.CheckoutOptions) (localgit.CheckoutPlan, error)

// BranchSwitchFunc runs or fakes a local branch switch.
type BranchSwitchFunc func(context.Context, localgit.SwitchBranchOptions) (localgit.SwitchBranchResult, error)

// Options configures a Runner.
type Options struct {
	Client       Client
	Config       *config.Config
	InstanceURL  string
	CWD          string
	ShellRunner  shell.Runner
	GitRunner    localgit.Runner
	Checkout     CheckoutFunc
	BranchSwitch BranchSwitchFunc
}

// Runner executes actions and returns ResultMsg values for the UI.
type Runner struct {
	client       Client
	cfg          *config.Config
	instanceURL  string
	cwd          string
	shellRunner  shell.Runner
	gitRunner    localgit.Runner
	checkout     CheckoutFunc
	branchSwitch BranchSwitchFunc
}

// New constructs a Runner with production defaults for omitted runners.
func New(opts Options) Runner {
	cfg := opts.Config
	if cfg == nil {
		cfg = &config.Config{}
	}
	shellRunner := opts.ShellRunner
	if shellRunner == nil {
		shellRunner = shell.ExecRunner{}
	}
	checkout := opts.Checkout
	if checkout == nil {
		checkout = localgit.RunCheckout
	}
	branchSwitch := opts.BranchSwitch
	if branchSwitch == nil {
		branchSwitch = localgit.SwitchBranch
	}
	return Runner{
		client:       opts.Client,
		cfg:          cfg,
		instanceURL:  opts.InstanceURL,
		cwd:          opts.CWD,
		shellRunner:  shellRunner,
		gitRunner:    opts.GitRunner,
		checkout:     checkout,
		branchSwitch: branchSwitch,
	}
}

// Dispatch returns a Bubble Tea command that executes intent off the update
// path and reports the result back as actions.ResultMsg.
func (r Runner) Dispatch(intent uiactions.Intent) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()

		message, err := r.run(ctx, intent)
		if err != nil {
			return uiactions.ResultMsg{Intent: intent, Status: uiactions.ResultErrored, Err: err}
		}
		return uiactions.ResultMsg{Intent: intent, Status: uiactions.ResultSucceeded, Message: message}
	}
}

func (r Runner) run(ctx context.Context, intent uiactions.Intent) (string, error) {
	if intent.Kind == uiactions.KindSwitchBranch {
		branch, err := r.branchSwitch(ctx, localgit.SwitchBranchOptions{
			RepoPath: intent.Target.RepositoryPath,
			Branch:   intent.Target.Title,
			Runner:   r.gitRunner,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Switched to %s in %s.", branch.Branch, branch.RepoPath), nil
	}
	if r.client == nil {
		return "", fmt.Errorf("%s is unavailable: no Gitea client", actionLabel(intent.Kind))
	}
	owner, repo, err := splitRepo(intent.Target.Repo)
	if err != nil {
		return "", err
	}
	index := intent.Target.Number

	switch intent.Kind {
	case uiactions.KindComment:
		body := strings.TrimSpace(intent.Prompt.Value)
		if body == "" {
			return "", fmt.Errorf("comment body cannot be empty")
		}
		if _, err := r.client.AddComment(owner, repo, index, body); err != nil {
			return "", err
		}
		return fmt.Sprintf("Commented on %s#%d.", intent.Target.Repo, index), nil

	case uiactions.KindClose:
		if err := r.setState(owner, repo, index, intent.Target.RowKind, data.ItemStateClosed); err != nil {
			return "", err
		}
		return fmt.Sprintf("Closed %s#%d.", intent.Target.Repo, index), nil

	case uiactions.KindReopen:
		if err := r.setState(owner, repo, index, intent.Target.RowKind, data.ItemStateOpen); err != nil {
			return "", err
		}
		return fmt.Sprintf("Reopened %s#%d.", intent.Target.Repo, index), nil

	case uiactions.KindMerge:
		style, err := mergeStyle(intent.Prompt.Value)
		if err != nil {
			return "", err
		}
		merged, err := r.client.MergePullRequest(owner, repo, index, data.MergeOptions{Style: style})
		if err != nil {
			return "", err
		}
		if !merged {
			return fmt.Sprintf("Merge requested for %s#%d.", intent.Target.Repo, index), nil
		}
		return fmt.Sprintf("Merged %s#%d.", intent.Target.Repo, index), nil

	case uiactions.KindReview:
		event, err := reviewEvent(intent.Prompt.Value)
		if err != nil {
			return "", err
		}
		if _, err := r.client.SubmitPullReview(owner, repo, index, data.PullReviewOptions{Event: event}); err != nil {
			return "", err
		}
		return fmt.Sprintf("Submitted review for %s#%d.", intent.Target.Repo, index), nil

	case uiactions.KindExternalDiff:
		diff, err := r.client.GetPullDiff(owner, repo, index)
		if err != nil {
			return "", err
		}
		command := r.cfg.Pager.DiffCommand()
		cmd := shell.BuildCommand(command, diff, r.cwd)
		out, err := r.shellRunner.Run(ctx, cmd)
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return "", fmt.Errorf("run diff pager %q: %w: %s", command, err, msg)
			}
			return "", fmt.Errorf("run diff pager %q: %w", command, err)
		}
		return fmt.Sprintf("Viewed diff for %s#%d.", intent.Target.Repo, index), nil

	case uiactions.KindCheckout:
		plan, err := r.checkout(ctx, localgit.CheckoutOptions{
			RepoName:       intent.Target.Repo,
			CWD:            r.cwd,
			InstanceURL:    r.instanceURL,
			RepoPaths:      r.cfg.RepoPaths,
			Remote:         r.cfg.Git.Remote,
			BranchTemplate: r.cfg.Git.PRBranchTemplate,
			PrIndex:        index,
			Runner:         r.gitRunner,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Checked out %s in %s.", plan.Branch, plan.RepoPath), nil

	case uiactions.KindRerunRun:
		runID := targetRunID(intent.Target)
		if err := r.client.RerunActionRun(ctx, owner, repo, runID); err != nil {
			return "", err
		}
		return fmt.Sprintf("Rerun requested for %s run #%d.", intent.Target.Repo, intent.Target.Number), nil

	case uiactions.KindCancelRun:
		runID := targetRunID(intent.Target)
		if err := r.client.CancelActionRun(ctx, owner, repo, runID); err != nil {
			return "", err
		}
		return fmt.Sprintf("Cancel requested for %s run #%d.", intent.Target.Repo, intent.Target.Number), nil
	default:
		return "", fmt.Errorf("unsupported action %q", intent.Kind)
	}
}

func targetRunID(target uiactions.Target) int64 {
	if target.RunID != 0 {
		return target.RunID
	}
	return target.Number
}

func (r Runner) setState(owner, repo string, index int64, kind uiactions.RowKind, state data.ItemState) error {
	switch kind {
	case uiactions.RowKindPullRequest:
		return r.client.SetPullState(owner, repo, index, state)
	case uiactions.RowKindIssue:
		return r.client.SetIssueState(owner, repo, index, state)
	default:
		return fmt.Errorf("unsupported row kind %q", kind)
	}
}

func splitRepo(repoName string) (string, string, error) {
	owner, repo, ok := data.SplitOwnerRepo(repoName)
	if !ok {
		return "", "", fmt.Errorf("invalid repository %q", repoName)
	}
	return owner, repo, nil
}

func reviewEvent(value string) (data.PullReviewEvent, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "comment":
		return data.PullReviewEventComment, nil
	case "approve", "approved":
		return data.PullReviewEventApprove, nil
	case "request_changes", "request-changes", "changes_requested":
		return data.PullReviewEventRequestChanges, nil
	default:
		return "", fmt.Errorf("unsupported review action %q", value)
	}
}

func mergeStyle(value string) (data.MergeStyle, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", string(data.MergeStyleMerge), "confirm":
		return data.MergeStyleMerge, nil
	case string(data.MergeStyleSquash):
		return data.MergeStyleSquash, nil
	case string(data.MergeStyleRebase):
		return data.MergeStyleRebase, nil
	case string(data.MergeStyleRebaseMerge):
		return data.MergeStyleRebaseMerge, nil
	case string(data.MergeStyleFastForwardOnly), "ff-only":
		return data.MergeStyleFastForwardOnly, nil
	default:
		return "", fmt.Errorf("unsupported merge strategy %q", value)
	}
}

func actionLabel(kind uiactions.Kind) string {
	switch kind {
	case uiactions.KindComment:
		return "comment"
	case uiactions.KindMerge:
		return "merge"
	case uiactions.KindClose:
		return "close"
	case uiactions.KindReopen:
		return "reopen"
	case uiactions.KindReview:
		return "review"
	case uiactions.KindExternalDiff:
		return "external diff"
	case uiactions.KindCheckout:
		return "checkout"
	case uiactions.KindSwitchBranch:
		return "switch branch"
	case uiactions.KindRerunRun:
		return "rerun"
	case uiactions.KindCancelRun:
		return "cancel run"
	default:
		return string(kind)
	}
}
