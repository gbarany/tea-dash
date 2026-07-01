// Command tea-dash is a terminal dashboard for Gitea.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/auth"
	"github.com/gbarany/tea-dash/internal/build"
	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui"
)

func main() {
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-v", "--version", "version":
			fmt.Println("tea-dash", build.String())
			return
		case "-h", "--help", "help":
			fmt.Println(usage)
			return
		}
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tea-dash:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ov := auth.Overrides{
		Login:      firstNonEmpty(cfg.Instance.Login, cfg.Login),
		URL:        cfg.Instance.URL,
		Token:      cfg.Instance.Token,
		Insecure:   cfg.Instance.Insecure,
		CACertPath: expandHome(cfg.Instance.CACert),
	}
	authCfg, err := auth.Resolve(ov)
	if err != nil {
		return fmt.Errorf("authentication: %w", err)
	}

	ctx := context.Background()
	client, err := gitea.NewClient(ctx, authCfg)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", authCfg.URL, err)
	}

	p := tea.NewProgram(ui.New(cfg, client))
	_, err = p.Run()
	return err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// expandHome expands a leading ~ to the user's home directory.
func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

const usage = `tea-dash — a terminal dashboard for Gitea

Usage:
  tea-dash            start the dashboard (your open pull requests)
  tea-dash --version  print version information
  tea-dash --help     show this help

tea-dash reuses your ` + "`tea`" + ` login (run ` + "`tea login add`" + ` once), or set
instance.url + instance.token in ~/.config/tea-dash/config.yml.`
