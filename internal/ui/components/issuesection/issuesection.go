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
	programCtx := ctx
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
		Pageable:     true,
		Fetch: func(fetchCtx stdctx.Context, c *gitea.Client, f config.PrIssueFilter, limit, page int) ([]data.Issue, int, error) {
			if repo := effectiveRepo(ctx, cfg); repo != "" {
				return c.ListRepoIssuesPage(fetchCtx, repo, f, limit, page)
			}
			if programCtx != nil && programCtx.Config != nil && len(programCtx.Config.Repos) > 0 {
				return c.ListReposIssuesPage(fetchCtx, programCtx.Config.Repos, f, limit, page)
			}
			return c.SearchIssuesPage(fetchCtx, f, limit, page)
		},
		BuildRow: func(issue data.Issue) table.Row {
			// Recomputed per call, not frozen at construction — see
			// pullsection.NewModel's identical comment.
			columnNames := section.ColumnNamesFromConfig(cfg.Columns, section.DefaultColumnDefinitions(ctx.MainContentWidth))
			return issueBuildRowWithColumns(issue, columnNames, ctx)
		},
		Columns: func(width int) []table.Column {
			return section.ColumnsFromConfig(cfg.Columns, section.DefaultColumnDefinitions(width))
		},
	})
}

func effectiveRepo(ctx *appctx.ProgramContext, cfg config.SectionConfig) string {
	if cfg.Repo != "" {
		return cfg.Repo
	}
	if ctx == nil || !ctx.SmartFiltering {
		return ""
	}
	return ctx.CurrentRepo
}

func issueBuildRowWithColumns(issue data.Issue, columns []string, ctx *appctx.ProgramContext) table.Row {
	row := make(table.Row, 0, len(columns))
	for _, column := range columns {
		row = append(row, issueColumnValue(issue, column, ctx))
	}
	return row
}

func issueColumnValue(issue data.Issue, column string, ctx *appctx.ProgramContext) string {
	author := ""
	if issue.Author != "" {
		author = "@" + issue.Author
	}
	switch column {
	case "number":
		return fmt.Sprintf("#%d", issue.Number)
	case "title":
		return issue.Title
	case "repo":
		return issue.RepoNameWithOwner
	case "author":
		return author
	case "state":
		return section.StateCell(issue.State, ctx.Icons, ctx.Styles)
	case "updated":
		return section.HumanizeTime(issue.UpdatedAt)
	default:
		return ""
	}
}
