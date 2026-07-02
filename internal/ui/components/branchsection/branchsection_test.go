package branchsection

import (
	"strings"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/config"
	localgit "github.com/gbarany/tea-dash/internal/git"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func newModel(t *testing.T) *Model {
	t.Helper()
	ctx := &context.ProgramContext{
		Config:            &config.Config{},
		Styles:            context.DefaultStyles(),
		MainContentWidth:  120,
		MainContentHeight: 20,
	}
	return NewModel(0, ctx, config.SectionConfig{Title: "Local Branches"})
}

func TestImplementsSection(t *testing.T) {
	var _ section.Section = (*Model)(nil)
}

func TestFetchedMsgBuildsRows(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("b1")
	next, _ := m.Update(SectionBranchesFetchedMsg{
		Rows: []localgit.Branch{{
			Repository: "tea-dash", Name: "feature/repo-branches", Current: true,
			Upstream: "origin/feature/repo-branches", Ahead: 2, Behind: 1,
			Commit: "abc1234", Subject: "Add branch dashboard", UpdatedAt: time.Now().Add(-time.Hour),
		}},
		TotalCount: 1,
		TaskId:     "b1",
	})
	m = next.(*Model)

	if m.GetTotalCount() != 1 || m.NumRows() != 1 {
		t.Fatalf("counts: total=%d rows=%d", m.GetTotalCount(), m.NumRows())
	}
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	for _, want := range []string{"feature/repo-branches", "tea-dash", "origin/feature/repo-branches", "current", "ahead 2", "behind 1", "abc1234"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("row %q missing %q", joined, want)
		}
	}
}

func TestRepositoriesFromConfigUsesConfiguredLocalRepos(t *testing.T) {
	cfg := &config.Config{LocalRepos: []config.LocalRepoConfig{
		{Name: "tea-dash", Path: "/tmp/tea-dash"},
		{Name: "other", Path: "/tmp/other"},
	}}
	repos, err := repositoriesFromConfig(cfg, func() (string, error) {
		return "/tmp/ignored", nil
	})
	if err != nil {
		t.Fatalf("repositoriesFromConfig() error: %v", err)
	}
	if len(repos) != 2 || repos[0].Name != "tea-dash" || repos[0].Path != "/tmp/tea-dash" ||
		repos[1].Name != "other" || repos[1].Path != "/tmp/other" {
		t.Fatalf("repositoriesFromConfig() = %+v", repos)
	}
}

func TestRepositoriesFromConfigFallsBackToWorkingDirectory(t *testing.T) {
	repos, err := repositoriesFromConfig(&config.Config{}, func() (string, error) {
		return "/tmp/current", nil
	})
	if err != nil {
		t.Fatalf("repositoriesFromConfig() error: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "" || repos[0].Path != "/tmp/current" {
		t.Fatalf("repositoriesFromConfig() = %+v, want cwd fallback", repos)
	}
}
