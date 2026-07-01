package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

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
// them) across all accessible repos, in the given state ("open"/"closed"/"all").
// It uses the cross-repo search endpoint with the me-scoping boolean created=true.
func (c *Client) SearchMyPulls(ctx context.Context, state string) ([]data.PullRequest, error) {
	if state == "" {
		state = "open"
	}
	q := url.Values{}
	q.Set("type", "pulls")
	q.Set("created", "true")
	q.Set("state", state)
	q.Set("limit", "50")

	var rows []searchIssue
	if err := c.rawGet(ctx, "/repos/issues/search?"+q.Encode(), &rows); err != nil {
		return nil, err
	}

	prs := make([]data.PullRequest, 0, len(rows))
	for _, it := range rows {
		prs = append(prs, mapSearchIssue(it))
	}
	return prs, nil
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
// shared HTTP client and token, decoding the JSON body into out.
func (c *Client) rawGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1"+path, nil)
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
		return fmt.Errorf("gitea GET %s: %s: %s", path, resp.Status, string(body))
	}
	return json.Unmarshal(body, out)
}
