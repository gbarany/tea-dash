// Package branchsection is the read-only local git branches dashboard section.
package branchsection

import (
	stdctx "context"
	"os"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	localgit "github.com/gbarany/tea-dash/internal/git"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

// SectionType is the routing type tag for local branch sections.
const SectionType = "branch"

// Model is the branches section. It wraps the generic section so the branches
// view can use columns sized for branch/upstream/status data.
type Model struct {
	*section.Model[localgit.Branch]
}

// SectionBranchesFetchedMsg is the fetch payload carried in TaskFinishedMsg.Msg.
type SectionBranchesFetchedMsg = section.RowsFetchedMsg[localgit.Branch]

// NewModel builds a local branches section.
func NewModel(id int, ctx *appctx.ProgramContext, cfg config.SectionConfig) *Model {
	m := &Model{Model: section.New(section.Options[localgit.Branch]{
		Id:           id,
		Ctx:          ctx,
		Config:       cfg,
		Type:         SectionType,
		FilterKind:   "",
		LoadingText:  "Loading local branches...",
		EmptyText:    "No local branches.",
		EmptyHint:    "Configure localRepos with paths to git checkouts, or start tea-dash inside a repository.",
		SingularForm: "branch",
		PluralForm:   "branches",
		Limit:        func(c *config.Config) int { return c.Defaults.BranchesLimit },
		Fetch: func(fetchCtx stdctx.Context, _ *gitea.Client, _ config.PrIssueFilter, limit int) ([]localgit.Branch, int, error) {
			repos, err := repositoriesFromConfig(ctx.Config, os.Getwd)
			if err != nil {
				return nil, 0, err
			}
			branches, err := localgit.ListBranchesForRepositories(fetchCtx, repos)
			if err != nil {
				return nil, 0, err
			}
			total := len(branches)
			if limit > 0 && len(branches) > limit {
				branches = branches[:limit]
			}
			return branches, total, nil
		},
		BuildRow: branchBuildRow,
	})}
	m.applyColumns(ctx.MainContentWidth)
	return m
}

// Update delegates to the generic section and returns the wrapper so branch
// sections keep their custom column behavior after updates.
func (m *Model) Update(msg tea.Msg) (section.Section, tea.Cmd) {
	next, cmd := m.Model.Update(msg)
	if typed, ok := next.(*section.Model[localgit.Branch]); ok {
		m.Model = typed
	}
	return m, cmd
}

// UpdateProgramContext delegates generic resize work, then restores branch
// columns instead of the PR/issue defaults.
func (m *Model) UpdateProgramContext(ctx *appctx.ProgramContext) {
	m.Model.UpdateProgramContext(ctx)
	m.applyColumns(ctx.MainContentWidth)
}

func (m *Model) applyColumns(width int) {
	cols := branchColumns(width)
	m.Columns = cols
	m.Table.SetColumns(cols)
}

func repositoriesFromConfig(cfg *config.Config, getwd func() (string, error)) ([]localgit.Repository, error) {
	if cfg != nil && len(cfg.LocalRepos) > 0 {
		repos := make([]localgit.Repository, 0, len(cfg.LocalRepos))
		for _, r := range cfg.LocalRepos {
			repos = append(repos, localgit.Repository{
				Name: strings.TrimSpace(r.Name),
				Path: strings.TrimSpace(r.Path),
			})
		}
		return repos, nil
	}
	wd, err := getwd()
	if err != nil {
		return nil, err
	}
	return []localgit.Repository{{Path: wd}}, nil
}

func branchBuildRow(branch localgit.Branch) table.Row {
	current := ""
	if branch.Current {
		current = "*"
	}
	upstream := branch.Upstream
	if upstream == "" {
		upstream = "local"
	}
	return table.Row{
		current,
		branch.Name,
		branch.Repository,
		upstream,
		branch.Status(),
		branch.Commit,
	}
}

func branchColumns(mainWidth int) []table.Column {
	const (
		markW     = 3
		repoW     = 20
		upstreamW = 28
		statusW   = 22
		commitW   = 8
	)
	branchW := mainWidth - (markW + repoW + upstreamW + statusW + commitW) - 6
	if branchW < 20 {
		branchW = 20
	}
	return []table.Column{
		{Title: "#", Width: markW},
		{Title: "Branch", Width: branchW},
		{Title: "Repo", Width: repoW},
		{Title: "Upstream", Width: upstreamW},
		{Title: "Status", Width: statusW},
		{Title: "Commit", Width: commitW},
	}
}
