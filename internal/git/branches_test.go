package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestListBranchesReadsLocalBranchesAndUpstreamStatus(t *testing.T) {
	repoPath := newBranchRepo(t)

	branches, err := ListBranches(context.Background(), Repository{Name: "tea-dash", Path: repoPath})
	if err != nil {
		t.Fatalf("ListBranches() error: %v", err)
	}

	byName := map[string]Branch{}
	for _, b := range branches {
		byName[b.Name] = b
		if b.Repository != "tea-dash" {
			t.Fatalf("Branch.Repository = %q, want tea-dash", b.Repository)
		}
		if b.RepositoryPath != repoPath {
			t.Fatalf("Branch.RepositoryPath = %q, want %q", b.RepositoryPath, repoPath)
		}
	}

	feature, ok := byName["feature"]
	if !ok {
		t.Fatalf("branches missing feature: %+v", branches)
	}
	if !feature.Current {
		t.Fatal("feature should be marked current")
	}
	if feature.Upstream != "origin/feature" || feature.Ahead != 1 || feature.Behind != 0 {
		t.Fatalf("feature upstream status = upstream %q ahead %d behind %d, want origin/feature ahead 1 behind 0",
			feature.Upstream, feature.Ahead, feature.Behind)
	}
	if feature.Commit == "" || feature.Subject != "local feature work" {
		t.Fatalf("feature commit metadata = %q %q, want short commit and subject", feature.Commit, feature.Subject)
	}

	main, ok := byName["main"]
	if !ok {
		t.Fatalf("branches missing main: %+v", branches)
	}
	if main.Current {
		t.Fatal("main should not be marked current")
	}
	if main.Upstream != "origin/main" || main.Ahead != 0 || main.Behind != 1 {
		t.Fatalf("main upstream status = upstream %q ahead %d behind %d, want origin/main ahead 0 behind 1",
			main.Upstream, main.Ahead, main.Behind)
	}
}

func TestListBranchesForRepositoriesAggregatesAndLabelsRepos(t *testing.T) {
	first := newSingleBranchRepo(t, "first repo")
	second := newSingleBranchRepo(t, "second repo")

	branches, err := ListBranchesForRepositories(context.Background(), []Repository{
		{Name: "first", Path: first},
		{Name: "second", Path: second},
	})
	if err != nil {
		t.Fatalf("ListBranchesForRepositories() error: %v", err)
	}

	if len(branches) != 2 {
		t.Fatalf("len(branches) = %d, want 2: %+v", len(branches), branches)
	}
	seen := map[string]bool{}
	for _, b := range branches {
		seen[b.Repository] = true
	}
	if !seen["first"] || !seen["second"] {
		t.Fatalf("repositories seen = %+v, want first and second", seen)
	}
}

func TestParseTrackStatus(t *testing.T) {
	cases := []struct {
		in      string
		ahead   int
		behind  int
		gone    bool
		wantErr bool
	}{
		{in: "", ahead: 0, behind: 0},
		{in: "[ahead 2]", ahead: 2},
		{in: "[behind 3]", behind: 3},
		{in: "[ahead 2, behind 3]", ahead: 2, behind: 3},
		{in: "[gone]", gone: true},
		{in: "[ahead nope]", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseTrackStatus(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseTrackStatus(%q) expected error, got nil", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTrackStatus(%q) error: %v", tc.in, err)
			}
			if got.Ahead != tc.ahead || got.Behind != tc.behind || got.Gone != tc.gone {
				t.Fatalf("parseTrackStatus(%q) = %+v, want ahead=%d behind=%d gone=%v",
					tc.in, got, tc.ahead, tc.behind, tc.gone)
			}
		})
	}
}

func newBranchRepo(t *testing.T) string {
	t.Helper()
	repo := newSingleBranchRepo(t, "base")

	runGit(t, repo, "update-ref", "refs/remotes/origin/feature", "HEAD")
	runGit(t, repo, "branch", "feature")
	runGit(t, repo, "branch", "--set-upstream-to", "origin/feature", "feature")
	runGit(t, repo, "checkout", "feature")
	writeFile(t, repo, "feature.txt", "local feature work\n")
	runGit(t, repo, "add", "feature.txt")
	runGit(t, repo, "commit", "-m", "local feature work")

	runGit(t, repo, "checkout", "main")
	runGit(t, repo, "checkout", "-b", "remote-main")
	writeFile(t, repo, "remote.txt", "remote main work\n")
	runGit(t, repo, "add", "remote.txt")
	runGit(t, repo, "commit", "-m", "remote main work")
	remoteMain := gitOutput(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "checkout", "main")
	runGit(t, repo, "branch", "-D", "remote-main")
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", remoteMain)
	runGit(t, repo, "branch", "--set-upstream-to", "origin/main", "main")

	runGit(t, repo, "checkout", "feature")
	return repo
}

func newSingleBranchRepo(t *testing.T, subject string) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "--initial-branch", "main")
	runGit(t, repo, "config", "user.name", "tea-dash tests")
	runGit(t, repo, "config", "user.email", "tea-dash@example.invalid")
	runGit(t, repo, "remote", "add", "origin", "https://example.invalid/repo.git")
	writeFile(t, repo, "README.md", subject+"\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", subject)
	runGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runGit(t, repo, "branch", "--set-upstream-to", "origin/main", "main")
	return repo
}

func writeFile(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	_ = gitOutput(t, repo, args...)
}

func gitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}
