package auth

import (
	"os"
	"path/filepath"
	"testing"
)

const teaConfig = `logins:
    - name: personal
      url: https://gitea.example.org
      token: personaltoken
      default: false
      insecure: false
      user: me
    - name: work
      url: https://git.work.example
      token: worktoken
      default: true
      insecure: true
      user: me
`

func writeTeaConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(p, []byte(teaConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolvePicksDefaultLogin(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "")
	got, err := ResolveFromFile(writeTeaConfig(t), Overrides{})
	if err != nil {
		t.Fatalf("ResolveFromFile: %v", err)
	}
	if got.URL != "https://git.work.example" || got.Token != "worktoken" || !got.Insecure {
		t.Fatalf("resolved = %+v, want the default (work) login", got)
	}
}

func TestResolvePicksNamedLogin(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "")
	got, err := ResolveFromFile(writeTeaConfig(t), Overrides{Login: "personal"})
	if err != nil {
		t.Fatalf("ResolveFromFile: %v", err)
	}
	if got.URL != "https://gitea.example.org" || got.Token != "personaltoken" || got.Insecure {
		t.Fatalf("resolved = %+v, want the personal login", got)
	}
}

func TestResolveOverridesAndEnvWin(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "envtoken")
	got, err := ResolveFromFile(writeTeaConfig(t), Overrides{URL: "https://override.example"})
	if err != nil {
		t.Fatalf("ResolveFromFile: %v", err)
	}
	if got.URL != "https://override.example" || got.Token != "envtoken" {
		t.Fatalf("resolved = %+v, want override URL + env token", got)
	}
}

func TestResolveMissingTokenErrors(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "https://only-url.example")
	t.Setenv("TEA_DASH_TOKEN", "")
	// No tea config file at this path -> no login token available.
	_, err := ResolveFromFile(filepath.Join(t.TempDir(), "missing.yml"), Overrides{})
	if err == nil {
		t.Fatal("expected an error when no token can be resolved")
	}
}
