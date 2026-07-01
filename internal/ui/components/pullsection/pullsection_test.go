package pullsection

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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

// newSearchableModel builds a model whose ctx has a StartTask so enter's
// refetch does not panic on a nil StartTask closure.
func newSearchableModel(t *testing.T) *Model {
	t.Helper()
	ctx := &context.ProgramContext{
		Styles: context.DefaultStyles(), MainContentWidth: 100, MainContentHeight: 20,
		StartTask: func(context.Task) tea.Cmd { return nil },
	}
	return NewModel(0, ctx, config.SectionConfig{Title: "My Pull Requests"})
}

func TestSetIsSearchingFocusesAndShowsBar(t *testing.T) {
	m := newModel(t)
	if m.IsSearchFocused() {
		t.Fatal("a fresh section should not be searching")
	}
	notSearchingView := m.View()
	m.SetIsSearching(true)
	if !m.IsSearchFocused() {
		t.Fatal("SetIsSearching(true) should focus the search bar")
	}
	// While searching the view is the search bar prepended above the body.
	searchingView := m.View()
	if !strings.HasPrefix(searchingView, m.SearchBar.View()) {
		t.Fatalf("searching View() should start with the search bar, got:\n%s", searchingView)
	}
	if !strings.Contains(searchingView, notSearchingView) {
		t.Fatalf("searching View() should still contain the section body, got:\n%s", searchingView)
	}
}

func TestTypingWhileFocusedRoutesToSearchBar(t *testing.T) {
	m := newModel(t)
	m.SetIsSearching(true)
	for _, r := range "bug" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	if got := m.SearchBar.Value(); got != "bug" {
		t.Fatalf("after typing while focused, SearchBar.Value() = %q, want %q", got, "bug")
	}
}

func TestSearchEnterAppliesKeywordAndRefetches(t *testing.T) {
	m := newSearchableModel(t)
	m.SetIsSearching(true)
	for _, r := range "bug" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*Model)
	if m.Config.Filter.Q != "bug" {
		t.Fatalf("enter should apply the keyword: Config.Filter.Q = %q, want %q", m.Config.Filter.Q, "bug")
	}
	if m.IsSearchFocused() {
		t.Fatal("enter should unfocus the search bar")
	}
	if cmd == nil {
		t.Fatal("enter should return a non-nil refetch command")
	}
}

func TestSearchEscRevertsAndUnfocuses(t *testing.T) {
	m := newModel(t)
	m.Config.Filter.Q = "applied"
	m.SetIsSearching(true)
	for _, r := range "typed" {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(*Model)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(*Model)
	if m.IsSearchFocused() {
		t.Fatal("esc should unfocus the search bar")
	}
	if m.Config.Filter.Q != "applied" {
		t.Fatalf("esc should not change the applied keyword: Q = %q, want %q", m.Config.Filter.Q, "applied")
	}
	if got := m.SearchBar.Value(); got != "applied" {
		t.Fatalf("esc should revert the bar to the applied keyword: Value() = %q, want %q", got, "applied")
	}
	if cmd != nil {
		t.Fatalf("esc should return a nil command, got %v", cmd)
	}
}

func TestImplementsSection(t *testing.T) {
	var _ section.Section = (*Model)(nil)
}

func TestFetchedMsgBuildsRows(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("t1")
	next, _ := m.Update(SectionPullRequestsFetchedMsg{
		Rows: []data.PullRequest{{
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
		Rows: []data.PullRequest{{
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
		Rows: []data.PullRequest{{Number: 1}}, TotalCount: 1, TaskId: "t1", // stale
	})
	m = next.(*Model)
	if m.NumRows() != 0 {
		t.Fatalf("stale fetch was applied: rows=%d", m.NumRows())
	}
}
