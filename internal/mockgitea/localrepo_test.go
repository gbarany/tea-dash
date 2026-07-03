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
}
