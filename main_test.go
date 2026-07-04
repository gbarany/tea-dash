package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/config"
)

func TestLaunchOptionsDetectsMatchingCurrentRepo(t *testing.T) {
	cwd := makeGitDir(t, "origin", "https://gitea.example.com/acme/widgets.git")
	got := launchOptions(&config.Config{}, "https://gitea.example.com", cwd, false)
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
	got := launchOptions(&config.Config{SmartFilteringAtLaunch: &disabled}, "https://gitea.example.com", cwd, false)
	if got.CurrentRepo != "" {
		t.Fatalf("CurrentRepo = %q, want empty when smart filtering is disabled", got.CurrentRepo)
	}
	if got.SmartFiltering {
		t.Fatal("SmartFiltering should be disabled by config")
	}
}

func TestLaunchOptionsForcesOffForMock(t *testing.T) {
	cwd := makeGitDir(t, "origin", "https://gitea.example.com/acme/widgets.git")
	got := launchOptions(&config.Config{}, "https://gitea.example.com", cwd, true)
	if got.SmartFiltering || got.CurrentRepo != "" {
		t.Fatalf("launchOptions(mock=true) = %+v, want zero value regardless of a matching cwd repo", got)
	}
}

func TestParseArgsMock(t *testing.T) {
	opts, err := parseArgs([]string{"--mock"})
	if err != nil || !opts.mock {
		t.Fatalf("parseArgs(--mock) = %+v, %v", opts, err)
	}
}

func TestParseArgsConfigLongFlag(t *testing.T) {
	opts, err := parseArgs([]string{"--config", "custom.yml"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.configPath != "custom.yml" {
		t.Fatalf("configPath = %q, want custom.yml", opts.configPath)
	}
}

func TestParseArgsConfigEqualsFlag(t *testing.T) {
	opts, err := parseArgs([]string{"--config=custom.yml"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.configPath != "custom.yml" {
		t.Fatalf("configPath = %q, want custom.yml", opts.configPath)
	}
}

func TestParseArgsConfigShortFlag(t *testing.T) {
	opts, err := parseArgs([]string{"-c", "custom.yml"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opts.configPath != "custom.yml" {
		t.Fatalf("configPath = %q, want custom.yml", opts.configPath)
	}
}

func TestParseArgsMissingConfigValueErrors(t *testing.T) {
	if _, err := parseArgs([]string{"--config"}); err == nil {
		t.Fatal("parseArgs should reject --config without a value")
	}
}

func TestParseArgsVersion(t *testing.T) {
	opts, err := parseArgs([]string{"--version"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if !opts.showVersion {
		t.Fatal("showVersion should be true")
	}
}

func TestParseArgsDebug(t *testing.T) {
	opts, err := parseArgs([]string{"--debug", "--config", "custom.yml"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if !opts.debug {
		t.Fatal("debug should be true")
	}
	if opts.configPath != "custom.yml" {
		t.Fatalf("configPath = %q, want custom.yml", opts.configPath)
	}
}

func TestUsageMentionsDebugFlag(t *testing.T) {
	if !strings.Contains(usage, "--debug") {
		t.Fatalf("usage should mention --debug:\n%s", usage)
	}
}

func TestStartDebugLogCreatesDebugLogInCurrentDirectory(t *testing.T) {
	cwd := t.TempDir()
	f, err := startDebugLog(true, cwd)
	if err != nil {
		t.Fatalf("startDebugLog: %v", err)
	}
	if f == nil {
		t.Fatal("startDebugLog should return a file when enabled")
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close debug log: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(cwd, "debug.log"))
	if err != nil {
		t.Fatalf("read debug.log: %v", err)
	}
	if !strings.Contains(string(raw), "tea-dash debug log") {
		t.Fatalf("debug.log = %q, want header", string(raw))
	}
}

func TestStartDebugLogDisabledDoesNothing(t *testing.T) {
	cwd := t.TempDir()
	f, err := startDebugLog(false, cwd)
	if err != nil {
		t.Fatalf("startDebugLog: %v", err)
	}
	if f != nil {
		t.Fatal("startDebugLog should return nil when disabled")
	}
	if _, err := os.Stat(filepath.Join(cwd, "debug.log")); !os.IsNotExist(err) {
		t.Fatalf("debug.log should not exist when debug is disabled, stat err=%v", err)
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
