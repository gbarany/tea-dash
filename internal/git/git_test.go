package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		host   string
		owner  string
		repo   string
		remote string
	}{
		{
			name:   "https with git suffix",
			raw:    "https://git.example.com/acme/widgets.git",
			host:   "git.example.com",
			owner:  "acme",
			repo:   "widgets",
			remote: "acme/widgets",
		},
		{
			name:   "https with port",
			raw:    "http://git.example.com:3000/acme/widgets",
			host:   "git.example.com:3000",
			owner:  "acme",
			repo:   "widgets",
			remote: "acme/widgets",
		},
		{
			name:   "ssh URL with port",
			raw:    "ssh://git@git.example.com:2222/acme/widgets.git",
			host:   "git.example.com:2222",
			owner:  "acme",
			repo:   "widgets",
			remote: "acme/widgets",
		},
		{
			name:   "ssh URL preserves explicit port 443",
			raw:    "ssh://git@git.example.com:443/acme/widgets.git",
			host:   "git.example.com:443",
			owner:  "acme",
			repo:   "widgets",
			remote: "acme/widgets",
		},
		{
			name:   "scp SSH",
			raw:    "git@git.example.com:acme/widgets.git",
			host:   "git.example.com",
			owner:  "acme",
			repo:   "widgets",
			remote: "acme/widgets",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRemoteURL(tt.raw)
			if err != nil {
				t.Fatalf("ParseRemoteURL: %v", err)
			}
			if got.Host != tt.host || got.Owner != tt.owner || got.Repo != tt.repo || got.FullName() != tt.remote {
				t.Fatalf("ParseRemoteURL = %+v, FullName=%q", got, got.FullName())
			}
		})
	}
}

func TestRemoteMatchesInstanceURL(t *testing.T) {
	remote, err := ParseRemoteURL("git@gitea.example.com:acme/api.git")
	if err != nil {
		t.Fatalf("ParseRemoteURL: %v", err)
	}
	if !remote.MatchesInstanceURL("https://gitea.example.com") {
		t.Fatal("remote should match same instance host")
	}
	if remote.MatchesInstanceURL("https://other.example.com") {
		t.Fatal("remote should not match a different instance host")
	}

	withPort, err := ParseRemoteURL("ssh://git@gitea.example.com:2222/acme/api.git")
	if err != nil {
		t.Fatalf("ParseRemoteURL with port: %v", err)
	}
	if !withPort.MatchesInstanceURL("https://gitea.example.com:2222") {
		t.Fatal("remote with port should match same instance host and port")
	}
	if withPort.MatchesInstanceURL("https://gitea.example.com") {
		t.Fatal("remote with port should not match instance without that port")
	}
}

func TestResolveRepoPathExactWildcardAndCWD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	paths := map[string]string{
		"acme/*":   "~/work/acme/{{.Repo}}",
		"acme/api": "~/work/exact-api",
	}

	got, err := ResolveRepoPath("acme/api", "/not/used", "https://gitea.example.com", paths)
	if err != nil {
		t.Fatalf("ResolveRepoPath exact: %v", err)
	}
	if want := filepath.Join(home, "work", "exact-api"); got != want {
		t.Fatalf("ResolveRepoPath exact = %q, want %q", got, want)
	}

	got, err = ResolveRepoPath("acme/web", "/not/used", "https://gitea.example.com", paths)
	if err != nil {
		t.Fatalf("ResolveRepoPath wildcard: %v", err)
	}
	if want := filepath.Join(home, "work", "acme", "web"); got != want {
		t.Fatalf("ResolveRepoPath wildcard = %q, want %q", got, want)
	}

	cwd := makeGitDir(t, "https://gitea.example.com/acme/api.git")
	got, err = ResolveRepoPath("acme/api", cwd, "https://gitea.example.com", nil)
	if err != nil {
		t.Fatalf("ResolveRepoPath cwd: %v", err)
	}
	if got != cwd {
		t.Fatalf("ResolveRepoPath cwd = %q, want %q", got, cwd)
	}
}

func TestResolveRepoPathRejectsCWDHostMismatch(t *testing.T) {
	cwd := makeGitDir(t, "https://other.example.com/acme/api.git")
	_, err := ResolveRepoPath("acme/api", cwd, "https://gitea.example.com", nil)
	if err == nil || !strings.Contains(err.Error(), "no local checkout") {
		t.Fatalf("ResolveRepoPath host mismatch error = %v", err)
	}
}

func TestResolveCurrentRepoMatchesConfiguredInstance(t *testing.T) {
	cwd := makeGitDir(t, "https://gitea.example.com/acme/api.git")
	got, ok, err := ResolveCurrentRepo(cwd, "https://gitea.example.com", "origin")
	if err != nil {
		t.Fatalf("ResolveCurrentRepo: %v", err)
	}
	if !ok {
		t.Fatal("ResolveCurrentRepo should find the matching cwd remote")
	}
	if got.FullName() != "acme/api" {
		t.Fatalf("ResolveCurrentRepo = %+v, want acme/api", got)
	}
}

func TestResolveCurrentRepoUsesConfiguredRemote(t *testing.T) {
	cwd := makeGitDirWithRemote(t, "upstream", "git@gitea.example.com:acme/widgets.git")
	got, ok, err := ResolveCurrentRepo(cwd, "https://gitea.example.com", "upstream")
	if err != nil {
		t.Fatalf("ResolveCurrentRepo: %v", err)
	}
	if !ok || got.FullName() != "acme/widgets" {
		t.Fatalf("ResolveCurrentRepo = %+v, ok=%v, want acme/widgets", got, ok)
	}
}

func TestResolveCurrentRepoIsBestEffort(t *testing.T) {
	for _, tt := range []struct {
		name string
		cwd  string
		url  string
	}{
		{name: "not a git repository", cwd: t.TempDir(), url: "https://gitea.example.com"},
		{name: "host mismatch", cwd: makeGitDir(t, "https://other.example.com/acme/api.git"), url: "https://gitea.example.com"},
		{name: "unparseable remote", cwd: makeGitDir(t, "not-a-remote"), url: "https://gitea.example.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ResolveCurrentRepo(tt.cwd, tt.url, "origin")
			if err != nil {
				t.Fatalf("ResolveCurrentRepo should not fail startup: %v", err)
			}
			if ok || got.FullName() != "" {
				t.Fatalf("ResolveCurrentRepo = %+v, ok=%v, want no detected repo", got, ok)
			}
		})
	}
}

func TestBranchNameFromTemplate(t *testing.T) {
	got, err := BranchNameFromTemplate("review/{{.Owner}}-{{.Repo}}-{{.PrIndex}}", "acme/api", 42)
	if err != nil {
		t.Fatalf("BranchNameFromTemplate: %v", err)
	}
	if got != "review/acme-api-42" {
		t.Fatalf("BranchNameFromTemplate = %q", got)
	}

	got, err = BranchNameFromTemplate("", "acme/api", 42)
	if err != nil {
		t.Fatalf("BranchNameFromTemplate default: %v", err)
	}
	if got != "pr-42" {
		t.Fatalf("BranchNameFromTemplate default = %q", got)
	}
}

func TestIssueBranchNameFromTemplate(t *testing.T) {
	got, err := IssueBranchNameFromTemplate("issue/{{.Owner}}-{{.Repo}}-{{.IssueIndex}}", "acme/api", 42)
	if err != nil {
		t.Fatalf("IssueBranchNameFromTemplate: %v", err)
	}
	if got != "issue/acme-api-42" {
		t.Fatalf("IssueBranchNameFromTemplate = %q", got)
	}

	got, err = IssueBranchNameFromTemplate("", "acme/api", 42)
	if err != nil {
		t.Fatalf("IssueBranchNameFromTemplate default: %v", err)
	}
	if got != "issue-42" {
		t.Fatalf("IssueBranchNameFromTemplate default = %q", got)
	}
}

func TestRunCheckoutDirtyTreeRefusal(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{{Stdout: " M file.txt\n", ExitCode: 0}}}

	_, err := RunCheckout(context.Background(), CheckoutOptions{
		RepoName:  "acme/api",
		RepoPaths: map[string]string{"acme/api": repo},
		PrIndex:   42,
		Runner:    runner,
	})
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("RunCheckout dirty error = %v", err)
	}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0].Args, []string{"status", "--porcelain"}) {
		t.Fatalf("commands = %#v", runner.commands)
	}
}

func TestRunCheckoutFetchesAndCreatesMissingBranch(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{ExitCode: 0}, // status
		{ExitCode: 0}, // fetch
		{ExitCode: 1}, // show-ref missing branch
		{ExitCode: 0}, // switch -c
	}}

	plan, err := RunCheckout(context.Background(), CheckoutOptions{
		RepoName:       "acme/api",
		RepoPaths:      map[string]string{"acme/api": repo},
		Remote:         "upstream",
		BranchTemplate: "review/{{.Owner}}-{{.Repo}}-{{.PrIndex}}",
		PrIndex:        42,
		Runner:         runner,
	})
	if err != nil {
		t.Fatalf("RunCheckout: %v", err)
	}
	if plan.Branch != "review/acme-api-42" {
		t.Fatalf("Branch = %q", plan.Branch)
	}
	if plan.FetchRefspec != "+refs/pull/42/head:refs/remotes/upstream/pull/42/head" {
		t.Fatalf("FetchRefspec = %q", plan.FetchRefspec)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"status", "--porcelain"}},
		{Dir: repo, Name: "git", Args: []string{"fetch", "upstream", "+refs/pull/42/head:refs/remotes/upstream/pull/42/head"}},
		{Dir: repo, Name: "git", Args: []string{"show-ref", "--verify", "--quiet", "refs/heads/review/acme-api-42"}},
		{Dir: repo, Name: "git", Args: []string{"switch", "-c", "review/acme-api-42", "refs/remotes/upstream/pull/42/head"}},
	})
}

func TestRunCheckoutExistingBranchFastForwardOnly(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{ExitCode: 0}, // status
		{ExitCode: 0}, // fetch
		{ExitCode: 0}, // show-ref existing branch
		{ExitCode: 0}, // switch branch
		{ExitCode: 0}, // merge --ff-only
	}}

	_, err := RunCheckout(context.Background(), CheckoutOptions{
		RepoName:  "acme/api",
		RepoPaths: map[string]string{"acme/api": repo},
		PrIndex:   7,
		Runner:    runner,
	})
	if err != nil {
		t.Fatalf("RunCheckout: %v", err)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"status", "--porcelain"}},
		{Dir: repo, Name: "git", Args: []string{"fetch", "origin", "+refs/pull/7/head:refs/remotes/origin/pull/7/head"}},
		{Dir: repo, Name: "git", Args: []string{"show-ref", "--verify", "--quiet", "refs/heads/pr-7"}},
		{Dir: repo, Name: "git", Args: []string{"switch", "pr-7"}},
		{Dir: repo, Name: "git", Args: []string{"merge", "--ff-only", "refs/remotes/origin/pull/7/head"}},
	})
}

func TestRunCheckoutMissingRepoPath(t *testing.T) {
	runner := &fakeRunner{}
	_, err := RunCheckout(context.Background(), CheckoutOptions{
		RepoName:    "acme/api",
		CWD:         t.TempDir(),
		InstanceURL: "https://gitea.example.com",
		PrIndex:     7,
		Runner:      runner,
	})
	if err == nil || !strings.Contains(err.Error(), "no local checkout") {
		t.Fatalf("RunCheckout missing repo error = %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner should not be called, commands = %#v", runner.commands)
	}
}

func TestRunIssueCheckoutCreatesMissingBranch(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{ExitCode: 0}, // status
		{ExitCode: 1}, // show-ref missing branch
		{ExitCode: 0}, // switch -c
	}}

	plan, err := RunIssueCheckout(context.Background(), IssueCheckoutOptions{
		RepoName:       "acme/api",
		RepoPaths:      map[string]string{"acme/api": repo},
		BranchTemplate: "issue/{{.Owner}}-{{.Repo}}-{{.IssueIndex}}",
		IssueIndex:     42,
		Runner:         runner,
	})
	if err != nil {
		t.Fatalf("RunIssueCheckout: %v", err)
	}
	if plan.Branch != "issue/acme-api-42" {
		t.Fatalf("Branch = %q", plan.Branch)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"status", "--porcelain"}},
		{Dir: repo, Name: "git", Args: []string{"show-ref", "--verify", "--quiet", "refs/heads/issue/acme-api-42"}},
		{Dir: repo, Name: "git", Args: []string{"switch", "-c", "issue/acme-api-42"}},
	})
}

func TestRunIssueCheckoutSwitchesExistingBranch(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{ExitCode: 0}, // status
		{ExitCode: 0}, // show-ref existing branch
		{ExitCode: 0}, // switch branch
	}}

	_, err := RunIssueCheckout(context.Background(), IssueCheckoutOptions{
		RepoName:   "acme/api",
		RepoPaths:  map[string]string{"acme/api": repo},
		IssueIndex: 7,
		Runner:     runner,
	})
	if err != nil {
		t.Fatalf("RunIssueCheckout: %v", err)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"status", "--porcelain"}},
		{Dir: repo, Name: "git", Args: []string{"show-ref", "--verify", "--quiet", "refs/heads/issue-7"}},
		{Dir: repo, Name: "git", Args: []string{"switch", "issue-7"}},
	})
}

func TestRunIssueCheckoutDirtyTreeRefusal(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{{Stdout: " M file.txt\n", ExitCode: 0}}}

	_, err := RunIssueCheckout(context.Background(), IssueCheckoutOptions{
		RepoName:   "acme/api",
		RepoPaths:  map[string]string{"acme/api": repo},
		IssueIndex: 42,
		Runner:     runner,
	})
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("RunIssueCheckout dirty error = %v", err)
	}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0].Args, []string{"status", "--porcelain"}) {
		t.Fatalf("commands = %#v", runner.commands)
	}
}

func TestSwitchBranchRunsGitSwitch(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{Stdout: "main\n", ExitCode: 0},
		{ExitCode: 0},
	}}

	result, err := SwitchBranch(context.Background(), SwitchBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}
	if result.RepoPath != repo || result.Branch != "feature/local-ops" {
		t.Fatalf("result = %+v", result)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"branch", "--show-current"}},
		{Dir: repo, Name: "git", Args: []string{"switch", "feature/local-ops"}},
	})
}

func TestSwitchBranchRefusesCurrentBranch(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{{Stdout: "feature/local-ops\n", ExitCode: 0}}}

	_, err := SwitchBranch(context.Background(), SwitchBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err == nil || !strings.Contains(err.Error(), "already current") {
		t.Fatalf("SwitchBranch current error = %v", err)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"branch", "--show-current"}},
	})
}

func TestSwitchBranchDirtyTreeFailureIsActionable(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{Stdout: "main\n", ExitCode: 0},
		{ExitCode: 1, Stderr: "error: Your local changes to the following files would be overwritten by checkout:\n\tREADME.md\n"},
	}}

	_, err := SwitchBranch(context.Background(), SwitchBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err == nil || !strings.Contains(err.Error(), "commit, stash, or discard") {
		t.Fatalf("SwitchBranch dirty-tree error = %v", err)
	}
	// Review fix: the error message shows the repo's basename, not the
	// full local path (an opaque OS temp dir in tests, and in real --mock
	// runs — see TestDisplayRepoName).
	if !strings.Contains(err.Error(), filepath.Base(repo)) {
		t.Fatalf("SwitchBranch dirty-tree error = %v, want it to contain the repo basename %q", err, filepath.Base(repo))
	}
	if strings.Contains(err.Error(), repo) {
		t.Fatalf("SwitchBranch dirty-tree error = %v, leaks the full repo path %q", err, repo)
	}
}

// TestDisplayRepoName covers the review fix's basename helper directly:
// user-facing branch-action messages must never leak a full filesystem
// path (an opaque OS temp dir in --mock runs, e.g.
// "/tmp/tea-dash-mock-123456/kettle").
func TestDisplayRepoName(t *testing.T) {
	cases := map[string]string{
		"/tmp/tea-dash-mock-123456/kettle": "kettle",
		"/src/tea-dash":                    "tea-dash",
		"relative/path/repo":               "repo",
		"repo":                             "repo",
		"/src/tea-dash/":                   "tea-dash",
		"":                                 "",
		"/":                                "/",
	}
	for in, want := range cases {
		if got := DisplayRepoName(in); got != want {
			t.Fatalf("DisplayRepoName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSwitchBranchWorktreeConflictFailureIsActionable(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{Stdout: "main\n", ExitCode: 0},
		{ExitCode: 128, Stderr: "fatal: 'feature/local-ops' is already checked out at '/tmp/other'"},
	}}

	_, err := SwitchBranch(context.Background(), SwitchBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err == nil || !strings.Contains(err.Error(), "another worktree") {
		t.Fatalf("SwitchBranch worktree-conflict error = %v", err)
	}
}

func TestPushBranchRunsGitPushSetUpstream(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{{ExitCode: 0}}}

	result, err := PushBranch(context.Background(), PushBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Remote:   "origin",
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("PushBranch: %v", err)
	}
	if result.RepoPath != repo || result.Branch != "feature/local-ops" || result.Remote != "origin" {
		t.Fatalf("result = %+v", result)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"push", "-u", "origin", "feature/local-ops"}},
	})
}

func TestFastForwardBranchFetchesSwitchesAndMergesUpstream(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{Stdout: "upstream/feature/local-ops\n", ExitCode: 0},
		{ExitCode: 0},
		{ExitCode: 0},
		{ExitCode: 0},
	}}

	result, err := FastForwardBranch(context.Background(), FastForwardBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("FastForwardBranch: %v", err)
	}
	if result.RepoPath != repo || result.Branch != "feature/local-ops" || result.Upstream != "upstream/feature/local-ops" || result.Remote != "upstream" {
		t.Fatalf("result = %+v", result)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"rev-parse", "--abbrev-ref", "feature/local-ops@{upstream}"}},
		{Dir: repo, Name: "git", Args: []string{"fetch", "upstream"}},
		{Dir: repo, Name: "git", Args: []string{"switch", "feature/local-ops"}},
		{Dir: repo, Name: "git", Args: []string{"merge", "--ff-only", "upstream/feature/local-ops"}},
	})
}

func TestFastForwardBranchRequiresUpstream(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{{ExitCode: 128, Stderr: "fatal: no upstream configured"}}}

	_, err := FastForwardBranch(context.Background(), FastForwardBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err == nil || !strings.Contains(err.Error(), "no upstream") {
		t.Fatalf("FastForwardBranch upstream error = %v", err)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"rev-parse", "--abbrev-ref", "feature/local-ops@{upstream}"}},
	})
}

func TestForcePushBranchUsesForceWithLease(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{Stdout: "upstream/feature/local-ops\n", ExitCode: 0},
		{ExitCode: 0},
	}}

	result, err := ForcePushBranch(context.Background(), ForcePushBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("ForcePushBranch: %v", err)
	}
	if result.RepoPath != repo || result.Branch != "feature/local-ops" || result.Remote != "upstream" {
		t.Fatalf("result = %+v", result)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"rev-parse", "--abbrev-ref", "feature/local-ops@{upstream}"}},
		{Dir: repo, Name: "git", Args: []string{"push", "--force-with-lease", "upstream", "feature/local-ops"}},
	})
}

func TestForcePushBranchFallsBackToOriginWithoutUpstream(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{ExitCode: 128, Stderr: "fatal: no upstream configured"},
		{ExitCode: 0},
	}}

	result, err := ForcePushBranch(context.Background(), ForcePushBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("ForcePushBranch: %v", err)
	}
	if result.Remote != "origin" {
		t.Fatalf("remote = %q, want origin fallback", result.Remote)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"rev-parse", "--abbrev-ref", "feature/local-ops@{upstream}"}},
		{Dir: repo, Name: "git", Args: []string{"push", "--force-with-lease", "origin", "feature/local-ops"}},
	})
}

func TestDeleteBranchRunsSafeGitBranchDelete(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{
		{Stdout: "main\n", ExitCode: 0},
		{ExitCode: 0},
	}}

	result, err := DeleteBranch(context.Background(), DeleteBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if result.RepoPath != repo || result.Branch != "feature/local-ops" {
		t.Fatalf("result = %+v", result)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"branch", "--show-current"}},
		{Dir: repo, Name: "git", Args: []string{"branch", "-d", "feature/local-ops"}},
	})
}

func TestDeleteBranchRefusesCurrentBranch(t *testing.T) {
	repo := t.TempDir()
	runner := &fakeRunner{results: []Result{{Stdout: "feature/local-ops\n", ExitCode: 0}}}

	_, err := DeleteBranch(context.Background(), DeleteBranchOptions{
		RepoPath: repo,
		Branch:   "feature/local-ops",
		Runner:   runner,
	})
	if err == nil || !strings.Contains(err.Error(), "current") {
		t.Fatalf("DeleteBranch current error = %v", err)
	}
	assertCommands(t, runner.commands, []Command{
		{Dir: repo, Name: "git", Args: []string{"branch", "--show-current"}},
	})
}

func makeGitDir(t *testing.T, remoteURL string) string {
	return makeGitDirWithRemote(t, "origin", remoteURL)
}

func makeGitDirWithRemote(t *testing.T, remoteName, remoteURL string) string {
	t.Helper()
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[remote \"" + remoteName + "\"]\n\turl = " + remoteURL + "\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

type fakeRunner struct {
	results  []Result
	err      error
	commands []Command
}

func (f *fakeRunner) Run(_ context.Context, cmd Command) (Result, error) {
	f.commands = append(f.commands, cmd)
	if f.err != nil {
		return Result{}, f.err
	}
	if len(f.results) == 0 {
		return Result{}, errors.New("unexpected command: " + strings.Join(cmd.Args, " "))
	}
	res := f.results[0]
	f.results = f.results[1:]
	return res, nil
}

func assertCommands(t *testing.T, got, want []Command) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands:\n got: %#v\nwant: %#v", got, want)
	}
}
