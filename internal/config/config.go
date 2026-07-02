// Package config loads tea-dash's user configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

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
	// PRSections, IssuesSections, NotificationsSections, and ActionsSections
	// configure the tabs for their respective views. Empty falls back to a
	// default section.
	PRSections            []SectionConfig `yaml:"prSections"`
	IssuesSections        []SectionConfig `yaml:"issuesSections"`
	NotificationsSections []SectionConfig `yaml:"notificationsSections"`
	ActionsSections       []SectionConfig `yaml:"actionsSections"`
	// Defaults sets the startup view and per-view row limits.
	Defaults Defaults `yaml:"defaults"`
	// Pager configures external pager commands.
	Pager Pager `yaml:"pager"`
	// RepoPaths maps repo names or wildcard patterns (for example "fcmb/*")
	// to local checkout paths.
	RepoPaths map[string]string `yaml:"repoPaths"`
	// Git configures local git checkout behavior.
	Git Git `yaml:"git"`
}

// Defaults holds startup and limit defaults. Per-view limits set the row-fetch
// cap used when a section omits its own Limit. Precedence: section Limit ->
// per-view default -> 50.
type Defaults struct {
	View               string `yaml:"view"` // "prs" | "issues" | "notifications" | "actions"
	PRsLimit           int    `yaml:"prsLimit"`
	IssuesLimit        int    `yaml:"issuesLimit"`
	NotificationsLimit int    `yaml:"notificationsLimit"`
	ActionsLimit       int    `yaml:"actionsLimit"`
}

// Pager configures external pager commands.
type Pager struct {
	Diff string `yaml:"diff"`
}

// DiffCommand returns the configured diff pager command, then $PAGER, then the
// less fallback that preserves ANSI color.
func (p Pager) DiffCommand() string {
	if diff := strings.TrimSpace(p.Diff); diff != "" {
		return diff
	}
	if pager := strings.TrimSpace(os.Getenv("PAGER")); pager != "" {
		return pager
	}
	return "less -R"
}

// Git configures local git checkout behavior.
type Git struct {
	Remote           string `yaml:"remote"`
	PRBranchTemplate string `yaml:"prBranchTemplate"`
}

// RemoteName returns the configured remote name or the origin default.
func (g Git) RemoteName() string {
	if remote := strings.TrimSpace(g.Remote); remote != "" {
		return remote
	}
	return "origin"
}

// BranchTemplate returns the configured PR branch template or the default.
func (g Git) BranchTemplate() string {
	if tmpl := strings.TrimSpace(g.PRBranchTemplate); tmpl != "" {
		return tmpl
	}
	return "pr-{{.PrIndex}}"
}

// Instance selects and overrides the Gitea connection.
type Instance struct {
	Login        string `yaml:"login"`              // pick a named tea login
	URL          string `yaml:"url"`                // override instance URL
	Token        string `yaml:"token"`              // literal token
	TokenCommand string `yaml:"tokenCommand"`       // command whose stdout is the token (e.g. `op read ...`)
	TokenEnv     string `yaml:"tokenEnv"`           // name of an env var holding the token
	Insecure     bool   `yaml:"insecureSkipVerify"` // disable TLS verification
	CACert       string `yaml:"caCert"`             // path to a private CA bundle
}

// SectionConfig describes one dashboard section (a tab).
type SectionConfig struct {
	Title  string        `yaml:"title"`
	Repo   string        `yaml:"repo"`
	Filter PrIssueFilter `yaml:"filter"`
	// Limit caps this section's row fetch. 0 falls back to the per-view default
	// (defaults.prsLimit / defaults.issuesLimit / etc.), which in turn falls back to 50.
	// Precedence: section Limit -> per-view default -> 50.
	Limit int `yaml:"limit"`
}

// PrIssueFilter is the structured, config-driven filter for one section. Every
// field is optional; the zero value means "unconstrained" except State, which
// defaults to "open". Me-scoped fields take the sentinel "@me".
type PrIssueFilter struct {
	State     string   `yaml:"state"`     // open | closed | all (default open)
	Type      string   `yaml:"-"`         // pulls | issues (set by the section)
	Labels    []string `yaml:"labels"`    // label names (AND-ed via the search endpoint)
	Milestone string   `yaml:"milestone"` // milestone name
	// The me-scoped author fields accept "@me" only (a plain login is not
	// supported by the cross-repo search endpoint, which has no per-login author
	// filter). Validate rejects any other non-empty value.
	CreatedBy       string `yaml:"createdBy"`       // "@me" only
	AssignedBy      string `yaml:"assignedBy"`      // "@me" only
	Mentioned       string `yaml:"mentioned"`       // "@me" only
	ReviewRequested string `yaml:"reviewRequested"` // "@me" only (PRs only)
	Since           string `yaml:"since"`           // RFC3339 lower bound on updatedAt
	Sort            string `yaml:"sort"`            // e.g. recentupdate
	Q               string `yaml:"-"`               // live keyword (set by "/", never persisted)

	// Repo-scoped Actions filters. These are ignored by PR/issue search and are
	// mapped to Gitea's actions/runs query params by the Actions view.
	Status  string `yaml:"status"`
	Branch  string `yaml:"branch"`
	Event   string `yaml:"event"`
	HeadSHA string `yaml:"headSha"`
	Actor   string `yaml:"actor"`
}

// Validate rejects unsupported me-scoped author values. The cross-repo search
// endpoint has no per-login author filter, so CreatedBy/AssignedBy/Mentioned/
// ReviewRequested may only be empty or the "@me" sentinel.
func (f PrIssueFilter) Validate() error {
	for _, field := range []struct {
		name string
		val  string
	}{
		{"createdBy", f.CreatedBy},
		{"assignedBy", f.AssignedBy},
		{"mentioned", f.Mentioned},
		{"reviewRequested", f.ReviewRequested},
	} {
		if field.val != "" && field.val != "@me" {
			return fmt.Errorf("filter.%s = %q: only \"@me\" is supported (the cross-repo search endpoint has no per-login author filter)", field.name, field.val)
		}
	}
	return nil
}

// Validate checks the config for unsupported filter values and an invalid
// default view, returning the first error found.
func (c *Config) Validate() error {
	for _, s := range c.PRSections {
		if err := s.Filter.Validate(); err != nil {
			return err
		}
	}
	for _, s := range c.IssuesSections {
		if err := s.Filter.Validate(); err != nil {
			return err
		}
	}
	for _, s := range c.NotificationsSections {
		if err := s.Filter.Validate(); err != nil {
			return err
		}
	}
	for _, s := range c.ActionsSections {
		if strings.TrimSpace(s.Repo) == "" {
			continue
		}
		if _, err := ParseRepo(s.Repo); err != nil {
			return fmt.Errorf("actionsSections.repo: %w", err)
		}
	}
	switch c.Defaults.View {
	case "", "prs", "issues", "notifications", "actions":
	default:
		return fmt.Errorf("defaults.view = %q: want \"prs\", \"issues\", \"notifications\", or \"actions\"", c.Defaults.View)
	}
	return nil
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

// ExpandPath expands a leading "~" or "~/" to the current user's home
// directory. Other paths are returned unchanged.
func ExpandPath(p string) (string, error) {
	switch {
	case p == "~":
		return os.UserHomeDir()
	case strings.HasPrefix(p, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[2:]), nil
	default:
		return p, nil
	}
}

// MatchRepoPath returns the path mapped for repoName, preferring an exact key
// before evaluating wildcard patterns such as "fcmb/*". Mapped paths may use
// {{.Owner}}, {{.Repo}}, and {{.RepoName}} template variables, and are expanded
// through ExpandPath before being returned.
func MatchRepoPath(repoName string, repoPaths map[string]string) (string, bool, error) {
	repoName = strings.TrimSpace(repoName)
	if repoName == "" || len(repoPaths) == 0 {
		return "", false, nil
	}
	if p, ok := repoPaths[repoName]; ok {
		expanded, err := expandRepoPathMapping(p, repoName)
		return expanded, true, err
	}

	patterns := make([]string, 0, len(repoPaths))
	for pattern := range repoPaths {
		if strings.ContainsAny(pattern, "*?[") {
			patterns = append(patterns, pattern)
		}
	}
	sort.Strings(patterns)
	for _, pattern := range patterns {
		ok, err := path.Match(pattern, repoName)
		if err != nil {
			return "", false, fmt.Errorf("repoPaths pattern %q: %w", pattern, err)
		}
		if !ok {
			continue
		}
		expanded, err := expandRepoPathMapping(repoPaths[pattern], repoName)
		return expanded, true, err
	}
	return "", false, nil
}

func expandRepoPathMapping(p, repoName string) (string, error) {
	if strings.Contains(p, "{{") {
		r, err := ParseRepo(repoName)
		if err != nil {
			return "", err
		}
		tmpl, err := template.New("repoPath").Option("missingkey=error").Parse(p)
		if err != nil {
			return "", fmt.Errorf("repoPaths template %q: %w", p, err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, struct {
			Owner    string
			Repo     string
			RepoName string
		}{
			Owner:    r.Owner,
			Repo:     r.Name,
			RepoName: repoName,
		}); err != nil {
			return "", fmt.Errorf("repoPaths template %q: %w", p, err)
		}
		p = buf.String()
	}
	return ExpandPath(p)
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
