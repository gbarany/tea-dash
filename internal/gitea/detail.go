package gitea

import (
	"fmt"

	sdk "code.gitea.io/sdk/gitea"

	"github.com/gbarany/tea-dash/internal/data"
)

// GetPullDetail fetches the read-only detail view of a single pull request.
// The typed SDK call runs under c.call so it cannot race the globally-pinned
// SDK context. Comments/Reviews/CI are populated by a later sub-plan.
func (c *Client) GetPullDetail(owner, repo string, index int64) (data.PullDetail, error) {
	var pr *sdk.PullRequest
	err := c.call(func() error {
		var e error
		pr, _, e = c.sdk.GetPullRequest(owner, repo, index)
		return e
	})
	if err != nil {
		return data.PullDetail{}, fmt.Errorf("get pull %s/%s#%d: %w", owner, repo, index, err)
	}
	return mapPullDetail(pr), nil
}

// GetIssueDetail fetches the read-only detail view of a single issue. The SDK
// call runs under c.call. Comments are populated by a later sub-plan.
func (c *Client) GetIssueDetail(owner, repo string, index int64) (data.IssueDetail, error) {
	var iss *sdk.Issue
	err := c.call(func() error {
		var e error
		iss, _, e = c.sdk.GetIssue(owner, repo, index)
		return e
	})
	if err != nil {
		return data.IssueDetail{}, fmt.Errorf("get issue %s/%s#%d: %w", owner, repo, index, err)
	}
	return mapIssueDetail(iss), nil
}

// mapPullDetail is the pure mapping from the SDK pull request onto the domain
// detail type. It is safe against nil input and against the SDK's optional
// *int stat fields, which are only present when the PR was fetched with diff
// stats and are otherwise nil. Kept separate from the transport so it can be
// unit-tested without a live server.
func mapPullDetail(pr *sdk.PullRequest) data.PullDetail {
	if pr == nil {
		return data.PullDetail{}
	}
	d := data.PullDetail{
		Body:      pr.Body,
		Mergeable: pr.Mergeable,
		Merged:    pr.HasMerged,
	}
	if pr.Base != nil {
		d.BaseRef = pr.Base.Ref
	}
	if pr.Head != nil {
		d.HeadRef = pr.Head.Ref
		d.HeadSHA = pr.Head.Sha
	}
	if pr.Additions != nil {
		d.Additions = *pr.Additions
	}
	if pr.Deletions != nil {
		d.Deletions = *pr.Deletions
	}
	if pr.ChangedFiles != nil {
		d.ChangedFiles = *pr.ChangedFiles
	}
	return d
}

// mapIssueDetail is the pure mapping from the SDK issue onto the domain detail
// type. Safe against nil input.
func mapIssueDetail(iss *sdk.Issue) data.IssueDetail {
	if iss == nil {
		return data.IssueDetail{}
	}
	return data.IssueDetail{Body: iss.Body}
}
