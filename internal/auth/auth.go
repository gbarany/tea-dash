// Package auth resolves the Gitea instance URL + token tea-dash connects with,
// reusing the `tea` CLI's own login config when present.
package auth

import (
	"errors"
	"os"
	"path/filepath"

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
	Login      string // pick a named tea login
	URL        string
	Token      string
	Insecure   bool
	CACertPath string
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
	logins := readTeaLogins(path)
	login := pickLogin(logins, ov.Login)

	url := firstNonEmpty(ov.URL, os.Getenv("TEA_DASH_URL"), loginField(login, func(l teaLogin) string { return l.URL }))
	token := firstNonEmpty(ov.Token, os.Getenv("TEA_DASH_TOKEN"), loginField(login, func(l teaLogin) string { return l.Token }))

	if url == "" {
		return Config{}, errors.New("no Gitea instance URL: run `tea login add`, or set instance.url / TEA_DASH_URL")
	}
	if token == "" {
		return Config{}, errors.New("no Gitea token: run `tea login add`, or set instance.token / TEA_DASH_TOKEN")
	}

	insecure := ov.Insecure
	if login != nil && login.Insecure {
		insecure = true
	}
	return Config{URL: url, Token: token, Insecure: insecure, CACertPath: ov.CACertPath}, nil
}

// readTeaLogins returns tea's logins, or nil if the file is absent/unreadable.
func readTeaLogins(path string) []teaLogin {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg teaConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return cfg.Logins
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
