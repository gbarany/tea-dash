// Package config loads tea-dash's user configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the user configuration for tea-dash.
type Config struct {
	// Instance overrides / selects the Gitea login (else tea's config is reused).
	Instance Instance `yaml:"instance"`
	// Login is a deprecated alias for Instance.Login (tea login profile name).
	Login string `yaml:"login"`
	// Repos lists repositories to watch. Unused in M0; per-repo sections
	// return in M1.
	Repos []string `yaml:"repos"`
	// PRSections and IssuesSections configure the tabs for the pulls and issues
	// views respectively. Empty falls back to a single me-scoped default section.
	PRSections     []SectionConfig `yaml:"prSections"`
	IssuesSections []SectionConfig `yaml:"issuesSections"`
	// Defaults sets the startup view and per-view row limits.
	Defaults Defaults `yaml:"defaults"`
}

// Defaults holds startup and limit defaults.
type Defaults struct {
	View        string `yaml:"view"` // "prs" | "issues"
	PRsLimit    int    `yaml:"prsLimit"`
	IssuesLimit int    `yaml:"issuesLimit"`
}

// Instance selects and overrides the Gitea connection.
type Instance struct {
	Login    string `yaml:"login"`              // pick a named tea login
	URL      string `yaml:"url"`                // override instance URL
	Token    string `yaml:"token"`              // override token
	Insecure bool   `yaml:"insecureSkipVerify"` // disable TLS verification
	CACert   string `yaml:"caCert"`             // path to a private CA bundle
}

// SectionConfig describes one dashboard section (a tab).
type SectionConfig struct {
	Title  string        `yaml:"title"`
	Filter PrIssueFilter `yaml:"filter"`
	Limit  int           `yaml:"limit"` // 0 -> defaults
}

// PrIssueFilter is the structured, config-driven filter for one section. Every
// field is optional; the zero value means "unconstrained" except State, which
// defaults to "open". Me-scoped fields take a login or the sentinel "@me".
type PrIssueFilter struct {
	State           string   `yaml:"state"`           // open | closed | all (default open)
	Type            string   `yaml:"type"`            // pulls | issues (set by the section)
	Labels          []string `yaml:"labels"`          // label names (AND-ed via the search endpoint)
	Milestone       string   `yaml:"milestone"`       // milestone name
	CreatedBy       string   `yaml:"createdBy"`       // login or "@me"
	AssignedBy      string   `yaml:"assignedBy"`      // login or "@me"
	Mentioned       string   `yaml:"mentioned"`       // login or "@me"
	ReviewRequested string   `yaml:"reviewRequested"` // login or "@me" (PRs only)
	Since           string   `yaml:"since"`           // RFC3339 lower bound on updatedAt
	Sort            string   `yaml:"sort"`            // e.g. recentupdate
	Q               string   `yaml:"-"`               // live keyword (set by "/", never persisted)
}

// WithDefaults fills the section-driven Type and the "open" State default,
// leaving user-set scope fields untouched.
func (f PrIssueFilter) WithDefaults(defaultType string) PrIssueFilter {
	if f.State == "" {
		f.State = "open"
	}
	if f.Type == "" {
		f.Type = defaultType
	}
	return f
}

// Repo is a parsed owner/name repository reference.
type Repo struct {
	Owner string
	Name  string
}

func (r Repo) String() string { return r.Owner + "/" + r.Name }

// ParseRepo parses an "owner/name" string.
func ParseRepo(s string) (Repo, error) {
	owner, name, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return Repo{}, fmt.Errorf("invalid repo %q, want \"owner/name\"", s)
	}
	return Repo{Owner: owner, Name: name}, nil
}

// ParsedRepos returns the configured repos parsed into Repo values.
func (c *Config) ParsedRepos() ([]Repo, error) {
	repos := make([]Repo, 0, len(c.Repos))
	for _, s := range c.Repos {
		r, err := ParseRepo(s)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, nil
}

// Path returns the config file path:
// $XDG_CONFIG_HOME/tea-dash/config.yml (falling back to ~/.config).
func Path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "tea-dash", "config.yml"), nil
}

// Load reads the config file. A missing file is not an error: it returns an
// empty Config so tea-dash can fall back to the repository in $PWD.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}
