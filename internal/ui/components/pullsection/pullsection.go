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
	programCtx := ctx
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
		Pageable:     true,
		Fetch: func(fetchCtx stdctx.Context, c *gitea.Client, f config.PrIssueFilter, limit, page int) ([]data.PullRequest, int, error) {
			if repo := effectiveRepo(ctx, cfg, f); repo != "" {
				return c.ListRepoPullsPage(fetchCtx, repo, f, limit, page)
			}
			if f.ReviewRequested == "" && programCtx != nil && programCtx.Config != nil && len(programCtx.Config.Repos) > 0 {
				return c.ListReposPullsPage(fetchCtx, programCtx.Config.Repos, f, limit, page)
			}
			return c.SearchPullsPage(fetchCtx, f, limit, page)
		},
		BuildRow: func(pr data.PullRequest) table.Row {
			// Recompute the current column set on every call (not once at
			// construction): ctx.MainContentWidth changes across resizes,
			// and the generic section.Model rebuilds rows on
			// UpdateProgramContext, so a stale/frozen column list here
			// would leave row cell counts out of sync with SetColumns
			// (dropped columns responsively per SixColumnSpec.Fit).
			columnNames := section.ColumnNamesFromConfig(cfg.Columns, section.DefaultColumnDefinitions(ctx.MainContentWidth))
			return prBuildRowWithColumns(pr, columnNames)
		},
		Columns: func(width int) []table.Column {
			return section.ColumnsFromConfig(cfg.Columns, section.DefaultColumnDefinitions(width))
		},
	})
}

func effectiveRepo(ctx *appctx.ProgramContext, cfg config.SectionConfig, f config.PrIssueFilter) string {
	if cfg.Repo != "" {
		return cfg.Repo
	}
	if f.ReviewRequested != "" {
		return ""
	}
	if ctx == nil || !ctx.SmartFiltering {
		return ""
	}
	return ctx.CurrentRepo
}

// prState renders a draft PR's state as "draft" and otherwise the raw state.
func prState(pr data.PullRequest) string {
	if pr.Draft {
		return "draft"
	}
	return pr.State
}

// prBuildRow maps a PR into the default table row.
func prBuildRow(pr data.PullRequest) table.Row {
	return prBuildRowWithColumns(pr, section.DefaultColumnNames())
}

func prBuildRowWithColumns(pr data.PullRequest, columns []string) table.Row {
	row := make(table.Row, 0, len(columns))
	for _, column := range columns {
		row = append(row, prColumnValue(pr, column))
	}
	return row
}

func prColumnValue(pr data.PullRequest, column string) string {
	author := ""
	if pr.Author != "" {
		author = "@" + pr.Author
	}
	switch column {
	case "number":
		return fmt.Sprintf("#%d", pr.Number)
	case "title":
		return pr.Title
	case "repo":
		return pr.RepoNameWithOwner
	case "author":
		return author
	case "state":
		return prState(pr)
	case "updated":
		return section.HumanizeTime(pr.UpdatedAt)
	default:
		return ""
	}
}
