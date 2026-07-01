package config

import (
	"os"
	"path/filepath"
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
  view: issues
  prsLimit: 25
  issuesLimit: 40
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
`
	var c Config
	if err := yaml.Unmarshal([]byte(y), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Defaults.View != "issues" || c.Defaults.PRsLimit != 25 || c.Defaults.IssuesLimit != 40 {
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
