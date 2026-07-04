// Package branchsection is the read-only local git branches dashboard section.
package branchsection

import (
	stdctx "context"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	localgit "github.com/gbarany/tea-dash/internal/git"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
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
		Fetch: func(fetchCtx stdctx.Context, _ *gitea.Client, _ config.PrIssueFilter, limit, _ int) ([]localgit.Branch, int, error) {
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
		BuildRow: func(branch localgit.Branch) table.Row {
			// Recomputed per call, not frozen at construction: ctx.MainContentWidth
			// changes across resizes, and UpdateProgramContext rebuilds rows after
			// re-fitting columns, so a stale column list here would leave row cell
			// counts out of sync with SetColumns (columns responsively drop per
			// SixColumnSpec.Fit).
			return branchBuildRowWithColumns(branch, branchColumnNames(ctx.MainContentWidth), ctx)
		},
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

// UpdateProgramContext resizes the table, then restores branch columns
// instead of the PR/issue defaults. It calls BaseModel.UpdateProgramContext
// directly — bypassing the embedded section.Model[localgit.Branch]'s own
// UpdateProgramContext override, which would otherwise do the same
// column+row work twice, once against the wrong (generic default) column
// shape.
func (m *Model) UpdateProgramContext(ctx *appctx.ProgramContext) {
	m.Model.BaseModel.UpdateProgramContext(ctx)
	m.applyColumns(ctx.MainContentWidth)
}

// applyColumns applies the branches column layout, safely rebuilding rows
// to match — see BaseModel.SafelySetColumnsAndRows.
func (m *Model) applyColumns(width int) {
	m.SafelySetColumnsAndRows(branchColumns(width), m.BuildRows)
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

func branchBuildRowWithColumns(branch localgit.Branch, columns []string, ctx *appctx.ProgramContext) table.Row {
	row := make(table.Row, 0, len(columns))
	for _, column := range columns {
		row = append(row, branchColumnValue(branch, column, ctx))
	}
	return row
}

func branchColumnValue(branch localgit.Branch, column string, ctx *appctx.ProgramContext) string {
	switch column {
	case "mark":
		if branch.Current {
			return "*"
		}
		return ""
	case "branch":
		return branch.Name
	case "repo":
		return branch.Repository
	case "upstream":
		if branch.Upstream == "" {
			return "local"
		}
		return branch.Upstream
	case "state":
		return branchStateCell(branch, ctx)
	case "commit":
		return branch.Commit
	default:
		return ""
	}
}

// branchStateCell renders branch.Status(), coloring only the ahead/behind
// arrow parts: AheadArrow/BehindArrow have glyphs but no styles.StateColors
// entry (T2 review note, same as notification Unread), so they're drawn
// from ctx.Styles.DimText (an explicit style, "dim for arrows" per the
// plan) via section.GlyphText rather than indexed on a guaranteed
// StateColors miss. "current"/"gone"/"synced"/"local"/"checked out" have no
// dedicated icons.State (the "mark" column's "*" already conveys current)
// and stay plain text, exactly like Status() rendered them before.
func branchStateCell(branch localgit.Branch, ctx *appctx.ProgramContext) string {
	parts := make([]string, 0, 3)
	if branch.Current {
		parts = append(parts, "current")
	}
	switch {
	case branch.UpstreamGone:
		parts = append(parts, "gone")
	case branch.Ahead > 0 && branch.Behind > 0:
		parts = append(parts,
			arrowText(ctx, icons.AheadArrow, fmt.Sprintf("ahead %d", branch.Ahead))+
				", "+arrowText(ctx, icons.BehindArrow, fmt.Sprintf("behind %d", branch.Behind)))
	case branch.Ahead > 0:
		parts = append(parts, arrowText(ctx, icons.AheadArrow, fmt.Sprintf("ahead %d", branch.Ahead)))
	case branch.Behind > 0:
		parts = append(parts, arrowText(ctx, icons.BehindArrow, fmt.Sprintf("behind %d", branch.Behind)))
	case branch.Upstream != "":
		parts = append(parts, "synced")
	default:
		parts = append(parts, "local")
	}
	if !branch.Current && branch.WorktreePath != "" {
		parts = append(parts, "checked out")
	}
	return strings.Join(parts, " · ")
}

func arrowText(ctx *appctx.ProgramContext, state icons.State, text string) string {
	return section.GlyphText(ctx.Icons, state, text, ctx.Styles.DimText)
}

// branchColumnSpec is the branches section's 6-column layout, sharing the
// same section.SixColumnSpec responsive-drop behavior (Upstream dropped
// first, then Commit, then Repo — #/Branch/Status always survive).
func branchColumnSpec() section.SixColumnSpec {
	return section.SixColumnSpec{
		Index:   section.ColumnDefinition{Name: "mark", Title: "#", Width: 3},
		Grow:    section.ColumnDefinition{Name: "branch", Title: "Branch", Width: 20},
		Repo:    section.ColumnDefinition{Name: "repo", Title: "Repo", Width: 20},
		Fourth:  section.ColumnDefinition{Name: "upstream", Title: "Upstream", Width: 28},
		State:   section.ColumnDefinition{Name: "state", Title: "Status", Width: 22},
		Updated: section.ColumnDefinition{Name: "commit", Title: "Commit", Width: 8},
	}
}

func branchColumnDefinitions(mainWidth int) []section.ColumnDefinition {
	return branchColumnSpec().Fit(mainWidth)
}

func branchColumns(mainWidth int) []table.Column {
	return section.ColumnsFromDefinitions(branchColumnDefinitions(mainWidth))
}

func branchColumnNames(mainWidth int) []string {
	return section.ColumnNamesFromDefinitions(branchColumnDefinitions(mainWidth))
}
