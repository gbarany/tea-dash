// Package pullsection is the pull-requests dashboard section: thin wiring over
// the generic section.Model parameterized for data.PullRequest.
package pullsection

import (
	stdctx "context"
	"fmt"

	"charm.land/bubbles/v2/table"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

// SectionType is the routing type tag for pull-request sections.
const SectionType = "pr"

// Model is the pull-requests section (the generic section specialized for PRs).
type Model = section.Model[data.PullRequest]

// SectionPullRequestsFetchedMsg is the fetch payload carried in TaskFinishedMsg.Msg.
type SectionPullRequestsFetchedMsg = section.RowsFetchedMsg[data.PullRequest]

// NewModel builds a pull-requests section.
func NewModel(id int, ctx *appctx.ProgramContext, cfg config.SectionConfig) *Model {
	return section.New(section.Options[data.PullRequest]{
		Id:           id,
		Ctx:          ctx,
		Config:       cfg,
		Type:         SectionType,
		FilterKind:   "pulls",
		LoadingText:  "Loading pull requests…",
		EmptyText:    "No pull requests match this filter.",
		EmptyHint:    "This board shows PRs you created across all repos on your Gitea instance. Switch sections with h/l.",
		SingularForm: "pull request",
		PluralForm:   "pull requests",
		Limit:        func(c *config.Config) int { return c.Defaults.PRsLimit },
		Fetch: func(ctx stdctx.Context, c *gitea.Client, f config.PrIssueFilter, limit int) ([]data.PullRequest, int, error) {
			return c.SearchPulls(ctx, f, limit)
		},
		BuildRow: prBuildRow,
	})
}

// prState renders a draft PR's state as "draft" and otherwise the raw state.
func prState(pr data.PullRequest) string {
	if pr.Draft {
		return "draft"
	}
	return pr.State
}

// prBuildRow maps a PR into the table's 6-cell row.
func prBuildRow(pr data.PullRequest) table.Row {
	author := ""
	if pr.Author != "" {
		author = "@" + pr.Author
	}
	return table.Row{
		fmt.Sprintf("#%d", pr.Number),
		pr.Title,
		pr.RepoNameWithOwner,
		author,
		prState(pr),
		section.HumanizeTime(pr.UpdatedAt),
	}
}
