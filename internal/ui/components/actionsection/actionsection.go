// Package actionsection is the Actions dashboard section: thin wiring over the
// generic section.Model parameterized for repo-scoped data.ActionRun rows.
package actionsection

import (
	stdctx "context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

// SectionType is the routing type tag for Actions sections.
const SectionType = "action"

// Model is the Actions section (the generic section specialized for action
// runs, with wider state columns for status/conclusion).
type Model struct {
	*section.Model[data.ActionRun]
}

// SectionActionsFetchedMsg is the fetch payload carried in TaskFinishedMsg.Msg.
type SectionActionsFetchedMsg = section.RowsFetchedMsg[data.ActionRun]

var _ section.Section = (*Model)(nil)

// NewModel builds an Actions section.
func NewModel(id int, ctx *appctx.ProgramContext, cfg config.SectionConfig) *Model {
	base := section.New(section.Options[data.ActionRun]{
		Id:           id,
		Ctx:          ctx,
		Config:       cfg,
		Type:         SectionType,
		FilterKind:   "",
		LoadingText:  "Loading action runs...",
		EmptyText:    actionEmptyText(cfg),
		EmptyHint:    actionEmptyHint(cfg),
		SingularForm: "action run",
		PluralForm:   "action runs",
		Limit:        func(c *config.Config) int { return c.Defaults.ActionsLimit },
		Fetch: func(ctx stdctx.Context, c *gitea.Client, f config.PrIssueFilter, limit, _ int) ([]data.ActionRun, int, error) {
			repoRef := strings.TrimSpace(cfg.Repo)
			if repoRef == "" {
				return nil, 0, nil
			}
			if c == nil {
				return nil, 0, errors.New("Gitea client is not configured")
			}
			repo, err := config.ParseRepo(repoRef)
			if err != nil {
				return nil, 0, err
			}
			if limit <= 0 {
				limit = 50
			}
			return c.ListActionRuns(ctx, repo.Owner, repo.Name, gitea.ActionRunListOptions{
				Event:   f.Event,
				Branch:  f.Branch,
				Status:  f.Status,
				Actor:   f.Actor,
				HeadSHA: f.HeadSHA,
				Limit:   limit,
			})
		},
		BuildRow: func(run data.ActionRun) table.Row {
			// Recomputed per call, not frozen at construction: ctx.MainContentWidth
			// changes across resizes, and UpdateProgramContext rebuilds rows after
			// re-fitting columns, so a stale column list here would leave row cell
			// counts out of sync with SetColumns (columns responsively drop per
			// SixColumnSpec.Fit).
			return actionBuildRowWithColumns(run, actionColumnNames(ctx.MainContentWidth))
		},
	})
	m := &Model{Model: base}
	m.setActionColumns(ctx.MainContentWidth)
	return m
}

// Update preserves the actionsection wrapper when the embedded generic section
// handles a message.
func (m *Model) Update(msg tea.Msg) (section.Section, tea.Cmd) {
	next, cmd := m.Model.Update(msg)
	if base, ok := next.(*section.Model[data.ActionRun]); ok {
		m.Model = base
	}
	return m, cmd
}

// UpdateProgramContext resizes the table, then reapplies the Actions
// columns (instead of the generic model's PR/issue-shaped default). It
// calls BaseModel.UpdateProgramContext directly — bypassing the embedded
// section.Model[data.ActionRun]'s own UpdateProgramContext override, which
// would otherwise do the same column+row work twice, once against the
// wrong (generic default) column shape.
func (m *Model) UpdateProgramContext(ctx *appctx.ProgramContext) {
	m.Model.BaseModel.UpdateProgramContext(ctx)
	m.setActionColumns(ctx.MainContentWidth)
}

// setActionColumns applies the Actions column layout, safely rebuilding
// rows to match — see BaseModel.SafelySetColumnsAndRows.
func (m *Model) setActionColumns(mainWidth int) {
	m.SafelySetColumnsAndRows(actionColumns(mainWidth), m.BuildRows)
}

func actionEmptyText(cfg config.SectionConfig) string {
	if strings.TrimSpace(cfg.Repo) == "" {
		return "No configured Actions sections."
	}
	return "No action runs match this filter."
}

func actionEmptyHint(cfg config.SectionConfig) string {
	if strings.TrimSpace(cfg.Repo) == "" {
		return "Add actionsSections with repo: owner/name to your tea-dash config."
	}
	return "This board shows repo-scoped Gitea Actions workflow runs when the server exposes the Actions API."
}

// actionColumnSpec is the Actions section's 6-column layout: wider state
// and actor columns than the PR/issue default (status/conclusion strings
// and "@actor event" pairs run longer), sharing the same
// section.SixColumnSpec responsive-drop behavior (Actor/Event dropped
// first, then Updated, then Repo — #/Title/Status always survive).
func actionColumnSpec() section.SixColumnSpec {
	return section.SixColumnSpec{
		Index:   section.ColumnDefinition{Name: "number", Title: "#", Width: 8},
		Grow:    section.ColumnDefinition{Name: "title", Title: "Title", Width: 20},
		Repo:    section.ColumnDefinition{Name: "repo", Title: "Repo", Width: 22},
		Fourth:  section.ColumnDefinition{Name: "actor", Title: "Actor/Event", Width: 18},
		State:   section.ColumnDefinition{Name: "state", Title: "Status", Width: 18},
		Updated: section.ColumnDefinition{Name: "updated", Title: "Updated", Width: 10},
	}
}

func actionColumnDefinitions(mainWidth int) []section.ColumnDefinition {
	return actionColumnSpec().Fit(mainWidth)
}

func actionColumns(mainWidth int) []table.Column {
	return section.ColumnsFromDefinitions(actionColumnDefinitions(mainWidth))
}

func actionColumnNames(mainWidth int) []string {
	return section.ColumnNamesFromDefinitions(actionColumnDefinitions(mainWidth))
}

func actionBuildRowWithColumns(run data.ActionRun, columns []string) table.Row {
	row := make(table.Row, 0, len(columns))
	for _, column := range columns {
		row = append(row, actionColumnValue(run, column))
	}
	return row
}

func actionColumnValue(run data.ActionRun, column string) string {
	switch column {
	case "number":
		return fmt.Sprintf("#%d", run.GetNumber())
	case "title":
		return run.GetTitle()
	case "repo":
		return run.RepoNameWithOwner
	case "actor":
		return actionActorEvent(run)
	case "state":
		return actionStatus(run)
	case "updated":
		return section.HumanizeTime(run.GetUpdatedAt())
	default:
		return ""
	}
}

func actionActorEvent(run data.ActionRun) string {
	var parts []string
	if run.Actor != "" {
		parts = append(parts, "@"+run.Actor)
	}
	if run.Event != "" {
		parts = append(parts, run.Event)
	}
	return strings.Join(parts, " ")
}

func actionStatus(run data.ActionRun) string {
	switch {
	case run.Status != "" && run.Conclusion != "":
		return run.Status + "/" + run.Conclusion
	case run.Conclusion != "":
		return run.Conclusion
	default:
		return run.Status
	}
}
