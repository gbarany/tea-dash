package config

import (
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

func TestSectionConfigZeroValue(t *testing.T) {
	var s SectionConfig
	s.Title = "My Pull Requests"
	if s.Title != "My Pull Requests" {
		t.Fatalf("SectionConfig.Title = %q", s.Title)
	}
}
