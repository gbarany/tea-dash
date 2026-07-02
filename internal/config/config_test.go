package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseRepoValid(t *testing.T) {
	cases := map[string]Repo{
		"gitea/tea":     {Owner: "gitea", Name: "tea"},
		"  gbarany/x  ": {Owner: "gbarany", Name: "x"},
	}
	for in, want := range cases {
		got, err := ParseRepo(in)
		if err != nil {
			t.Fatalf("ParseRepo(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Fatalf("ParseRepo(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseRepoInvalid(t *testing.T) {
	for _, in := range []string{"", "noslash", "a/b/c", "/x", "x/", "  "} {
		if _, err := ParseRepo(in); err == nil {
			t.Fatalf("ParseRepo(%q) expected an error, got nil", in)
		}
	}
}

func TestParsedRepos(t *testing.T) {
	c := &Config{Repos: []string{"gitea/tea", "gbarany/tea-dash"}}
	repos, err := c.ParsedRepos()
	if err != nil {
		t.Fatalf("ParsedRepos() error: %v", err)
	}
	if len(repos) != 2 || repos[0].String() != "gitea/tea" {
		t.Fatalf("ParsedRepos() = %v", repos)
	}
}

func TestConfigValidateRejectsBadGlobalRepo(t *testing.T) {
	cfg := &Config{Repos: []string{"acme/widgets", "owner/repo/extra"}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() should reject malformed repos entries")
	}
	if !strings.Contains(err.Error(), "repos[1]") {
		t.Fatalf("Validate() error = %v, want it to identify repos[1]", err)
	}
}

func TestUnmarshalInstance(t *testing.T) {
	const y = `
instance:
  login: work
  url: https://git.example.com
  token: abc
  insecureSkipVerify: true
  caCert: /etc/ssl/corp.pem
`
	var c Config
	if err := yaml.Unmarshal([]byte(y), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Instance.Login != "work" || c.Instance.URL != "https://git.example.com" ||
		c.Instance.Token != "abc" || !c.Instance.Insecure || c.Instance.CACert != "/etc/ssl/corp.pem" {
		t.Fatalf("instance = %+v", c.Instance)
	}
}

func TestUnmarshalSectionsAndDefaults(t *testing.T) {
	const y = `
defaults:
  view: notifications
  prsLimit: 25
  issuesLimit: 40
  notificationsLimit: 30
  actionsLimit: 20
  branchesLimit: 100
prSections:
  - title: My PRs
    filter:
      state: open
      createdBy: "@me"
  - title: Review Requested
    filter:
      reviewRequested: "@me"
issuesSections:
  - title: My Issues
    filter:
      state: open
      createdBy: "@me"
notificationsSections:
  - title: Inbox
    limit: 15
actionsSections:
  - title: CI
    repo: acme/widgets
    limit: 10
    filter:
      status: in_progress
      branch: main
      event: push
      headSha: abc123
      actor: octo
branchSections:
  - title: Local Branches
    limit: 25
localRepos:
  - name: tea-dash
    path: /path/to/tea-dash
`
	var c Config
	if err := yaml.Unmarshal([]byte(y), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Defaults.View != "notifications" || c.Defaults.PRsLimit != 25 || c.Defaults.IssuesLimit != 40 ||
		c.Defaults.NotificationsLimit != 30 || c.Defaults.ActionsLimit != 20 || c.Defaults.BranchesLimit != 100 {
		t.Fatalf("defaults = %+v", c.Defaults)
	}
	if len(c.PRSections) != 2 || c.PRSections[0].Title != "My PRs" ||
		c.PRSections[1].Filter.ReviewRequested != "@me" {
		t.Fatalf("prSections = %+v", c.PRSections)
	}
	if len(c.IssuesSections) != 1 || c.IssuesSections[0].Title != "My Issues" ||
		c.IssuesSections[0].Filter.CreatedBy != "@me" {
		t.Fatalf("issuesSections = %+v", c.IssuesSections)
	}
	if len(c.NotificationsSections) != 1 || c.NotificationsSections[0].Title != "Inbox" ||
		c.NotificationsSections[0].Limit != 15 {
		t.Fatalf("notificationsSections = %+v", c.NotificationsSections)
	}
	if len(c.ActionsSections) != 1 || c.ActionsSections[0].Title != "CI" ||
		c.ActionsSections[0].Repo != "acme/widgets" || c.ActionsSections[0].Limit != 10 {
		t.Fatalf("actionsSections = %+v", c.ActionsSections)
	}
	filter := c.ActionsSections[0].Filter
	if filter.Status != "in_progress" || filter.Branch != "main" || filter.Event != "push" ||
		filter.HeadSHA != "abc123" || filter.Actor != "octo" {
		t.Fatalf("actions filter = %+v", filter)
	}
	if len(c.BranchSections) != 1 || c.BranchSections[0].Title != "Local Branches" ||
		c.BranchSections[0].Limit != 25 {
		t.Fatalf("branchSections = %+v", c.BranchSections)
	}
	if len(c.LocalRepos) != 1 || c.LocalRepos[0].Name != "tea-dash" ||
		c.LocalRepos[0].Path != "/path/to/tea-dash" {
		t.Fatalf("localRepos = %+v", c.LocalRepos)
	}
}

func TestSmartFilteringAtLaunchDefaultsToEnabled(t *testing.T) {
	var c Config
	if !c.SmartFilteringEnabled() {
		t.Fatal("SmartFilteringEnabled should default to true when smartFilteringAtLaunch is omitted")
	}
}

func TestSmartFilteringAtLaunchCanBeDisabled(t *testing.T) {
	const y = `smartFilteringAtLaunch: false`
	var c Config
	if err := yaml.Unmarshal([]byte(y), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.SmartFilteringEnabled() {
		t.Fatal("SmartFilteringEnabled should be false when smartFilteringAtLaunch is false")
	}
}

func TestUnmarshalPagerRepoPathsAndGit(t *testing.T) {
	const y = `
pager:
  diff: delta --paging=always
repoPaths:
  acme/api: ~/src/acme-api
  acme/*: ~/src/acme/{{.Repo}}
git:
  remote: upstream
  prBranchTemplate: review/{{.Owner}}-{{.Repo}}-{{.PrIndex}}
`
	var c Config
	if err := yaml.Unmarshal([]byte(y), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Pager.Diff != "delta --paging=always" {
		t.Fatalf("pager.diff = %q", c.Pager.Diff)
	}
	if c.RepoPaths["acme/api"] != "~/src/acme-api" || c.RepoPaths["acme/*"] != "~/src/acme/{{.Repo}}" {
		t.Fatalf("repoPaths = %+v", c.RepoPaths)
	}
	if c.Git.Remote != "upstream" || c.Git.PRBranchTemplate != "review/{{.Owner}}-{{.Repo}}-{{.PrIndex}}" {
		t.Fatalf("git = %+v", c.Git)
	}
}

func TestUnmarshalKeybindings(t *testing.T) {
	const y = `
keybindings:
  universal:
    - key: tab
      builtin: nextSection
  prs:
    - key: O
      builtin: checkout
    - key: g
      name: lazygit
      command: cd {{.RepoPath}} && lazygit
  issues:
    - key: i
      command: echo {{.IssueNumber}}
    - key: M
      builtin: setMilestone
  notifications:
    - key: D
      builtin: markAllRead
  actions:
    - key: a
      command: echo {{.RunID}}
  branches:
    - key: B
      command: git -C {{.RepoPath}} status
`
	var c Config
	if err := yaml.Unmarshal([]byte(y), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(c.Keybindings.Universal) != 1 || c.Keybindings.Universal[0].Key != "tab" ||
		c.Keybindings.Universal[0].Builtin != "nextSection" {
		t.Fatalf("universal keybindings = %+v", c.Keybindings.Universal)
	}
	if len(c.Keybindings.PRs) != 2 || c.Keybindings.PRs[1].Name != "lazygit" ||
		!strings.Contains(c.Keybindings.PRs[1].Command, "lazygit") {
		t.Fatalf("prs keybindings = %+v", c.Keybindings.PRs)
	}
	if c.Keybindings.Notifications[0].Builtin != "markAllRead" ||
		c.Keybindings.Actions[0].Key != "a" ||
		c.Keybindings.Branches[0].Command == "" {
		t.Fatalf("keybindings = %+v", c.Keybindings)
	}
}

func TestConfigValidateKeybindingsRequireKeyAndAction(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{
			name: "missing key",
			cfg: Config{Keybindings: Keybindings{Universal: []Keybinding{{
				Builtin: "refresh",
			}}}},
		},
		{
			name: "missing builtin and command",
			cfg: Config{Keybindings: Keybindings{PRs: []Keybinding{{
				Key: "g",
			}}}},
		},
		{
			name: "both builtin and command",
			cfg: Config{Keybindings: Keybindings{PRs: []Keybinding{{
				Key: "g", Builtin: "checkout", Command: "lazygit",
			}}}},
		},
		{
			name: "unsupported scoped builtin",
			cfg: Config{Keybindings: Keybindings{Issues: []Keybinding{{
				Key: "m", Builtin: "merge",
			}}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err == nil {
				t.Fatal("Validate() should reject the invalid keybinding")
			}
		})
	}

	ok := Config{Keybindings: Keybindings{Universal: []Keybinding{
		{Key: "R", Builtin: "refreshAll"},
		{Key: "t", Builtin: "toggleSmartFiltering"},
		{Key: "g", Command: "lazygit"},
	}, PRs: []Keybinding{
		{Key: "a", Builtin: "assign"},
		{Key: "L", Builtin: "addLabel"},
		{Key: "u", Builtin: "update"},
		{Key: "W", Builtin: "ready"},
		{Key: "D", Builtin: "draft"},
	}, Issues: []Keybinding{
		{Key: "A", Builtin: "unassign"},
		{Key: "U", Builtin: "removeLabel"},
	}}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate() rejected valid keybindings: %v", err)
	}
}

func TestPagerDiffCommandDefaults(t *testing.T) {
	t.Setenv("PAGER", "bat --plain")
	if got := (Pager{}).DiffCommand(); got != "bat --plain" {
		t.Fatalf("DiffCommand() = %q, want env pager", got)
	}
	t.Setenv("PAGER", "")
	if got := (Pager{}).DiffCommand(); got != "less -R" {
		t.Fatalf("DiffCommand() = %q, want less -R fallback", got)
	}
	if got := (Pager{Diff: "delta"}).DiffCommand(); got != "delta" {
		t.Fatalf("DiffCommand() = %q, want configured command", got)
	}
}

func TestGitDefaults(t *testing.T) {
	var g Git
	if got := g.RemoteName(); got != "origin" {
		t.Fatalf("RemoteName() = %q, want origin", got)
	}
	if got := g.BranchTemplate(); got != "pr-{{.PrIndex}}" {
		t.Fatalf("BranchTemplate() = %q, want default template", got)
	}
	g = Git{Remote: "upstream", PRBranchTemplate: "review/{{.PrIndex}}"}
	if got := g.RemoteName(); got != "upstream" {
		t.Fatalf("RemoteName() = %q, want upstream", got)
	}
	if got := g.BranchTemplate(); got != "review/{{.PrIndex}}" {
		t.Fatalf("BranchTemplate() = %q, want configured template", got)
	}
}

func TestMatchRepoPathExactBeforeWildcardAndExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	paths := map[string]string{
		"acme/*":   "~/src/acme/{{.Repo}}",
		"acme/api": "~/src/exact",
	}
	got, ok, err := MatchRepoPath("acme/api", paths)
	if err != nil {
		t.Fatalf("MatchRepoPath exact: %v", err)
	}
	if !ok {
		t.Fatal("MatchRepoPath exact did not match")
	}
	if want := filepath.Join(home, "src", "exact"); got != want {
		t.Fatalf("MatchRepoPath exact = %q, want %q", got, want)
	}

	got, ok, err = MatchRepoPath("acme/web", paths)
	if err != nil {
		t.Fatalf("MatchRepoPath wildcard: %v", err)
	}
	if !ok {
		t.Fatal("MatchRepoPath wildcard did not match")
	}
	if want := filepath.Join(home, "src", "acme", "web"); got != want {
		t.Fatalf("MatchRepoPath wildcard = %q, want %q", got, want)
	}
}

func TestMatchRepoPathRejectsBadWildcard(t *testing.T) {
	_, _, err := MatchRepoPath("acme/api", map[string]string{"acme/[": "/tmp/repo"})
	if err == nil || !strings.Contains(err.Error(), "acme/[") {
		t.Fatalf("MatchRepoPath bad wildcard error = %v", err)
	}
}

func TestFilterValidateRejectsNonMe(t *testing.T) {
	if err := (PrIssueFilter{CreatedBy: "alice"}).Validate(); err == nil {
		t.Fatal("Validate() should reject a plain login (only \"@me\" is supported)")
	}
	if err := (PrIssueFilter{CreatedBy: "@me"}).Validate(); err != nil {
		t.Fatalf("Validate() rejected \"@me\": %v", err)
	}
}

func TestConfigValidateBadView(t *testing.T) {
	if err := (&Config{Defaults: Defaults{View: "nope"}}).Validate(); err == nil {
		t.Fatal("Validate() should reject an unknown defaults.view")
	}
	for _, view := range []string{"", "prs", "issues", "notifications", "actions", "branches"} {
		if err := (&Config{Defaults: Defaults{View: view}}).Validate(); err != nil {
			t.Fatalf("Validate() rejected valid view %q: %v", view, err)
		}
	}
}

func TestConfigValidateRejectsLocalRepoWithoutPath(t *testing.T) {
	cfg := &Config{LocalRepos: []LocalRepoConfig{{Name: "missing"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject a local repo without a path")
	}
}

func TestConfigValidateRejectsBadSectionFilter(t *testing.T) {
	cfg := &Config{
		PRSections: []SectionConfig{
			{Title: "Bad", Filter: PrIssueFilter{CreatedBy: "alice"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject a section with a non-@me author filter")
	}
}

func TestConfigValidateAllowsRepoScopedLoginFilters(t *testing.T) {
	cfg := &Config{
		IssuesSections: []SectionConfig{
			{Title: "Alice bugs", Repo: "acme/widgets", Filter: PrIssueFilter{CreatedBy: "alice"}},
		},
		PRSections: []SectionConfig{
			{Title: "Assigned PRs", Repo: "acme/widgets", Filter: PrIssueFilter{AssignedBy: "alice"}},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() should allow plain login filters on repo-scoped sections: %v", err)
	}
}

func TestConfigValidateAllowsGlobalReposLoginFilters(t *testing.T) {
	cfg := &Config{
		Repos: []string{"acme/widgets", "acme/api"},
		IssuesSections: []SectionConfig{
			{Title: "Alice bugs", Filter: PrIssueFilter{CreatedBy: "alice"}},
		},
		PRSections: []SectionConfig{
			{Title: "Assigned PRs", Filter: PrIssueFilter{AssignedBy: "alice"}},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() should allow plain login filters when global repos fan-out is configured: %v", err)
	}
}

func TestConfigValidateRejectsBadRepoScopedRepo(t *testing.T) {
	cfg := &Config{
		PRSections: []SectionConfig{
			{Title: "Bad repo", Repo: "owner/repo/extra"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject malformed prSections.repo")
	}
}

func TestConfigValidateRejectsRepoScopedReviewRequested(t *testing.T) {
	cfg := &Config{
		PRSections: []SectionConfig{
			{Title: "Needs review", Repo: "acme/widgets", Filter: PrIssueFilter{ReviewRequested: "@me"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() should reject repo-scoped reviewRequested because the repo endpoint cannot express it")
	}
}

func TestConfigValidateRejectsBadActionRepo(t *testing.T) {
	if err := (&Config{ActionsSections: []SectionConfig{{
		Title: "Actions",
		Repo:  "owner/repo/extra",
	}}}).Validate(); err == nil {
		t.Fatal("Validate() should reject malformed actionsSections.repo")
	}
	if err := (&Config{ActionsSections: []SectionConfig{{
		Title: "Actions",
	}}}).Validate(); err != nil {
		t.Fatalf("Validate() should allow a blank actions repo for the empty state: %v", err)
	}
}

func TestLoadMissingFileIsEmptyConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned a nil *Config")
	}
	if len(cfg.Repos) != 0 || cfg.Login != "" || cfg.Instance != (Instance{}) {
		t.Fatalf("Load() = %+v, want an empty config", cfg)
	}
}

func TestLoadMalformedFileErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p := filepath.Join(dir, "tea-dash", "config.yml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("instance:\n  url: \"unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected a parse error for malformed config YAML")
	}
}
