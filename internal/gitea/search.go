package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
)

// searchIssue is a tolerant decode of a row from GET /repos/issues/search.
// Unknown fields are ignored. Pull requests carry a non-nil "pull_request".
type searchIssue struct {
	Number  int64  `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	User    *struct {
		Login    string `json:"login"`
		FullName string `json:"full_name"`
	} `json:"user"`
	Labels []struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"labels"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Repository *struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest *struct {
		Merged bool `json:"merged"`
		Draft  bool `json:"draft"`
	} `json:"pull_request"`
}

// SearchMyPulls returns the authenticated user's pull requests (authored by
// them) across all accessible repos, in the given state ("open"/"closed"/"all"),
// plus the server's total count (from the X-Total-Count header). It uses the
// cross-repo search endpoint with the me-scoping boolean created=true. The page
// is capped at limit=50 (full pagination is deferred), so total may exceed the
// number of returned PRs.
func (c *Client) SearchMyPulls(ctx context.Context, state string) ([]data.PullRequest, int, error) {
	if state == "" {
		state = "open"
	}
	q := url.Values{}
	q.Set("type", "pulls")
	q.Set("created", "true")
	q.Set("state", state)
	q.Set("limit", "50")

	var rows []searchIssue
	header, err := c.rawGet(ctx, "/repos/issues/search?"+q.Encode(), &rows)
	if err != nil {
		return nil, 0, err
	}

	prs := make([]data.PullRequest, 0, len(rows))
	for _, it := range rows {
		prs = append(prs, mapSearchIssue(it))
	}

	total := len(prs)
	if v := header.Get("X-Total-Count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			total = n
		}
	}
	return prs, total, nil
}

// SearchIssues is the issues counterpart of the pull-request search. This is a
// stub kept so components depending on it compile during parallel M1b work; the
// real implementation (structured filter -> query params) lands in the M1b
// search-layer change.
func (c *Client) SearchIssues(ctx context.Context, f config.PrIssueFilter) ([]data.Issue, int, error) {
	_ = ctx
	_ = f
	return nil, 0, nil
}

func mapSearchIssue(it searchIssue) data.PullRequest {
	pr := data.PullRequest{
		Number:    it.Number,
		Title:     it.Title,
		State:     it.State,
		HTMLURL:   it.HTMLURL,
		CreatedAt: it.CreatedAt,
		UpdatedAt: it.UpdatedAt,
	}
	if it.User != nil {
		pr.Author = it.User.Login
	}
	if it.Repository != nil {
		pr.RepoNameWithOwner = it.Repository.FullName
	}
	if it.PullRequest != nil {
		pr.Draft = it.PullRequest.Draft
		if it.PullRequest.Merged {
			pr.State = "merged"
		}
	}
	for _, l := range it.Labels {
		pr.Labels = append(pr.Labels, data.Label{Name: l.Name, Color: l.Color})
	}
	return pr
}

// rawGet issues an authenticated GET against {baseURL}/api/v1{path} using the
// shared HTTP client and token, decoding the JSON body into out. It returns the
// response headers (which survive the body close) so callers can read metadata
// such as X-Total-Count.
func (c *Client) rawGet(ctx context.Context, path string, out any) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitea GET %s: %s: %s", path, resp.Status, truncate(body, 500))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, fmt.Errorf("decoding gitea GET %s: %w", path, err)
	}
	return resp.Header, nil
}

// truncate returns b as a string, clipped to max bytes with an ellipsis marker
// appended when it was longer, so an HTML error page cannot flood the TUI.
func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
