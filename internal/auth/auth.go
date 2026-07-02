// Package auth resolves the Gitea instance URL + token tea-dash connects with,
// reusing the `tea` CLI's own login config when present.
package auth

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the resolved connection credential.
type Config struct {
	URL        string
	Token      string
	Insecure   bool
	CACertPath string
}

// Overrides come from tea-dash's own config (its `instance:` block).
type Overrides struct {
	Login        string // pick a named tea login
	URL          string
	Token        string // literal token (instance.token)
	TokenCommand string // shell command whose stdout is the token
	TokenEnv     string // name of an env var to read the token from
	Insecure     bool
	CACertPath   string
}

// teaLogin mirrors one entry in tea's config.yml `logins:` list.
type teaLogin struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Token    string `yaml:"token"`
	Default  bool   `yaml:"default"`
	Insecure bool   `yaml:"insecure"`
	User     string `yaml:"user"`
}

type teaConfigFile struct {
	Logins []teaLogin `yaml:"logins"`
}

// TeaConfigPath returns the path to tea's config.yml, using the same per-OS
// config directory tea itself uses: os.UserConfigDir()/tea/config.yml
// (e.g. ~/Library/Application Support/tea on macOS, ~/.config/tea on Linux).
func TeaConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tea", "config.yml"), nil
}

// Resolve reads tea's config from its default location and resolves auth.
func Resolve(ov Overrides) (Config, error) {
	path, err := TeaConfigPath()
	if err != nil {
		return Config{}, err
	}
	return ResolveFromFile(path, ov)
}

// ResolveFromFile resolves auth against a specific tea config path (used in
// tests). A missing file is not an error: overrides/env may still suffice.
func ResolveFromFile(path string, ov Overrides) (Config, error) {
	logins, err := readTeaLogins(path)
	if err != nil {
		return Config{}, err
	}
	login := pickLogin(logins, ov.Login)
	if ov.Login != "" && login == nil {
		return Config{}, fmt.Errorf("tea login %q not found", ov.Login)
	}

	url := firstNonEmpty(ov.URL, os.Getenv("TEA_DASH_URL"), loginField(login, func(l teaLogin) string { return l.URL }))
	if url == "" {
		return Config{}, errors.New("no Gitea instance URL: run `tea login add`, or set instance.url / TEA_DASH_URL")
	}

	token, err := resolveToken(ov, login)
	if err != nil {
		return Config{}, err
	}
	if token == "" {
		return Config{}, tokenError(login)
	}

	insecure := ov.Insecure
	if login != nil && login.Insecure {
		insecure = true
	}
	return Config{URL: url, Token: token, Insecure: insecure, CACertPath: ov.CACertPath}, nil
}

// readTeaLogins returns tea's logins. A missing file is not an error (returns
// nil, nil); other read/parse failures are surfaced so real config problems are
// not silently swallowed.
func readTeaLogins(path string) ([]teaLogin, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading tea config %s: %w", path, err)
	}
	var cfg teaConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing tea config %s: %w", path, err)
	}
	return cfg.Logins, nil
}

// pickLogin selects a login: by name if given, else the default, else the sole
// login, else nil.
func pickLogin(logins []teaLogin, name string) *teaLogin {
	if name != "" {
		for i := range logins {
			if logins[i].Name == name {
				return &logins[i]
			}
		}
		return nil
	}
	for i := range logins {
		if logins[i].Default {
			return &logins[i]
		}
	}
	if len(logins) == 1 {
		return &logins[0]
	}
	return nil
}

func loginField(l *teaLogin, get func(teaLogin) string) string {
	if l == nil {
		return ""
	}
	return get(*l)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolveToken resolves the API token, in precedence order:
//
//	instance.token > instance.tokenCommand > instance.tokenEnv > TEA_DASH_TOKEN >
//	the selected tea login's token
//
// A configured tokenCommand that fails (or yields nothing) is a hard error
// rather than a silent fall-through, so a misconfigured secret manager surfaces.
func resolveToken(ov Overrides, login *teaLogin) (string, error) {
	if ov.Token != "" {
		return ov.Token, nil
	}
	if ov.TokenCommand != "" {
		out, err := runTokenCommand(ov.TokenCommand)
		if err != nil {
			return "", fmt.Errorf("instance.tokenCommand failed: %w", err)
		}
		if out == "" {
			return "", fmt.Errorf("instance.tokenCommand %q produced no output", ov.TokenCommand)
		}
		return out, nil
	}
	if ov.TokenEnv != "" {
		if v := os.Getenv(ov.TokenEnv); v != "" {
			return v, nil
		}
	}
	if v := os.Getenv("TEA_DASH_TOKEN"); v != "" {
		return v, nil
	}
	return loginField(login, func(l teaLogin) string { return l.Token }), nil
}

// runTokenCommand runs command through the user's $SHELL and returns its
// trimmed stdout. On failure it includes stderr so the cause is actionable.
func runTokenCommand(command string) (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	cmd := exec.Command(shell, "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// tokenError builds an actionable error for the no-token case, calling out the
// common situation where a tea login exists but its token lives in the OS
// keychain (which tea-dash cannot read) rather than tea's config file.
func tokenError(login *teaLogin) error {
	if login != nil {
		return fmt.Errorf("found tea login %q but no usable token: its token is not in tea's config "+
			"file (tea may keep it in your OS keychain, which tea-dash cannot read). Set instance.token, "+
			"instance.tokenCommand (a command that prints a token), or TEA_DASH_TOKEN", login.Name)
	}
	return errors.New("no Gitea token: run `tea login add`, or set instance.token / instance.tokenCommand / TEA_DASH_TOKEN")
}
