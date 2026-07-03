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
		BuildRow: actionBuildRow,
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

// UpdateProgramContext resizes the table and reapplies the Actions columns.
func (m *Model) UpdateProgramContext(ctx *appctx.ProgramContext) {
	m.Model.UpdateProgramContext(ctx)
	m.setActionColumns(ctx.MainContentWidth)
}

func (m *Model) setActionColumns(mainWidth int) {
	m.Columns = actionColumns(mainWidth)
	m.Table.SetColumns(m.Columns)
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

func actionColumns(mainWidth int) []table.Column {
	const (
		numW     = 8
		repoW    = 22
		actorW   = 18
		stateW   = 18
		updatedW = 10
	)
	titleW := mainWidth - (numW + repoW + actorW + stateW + updatedW) - 6
	if titleW < 20 {
		titleW = 20
	}
	return []table.Column{
		{Title: "#", Width: numW},
		{Title: "Title", Width: titleW},
		{Title: "Repo", Width: repoW},
		{Title: "Actor/Event", Width: actorW},
		{Title: "Status", Width: stateW},
		{Title: "Updated", Width: updatedW},
	}
}

func actionBuildRow(run data.ActionRun) table.Row {
	return table.Row{
		fmt.Sprintf("#%d", run.GetNumber()),
		run.GetTitle(),
		run.RepoNameWithOwner,
		actionActorEvent(run),
		actionStatus(run),
		section.HumanizeTime(run.GetUpdatedAt()),
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
