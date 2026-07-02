package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	sdk "code.gitea.io/sdk/gitea"

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

// ListRepoPullsPage returns pull requests from one repository using the repo
// issues endpoint with type=pulls. That endpoint supports the richer issue-style
// filters (labels, milestones, keyword, created_by/assigned_by/mentioned_by)
// that the typed pull-list endpoint lacks.
func (c *Client) ListRepoPullsPage(ctx context.Context, repoFull string, f config.PrIssueFilter, limit, page int) ([]data.PullRequest, int, error) {
	f = f.WithDefaults("pulls")
	rows, total, err := c.listRepoIssueRowsPage(ctx, repoFull, f, sdk.IssueTypePull, limit, page)
	if err != nil {
		return nil, 0, err
	}
	prs := make([]data.PullRequest, 0, len(rows))
	for _, it := range rows {
		if it != nil {
			prs = append(prs, mapSDKIssueToPull(repoFull, it))
		}
	}
	return prs, total, nil
}

// ListRepoIssuesPage returns issues from one repository through the typed SDK,
// preserving repo-scoped login filters that cross-repo search cannot express.
func (c *Client) ListRepoIssuesPage(ctx context.Context, repoFull string, f config.PrIssueFilter, limit, page int) ([]data.Issue, int, error) {
	f = f.WithDefaults("issues")
	rows, total, err := c.listRepoIssueRowsPage(ctx, repoFull, f, sdk.IssueTypeIssue, limit, page)
	if err != nil {
		return nil, 0, err
	}
	issues := make([]data.Issue, 0, len(rows))
	for _, it := range rows {
		if it != nil {
			issues = append(issues, mapSDKIssue(repoFull, it))
		}
	}
	return issues, total, nil
}

// ListReposPullsPage returns one global page of pull requests fanned out across
// a fixed repo list. It fetches enough per-repo pages to build the global prefix
// for page, sorts by updated time, then slices the requested page. This preserves
// progressive loading without duplicate rows while keeping the section API
// stateless.
func (c *Client) ListReposPullsPage(ctx context.Context, repos []string, f config.PrIssueFilter, limit, page int) ([]data.PullRequest, int, error) {
	limit, page = normalizedLimitPage(limit, page)
	var all []data.PullRequest
	total := 0
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		for p := 1; p <= page; p++ {
			rows, repoTotal, err := c.ListRepoPullsPage(ctx, repo, f, limit, p)
			if err != nil {
				return nil, 0, err
			}
			if p == 1 {
				total += repoTotal
			}
			all = append(all, rows...)
		}
	}
	sortRowsByUpdatedDesc(all)
	return pageSlice(all, limit, page), total, nil
}

// ListReposIssuesPage is the issue equivalent of ListReposPullsPage.
func (c *Client) ListReposIssuesPage(ctx context.Context, repos []string, f config.PrIssueFilter, limit, page int) ([]data.Issue, int, error) {
	limit, page = normalizedLimitPage(limit, page)
	var all []data.Issue
	total := 0
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		for p := 1; p <= page; p++ {
			rows, repoTotal, err := c.ListRepoIssuesPage(ctx, repo, f, limit, p)
			if err != nil {
				return nil, 0, err
			}
			if p == 1 {
				total += repoTotal
			}
			all = append(all, rows...)
		}
	}
	sortRowsByUpdatedDesc(all)
	return pageSlice(all, limit, page), total, nil
}

func (c *Client) listRepoIssueRowsPage(ctx context.Context, repoFull string, f config.PrIssueFilter, typ sdk.IssueType, limit, page int) ([]*sdk.Issue, int, error) {
	_ = ctx // The SDK context is pinned at client construction; raw calls use per-request contexts.
	repo, err := config.ParseRepo(repoFull)
	if err != nil {
		return nil, 0, err
	}
	opt, err := c.repoIssueListOptions(f, typ, limit, page)
	if err != nil {
		return nil, 0, err
	}
	var rows []*sdk.Issue
	var resp *sdk.Response
	if err := c.call(func() error {
		var e error
		rows, resp, e = c.sdk.ListRepoIssues(repo.Owner, repo.Name, opt)
		return e
	}); err != nil {
		return nil, 0, fmt.Errorf("list repo issues %s: %w", repoFull, err)
	}
	return rows, totalFromSDKResponse(resp, len(rows)), nil
}

func (c *Client) repoIssueListOptions(f config.PrIssueFilter, typ sdk.IssueType, limit, page int) (sdk.ListIssueOption, error) {
	opt := sdk.ListIssueOption{
		ListOptions: sdk.ListOptions{Page: page, PageSize: normalizeLimit(limit)},
		State:       sdk.StateType(f.State),
		Type:        typ,
		Labels:      f.Labels,
		KeyWord:     f.Q,
		CreatedBy:   repoLoginFilter(f.CreatedBy, c.me),
		AssignedBy:  repoLoginFilter(f.AssignedBy, c.me),
		MentionedBy: repoLoginFilter(f.Mentioned, c.me),
	}
	if f.Milestone != "" {
		opt.Milestones = []string{f.Milestone}
	}
	if f.Since != "" {
		since, err := time.Parse(time.RFC3339, f.Since)
		if err != nil {
			return sdk.ListIssueOption{}, fmt.Errorf("parse filter.since %q: %w", f.Since, err)
		}
		opt.Since = since
	}
	return opt, nil
}

func repoLoginFilter(v, me string) string {
	if v == "@me" {
		return me
	}
	return v
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	return limit
}

func normalizedLimitPage(limit, page int) (int, int) {
	limit = normalizeLimit(limit)
	if page <= 0 {
		page = 1
	}
	return limit, page
}

func sortRowsByUpdatedDesc[T data.RowData](rows []T) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].GetUpdatedAt().Equal(rows[j].GetUpdatedAt()) {
			if rows[i].GetRepoNameWithOwner() == rows[j].GetRepoNameWithOwner() {
				return rows[i].GetNumber() > rows[j].GetNumber()
			}
			return rows[i].GetRepoNameWithOwner() < rows[j].GetRepoNameWithOwner()
		}
		return rows[i].GetUpdatedAt().After(rows[j].GetUpdatedAt())
	})
}

func pageSlice[T data.RowData](rows []T, limit, page int) []T {
	start := (page - 1) * limit
	if start >= len(rows) {
		return nil
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

func totalFromSDKResponse(resp *sdk.Response, fallback int) int {
	if resp != nil && resp.Response != nil {
		if v := resp.Header.Get("X-Total-Count"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
	}
	return fallback
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

func mapSDKIssueToPull(repoFull string, it *sdk.Issue) data.PullRequest {
	pr := data.PullRequest{
		Number:            it.Index,
		Title:             it.Title,
		State:             string(it.State),
		HTMLURL:           it.HTMLURL,
		RepoNameWithOwner: repoFull,
		CreatedAt:         it.Created,
		UpdatedAt:         it.Updated,
		Labels:            mapSDKLabels(it.Labels),
	}
	if it.Poster != nil {
		pr.Author = it.Poster.UserName
	}
	if it.Repository != nil && it.Repository.FullName != "" {
		pr.RepoNameWithOwner = it.Repository.FullName
	}
	if it.PullRequest != nil && it.PullRequest.HasMerged {
		pr.State = "merged"
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

func mapSDKIssue(repoFull string, it *sdk.Issue) data.Issue {
	issue := data.Issue{
		Number:            it.Index,
		Title:             it.Title,
		State:             string(it.State),
		HTMLURL:           it.HTMLURL,
		RepoNameWithOwner: repoFull,
		CreatedAt:         it.Created,
		UpdatedAt:         it.Updated,
		Labels:            mapSDKLabels(it.Labels),
	}
	if it.Poster != nil {
		issue.Author = it.Poster.UserName
	}
	if it.Repository != nil && it.Repository.FullName != "" {
		issue.RepoNameWithOwner = it.Repository.FullName
	}
	return issue
}

func mapSDKLabels(labels []*sdk.Label) []data.Label {
	out := make([]data.Label, 0, len(labels))
	for _, l := range labels {
		if l != nil {
			out = append(out, data.Label{Name: l.Name, Color: l.Color})
		}
	}
	return out
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
