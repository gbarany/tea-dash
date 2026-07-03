package mockgitea

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// SeedLocalRepo creates a throwaway git repository under parent with a few
// branches so the Branches view (internal/git) has demo content beyond the
// mock Gitea data --mock otherwise provides. Returns the repo path.
//
// feature/steamer and fix/slow-pour both have their upstream set to the
// local main branch (git branch --set-upstream-to=main, no actual remote
// involved) purely so internal/git.ListBranches' ahead/behind computation —
// which only fires for a branch that has an upstream configured at all — has
// something real to report: feature/steamer ends up genuinely 1 commit
// ahead of main ("ahead 1"), fix/slow-pour genuinely level with it
// ("synced"), and main itself is "current". Without this, every branch would
// read "local" (Branch.Status()'s ahead/behind branches never trigger).
// Setting up a real second remote (e.g. a bare clone) would show the same
// variety but needs careful two-repo sequencing for no functional gain here,
// so it's skipped.
func SeedLocalRepo(parent string) (string, error) {
	dir := filepath.Join(parent, "kettle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}

	run := func(args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Pin the commit identity (so this works on a machine with no git
		// user.name/user.email configured) and point GIT_CONFIG_GLOBAL at
		// the null device to isolate the invoking user's global git config
		// — their personal hooks/aliases/signing setup (commit.gpgsign and
		// friends) must not be able to interfere with, slow down, or block
		// seeding a throwaway demo repo. GIT_CONFIG_GLOBAL over redirecting
		// HOME: it isolates exactly the one file that matters here without
		// also relocating unrelated HOME-rooted lookups.
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_AUTHOR_NAME=demo", "GIT_AUTHOR_EMAIL=demo@teahouse.local",
			"GIT_COMMITTER_NAME=demo", "GIT_COMMITTER_EMAIL=demo@teahouse.local",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %w: %s", args, err, out)
		}
		return nil
	}

	steps := [][]string{
		{"init", "-b", "main"},
		{"commit", "--allow-empty", "-m", "feat: initial kettle service"},
		{"commit", "--allow-empty", "-m", "feat: add temperature probe"},
		{"branch", "feature/steamer"},
		{"branch", "fix/slow-pour"},
		{"checkout", "-q", "feature/steamer"},
		{"commit", "--allow-empty", "-m", "wip: steam wand support"},
		{"checkout", "-q", "main"},
		{"branch", "--set-upstream-to=main", "feature/steamer"},
		{"branch", "--set-upstream-to=main", "fix/slow-pour"},
	}
	for _, step := range steps {
		if err := run(step...); err != nil {
			return "", err
		}
	}
	return dir, nil
}
