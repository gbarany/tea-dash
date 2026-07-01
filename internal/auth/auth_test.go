package auth

import (
	"os"
	"path/filepath"
	"strings"
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
	return writeConfig(t, teaConfig)
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
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

func TestResolveAmbiguousNoDefaultErrors(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "")
	const cfg = `logins:
    - name: a
      url: https://a.example
      token: atoken
      default: false
    - name: b
      url: https://b.example
      token: btoken
      default: false
`
	// Two logins, neither default: nothing can be selected, so no URL resolves.
	_, err := ResolveFromFile(writeConfig(t, cfg), Overrides{})
	if err == nil {
		t.Fatal("expected an error when no login can be selected (ambiguous)")
	}
}

func TestResolveNamedLoginNotFoundErrors(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "")
	_, err := ResolveFromFile(writeTeaConfig(t), Overrides{Login: "nope"})
	if err == nil {
		t.Fatal("expected an error for an unknown login name")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("error %q should name the missing login", err)
	}
}

func TestResolveSingleLoginMissingTokenErrors(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "")
	const cfg = `logins:
    - name: only
      url: https://only.example
      token: ""
      default: false
`
	// A sole login is auto-selected; its URL resolves but the empty token must error.
	_, err := ResolveFromFile(writeConfig(t, cfg), Overrides{})
	if err == nil {
		t.Fatal("expected an error when the resolved login has no token")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Fatalf("error %q should mention the missing token", err)
	}
}

func TestResolveMalformedConfigErrors(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "")
	const bad = `logins:
    - name: x
      url: "unterminated
`
	_, err := ResolveFromFile(writeConfig(t, bad), Overrides{})
	if err == nil {
		t.Fatal("expected a parse error for malformed tea config YAML")
	}
}
