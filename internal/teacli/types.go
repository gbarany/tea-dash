package teacli

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// User is a subset of a Gitea user object.
type User struct {
	Login    string `json:"login"`
	FullName string `json:"full_name"`
}

// Label is a subset of a Gitea label object.
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// PullRequest is a subset of the Gitea pull-request object returned by
// GET /repos/{owner}/{repo}/pulls.
type PullRequest struct {
	ID        int64     `json:"id"`
	Number    int64     `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Draft     bool      `json:"draft"`
	Merged    bool      `json:"merged"`
	HTMLURL   string    `json:"html_url"`
	Poster    *User     `json:"user"`
	Labels    []Label   `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RepoPullsEndpoint builds the `tea api` endpoint for listing a repository's
// pull requests. state is one of "open", "closed" or "all"; empty means open.
func RepoPullsEndpoint(owner, repo, state string) string {
	if state == "" {
		state = "open"
	}
	q := url.Values{}
	q.Set("state", state)
	return fmt.Sprintf("/repos/%s/%s/pulls?%s",
		url.PathEscape(owner), url.PathEscape(repo), q.Encode())
}

// ListRepoPulls returns the pull requests of owner/repo in the given state.
func (c *Client) ListRepoPulls(ctx context.Context, owner, repo, state string) ([]PullRequest, error) {
	var prs []PullRequest
	if err := c.API(ctx, RepoPullsEndpoint(owner, repo, state), &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

// ListCurrentRepoPulls lists pull requests for the repository in $PWD, relying
// on tea's {owner}/{repo} placeholder expansion from the local git remote.
func (c *Client) ListCurrentRepoPulls(ctx context.Context, state string) ([]PullRequest, error) {
	if state == "" {
		state = "open"
	}
	var prs []PullRequest
	if err := c.API(ctx, "/repos/{owner}/{repo}/pulls?state="+state, &prs); err != nil {
		return nil, err
	}
	return prs, nil
}
