package gitea

import (
	"fmt"

	sdk "code.gitea.io/sdk/gitea"

	"github.com/gbarany/tea-dash/internal/data"
)

// GetPullDetail fetches the read-only detail view of a single pull request.
//
// The primary payload is the PR itself (body/base/head/stats); its fetch error
// is fatal. Comments, reviews, and CI are secondary decorations fetched
// best-effort: each runs as its own c.call, and a failure on any one of them
// leaves the corresponding slice/struct empty rather than failing the whole
// detail. Sub-fetch errors are intentionally swallowed here — the detail view
// must still render when, e.g., the combined-status endpoint is disabled on the
// server or a review list momentarily 500s. Every typed SDK call goes through
// c.call, the wrapper point for SDK calls.
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
	d := mapPullDetail(pr)

	// Best-effort comments.
	var comments []*sdk.Comment
	if e := c.call(func() error {
		var ce error
		comments, _, ce = c.sdk.ListIssueComments(owner, repo, index, sdk.ListIssueCommentOptions{})
		return ce
	}); e == nil {
		d.Comments = mapComments(comments)
	}

	// Best-effort reviews.
	var reviews []*sdk.PullReview
	if e := c.call(func() error {
		var re error
		reviews, _, re = c.sdk.ListPullReviews(owner, repo, index, sdk.ListPullReviewsOptions{})
		return re
	}); e == nil {
		d.Reviews = mapReviews(reviews)
	}

	// Best-effort CI: only meaningful when we actually have a head SHA to query.
	if pr != nil && pr.Head != nil && pr.Head.Sha != "" {
		var cs *sdk.CombinedStatus
		if e := c.call(func() error {
			var se error
			cs, _, se = c.sdk.GetCombinedStatus(owner, repo, pr.Head.Sha)
			return se
		}); e == nil {
			d.CI = mapCombinedStatus(cs)
		}
	}

	return d, nil
}

// GetIssueDetail fetches the read-only detail view of a single issue. The issue
// fetch is primary and fatal on error; comments are fetched best-effort with
// the same rationale as GetPullDetail — a failed comment fetch leaves Comments
// empty rather than failing the issue detail. Both SDK calls run through c.call.
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
	d := mapIssueDetail(iss)

	// Best-effort comments.
	var comments []*sdk.Comment
	if e := c.call(func() error {
		var ce error
		comments, _, ce = c.sdk.ListIssueComments(owner, repo, index, sdk.ListIssueCommentOptions{})
		return ce
	}); e == nil {
		d.Comments = mapComments(comments)
	}

	return d, nil
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

// mapComments maps SDK issue/PR comments onto the domain Comment type. Pure and
// nil-safe: nil slice entries are skipped, and a nil Poster maps to an empty
// author. Note the SDK's User.UserName field carries the login (json:"login").
func mapComments(cs []*sdk.Comment) []data.Comment {
	if len(cs) == 0 {
		return nil
	}
	out := make([]data.Comment, 0, len(cs))
	for _, c := range cs {
		if c == nil {
			continue
		}
		cm := data.Comment{
			Body:      c.Body,
			CreatedAt: c.Created,
		}
		if c.Poster != nil {
			cm.Author = c.Poster.UserName
		}
		out = append(out, cm)
	}
	return out
}

// mapReviews maps SDK pull reviews onto the domain Review type, dropping draft
// reviews whose state is empty (ReviewStateUnknown) or PENDING — those are not
// yet submitted and would otherwise render as blank entries. APPROVED /
// REQUEST_CHANGES / COMMENT and any other concrete state are kept. Pure and
// nil-safe: nil entries are skipped and a nil Reviewer maps to an empty author.
func mapReviews(rs []*sdk.PullReview) []data.Review {
	if len(rs) == 0 {
		return nil
	}
	out := make([]data.Review, 0, len(rs))
	for _, r := range rs {
		if r == nil {
			continue
		}
		if r.State == sdk.ReviewStateUnknown || r.State == sdk.ReviewStatePending {
			continue
		}
		rv := data.Review{
			State:       data.ReviewState(r.State),
			Body:        r.Body,
			SubmittedAt: r.Submitted,
		}
		if r.Reviewer != nil {
			rv.Author = r.Reviewer.UserName
		}
		out = append(out, rv)
	}
	return out
}

// mapCombinedStatus maps the SDK combined commit status onto the domain
// CIStatus. Pure and nil-safe: a nil status maps to the zero CIStatus, and nil
// per-status entries are skipped. Each check's TargetURL prefers the status's
// TargetURL and falls back to its API URL when TargetURL is empty.
func mapCombinedStatus(cs *sdk.CombinedStatus) data.CIStatus {
	if cs == nil {
		return data.CIStatus{}
	}
	ci := data.CIStatus{
		State: data.CIState(cs.State),
		SHA:   cs.SHA,
		Total: cs.TotalCount,
	}
	if len(cs.Statuses) > 0 {
		ci.Checks = make([]data.Check, 0, len(cs.Statuses))
		for _, s := range cs.Statuses {
			if s == nil {
				continue
			}
			targetURL := s.TargetURL
			if targetURL == "" {
				targetURL = s.URL
			}
			ci.Checks = append(ci.Checks, data.Check{
				Context:     s.Context,
				State:       data.CheckState(s.State),
				Description: s.Description,
				TargetURL:   targetURL,
			})
		}
	}
	return ci
}
