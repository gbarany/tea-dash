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
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the user configuration for tea-dash.
type Config struct {
	// Include lists YAML files to load before this file. Paths are relative to
	// the file declaring them unless they are absolute or start with "~".
	Include []string `yaml:"include"`
	// Instance overrides / selects the Gitea login (else tea's config is reused).
	Instance Instance `yaml:"instance"`
	// Login is a deprecated alias for Instance.Login (tea login profile name).
	Login string `yaml:"login"`
	// Repos lists remote Gitea repositories to watch. A PR/issue section with no
	// explicit Repo fans out across this list; with no Repos, it falls back to
	// the instance-wide cross-repo search endpoint.
	Repos []string `yaml:"repos"`
	// SmartFilteringAtLaunch scopes PR/issue sections with no explicit repo to
	// the git repository tea-dash was launched from, when that checkout's remote
	// host matches the configured Gitea instance. Nil means enabled.
	SmartFilteringAtLaunch *bool `yaml:"smartFilteringAtLaunch"`
	// ConfirmQuit asks before quitting with q/ctrl+c. Nil means disabled,
	// matching gh-dash's default confirmQuit: false behavior.
	ConfirmQuit *bool `yaml:"confirmQuit"`
	// LocalRepos lists local git repository paths for read-only branch status.
	LocalRepos []LocalRepoConfig `yaml:"localRepos"`
	// PRSections, IssuesSections, NotificationsSections, ActionsSections, and BranchSections
	// configure tabs for their respective views. Empty falls back to a default
	// section.
	PRSections            []SectionConfig `yaml:"prSections"`
	IssuesSections        []SectionConfig `yaml:"issuesSections"`
	NotificationsSections []SectionConfig `yaml:"notificationsSections"`
	ActionsSections       []SectionConfig `yaml:"actionsSections"`
	BranchSections        []SectionConfig `yaml:"branchSections"`
	// Defaults sets startup behavior, optional auto-refresh, and per-view row limits.
	Defaults Defaults `yaml:"defaults"`
	// Pager configures external pager commands.
	Pager Pager `yaml:"pager"`
	// RepoPaths maps repo names or wildcard patterns (for example "acme/*")
	// to local checkout paths.
	RepoPaths map[string]string `yaml:"repoPaths"`
	// Git configures local git checkout behavior.
	Git Git `yaml:"git"`
	// Keybindings overrides built-in keys and adds custom shell commands.
	Keybindings Keybindings `yaml:"keybindings"`
	// Theme customizes core UI colors. The shape intentionally mirrors gh-dash's
	// theme.colors block for portable terminal color schemes.
	Theme Theme `yaml:"theme"`
}

// SmartFilteringEnabled reports whether cwd repository scoping is enabled at
// startup. It defaults on, matching gh-dash's repo-aware launch behavior.
func (c *Config) SmartFilteringEnabled() bool {
	if c == nil || c.SmartFilteringAtLaunch == nil {
		return true
	}
	return *c.SmartFilteringAtLaunch
}

// ConfirmQuitEnabled reports whether tea-dash should ask before quitting. It
// defaults off, matching gh-dash.
func (c *Config) ConfirmQuitEnabled() bool {
	if c == nil || c.ConfirmQuit == nil {
		return false
	}
	return *c.ConfirmQuit
}

// Defaults holds startup and limit defaults. Per-view limits set the row-fetch
// cap used when a section omits its own Limit. Precedence: section Limit ->
// per-view default -> 50.
type Defaults struct {
	View                     string        `yaml:"view"` // "prs" | "issues" | "notifications" | "actions" | "branches"
	Preview                  PreviewConfig `yaml:"preview"`
	RefetchIntervalMinutes   int           `yaml:"refetchIntervalMinutes"`
	PRsLimit                 int           `yaml:"prsLimit"`
	IssuesLimit              int           `yaml:"issuesLimit"`
	NotificationsLimit       int           `yaml:"notificationsLimit"`
	IncludeReadNotifications *bool         `yaml:"includeReadNotifications"`
	ActionsLimit             int           `yaml:"actionsLimit"`
	BranchesLimit            int           `yaml:"branchesLimit"`
}

// PreviewConfig controls the side preview pane. Open is a pointer so an omitted
// value can preserve tea-dash's default-open behavior while open: false remains
// expressible.
type PreviewConfig struct {
	Open  *bool `yaml:"open"`
	Width int   `yaml:"width"`
}

// PreviewOpen returns the configured initial preview state. Omitted open
// defaults to true.
func (p PreviewConfig) PreviewOpen() bool {
	if p.Open == nil {
		return true
	}
	return *p.Open
}

// PreviewWidth returns the configured preview width. 0 means automatic width.
func (p PreviewConfig) PreviewWidth() int {
	if p.Width < 0 {
		return 0
	}
	return p.Width
}

// IncludeReadNotificationsEnabled reports whether notification sections with no
// explicit status filter should include read notifications. It defaults to true
// to match gh-dash and GitHub's notification view; configuring false restores
// an unread/pinned-only inbox.
func (d Defaults) IncludeReadNotificationsEnabled() bool {
	if d.IncludeReadNotifications == nil {
		return true
	}
	return *d.IncludeReadNotifications
}

// RefetchInterval returns the configured automatic refetch interval. A zero or
// negative value disables background refreshes, preserving manual-refresh-only
// behavior unless the user explicitly opts in.
func (d Defaults) RefetchInterval() time.Duration {
	if d.RefetchIntervalMinutes <= 0 {
		return 0
	}
	return time.Duration(d.RefetchIntervalMinutes) * time.Minute
}

// Pager configures external pager commands.
type Pager struct {
	Diff string `yaml:"diff"`
}

// Theme customizes tea-dash's visual styling.
type Theme struct {
	// Icons selects the glyph set used for state indicators (rows, previews,
	// tabs): "unicode" (default), "nerd", or "ascii". Empty means "unicode".
	Icons  string      `yaml:"icons"`
	Colors ThemeColors `yaml:"colors"`
}

// ThemeColors groups text, background, border, and state color overrides.
type ThemeColors struct {
	Text       ThemeTextColors       `yaml:"text"`
	Background ThemeBackgroundColors `yaml:"background"`
	Border     ThemeBorderColors     `yaml:"border"`
	State      ThemeStateColors      `yaml:"state"`
}

// ThemeStateColors customizes the colors used for PR/issue/CI state
// indicators. The shape mirrors gh-dash-style state coloring; unset fields
// fall back to tea-dash's built-in gh-convention defaults.
type ThemeStateColors struct {
	Open    string `yaml:"open"`
	Draft   string `yaml:"draft"`
	Merged  string `yaml:"merged"`
	Closed  string `yaml:"closed"`
	Success string `yaml:"success"`
	Failure string `yaml:"failure"`
	Running string `yaml:"running"`
	Neutral string `yaml:"neutral"`
}

// ThemeTextColors customizes foreground colors used by the TUI.
type ThemeTextColors struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
	Inverted  string `yaml:"inverted"`
	Faint     string `yaml:"faint"`
	Warning   string `yaml:"warning"`
	Success   string `yaml:"success"`
	Actor     string `yaml:"actor"`
}

// ThemeBackgroundColors customizes background colors used by the TUI.
type ThemeBackgroundColors struct {
	Selected string `yaml:"selected"`
}

// ThemeBorderColors customizes border colors used by the TUI.
type ThemeBorderColors struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
	Faint     string `yaml:"faint"`
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
	Remote              string `yaml:"remote"`
	PRBranchTemplate    string `yaml:"prBranchTemplate"`
	IssueBranchTemplate string `yaml:"issueBranchTemplate"`
}

// Keybindings groups configurable bindings by scope. Universal bindings are
// active in every view; scoped bindings apply only while that view is active.
type Keybindings struct {
	Universal     []Keybinding `yaml:"universal"`
	PRs           []Keybinding `yaml:"prs"`
	Issues        []Keybinding `yaml:"issues"`
	Notifications []Keybinding `yaml:"notifications"`
	Actions       []Keybinding `yaml:"actions"`
	Branches      []Keybinding `yaml:"branches"`
}

// Keybinding is one user-configured key entry. Builtin remaps a native command;
// Command runs a shell command using the selected row as template context.
type Keybinding struct {
	Key     string `yaml:"key"`
	Builtin string `yaml:"builtin"`
	Command string `yaml:"command"`
	Name    string `yaml:"name"`
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

// IssueBranchTemplateOrDefault returns the configured issue branch template or
// the default.
func (g Git) IssueBranchTemplateOrDefault() string {
	if tmpl := strings.TrimSpace(g.IssueBranchTemplate); tmpl != "" {
		return tmpl
	}
	return "issue-{{.IssueIndex}}"
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
	// Columns optionally selects and orders visible table columns for PR/issue
	// sections. Entries may be YAML strings or objects with name/title/width.
	Columns []ColumnConfig `yaml:"columns"`
	// Limit caps this section's row fetch. 0 falls back to the per-view default
	// (defaults.prsLimit / defaults.issuesLimit / etc.), which in turn falls back to 50.
	// Precedence: section Limit -> per-view default -> 50.
	Limit int `yaml:"limit"`
}

// ColumnConfig selects one visible PR/issue table column.
type ColumnConfig struct {
	Name  string `yaml:"name"`
	Title string `yaml:"title"`
	Width int    `yaml:"width"`
}

var prIssueColumnNames = []string{"number", "title", "repo", "author", "state", "updated"}

var prIssueColumnNameSet = func() map[string]struct{} {
	out := make(map[string]struct{}, len(prIssueColumnNames))
	for _, name := range prIssueColumnNames {
		out[name] = struct{}{}
	}
	return out
}()

// UnmarshalYAML accepts either a terse string entry ("title") or an object:
// {name: title, width: 42, title: Summary}.
func (c *ColumnConfig) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		c.Name = strings.TrimSpace(value.Value)
		return nil
	case yaml.MappingNode:
		type rawColumnConfig ColumnConfig
		var raw rawColumnConfig
		if err := value.Decode(&raw); err != nil {
			return err
		}
		*c = ColumnConfig(raw)
		c.Name = strings.TrimSpace(c.Name)
		c.Title = strings.TrimSpace(c.Title)
		return nil
	default:
		return fmt.Errorf("column must be a string or mapping")
	}
}

// LocalRepoConfig describes one local git checkout to include in the branches
// view. Name is optional; Path must point at a git repository worktree.
type LocalRepoConfig struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
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

// Validate rejects unsupported cross-repo author values. The cross-repo search
// endpoint has no per-login author filter, so CreatedBy/AssignedBy/Mentioned/
// ReviewRequested may only be empty or the "@me" sentinel.
func (f PrIssueFilter) Validate() error {
	return f.validate(false)
}

// ValidateForRepo validates a filter in the context of a section's optional
// repo. Repo-scoped sections can use per-login CreatedBy/AssignedBy/Mentioned
// filters because the repo issues endpoint supports them.
func (f PrIssueFilter) ValidateForRepo(repo string) error {
	return f.validate(strings.TrimSpace(repo) != "")
}

func (f PrIssueFilter) validate(repoScoped bool) error {
	for _, field := range []struct {
		name string
		val  string
	}{
		{"createdBy", f.CreatedBy},
		{"assignedBy", f.AssignedBy},
		{"mentioned", f.Mentioned},
	} {
		if field.val != "" && field.val != "@me" && !repoScoped {
			return fmt.Errorf("filter.%s = %q: only \"@me\" is supported (the cross-repo search endpoint has no per-login author filter)", field.name, field.val)
		}
	}
	if repoScoped && f.ReviewRequested != "" {
		return fmt.Errorf("filter.reviewRequested is only supported on cross-repo sections")
	}
	if !repoScoped && f.ReviewRequested != "" && f.ReviewRequested != "@me" {
		return fmt.Errorf("filter.reviewRequested = %q: only \"@me\" is supported", f.ReviewRequested)
	}
	return nil
}

func validatePrIssueSection(kind string, idx int, s SectionConfig, hasGlobalRepos bool) error {
	if kind == "issuesSections" && strings.TrimSpace(s.Filter.ReviewRequested) != "" {
		return fmt.Errorf("%s.filter.reviewRequested is only supported for PR sections", kind)
	}
	hasSectionRepo := strings.TrimSpace(s.Repo) != ""
	if hasSectionRepo {
		if _, err := ParseRepo(s.Repo); err != nil {
			return fmt.Errorf("%s[%d].repo: %w", kind, idx, err)
		}
	}
	if err := validateSectionColumns(kind, idx, s.Columns); err != nil {
		return err
	}
	repoScoped := hasSectionRepo || (hasGlobalRepos && strings.TrimSpace(s.Filter.ReviewRequested) == "")
	if err := s.Filter.validate(repoScoped); err != nil {
		return fmt.Errorf("%s[%d].%w", kind, idx, err)
	}
	return nil
}

func validateSectionColumns(kind string, sectionIndex int, columns []ColumnConfig) error {
	for i, col := range columns {
		name := strings.TrimSpace(col.Name)
		if name == "" {
			return fmt.Errorf("%s[%d].columns[%d].name is required", kind, sectionIndex, i)
		}
		if _, ok := prIssueColumnNameSet[name]; !ok {
			return fmt.Errorf("%s[%d].columns[%d].name = %q: supported columns are %s", kind, sectionIndex, i, col.Name, strings.Join(prIssueColumnNames, ", "))
		}
		if col.Width < 0 {
			return fmt.Errorf("%s[%d].columns[%d].width must be >= 0", kind, sectionIndex, i)
		}
	}
	return nil
}

func rejectUnsupportedColumns(kind string, sectionIndex int, columns []ColumnConfig) error {
	if len(columns) == 0 {
		return nil
	}
	return fmt.Errorf("%s[%d].columns is only supported for prSections and issuesSections", kind, sectionIndex)
}

// Validate checks the config for unsupported filter values and an invalid
// default view, returning the first error found.
func (c *Config) Validate() error {
	for i, repo := range c.Repos {
		if _, err := ParseRepo(repo); err != nil {
			return fmt.Errorf("repos[%d]: %w", i, err)
		}
	}
	for i, s := range c.PRSections {
		if err := validatePrIssueSection("prSections", i, s, len(c.Repos) > 0); err != nil {
			return err
		}
	}
	for i, s := range c.IssuesSections {
		if err := validatePrIssueSection("issuesSections", i, s, len(c.Repos) > 0); err != nil {
			return err
		}
	}
	for i, s := range c.NotificationsSections {
		if err := rejectUnsupportedColumns("notificationsSections", i, s.Columns); err != nil {
			return err
		}
		if err := s.Filter.Validate(); err != nil {
			return err
		}
	}
	for i, s := range c.ActionsSections {
		if err := rejectUnsupportedColumns("actionsSections", i, s.Columns); err != nil {
			return err
		}
		if strings.TrimSpace(s.Repo) == "" {
			continue
		}
		if _, err := ParseRepo(s.Repo); err != nil {
			return fmt.Errorf("actionsSections.repo: %w", err)
		}
	}
	for i, s := range c.BranchSections {
		if err := rejectUnsupportedColumns("branchSections", i, s.Columns); err != nil {
			return err
		}
		if err := s.Filter.Validate(); err != nil {
			return err
		}
	}
	for _, r := range c.LocalRepos {
		if strings.TrimSpace(r.Path) == "" {
			return fmt.Errorf("localRepos entry %q: path is required", r.Name)
		}
	}
	if err := c.Keybindings.Validate(); err != nil {
		return err
	}
	switch c.Defaults.View {
	case "", "prs", "issues", "notifications", "actions", "branches":
	default:
		return fmt.Errorf("defaults.view = %q: want \"prs\", \"issues\", \"notifications\", \"actions\", or \"branches\"", c.Defaults.View)
	}
	switch c.Theme.Icons {
	case "", "unicode", "nerd", "ascii":
	default:
		return fmt.Errorf("theme.icons = %q: want \"unicode\", \"nerd\", or \"ascii\"", c.Theme.Icons)
	}
	return nil
}

// Validate rejects keybinding entries that cannot be matched or executed.
func (k Keybindings) Validate() error {
	for _, group := range []struct {
		name     string
		bindings []Keybinding
		builtins map[string]struct{}
	}{
		{"keybindings.universal", k.Universal, universalBuiltins},
		{"keybindings.prs", k.PRs, prBuiltins},
		{"keybindings.issues", k.Issues, issueBuiltins},
		{"keybindings.notifications", k.Notifications, notificationBuiltins},
		{"keybindings.actions", k.Actions, actionBuiltins},
		{"keybindings.branches", k.Branches, branchBuiltins},
	} {
		for i, b := range group.bindings {
			if strings.TrimSpace(b.Key) == "" {
				return fmt.Errorf("%s[%d].key is required", group.name, i)
			}
			hasBuiltin := strings.TrimSpace(b.Builtin) != ""
			hasCommand := strings.TrimSpace(b.Command) != ""
			if hasBuiltin == hasCommand {
				return fmt.Errorf("%s[%d] must set builtin or command", group.name, i)
			}
			if hasBuiltin {
				name := normalizeBuiltinName(b.Builtin)
				if _, ok := group.builtins[name]; !ok {
					return fmt.Errorf("%s[%d].builtin = %q is not supported in this scope", group.name, i, b.Builtin)
				}
			}
		}
	}
	return nil
}

var universalBuiltins = builtinSet(
	"refresh", "refreshAll", "openGithub", "open", "search", "togglePreview",
	"toggleSmartFiltering", "toggleSmartFilter", "currentRepo",
	"up", "down", "firstLine", "lastLine",
	"pageUp", "pageDown", "scrollUp", "scrollDown", "prevSection",
	"previousSection", "nextSection", "switchView", "copyurl", "copyNumber",
	"help", "quit", "redraw", "expand", "summaryViewMore",
)

var prBuiltins = builtinSet(
	"comment", "assign", "unassign", "addLabel", "removeLabel", "merge",
	"update", "updateBranch", "ready", "markReady", "draft", "markDraft",
	"watch", "watchChecks", "checks", "close", "reopen", "diff", "checkout", "approve", "review",
	"requestReview", "requestReviewer", "requestReviewers",
	"removeReview", "removeReviewer", "removeReviewers", "removeRequestedReviewers",
	"viewIssues", "summaryViewMore", "expand", "prevSidebarTab", "nextSidebarTab",
)

var issueBuiltins = builtinSet(
	"comment", "assign", "unassign", "addLabel", "removeLabel", "close", "reopen",
	"milestone", "setMilestone", "checkout", "subscribe", "unsubscribe",
	"milestone", "setMilestone", "checkout", "subscribe", "unsubscribe", "viewPrs",
)

var notificationBuiltins = builtinSet(
	"openGithub", "open", "markAsRead", "markRead", "markAsUnread",
	"markUnread", "markAllAsRead", "markAllRead", "markAsDone", "markDone",
	"markAllAsDone", "markAllDone", "pin", "unpin", "togglePin", "togglePinned", "toggleBookmark",
)

var actionBuiltins = builtinSet("rerun", "rerunRun", "cancel", "cancelRun", "logs", "viewLogs")

var branchBuiltins = builtinSet("checkout", "push", "forcePush", "fastForward", "delete")

func builtinSet(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[normalizeBuiltinName(name)] = struct{}{}
	}
	return out
}

func normalizeBuiltinName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	return strings.ToLower(name)
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
// before evaluating wildcard patterns such as "acme/*". Mapped paths may use
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

const envConfigPath = "TEA_DASH_CONFIG"

// Path returns the default config file path:
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

// ResolvePath returns the config path tea-dash should read, following the same
// precedence as the CLI:
//
//	explicit path -> TEA_DASH_CONFIG -> repo-root .tea-dash.yml/.tea-dash.yaml -> XDG default
func ResolvePath(explicit string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return ResolvePathFrom(explicit, cwd)
}

// ResolvePathFrom is ResolvePath with an injected cwd for tests.
func ResolvePathFrom(explicit, cwd string) (string, error) {
	if p := strings.TrimSpace(explicit); p != "" {
		return ExpandPath(p)
	}
	if p := strings.TrimSpace(os.Getenv(envConfigPath)); p != "" {
		return ExpandPath(p)
	}
	if p, ok := findRepoConfig(cwd); ok {
		return p, nil
	}
	return Path()
}

func findRepoConfig(cwd string) (string, bool) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			for _, name := range []string{".tea-dash.yml", ".tea-dash.yaml"} {
				p := filepath.Join(dir, name)
				if _, err := os.Stat(p); err == nil {
					return p, true
				}
			}
			return "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Load reads the config file. A missing file is not an error: it returns an
// empty Config so tea-dash can fall back to the repository in $PWD.
func Load(configPath ...string) (*Config, error) {
	explicit := ""
	if len(configPath) > 0 {
		explicit = configPath[0]
	}
	path, err := ResolvePath(explicit)
	if err != nil {
		return nil, err
	}
	raw, err := loadConfigMap(path, nil)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
				return &Config{}, nil
			}
		}
		return nil, err
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("preparing %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}

func loadConfigMap(file string, stack []string) (map[string]any, error) {
	path, err := resolveConfigPath(file)
	if err != nil {
		return nil, err
	}
	for _, seen := range stack {
		if seen == path {
			return nil, fmt.Errorf("include cycle: %s", strings.Join(append(stack, path), " -> "))
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var current map[string]any
	if err := yaml.Unmarshal(data, &current); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if current == nil {
		current = map[string]any{}
	}

	includes, err := includePaths(current["include"])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	merged := map[string]any{}
	nextStack := append(stack, path)
	for _, include := range includes {
		includePath, err := resolveIncludePath(include, filepath.Dir(path))
		if err != nil {
			return nil, fmt.Errorf("%s include %q: %w", path, include, err)
		}
		included, err := loadConfigMap(includePath, nextStack)
		if err != nil {
			return nil, err
		}
		merged = mergeYAMLMaps(merged, included)
	}
	return mergeYAMLMaps(merged, current), nil
}

func resolveConfigPath(file string) (string, error) {
	expanded, err := ExpandPath(file)
	if err != nil {
		return "", err
	}
	return filepath.Abs(expanded)
}

func resolveIncludePath(include, baseDir string) (string, error) {
	include = strings.TrimSpace(include)
	if include == "" {
		return "", errors.New("path is required")
	}
	expanded, err := ExpandPath(include)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(expanded) {
		return expanded, nil
	}
	return filepath.Join(baseDir, expanded), nil
}

func includePaths(v any) ([]string, error) {
	switch v := v.(type) {
	case nil:
		return nil, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("include[%d] must be a string", i)
			}
			if strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out, nil
	case []string:
		return v, nil
	default:
		return nil, fmt.Errorf("include must be a string or list of strings")
	}
}

func mergeYAMLMaps(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if baseMap, ok := out[k].(map[string]any); ok {
			if overlayMap, ok := v.(map[string]any); ok {
				out[k] = mergeYAMLMaps(baseMap, overlayMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}
