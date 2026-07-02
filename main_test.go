package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gbarany/tea-dash/internal/config"
)

func TestLaunchOptionsDetectsMatchingCurrentRepo(t *testing.T) {
	cwd := makeGitDir(t, "origin", "https://gitea.example.com/acme/widgets.git")
	got := launchOptions(&config.Config{}, "https://gitea.example.com", cwd)
	if got.CurrentRepo != "acme/widgets" {
		t.Fatalf("CurrentRepo = %q, want acme/widgets", got.CurrentRepo)
	}
	if !got.SmartFiltering {
		t.Fatal("SmartFiltering should be enabled for a matching cwd repo")
	}
}

func TestLaunchOptionsHonorsSmartFilteringDisabled(t *testing.T) {
	cwd := makeGitDir(t, "origin", "https://gitea.example.com/acme/widgets.git")
	disabled := false
	got := launchOptions(&config.Config{SmartFilteringAtLaunch: &disabled}, "https://gitea.example.com", cwd)
	if got.CurrentRepo != "" {
		t.Fatalf("CurrentRepo = %q, want empty when smart filtering is disabled", got.CurrentRepo)
	}
	if got.SmartFiltering {
		t.Fatal("SmartFiltering should be disabled by config")
	}
}

func makeGitDir(t *testing.T, remoteName, remoteURL string) string {
	t.Helper()
	cwd := t.TempDir()
	gitDir := filepath.Join(cwd, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `[remote "` + remoteName + `"]
	url = ` + remoteURL + `
`
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return cwd
}
