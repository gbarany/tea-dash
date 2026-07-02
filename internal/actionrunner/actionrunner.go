// Package actionrunner executes UI action intents against Gitea, local git, and
// configured shell commands.
package actionrunner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
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
	AssignPullToMe(owner, repo string, index int64) error
	UnassignPullFromMe(owner, repo string, index int64) error
	AssignIssueToMe(owner, repo string, index int64) error
	UnassignIssueFromMe(owner, repo string, index int64) error
	AddLabels(owner, repo string, index int64, names []string) error
	RemoveLabels(owner, repo string, index int64, names []string) error
	SetIssueMilestone(owner, repo string, index int64, title string) error
	MergePullRequest(owner, repo string, index int64, opt data.MergeOptions) (bool, error)
	UpdatePullRequest(owner, repo string, index int64) error
	MarkPullReady(owner, repo string, index int64) (bool, error)
	MarkPullDraft(owner, repo string, index int64) (bool, error)
	SubmitPullReview(owner, repo string, index int64, opt data.PullReviewOptions) (data.Review, error)
	GetPullDiff(owner, repo string, index int64) ([]byte, error)
	RerunActionRun(ctx context.Context, owner, repo string, runID int64) error
	CancelActionRun(ctx context.Context, owner, repo string, runID int64) error
}

// CheckoutFunc runs or fakes a local PR checkout.
type CheckoutFunc func(context.Context, localgit.CheckoutOptions) (localgit.CheckoutPlan, error)

// IssueCheckoutFunc runs or fakes a local issue branch checkout.
type IssueCheckoutFunc func(context.Context, localgit.IssueCheckoutOptions) (localgit.IssueCheckoutPlan, error)

// BranchSwitchFunc runs or fakes a local branch switch.
type BranchSwitchFunc func(context.Context, localgit.SwitchBranchOptions) (localgit.SwitchBranchResult, error)

// ExecProcessFunc wraps Bubble Tea's ExecProcess for interactive shell
// commands. Tests replace it to avoid running a real process.
type ExecProcessFunc func(*exec.Cmd, tea.ExecCallback) tea.Cmd

// Options configures a Runner.
type Options struct {
	Client        Client
	Config        *config.Config
	InstanceURL   string
	CWD           string
	ShellRunner   shell.Runner
	GitRunner     localgit.Runner
	Checkout      CheckoutFunc
	IssueCheckout IssueCheckoutFunc
	BranchSwitch  BranchSwitchFunc
	ExecProcess   ExecProcessFunc
}

// Runner executes actions and returns ResultMsg values for the UI.
type Runner struct {
	client        Client
	cfg           *config.Config
	instanceURL   string
	cwd           string
	shellRunner   shell.Runner
	gitRunner     localgit.Runner
	checkout      CheckoutFunc
	issueCheckout IssueCheckoutFunc
	branchSwitch  BranchSwitchFunc
	execProcess   ExecProcessFunc
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
	issueCheckout := opts.IssueCheckout
	if issueCheckout == nil {
		issueCheckout = localgit.RunIssueCheckout
	}
	branchSwitch := opts.BranchSwitch
	if branchSwitch == nil {
		branchSwitch = localgit.SwitchBranch
	}
	execProcess := opts.ExecProcess
	if execProcess == nil {
		execProcess = tea.ExecProcess
	}
	return Runner{
		client:        opts.Client,
		cfg:           cfg,
		instanceURL:   opts.InstanceURL,
		cwd:           opts.CWD,
		shellRunner:   shellRunner,
		gitRunner:     opts.GitRunner,
		checkout:      checkout,
		issueCheckout: issueCheckout,
		branchSwitch:  branchSwitch,
		execProcess:   execProcess,
	}
}

// Dispatch returns a Bubble Tea command that executes intent off the update
// path and reports the result back as actions.ResultMsg.
func (r Runner) Dispatch(intent uiactions.Intent) tea.Cmd {
	if intent.Kind == uiactions.KindCustomCommand {
		return r.dispatchCustomCommand(intent)
	}
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

func (r Runner) dispatchCustomCommand(intent uiactions.Intent) tea.Cmd {
	command := strings.TrimSpace(intent.Command)
	if command == "" {
		return actionResult(intent, uiactions.ResultErrored, "", fmt.Errorf("custom command is empty"))
	}
	rendered, err := r.renderCustomCommand(command, intent.Target)
	if err != nil {
		return actionResult(intent, uiactions.ResultErrored, "", err)
	}
	dir := intent.Target.RepositoryPath
	if dir == "" {
		dir = r.cwd
	}
	cmd := shell.BuildExecCommand(rendered, nil, dir)
	name := strings.TrimSpace(intent.Name)
	if name == "" {
		name = "custom command"
	}
	return r.execProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return uiactions.ResultMsg{Intent: intent, Status: uiactions.ResultErrored, Err: fmt.Errorf("run custom command %q: %w", command, err)}
		}
		return uiactions.ResultMsg{Intent: intent, Status: uiactions.ResultSucceeded, Message: fmt.Sprintf("Ran %s for %s.", name, intent.Target.Title)}
	})
}

func actionResult(intent uiactions.Intent, status uiactions.ResultStatus, message string, err error) tea.Cmd {
	return func() tea.Msg {
		return uiactions.ResultMsg{Intent: intent, Status: status, Message: message, Err: err}
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

	case uiactions.KindAssign:
		if err := r.setAssignment(owner, repo, index, intent.Target.RowKind, true); err != nil {
			return "", err
		}
		return fmt.Sprintf("Assigned %s#%d to you.", intent.Target.Repo, index), nil

	case uiactions.KindUnassign:
		if err := r.setAssignment(owner, repo, index, intent.Target.RowKind, false); err != nil {
			return "", err
		}
		return fmt.Sprintf("Unassigned you from %s#%d.", intent.Target.Repo, index), nil

	case uiactions.KindAddLabel:
		names, err := parseLabelNames(intent.Prompt.Value)
		if err != nil {
			return "", err
		}
		if err := r.setLabels(owner, repo, index, intent.Target.RowKind, names, true); err != nil {
			return "", err
		}
		return fmt.Sprintf("Added labels to %s#%d.", intent.Target.Repo, index), nil

	case uiactions.KindRemoveLabel:
		names, err := parseLabelNames(intent.Prompt.Value)
		if err != nil {
			return "", err
		}
		if err := r.setLabels(owner, repo, index, intent.Target.RowKind, names, false); err != nil {
			return "", err
		}
		return fmt.Sprintf("Removed labels from %s#%d.", intent.Target.Repo, index), nil

	case uiactions.KindSetMilestone:
		title := strings.TrimSpace(intent.Prompt.Value)
		if title == "" {
			return "", fmt.Errorf("milestone title cannot be empty")
		}
		if intent.Target.RowKind != uiactions.RowKindIssue {
			return "", fmt.Errorf("set milestone is only available for issues")
		}
		if err := r.client.SetIssueMilestone(owner, repo, index, title); err != nil {
			return "", err
		}
		return fmt.Sprintf("Set milestone %q on %s#%d.", title, intent.Target.Repo, index), nil

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

	case uiactions.KindUpdateBranch:
		if err := r.client.UpdatePullRequest(owner, repo, index); err != nil {
			return "", err
		}
		return fmt.Sprintf("Updated %s#%d from its base branch.", intent.Target.Repo, index), nil

	case uiactions.KindMarkReady:
		changed, err := r.client.MarkPullReady(owner, repo, index)
		if err != nil {
			return "", err
		}
		if !changed {
			return fmt.Sprintf("%s#%d is already ready for review.", intent.Target.Repo, index), nil
		}
		return fmt.Sprintf("Marked %s#%d ready for review.", intent.Target.Repo, index), nil

	case uiactions.KindMarkDraft:
		changed, err := r.client.MarkPullDraft(owner, repo, index)
		if err != nil {
			return "", err
		}
		if !changed {
			return fmt.Sprintf("%s#%d is already draft.", intent.Target.Repo, index), nil
		}
		return fmt.Sprintf("Marked %s#%d draft.", intent.Target.Repo, index), nil

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
		switch intent.Target.RowKind {
		case uiactions.RowKindPullRequest:
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
		case uiactions.RowKindIssue:
			plan, err := r.issueCheckout(ctx, localgit.IssueCheckoutOptions{
				RepoName:       intent.Target.Repo,
				CWD:            r.cwd,
				InstanceURL:    r.instanceURL,
				RepoPaths:      r.cfg.RepoPaths,
				BranchTemplate: r.cfg.Git.IssueBranchTemplate,
				IssueIndex:     index,
				Runner:         r.gitRunner,
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Checked out issue branch %s in %s.", plan.Branch, plan.RepoPath), nil
		default:
			return "", fmt.Errorf("checkout is only available for pull requests and issues")
		}

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

type customCommandContext struct {
	RepoName    string
	RepoPath    string
	Number      int64
	PrIndex     int64
	PrNumber    int64
	IssueIndex  int64
	IssueNumber int64
	RunID       int64
	Title       string
	IssueTitle  string
	BranchName  string
	Author      string
	Sha         string
	SHA         string
	InstanceURL string
	URL         string
	Url         string
}

func (r Runner) renderCustomCommand(command string, target uiactions.Target) (string, error) {
	tmpl, err := template.New("custom-command").Option("missingkey=error").Parse(command)
	if err != nil {
		return "", fmt.Errorf("parse custom command template: %w", err)
	}
	url := target.URL
	ctx := customCommandContext{
		RepoName:    target.Repo,
		RepoPath:    target.RepositoryPath,
		Number:      target.Number,
		PrIndex:     target.Number,
		PrNumber:    target.Number,
		IssueIndex:  target.Number,
		IssueNumber: target.Number,
		RunID:       targetRunID(target),
		Title:       target.Title,
		IssueTitle:  target.Title,
		BranchName:  target.Title,
		Author:      target.Author,
		Sha:         target.SHA,
		SHA:         target.SHA,
		InstanceURL: r.instanceURL,
		URL:         url,
		Url:         url,
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("render custom command template: %w", err)
	}
	return buf.String(), nil
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

func (r Runner) setAssignment(owner, repo string, index int64, kind uiactions.RowKind, assign bool) error {
	switch kind {
	case uiactions.RowKindPullRequest:
		if assign {
			return r.client.AssignPullToMe(owner, repo, index)
		}
		return r.client.UnassignPullFromMe(owner, repo, index)
	case uiactions.RowKindIssue:
		if assign {
			return r.client.AssignIssueToMe(owner, repo, index)
		}
		return r.client.UnassignIssueFromMe(owner, repo, index)
	default:
		return fmt.Errorf("assign is only available for pull requests and issues")
	}
}

func parseLabelNames(input string) ([]string, error) {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("label names cannot be empty")
	}
	return out, nil
}

func (r Runner) setLabels(owner, repo string, index int64, kind uiactions.RowKind, names []string, add bool) error {
	switch kind {
	case uiactions.RowKindPullRequest, uiactions.RowKindIssue:
		if add {
			return r.client.AddLabels(owner, repo, index, names)
		}
		return r.client.RemoveLabels(owner, repo, index, names)
	default:
		return fmt.Errorf("labels are only available for pull requests and issues")
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
	case uiactions.KindAssign:
		return "assign"
	case uiactions.KindUnassign:
		return "unassign"
	case uiactions.KindAddLabel:
		return "add label"
	case uiactions.KindRemoveLabel:
		return "remove label"
	case uiactions.KindSetMilestone:
		return "set milestone"
	case uiactions.KindMerge:
		return "merge"
	case uiactions.KindUpdateBranch:
		return "update branch"
	case uiactions.KindMarkReady:
		return "mark ready"
	case uiactions.KindMarkDraft:
		return "mark draft"
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
