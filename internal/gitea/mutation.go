package gitea

import (
	"fmt"
	"strings"

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

// AddLabels adds repository labels, by exact name, to an issue or pull request.
func (c *Client) AddLabels(owner, repo string, index int64, names []string) error {
	ids, err := c.resolveLabelIDs(owner, repo, names)
	if err != nil {
		return err
	}
	err = c.call(func() error {
		_, _, e := c.sdk.AddIssueLabels(owner, repo, index, sdk.IssueLabelsOption{Labels: ids})
		return e
	})
	if err != nil {
		return fmt.Errorf("add labels to %s/%s#%d: %w", owner, repo, index, err)
	}
	return nil
}

// RemoveLabels removes repository labels, by exact name, from an issue or pull
// request. Gitea models PR labels through the issue-label API.
func (c *Client) RemoveLabels(owner, repo string, index int64, names []string) error {
	ids, err := c.resolveLabelIDs(owner, repo, names)
	if err != nil {
		return err
	}
	for _, id := range ids {
		labelID := id
		err = c.call(func() error {
			_, e := c.sdk.DeleteIssueLabel(owner, repo, index, labelID)
			return e
		})
		if err != nil {
			return fmt.Errorf("remove label %d from %s/%s#%d: %w", labelID, owner, repo, index, err)
		}
	}
	return nil
}

func (c *Client) resolveLabelIDs(owner, repo string, names []string) ([]int64, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("label names cannot be empty")
	}
	var labels []*sdk.Label
	err := c.call(func() error {
		var e error
		labels, _, e = c.sdk.ListRepoLabels(owner, repo, sdk.ListLabelsOptions{
			ListOptions: sdk.ListOptions{Page: -1},
		})
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("list labels for %s/%s: %w", owner, repo, err)
	}
	byName := make(map[string]int64, len(labels))
	for _, label := range labels {
		if label != nil {
			byName[label.Name] = label.ID
		}
	}
	ids := make([]int64, 0, len(names))
	for _, name := range names {
		id, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown label %q in %s/%s", name, owner, repo)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// SetIssueMilestone sets an issue milestone by exact milestone title.
func (c *Client) SetIssueMilestone(owner, repo string, index int64, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("milestone title cannot be empty")
	}

	var milestones []*sdk.Milestone
	err := c.call(func() error {
		var e error
		milestones, _, e = c.sdk.ListRepoMilestones(owner, repo, sdk.ListMilestoneOption{
			ListOptions: sdk.ListOptions{Page: -1},
			State:       sdk.StateAll,
			Name:        title,
		})
		return e
	})
	if err != nil {
		return fmt.Errorf("list milestones for %s/%s: %w", owner, repo, err)
	}

	var id int64
	for _, milestone := range milestones {
		if milestone != nil && milestone.Title == title {
			id = milestone.ID
			break
		}
	}
	if id == 0 {
		return fmt.Errorf("unknown milestone %q in %s/%s", title, owner, repo)
	}

	err = c.call(func() error {
		_, _, e := c.sdk.EditIssue(owner, repo, index, sdk.EditIssueOption{Milestone: &id})
		return e
	})
	if err != nil {
		return fmt.Errorf("set milestone %q on %s/%s#%d: %w", title, owner, repo, index, err)
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
			MergeWhenChecksSucceed: opt.AutoMerge,
		})
		return e
	})
	if err != nil {
		return false, fmt.Errorf("merge pull %s/%s#%d: %w", owner, repo, index, err)
	}
	return merged, nil
}

// UpdatePullRequest asks the server to update a PR branch with commits from its
// base branch.
func (c *Client) UpdatePullRequest(owner, repo string, index int64) error {
	err := c.call(func() error {
		_, e := c.sdk.UpdatePullRequest(owner, repo, index)
		return e
	})
	if err != nil {
		return fmt.Errorf("update pull request %s/%s#%d: %w", owner, repo, index, err)
	}
	return nil
}

// MarkPullReady removes Gitea/Forgejo's work-in-progress title prefix. The
// pinned SDK exposes no draft edit field, and Gitea documents WIP prefixes as
// the portable way to mark a pull request as not ready.
func (c *Client) MarkPullReady(owner, repo string, index int64) (bool, error) {
	return c.setPullDraft(owner, repo, index, false)
}

// MarkPullDraft adds Gitea/Forgejo's default work-in-progress title prefix.
func (c *Client) MarkPullDraft(owner, repo string, index int64) (bool, error) {
	return c.setPullDraft(owner, repo, index, true)
}

func (c *Client) setPullDraft(owner, repo string, index int64, draft bool) (bool, error) {
	var current *sdk.PullRequest
	err := c.call(func() error {
		var e error
		current, _, e = c.sdk.GetPullRequest(owner, repo, index)
		return e
	})
	if err != nil {
		return false, fmt.Errorf("get pull title %s/%s#%d: %w", owner, repo, index, err)
	}
	if current == nil {
		return false, fmt.Errorf("get pull title %s/%s#%d: empty response", owner, repo, index)
	}

	title, changed := draftTitle(current.Title, draft)
	if !changed {
		return false, nil
	}
	err = c.call(func() error {
		_, _, e := c.sdk.EditPullRequest(owner, repo, index, sdk.EditPullRequestOption{Title: title})
		return e
	})
	if err != nil {
		action := "mark pull ready"
		if draft {
			action = "mark pull draft"
		}
		return false, fmt.Errorf("%s %s/%s#%d: %w", action, owner, repo, index, err)
	}
	return true, nil
}

func draftTitle(title string, draft bool) (string, bool) {
	readyTitle, hadPrefix := stripWIPPrefix(title)
	if draft {
		if hadPrefix {
			return title, false
		}
		return "WIP: " + title, true
	}
	if !hadPrefix {
		return title, false
	}
	return readyTitle, true
}

func stripWIPPrefix(title string) (string, bool) {
	trimmedLeft := strings.TrimLeft(title, " \t")
	lower := strings.ToLower(trimmedLeft)
	for _, prefix := range []string{"wip:", "[wip]"} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimLeft(trimmedLeft[len(prefix):], " \t"), true
		}
	}
	return title, false
}

// SubscribeIssue subscribes the authenticated user to an issue.
func (c *Client) SubscribeIssue(owner, repo string, index int64) error {
	err := c.call(func() error {
		_, e := c.sdk.IssueSubscribe(owner, repo, index)
		return e
	})
	if err != nil {
		return fmt.Errorf("subscribe issue %s/%s#%d: %w", owner, repo, index, err)
	}
	return nil
}

// UnsubscribeIssue unsubscribes the authenticated user from an issue.
func (c *Client) UnsubscribeIssue(owner, repo string, index int64) error {
	err := c.call(func() error {
		_, e := c.sdk.IssueUnSubscribe(owner, repo, index)
		return e
	})
	if err != nil {
		return fmt.Errorf("unsubscribe issue %s/%s#%d: %w", owner, repo, index, err)
	}
	return nil
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

// RequestPullReviewers asks Gitea to request review from the given usernames.
func (c *Client) RequestPullReviewers(owner, repo string, index int64, reviewers []string) error {
	if len(reviewers) == 0 {
		return fmt.Errorf("reviewer usernames cannot be empty")
	}
	err := c.call(func() error {
		_, e := c.sdk.CreateReviewRequests(owner, repo, index, sdk.PullReviewRequestOptions{
			Reviewers: reviewers,
		})
		return e
	})
	if err != nil {
		return fmt.Errorf("request reviewers on %s/%s#%d: %w", owner, repo, index, err)
	}
	return nil
}

// RemovePullReviewers removes review requests for the given usernames.
func (c *Client) RemovePullReviewers(owner, repo string, index int64, reviewers []string) error {
	if len(reviewers) == 0 {
		return fmt.Errorf("reviewer usernames cannot be empty")
	}
	err := c.call(func() error {
		_, e := c.sdk.DeleteReviewRequests(owner, repo, index, sdk.PullReviewRequestOptions{
			Reviewers: reviewers,
		})
		return e
	})
	if err != nil {
		return fmt.Errorf("remove reviewers on %s/%s#%d: %w", owner, repo, index, err)
	}
	return nil
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
