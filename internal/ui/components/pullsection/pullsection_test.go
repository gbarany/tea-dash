package pullsection

import (
	"strings"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func newModel(t *testing.T) *Model {
	t.Helper()
	ctx := &context.ProgramContext{Styles: context.DefaultStyles(), MainContentWidth: 100, MainContentHeight: 20}
	m := NewModel(0, ctx, config.SectionConfig{Title: "My Pull Requests"})
	return m
}

func TestImplementsSection(t *testing.T) {
	var _ section.Section = (*Model)(nil)
}

func TestFetchedMsgBuildsRows(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("t1")
	next, _ := m.Update(SectionPullRequestsFetchedMsg{
		Prs: []data.PullRequest{{
			Number: 128, Title: "Add wiki CLI", RepoNameWithOwner: "gitea/tea",
			Author: "lunny", State: "open", UpdatedAt: time.Now().Add(-2 * time.Hour),
		}},
		TotalCount: 1, TaskId: "t1",
	})
	m = next.(*Model)

	if m.GetTotalCount() != 1 || m.NumRows() != 1 {
		t.Fatalf("counts: total=%d rows=%d", m.GetTotalCount(), m.NumRows())
	}
	if m.GetCurrRow() == nil || m.GetCurrRow().GetNumber() != 128 {
		t.Fatalf("GetCurrRow = %+v", m.GetCurrRow())
	}
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	for _, want := range []string{"#128", "Add wiki CLI", "gitea/tea", "@lunny", "open"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("row %q missing %q", joined, want)
		}
	}
}

func TestGetCurrRowNilBeforeFetch(t *testing.T) {
	m := newModel(t)
	if m.GetCurrRow() != nil {
		t.Fatalf("GetCurrRow() = %+v, want nil before any fetch", m.GetCurrRow())
	}
}

func TestDraftPRRowShowsDraft(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("t1")
	next, _ := m.Update(SectionPullRequestsFetchedMsg{
		Prs: []data.PullRequest{{
			Number: 9, Title: "WIP", RepoNameWithOwner: "gitea/tea",
			Author: "me", State: "open", Draft: true,
		}},
		TotalCount: 1, TaskId: "t1",
	})
	m = next.(*Model)
	row := m.BuildRows()[0]
	if !strings.Contains(strings.Join([]string(row), "|"), "draft") {
		t.Fatalf("draft PR row %v should show \"draft\"", row)
	}
}

func TestStaleFetchIgnored(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("t2") // an in-flight fetch t2 is expected
	next, _ := m.Update(SectionPullRequestsFetchedMsg{
		Prs: []data.PullRequest{{Number: 1}}, TotalCount: 1, TaskId: "t1", // stale
	})
	m = next.(*Model)
	if m.NumRows() != 0 {
		t.Fatalf("stale fetch was applied: rows=%d", m.NumRows())
	}
}
