// Command tea-dash is a terminal dashboard for Gitea.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/actionrunner"
	"github.com/gbarany/tea-dash/internal/auth"
	"github.com/gbarany/tea-dash/internal/build"
	"github.com/gbarany/tea-dash/internal/config"
	localgit "github.com/gbarany/tea-dash/internal/git"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui"
)

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "tea-dash:", err)
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if opts.showVersion {
		fmt.Println("tea-dash", build.String())
		return
	}
	if opts.showHelp {
		fmt.Println(usage)
		return
	}

	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "tea-dash:", err)
		os.Exit(1)
	}
}

type cliOptions struct {
	configPath  string
	debug       bool
	showVersion bool
	showHelp    bool
}

func parseArgs(args []string) (cliOptions, error) {
	var opts cliOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--debug":
			opts.debug = true
		case arg == "-v" || arg == "--version" || arg == "version":
			opts.showVersion = true
		case arg == "-h" || arg == "--help" || arg == "help":
			opts.showHelp = true
		case arg == "-c" || arg == "--config":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return cliOptions{}, fmt.Errorf("%s requires a path", arg)
			}
			i++
			opts.configPath = args[i]
		case strings.HasPrefix(arg, "--config="):
			path := strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
			if path == "" {
				return cliOptions{}, fmt.Errorf("--config requires a path")
			}
			opts.configPath = path
		default:
			return cliOptions{}, fmt.Errorf("unknown argument %q", arg)
		}
	}
	return opts, nil
}

func run(opts cliOptions) error {
	cwd, _ := os.Getwd()
	if cwd == "" {
		cwd = "."
	}
	debugLog, err := startDebugLog(opts.debug, cwd)
	if err != nil {
		return err
	}
	if debugLog != nil {
		defer debugLog.Close()
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	ov := auth.Overrides{
		Login:        firstNonEmpty(cfg.Instance.Login, cfg.Login),
		URL:          cfg.Instance.URL,
		Token:        cfg.Instance.Token,
		TokenCommand: cfg.Instance.TokenCommand,
		TokenEnv:     cfg.Instance.TokenEnv,
		Insecure:     cfg.Instance.Insecure,
		CACertPath:   expandHome(cfg.Instance.CACert),
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

	model := ui.NewWithOptions(cfg, client, launchOptions(cfg, authCfg.URL, cwd))
	runner := actionrunner.New(actionrunner.Options{
		Client:      client,
		Config:      cfg,
		InstanceURL: authCfg.URL,
		CWD:         cwd,
	})
	model.SetActionDispatcher(runner.Dispatch)

	p := tea.NewProgram(model)
	_, err = p.Run()
	return err
}

func startDebugLog(enabled bool, cwd string) (*os.File, error) {
	if !enabled {
		return nil, nil
	}
	f, err := os.OpenFile(filepath.Join(cwd, "debug.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening debug.log: %w", err)
	}
	if _, err := fmt.Fprintf(f, "tea-dash debug log started at %s\n", time.Now().Format(time.RFC3339)); err != nil {
		f.Close()
		return nil, fmt.Errorf("writing debug.log: %w", err)
	}
	return f, nil
}

func launchOptions(cfg *config.Config, instanceURL, cwd string) ui.Options {
	opts := ui.Options{SmartFiltering: cfg.SmartFilteringEnabled()}
	if !opts.SmartFiltering {
		return opts
	}
	remote, ok, err := localgit.ResolveCurrentRepo(cwd, instanceURL, cfg.Git.Remote)
	if err != nil || !ok {
		opts.SmartFiltering = false
		return opts
	}
	opts.CurrentRepo = remote.FullName()
	opts.SmartFiltering = opts.CurrentRepo != ""
	return opts
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
  tea-dash                 start the dashboard
  tea-dash --config <path> use a specific config file
  tea-dash --debug         append debug output to ./debug.log
  tea-dash --version       print version information
  tea-dash --help          show this help

Config lookup order:
  --config/-c, TEA_DASH_CONFIG, repo-local .tea-dash.yml/.tea-dash.yaml,
  then $XDG_CONFIG_HOME/tea-dash/config.yml.`
