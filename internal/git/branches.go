// Package git reads local repository state through the git executable.
package git

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repository is a local git repository known to tea-dash.
type Repository struct {
	Name string
	Path string
}

// Label returns the configured display name, falling back to the directory name.
func (r Repository) Label() string {
	if strings.TrimSpace(r.Name) != "" {
		return strings.TrimSpace(r.Name)
	}
	return filepath.Base(filepath.Clean(r.Path))
}

// Branch is one local branch plus its upstream tracking state.
type Branch struct {
	Repository     string
	RepositoryPath string
	Name           string
	Current        bool
	Upstream       string
	Ahead          int
	Behind         int
	UpstreamGone   bool
	Commit         string
	Subject        string
	UpdatedAt      time.Time
	WorktreePath   string
}

// RowData projection methods let Branch reuse the existing generic section.
func (b Branch) GetRepoNameWithOwner() string { return b.Repository }
func (b Branch) GetTitle() string             { return b.Name }
func (b Branch) GetNumber() int64             { return 0 }
func (b Branch) GetURL() string               { return "" }
func (b Branch) GetUpdatedAt() time.Time      { return b.UpdatedAt }

// ListBranches returns local branches for one repository.
func ListBranches(ctx context.Context, repo Repository) ([]Branch, error) {
	if strings.TrimSpace(repo.Path) == "" {
		return nil, fmt.Errorf("repository path is required")
	}
	format := strings.Join([]string{
		"%(refname:short)",
		"%(HEAD)",
		"%(upstream:short)",
		"%(upstream:track)",
		"%(objectname:short)",
		"%(contents:subject)",
		"%(committerdate:unix)",
		"%(worktreepath)",
	}, "%00")
	cmd := exec.CommandContext(ctx, "git", "-C", repo.Path, "for-each-ref", "--sort=refname", "--format="+format, "refs/heads")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("git for-each-ref %s: %w: %s", repo.Path, err, strings.TrimSpace(string(out)))
	}
	return parseForEachRef(repo, string(out))
}

// ListBranchesForRepositories returns branches for every configured repository.
func ListBranchesForRepositories(ctx context.Context, repos []Repository) ([]Branch, error) {
	var out []Branch
	for _, repo := range repos {
		branches, err := ListBranches(ctx, repo)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", repo.Label(), err)
		}
		out = append(out, branches...)
	}
	return out, nil
}

func parseForEachRef(repo Repository, out string) ([]Branch, error) {
	out = strings.TrimSuffix(out, "\n")
	if out == "" {
		return nil, nil
	}
	lines := strings.Split(out, "\n")
	branches := make([]Branch, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, "\x00")
		if len(fields) != 8 {
			return nil, fmt.Errorf("unexpected for-each-ref record with %d fields", len(fields))
		}
		track, err := parseTrackStatus(fields[3])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fields[0], err)
		}
		updated, err := parseUnixTime(fields[6])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fields[0], err)
		}
		branches = append(branches, Branch{
			Repository:     repo.Label(),
			RepositoryPath: repo.Path,
			Name:           fields[0],
			Current:        strings.TrimSpace(fields[1]) == "*",
			Upstream:       fields[2],
			Ahead:          track.Ahead,
			Behind:         track.Behind,
			UpstreamGone:   track.Gone,
			Commit:         fields[4],
			Subject:        fields[5],
			UpdatedAt:      updated,
			WorktreePath:   fields[7],
		})
	}
	return branches, nil
}

type trackStatus struct {
	Ahead  int
	Behind int
	Gone   bool
}

func parseTrackStatus(raw string) (trackStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return trackStatus{}, nil
	}
	if raw == "[gone]" {
		return trackStatus{Gone: true}, nil
	}
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return trackStatus{}, fmt.Errorf("unexpected upstream track status %q", raw)
	}
	raw = strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]")
	var status trackStatus
	for _, part := range strings.Split(raw, ", ") {
		key, val, ok := strings.Cut(part, " ")
		if !ok {
			return trackStatus{}, fmt.Errorf("unexpected upstream track status %q", part)
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			return trackStatus{}, fmt.Errorf("unexpected upstream track count %q: %w", val, err)
		}
		switch key {
		case "ahead":
			status.Ahead = n
		case "behind":
			status.Behind = n
		default:
			return trackStatus{}, fmt.Errorf("unexpected upstream track key %q", key)
		}
	}
	return status, nil
}

func parseUnixTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("unexpected commit time %q: %w", raw, err)
	}
	return time.Unix(sec, 0), nil
}
