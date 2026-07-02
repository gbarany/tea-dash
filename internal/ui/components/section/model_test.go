package section

import (
	stdctx "context"
	"testing"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

func TestModelFetchesNextPageAtBottomAndAppendsRows(t *testing.T) {
	var pages []int
	ctx := &appctx.ProgramContext{
		Styles:           contextStyles(t),
		MainContentWidth: 100, MainContentHeight: 20,
		Config:    &config.Config{Defaults: config.Defaults{PRsLimit: 1}},
		StartTask: func(appctx.Task) tea.Cmd { return nil },
	}
	m := New(Options[data.PullRequest]{
		Id:           0,
		Type:         "pr",
		FilterKind:   "pulls",
		Ctx:          ctx,
		Config:       config.SectionConfig{Title: "PRs"},
		LoadingText:  "Loading pull requests…",
		EmptyText:    "No pull requests.",
		EmptyHint:    "Try another filter.",
		SingularForm: "pull request",
		PluralForm:   "pull requests",
		Limit:        func(c *config.Config) int { return c.Defaults.PRsLimit },
		Pageable:     true,
		Fetch: func(_ stdctx.Context, _ *gitea.Client, _ config.PrIssueFilter, _ int, page int) ([]data.PullRequest, int, error) {
			pages = append(pages, page)
			return []data.PullRequest{{
				Number: int64(page), Title: "page row",
				RepoNameWithOwner: "gbarany/tea-dash",
			}}, 2, nil
		},
		BuildRow: func(pr data.PullRequest) table.Row {
			return table.Row{pr.GetTitle()}
		},
	})
	m.SetLastFetchID("t1")
	next, _ := m.Update(RowsFetchedMsg[data.PullRequest]{
		Rows: []data.PullRequest{{
			Number: 1, Title: "first",
			RepoNameWithOwner: "gbarany/tea-dash",
		}},
		TotalCount: 2,
		TaskId:     "t1",
		Page:       1,
	})
	m = next.(*Model[data.PullRequest])

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(*Model[data.PullRequest])
	if cmd == nil {
		t.Fatal("moving at the bottom with more rows should start fetching the next page")
	}
	msg := runFetchCommand(t, cmd)
	finished, ok := msg.(appctx.TaskFinishedMsg)
	if !ok {
		t.Fatalf("fetch command returned %T, want TaskFinishedMsg", msg)
	}
	fetched, ok := finished.Msg.(RowsFetchedMsg[data.PullRequest])
	if !ok {
		t.Fatalf("finished payload = %T, want RowsFetchedMsg[data.PullRequest]", finished.Msg)
	}
	if !fetched.Append || fetched.Page != 2 {
		t.Fatalf("fetched Append=%v Page=%d, want append page 2", fetched.Append, fetched.Page)
	}
	next, _ = m.Update(fetched)
	m = next.(*Model[data.PullRequest])

	if got := m.NumRows(); got != 2 {
		t.Fatalf("NumRows = %d, want appended rows count 2", got)
	}
	if len(pages) != 1 || pages[0] != 2 {
		t.Fatalf("fetched pages = %v, want [2]", pages)
	}
	if cmd := m.MaybeFetchNextPage(); cmd != nil {
		t.Fatal("all rows are loaded; MaybeFetchNextPage should return nil")
	}
}

func runFetchCommand(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if msg := c(); msg != nil {
				if _, ok := msg.(appctx.TaskFinishedMsg); ok {
					return msg
				}
			}
		}
		t.Fatal("batch did not include a TaskFinishedMsg")
	}
	return msg
}

func contextStyles(t *testing.T) appctx.Styles {
	t.Helper()
	return appctx.DefaultStyles()
}
