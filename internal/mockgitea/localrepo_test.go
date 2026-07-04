package mockgitea

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSeedLocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir, err := SeedLocalRepo(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", dir, "branch", "--format=%(refname:short)").Output()
	if err != nil {
		t.Fatal(err)
	}
	branches := strings.Fields(string(out))
	want := map[string]bool{"main": false, "feature/steamer": false, "fix/slow-pour": false}
	for _, b := range branches {
		if _, ok := want[b]; ok {
			want[b] = true
		}
	}
	for b, seen := range want {
		if !seen {
			t.Fatalf("missing branch %s in %v", b, branches)
		}
	}

	// Guard the upstream-tracking trick (git branch --set-upstream-to=main)
	// against silent removal: without it, internal/git.ListBranches' ahead/
	// behind computation never fires and every branch reads "local".
	revList := func(rangeSpec string) string {
		out, err := exec.Command("git", "-C", dir, "rev-list", "--count", rangeSpec).Output()
		if err != nil {
			t.Fatalf("git rev-list --count %s: %v", rangeSpec, err)
		}
		return strings.TrimSpace(string(out))
	}
	if got := revList("main..feature/steamer"); got != "1" {
		t.Fatalf("main..feature/steamer commit count = %q, want 1 (ahead 1)", got)
	}
	if got := revList("main..fix/slow-pour"); got != "0" {
		t.Fatalf("main..fix/slow-pour commit count = %q, want 0 (even with main)", got)
	}
	upstream, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "feature/steamer@{upstream}").Output()
	if err != nil {
		t.Fatalf("feature/steamer has no upstream configured: %v", err)
	}
	if got := strings.TrimSpace(string(upstream)); got != "main" {
		t.Fatalf("feature/steamer@{upstream} = %q, want main", got)
	}
}
