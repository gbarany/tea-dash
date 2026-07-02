package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// isMe reports whether s is the "@me" sentinel that maps to the search
// endpoint's me-scoping booleans (created/assigned/mentioned/review_requested).
func isMe(s string) bool { return s == "@me" }

// buildSearchParams renders a structured filter into the cross-repo
// /repos/issues/search query string. Me-scoped fields ("@me") become the
// endpoint's boolean flags (created=true etc.) rather than the per-repo
// created_by/assigned_by params, which the search endpoint ignores — this is
// the C1 guard. limit is clamped to a positive value (default 50).
func buildSearchParams(f config.PrIssueFilter, limit int) url.Values {
	return buildSearchParamsPage(f, limit, 0)
}

func buildSearchParamsPage(f config.PrIssueFilter, limit, page int) url.Values {
	q := url.Values{}
	q.Set("type", f.Type)
	q.Set("state", f.State)
	if f.Q != "" {
		q.Set("q", f.Q)
	}
	if len(f.Labels) > 0 {
		q.Set("labels", strings.Join(f.Labels, ","))
	}
	if f.Milestone != "" {
		q.Set("milestones", f.Milestone)
	}
	if isMe(f.CreatedBy) {
		q.Set("created", "true")
	}
	if isMe(f.AssignedBy) {
		q.Set("assigned", "true")
	}
	if isMe(f.Mentioned) {
		q.Set("mentioned", "true")
	}
	if isMe(f.ReviewRequested) {
		q.Set("review_requested", "true")
	}
	if f.Since != "" {
		q.Set("since", f.Since)
	}
	if f.Sort != "" {
		q.Set("sort", f.Sort)
	}
	if limit <= 0 {
		limit = 50
	}
	q.Set("limit", strconv.Itoa(limit))
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	return q
}

// search runs one page of the cross-repo /repos/issues/search endpoint for the
// given filter and returns the raw rows plus the server's total (from the
// X-Total-Count header, falling back to the returned row count). The page is
// capped at limit, so total may exceed len(rows) until callers request later
// pages.
func (c *Client) search(ctx context.Context, f config.PrIssueFilter, limit int) ([]searchIssue, int, error) {
	return c.searchPage(ctx, f, limit, 0)
}

func (c *Client) searchPage(ctx context.Context, f config.PrIssueFilter, limit, page int) ([]searchIssue, int, error) {
	var rows []searchIssue
	header, err := c.rawGet(ctx, "/repos/issues/search?"+buildSearchParamsPage(f, limit, page).Encode(), &rows)
	if err != nil {
		return nil, 0, err
	}
	total := len(rows)
	if v := header.Get("X-Total-Count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			total = n
		}
	}
	return rows, total, nil
}

// SearchPulls returns the pull requests matching f across all accessible repos,
// plus the server's total count. f.Type is forced to "pulls" and the "open"
// state default is applied. limit caps the page (0 -> buildSearchParams' 50).
func (c *Client) SearchPulls(ctx context.Context, f config.PrIssueFilter, limit int) ([]data.PullRequest, int, error) {
	return c.SearchPullsPage(ctx, f, limit, 0)
}

func (c *Client) SearchPullsPage(ctx context.Context, f config.PrIssueFilter, limit, page int) ([]data.PullRequest, int, error) {
	f = f.WithDefaults("pulls")
	rows, total, err := c.searchPage(ctx, f, limit, page)
	if err != nil {
		return nil, 0, err
	}
	prs := make([]data.PullRequest, 0, len(rows))
	for _, it := range rows {
		prs = append(prs, mapSearchIssue(it))
	}
	return prs, total, nil
}

// SearchIssues returns the issues matching f across all accessible repos, plus
// the server's total count. f.Type is forced to "issues" and the "open" state
// default is applied. limit caps the page (0 -> buildSearchParams' 50).
func (c *Client) SearchIssues(ctx context.Context, f config.PrIssueFilter, limit int) ([]data.Issue, int, error) {
	return c.SearchIssuesPage(ctx, f, limit, 0)
}

func (c *Client) SearchIssuesPage(ctx context.Context, f config.PrIssueFilter, limit, page int) ([]data.Issue, int, error) {
	f = f.WithDefaults("issues")
	rows, total, err := c.searchPage(ctx, f, limit, page)
	if err != nil {
		return nil, 0, err
	}
	issues := make([]data.Issue, 0, len(rows))
	for _, it := range rows {
		issues = append(issues, mapSearchIssueToIssue(it))
	}
	return issues, total, nil
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

// mapSearchIssueToIssue maps a search row into the issue domain type. Unlike
// mapSearchIssue it carries no Draft/Merged; State is used as returned.
func mapSearchIssueToIssue(it searchIssue) data.Issue {
	issue := data.Issue{
		Number:    it.Number,
		Title:     it.Title,
		State:     it.State,
		HTMLURL:   it.HTMLURL,
		CreatedAt: it.CreatedAt,
		UpdatedAt: it.UpdatedAt,
	}
	if it.User != nil {
		issue.Author = it.User.Login
	}
	if it.Repository != nil {
		issue.RepoNameWithOwner = it.Repository.FullName
	}
	for _, l := range it.Labels {
		issue.Labels = append(issue.Labels, data.Label{Name: l.Name, Color: l.Color})
	}
	return issue
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

// rawPost issues an authenticated POST against {baseURL}/api/v1{path} using the
// shared HTTP client and token. It accepts any 2xx response and ignores the
// body, which matches control endpoints that often return 202 or 204.
func (c *Client) rawPost(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitea POST %s: %s: %s", path, resp.Status, truncate(body, 500))
	}
	return nil
}

// truncate returns b as a string, clipped to max bytes with an ellipsis marker
// appended when it was longer, so an HTML error page cannot flood the TUI.
func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
