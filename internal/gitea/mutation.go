package gitea

import (
	"fmt"

	sdk "code.gitea.io/sdk/gitea"

	"github.com/gbarany/tea-dash/internal/data"
)

// AddComment creates an issue/PR comment and maps the returned comment into the
// domain shape used by detail views.
func (c *Client) AddComment(owner, repo string, index int64, body string) (data.Comment, error) {
	var comment *sdk.Comment
	err := c.call(func() error {
		var e error
		comment, _, e = c.sdk.CreateIssueComment(owner, repo, index, sdk.CreateIssueCommentOption{
			Body: body,
		})
		return e
	})
	if err != nil {
		return data.Comment{}, fmt.Errorf("add comment %s/%s#%d: %w", owner, repo, index, err)
	}
	mapped := mapComments([]*sdk.Comment{comment})
	if len(mapped) == 0 {
		return data.Comment{}, nil
	}
	return mapped[0], nil
}

// SetIssueState opens or closes an issue.
func (c *Client) SetIssueState(owner, repo string, index int64, state data.ItemState) error {
	s := sdk.StateType(state)
	err := c.call(func() error {
		_, _, e := c.sdk.EditIssue(owner, repo, index, sdk.EditIssueOption{State: &s})
		return e
	})
	if err != nil {
		return fmt.Errorf("set issue state %s/%s#%d to %s: %w", owner, repo, index, state, err)
	}
	return nil
}

// SetPullState opens or closes a pull request.
func (c *Client) SetPullState(owner, repo string, index int64, state data.ItemState) error {
	s := sdk.StateType(state)
	err := c.call(func() error {
		_, _, e := c.sdk.EditPullRequest(owner, repo, index, sdk.EditPullRequestOption{State: &s})
		return e
	})
	if err != nil {
		return fmt.Errorf("set pull state %s/%s#%d to %s: %w", owner, repo, index, state, err)
	}
	return nil
}

// AssignIssueToMe assigns the authenticated user to an issue while preserving
// existing assignees.
func (c *Client) AssignIssueToMe(owner, repo string, index int64) error {
	return c.setIssueAssignee(owner, repo, index, true)
}

// UnassignIssueFromMe removes the authenticated user from an issue while
// preserving every other assignee.
func (c *Client) UnassignIssueFromMe(owner, repo string, index int64) error {
	return c.setIssueAssignee(owner, repo, index, false)
}

func (c *Client) setIssueAssignee(owner, repo string, index int64, assign bool) error {
	var current *sdk.Issue
	err := c.call(func() error {
		var e error
		current, _, e = c.sdk.GetIssue(owner, repo, index)
		return e
	})
	if err != nil {
		return fmt.Errorf("get issue assignees %s/%s#%d: %w", owner, repo, index, err)
	}
	assignees := updateAssigneeLogins(issueAssigneeLogins(current), c.me, assign)
	err = c.call(func() error {
		_, _, e := c.sdk.EditIssue(owner, repo, index, sdk.EditIssueOption{Assignees: assignees})
		return e
	})
	if err != nil {
		action := "assign"
		if !assign {
			action = "unassign"
		}
		return fmt.Errorf("%s issue %s/%s#%d to %s: %w", action, owner, repo, index, c.me, err)
	}
	return nil
}

// AssignPullToMe assigns the authenticated user to a pull request while
// preserving existing assignees.
func (c *Client) AssignPullToMe(owner, repo string, index int64) error {
	return c.setPullAssignee(owner, repo, index, true)
}

// UnassignPullFromMe removes the authenticated user from a pull request while
// preserving every other assignee.
func (c *Client) UnassignPullFromMe(owner, repo string, index int64) error {
	return c.setPullAssignee(owner, repo, index, false)
}

func (c *Client) setPullAssignee(owner, repo string, index int64, assign bool) error {
	var current *sdk.PullRequest
	err := c.call(func() error {
		var e error
		current, _, e = c.sdk.GetPullRequest(owner, repo, index)
		return e
	})
	if err != nil {
		return fmt.Errorf("get pull assignees %s/%s#%d: %w", owner, repo, index, err)
	}
	assignees := updateAssigneeLogins(pullAssigneeLogins(current), c.me, assign)
	err = c.call(func() error {
		_, _, e := c.sdk.EditPullRequest(owner, repo, index, sdk.EditPullRequestOption{Assignees: assignees})
		return e
	})
	if err != nil {
		action := "assign"
		if !assign {
			action = "unassign"
		}
		return fmt.Errorf("%s pull %s/%s#%d to %s: %w", action, owner, repo, index, c.me, err)
	}
	return nil
}

func issueAssigneeLogins(issue *sdk.Issue) []string {
	if issue == nil {
		return nil
	}
	out := make([]string, 0, len(issue.Assignees))
	for _, user := range issue.Assignees {
		if user != nil && user.UserName != "" {
			out = append(out, user.UserName)
		}
	}
	return out
}

func pullAssigneeLogins(pr *sdk.PullRequest) []string {
	if pr == nil {
		return nil
	}
	out := make([]string, 0, len(pr.Assignees))
	for _, user := range pr.Assignees {
		if user != nil && user.UserName != "" {
			out = append(out, user.UserName)
		}
	}
	return out
}

func updateAssigneeLogins(current []string, me string, assign bool) []string {
	if me == "" {
		return current
	}
	seen := make(map[string]bool, len(current)+1)
	out := make([]string, 0, len(current)+1)
	for _, login := range current {
		if login == "" || seen[login] {
			continue
		}
		seen[login] = true
		if !assign && login == me {
			continue
		}
		out = append(out, login)
	}
	if assign && !seen[me] {
		out = append(out, me)
	}
	return out
}

// MergePullRequest merges a pull request with the requested strategy and
// optional server-side controls.
func (c *Client) MergePullRequest(owner, repo string, index int64, opt data.MergeOptions) (bool, error) {
	deleteBranch := opt.DeleteBranch
	var merged bool
	err := c.call(func() error {
		var e error
		merged, _, e = c.sdk.MergePullRequest(owner, repo, index, sdk.MergePullRequestOption{
			Style:                  sdk.MergeStyle(opt.Style),
			Title:                  opt.Title,
			Message:                opt.Message,
			DeleteBranchAfterMerge: &deleteBranch,
			ForceMerge:             opt.ForceMerge,
			HeadCommitId:           opt.HeadCommitID,
		})
		return e
	})
	if err != nil {
		return false, fmt.Errorf("merge pull %s/%s#%d: %w", owner, repo, index, err)
	}
	return merged, nil
}

// SubmitPullReview creates a submitted pull-request review.
func (c *Client) SubmitPullReview(owner, repo string, index int64, opt data.PullReviewOptions) (data.Review, error) {
	event, err := mapPullReviewEvent(opt.Event)
	if err != nil {
		return data.Review{}, fmt.Errorf("submit pull review %s/%s#%d: %w", owner, repo, index, err)
	}

	var review *sdk.PullReview
	err = c.call(func() error {
		var e error
		review, _, e = c.sdk.CreatePullReview(owner, repo, index, sdk.CreatePullReviewOptions{
			State: event,
			Body:  opt.Body,
		})
		return e
	})
	if err != nil {
		return data.Review{}, fmt.Errorf("submit pull review %s/%s#%d: %w", owner, repo, index, err)
	}
	mapped := mapReviews([]*sdk.PullReview{review})
	if len(mapped) == 0 {
		return data.Review{}, nil
	}
	return mapped[0], nil
}

func mapPullReviewEvent(event data.PullReviewEvent) (sdk.ReviewStateType, error) {
	switch event {
	case data.PullReviewEventApprove:
		return sdk.ReviewStateApproved, nil
	case data.PullReviewEventRequestChanges:
		return sdk.ReviewStateRequestChanges, nil
	case data.PullReviewEventComment:
		return sdk.ReviewStateComment, nil
	default:
		return "", fmt.Errorf("unsupported pull review event %q", event)
	}
}
