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
