package gitea

import (
	"fmt"

	sdk "code.gitea.io/sdk/gitea"
)

// GetPullDiff fetches the raw unified diff bytes for a pull request.
func (c *Client) GetPullDiff(owner, repo string, index int64) ([]byte, error) {
	var diff []byte
	err := c.call(func() error {
		var e error
		diff, _, e = c.sdk.GetPullRequestDiff(owner, repo, index, sdk.PullRequestDiffOptions{Binary: true})
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("get pull diff %s/%s#%d: %w", owner, repo, index, err)
	}
	return diff, nil
}
