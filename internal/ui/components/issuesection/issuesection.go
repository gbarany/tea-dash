// Package issuesection is the issues dashboard section: thin wiring over the
// generic section.Model parameterized for data.Issue.
package issuesection

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

// SectionType is the routing type tag for issue sections.
const SectionType = "issue"

// Model is the issues section (the generic section specialized for issues).
type Model = section.Model[data.Issue]

// SectionIssuesFetchedMsg is the fetch payload carried in TaskFinishedMsg.Msg.
type SectionIssuesFetchedMsg = section.RowsFetchedMsg[data.Issue]

// NewModel builds an issues section.
func NewModel(id int, ctx *appctx.ProgramContext, cfg config.SectionConfig) *Model {
	return section.New(section.Options[data.Issue]{
		Id:           id,
		Ctx:          ctx,
		Config:       cfg,
		Type:         SectionType,
		FilterKind:   "issues",
		LoadingText:  "Loading issues…",
		EmptyText:    "No open issues authored by you.",
		EmptyHint:    "This board shows issues you created across all repos on your Gitea instance.",
		SingularForm: "issue",
		PluralForm:   "issues",
		Limit:        func(c *config.Config) int { return c.Defaults.IssuesLimit },
		Fetch: func(ctx stdctx.Context, c *gitea.Client, f config.PrIssueFilter, limit int) ([]data.Issue, int, error) {
			return c.SearchIssues(ctx, f, limit)
		},
		BuildRow: issueBuildRow,
	})
}

// issueBuildRow maps an issue into the table's 6-cell row (unchanged formatting).
func issueBuildRow(issue data.Issue) table.Row {
	author := ""
	if issue.Author != "" {
		author = "@" + issue.Author
	}
	return table.Row{
		fmt.Sprintf("#%d", issue.Number),
		issue.Title,
		issue.RepoNameWithOwner,
		author,
		issue.State,
		section.HumanizeTime(issue.UpdatedAt),
	}
}
