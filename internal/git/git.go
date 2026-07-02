// Package git contains local checkout helpers for pull requests.
package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/gbarany/tea-dash/internal/config"
)

// RemoteRef is the parsed owner/repo identity from a git remote URL.
type RemoteRef struct {
	Host  string
	Owner string
	Repo  string
}

// FullName returns "owner/repo".
func (r RemoteRef) FullName() string {
	if r.Owner == "" || r.Repo == "" {
		return ""
	}
	return r.Owner + "/" + r.Repo
}

// MatchesInstanceURL reports whether this remote points at the configured
// Gitea/Forgejo instance host.
func (r RemoteRef) MatchesInstanceURL(instanceURL string) bool {
	want := normalizeRemoteHost(instanceURLHost(instanceURL))
	if want == "" {
		return false
	}
	return normalizeRemoteHost(r.Host) == want
}

// ParseRemoteURL parses common HTTPS and SSH git remote URL forms.
func ParseRemoteURL(raw string) (RemoteRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RemoteRef{}, errors.New("empty remote URL")
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return RemoteRef{}, err
		}
		owner, repo, err := parseRemotePath(u.Path)
		if err != nil {
			return RemoteRef{}, err
		}
		return RemoteRef{Host: normalizeRemoteHost(u.Host), Owner: owner, Repo: repo}, nil
	}

	hostPart, remotePath, ok := strings.Cut(raw, ":")
	if !ok || hostPart == "" || remotePath == "" {
		return RemoteRef{}, fmt.Errorf("unsupported remote URL %q", raw)
	}
	if at := strings.LastIndex(hostPart, "@"); at >= 0 {
		hostPart = hostPart[at+1:]
	}
	owner, repo, err := parseRemotePath(remotePath)
	if err != nil {
		return RemoteRef{}, err
	}
	return RemoteRef{Host: normalizeRemoteHost(hostPart), Owner: owner, Repo: repo}, nil
}

func parseRemotePath(rawPath string) (string, string, error) {
	p := strings.Trim(rawPath, "/")
	p = strings.TrimSuffix(p, ".git")
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("remote path %q does not contain owner/repo", rawPath)
	}
	owner := parts[len(parts)-2]
	repo := parts[len(parts)-1]
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("remote path %q does not contain owner/repo", rawPath)
	}
	return owner, repo, nil
}

func instanceURLHost(instanceURL string) string {
	instanceURL = strings.TrimSpace(instanceURL)
	if instanceURL == "" {
		return ""
	}
	u, err := url.Parse(instanceURL)
	if err != nil || u.Host == "" {
		return instanceURL
	}
	return u.Host
}

func normalizeRemoteHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

// ResolveRepoPath resolves repoName to a local checkout path from configured
// repoPaths first, then from cwd when cwd's origin remote matches repoName and
// instanceURL.
func ResolveRepoPath(repoName, cwd, instanceURL string, repoPaths map[string]string) (string, error) {
	if p, ok, err := config.MatchRepoPath(repoName, repoPaths); err != nil {
		return "", err
	} else if ok {
		return p, nil
	}

	if cwd != "" {
		if remoteURL, err := originRemoteURL(cwd); err == nil {
			if remote, err := ParseRemoteURL(remoteURL); err == nil &&
				remote.FullName() == strings.TrimSpace(repoName) &&
				(instanceURL == "" || remote.MatchesInstanceURL(instanceURL)) {
				return cwd, nil
			}
		}
	}
	return "", fmt.Errorf("no local checkout for %s; configure repoPaths", repoName)
}

func originRemoteURL(cwd string) (string, error) {
	configPath, err := gitConfigPath(cwd)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inOrigin := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = strings.Contains(line, `remote "origin"`) || strings.Contains(line, `remote 'origin'`)
			continue
		}
		if !inOrigin {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "url" {
			return strings.TrimSpace(val), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("origin remote URL not found")
}

func gitConfigPath(cwd string) (string, error) {
	gitPath := filepath.Join(cwd, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return filepath.Join(gitPath, "config"), nil
	}
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("%s is not a gitdir file", gitPath)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(cwd, gitDir)
	}
	return filepath.Join(gitDir, "config"), nil
}

// BranchNameFromTemplate renders a local branch name for a pull request.
func BranchNameFromTemplate(tmpl, repoName string, prIndex int64) (string, error) {
	r, err := config.ParseRepo(repoName)
	if err != nil {
		return "", err
	}
	tmpl = (config.Git{PRBranchTemplate: tmpl}).BranchTemplate()
	t, err := template.New("branch").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		Owner    string
		Repo     string
		RepoName string
		PrIndex  int64
	}{
		Owner:    r.Owner,
		Repo:     r.Name,
		RepoName: repoName,
		PrIndex:  prIndex,
	}); err != nil {
		return "", err
	}
	branch := strings.TrimSpace(buf.String())
	if branch == "" {
		return "", errors.New("branch template rendered an empty branch name")
	}
	return branch, nil
}

// Command is a git command invocation.
type Command struct {
	Dir  string
	Name string
	Args []string
}

// Result is a completed command result. Non-zero ExitCode means the process ran
// and failed; a non-nil Runner error means it could not be started or observed.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner runs git commands.
type Runner interface {
	Run(context.Context, Command) (Result, error)
}

// ExecRunner runs git commands with os/exec.
type ExecRunner struct{}

// Run executes cmd and captures stdout/stderr.
func (ExecRunner) Run(ctx context.Context, cmd Command) (Result, error) {
	c := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	c.Dir = cmd.Dir
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: 0}
	if err == nil {
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return Result{}, err
}

// CheckoutOptions configures pull-request checkout.
type CheckoutOptions struct {
	RepoName       string
	CWD            string
	InstanceURL    string
	RepoPaths      map[string]string
	Remote         string
	BranchTemplate string
	PrIndex        int64
	Runner         Runner
}

// CheckoutPlan is the resolved local checkout plan.
type CheckoutPlan struct {
	RepoPath     string
	Remote       string
	Branch       string
	FetchRefspec string
	RemoteRef    string
}

// PlanCheckout resolves paths, branch names, and refspecs without running git.
func PlanCheckout(opts CheckoutOptions) (CheckoutPlan, error) {
	if opts.PrIndex <= 0 {
		return CheckoutPlan{}, fmt.Errorf("invalid PR index %d", opts.PrIndex)
	}
	repoPath, err := ResolveRepoPath(opts.RepoName, opts.CWD, opts.InstanceURL, opts.RepoPaths)
	if err != nil {
		return CheckoutPlan{}, err
	}
	remote := (config.Git{Remote: opts.Remote}).RemoteName()
	branch, err := BranchNameFromTemplate(opts.BranchTemplate, opts.RepoName, opts.PrIndex)
	if err != nil {
		return CheckoutPlan{}, err
	}
	remoteRef := fmt.Sprintf("refs/remotes/%s/pull/%d/head", remote, opts.PrIndex)
	return CheckoutPlan{
		RepoPath:     repoPath,
		Remote:       remote,
		Branch:       branch,
		FetchRefspec: fmt.Sprintf("+refs/pull/%d/head:%s", opts.PrIndex, remoteRef),
		RemoteRef:    remoteRef,
	}, nil
}

// RunCheckout fetches and checks out a pull request branch. It refuses dirty
// worktrees. Existing local branches are advanced only through --ff-only.
func RunCheckout(ctx context.Context, opts CheckoutOptions) (CheckoutPlan, error) {
	plan, err := PlanCheckout(opts)
	if err != nil {
		return CheckoutPlan{}, err
	}
	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	status, err := runGit(ctx, runner, plan.RepoPath, "status", "--porcelain")
	if err != nil {
		return CheckoutPlan{}, err
	}
	if strings.TrimSpace(status.Stdout) != "" {
		return CheckoutPlan{}, fmt.Errorf("refusing checkout in dirty worktree %s", plan.RepoPath)
	}
	if _, err := runGit(ctx, runner, plan.RepoPath, "fetch", plan.Remote, plan.FetchRefspec); err != nil {
		return CheckoutPlan{}, err
	}

	exists, err := branchExists(ctx, runner, plan.RepoPath, plan.Branch)
	if err != nil {
		return CheckoutPlan{}, err
	}
	if !exists {
		if _, err := runGit(ctx, runner, plan.RepoPath, "switch", "-c", plan.Branch, plan.RemoteRef); err != nil {
			return CheckoutPlan{}, err
		}
		return plan, nil
	}
	if _, err := runGit(ctx, runner, plan.RepoPath, "switch", plan.Branch); err != nil {
		return CheckoutPlan{}, err
	}
	if _, err := runGit(ctx, runner, plan.RepoPath, "merge", "--ff-only", plan.RemoteRef); err != nil {
		return CheckoutPlan{}, fmt.Errorf("fast-forward existing branch %s: %w", plan.Branch, err)
	}
	return plan, nil
}

// SwitchBranchOptions configures switching to an existing local branch.
type SwitchBranchOptions struct {
	RepoPath string
	Branch   string
	Runner   Runner
}

// SwitchBranchResult is the local branch switch that completed.
type SwitchBranchResult struct {
	RepoPath string
	Branch   string
}

// SwitchBranch switches an existing local repository to branch. It refuses a
// no-op switch to the current branch and wraps common git failures with
// operator-facing recovery guidance.
func SwitchBranch(ctx context.Context, opts SwitchBranchOptions) (SwitchBranchResult, error) {
	repoPath := strings.TrimSpace(opts.RepoPath)
	branch := strings.TrimSpace(opts.Branch)
	if repoPath == "" {
		return SwitchBranchResult{}, errors.New("repository path is required")
	}
	if branch == "" {
		return SwitchBranchResult{}, errors.New("branch name is required")
	}
	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	current, err := runGit(ctx, runner, repoPath, "branch", "--show-current")
	if err != nil {
		return SwitchBranchResult{}, err
	}
	if strings.TrimSpace(current.Stdout) == branch {
		return SwitchBranchResult{}, fmt.Errorf("branch %s is already current in %s", branch, repoPath)
	}
	if _, err := runGit(ctx, runner, repoPath, "switch", branch); err != nil {
		return SwitchBranchResult{}, switchBranchError(repoPath, branch, err)
	}
	return SwitchBranchResult{RepoPath: repoPath, Branch: branch}, nil
}

func switchBranchError(repoPath, branch string, err error) error {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "local changes") && strings.Contains(msg, "would be overwritten"):
		return fmt.Errorf("could not switch to %s in %s: commit, stash, or discard local changes first: %w", branch, repoPath, err)
	case strings.Contains(msg, "already checked out"):
		return fmt.Errorf("could not switch to %s in %s: branch is already checked out in another worktree; use that worktree or switch it away first: %w", branch, repoPath, err)
	default:
		return fmt.Errorf("could not switch to %s in %s: %w", branch, repoPath, err)
	}
}

func branchExists(ctx context.Context, runner Runner, dir, branch string) (bool, error) {
	res, err := runner.Run(ctx, Command{Dir: dir, Name: "git", Args: []string{"show-ref", "--verify", "--quiet", "refs/heads/" + branch}})
	if err != nil {
		return false, err
	}
	switch res.ExitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, commandError(Command{Dir: dir, Name: "git", Args: []string{"show-ref", "--verify", "--quiet", "refs/heads/" + branch}}, res)
	}
}

func runGit(ctx context.Context, runner Runner, dir string, args ...string) (Result, error) {
	cmd := Command{Dir: dir, Name: "git", Args: args}
	res, err := runner.Run(ctx, cmd)
	if err != nil {
		return Result{}, err
	}
	if res.ExitCode != 0 {
		return res, commandError(cmd, res)
	}
	return res, nil
}

func commandError(cmd Command, res Result) error {
	msg := strings.TrimSpace(res.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(res.Stdout)
	}
	if msg == "" {
		msg = fmt.Sprintf("exit %d", res.ExitCode)
	}
	return fmt.Errorf("%s %s failed: %s", cmd.Name, strings.Join(cmd.Args, " "), msg)
}
