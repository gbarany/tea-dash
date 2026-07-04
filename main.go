// Command tea-dash is a terminal dashboard for Gitea.
package main

import (
	"context"
	"fmt"
	"net/url"
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
	"github.com/gbarany/tea-dash/internal/mockgitea"
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
	mock        bool
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
		case arg == "--mock":
			opts.mock = true
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

	cfg, authCfg, cleanup, err := resolveEnvironment(opts, cwd)
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	client, err := gitea.NewClient(ctx, authCfg)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", authCfg.URL, err)
	}

	model := ui.NewWithOptions(cfg, client, launchOptions(cfg, authCfg.URL, cwd, opts.mock))
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

// resolveEnvironment picks real config+auth, or the in-process mock stack
// when opts.mock is set. The returned cleanup func is never nil — callers
// should always defer it, mock path or not.
func resolveEnvironment(opts cliOptions, cwd string) (*config.Config, auth.Config, func(), error) {
	if opts.mock {
		srv := mockgitea.NewServer(mockgitea.DemoData(time.Now()))

		// parent (the MkdirTemp tree SeedLocalRepo writes the throwaway repo
		// under) is removed by cleanup below — closing the server alone would
		// leak it on every --mock run otherwise.
		// A seeding failure is non-fatal, but repoDir must never end up ""
		// here: DemoConfig("") would omit LocalRepos, and the branches
		// section then falls back to the invoking cwd's git repo — leaking
		// the operator's real branches into the demo. On failure, point the
		// section at an isolated non-repo path instead.
		var repoDir, parent string
		if p, err := os.MkdirTemp("", "tea-dash-mock-"); err != nil {
			repoDir = filepath.Join(os.TempDir(), "tea-dash-mock-unavailable")
			fmt.Fprintln(os.Stderr, "note: local demo repo unavailable; Branches view will not have demo data")
		} else {
			parent = p
			if dir, err := mockgitea.SeedLocalRepo(parent); err != nil {
				repoDir = parent
				fmt.Fprintln(os.Stderr, "note: could not seed the demo repo (is git installed?); Branches view will not have demo data")
			} else {
				repoDir = dir
			}
		}
		cleanup := func() {
			srv.Close()
			if parent != "" {
				os.RemoveAll(parent)
			}
		}

		var cfg *config.Config
		if opts.configPath != "" {
			// An explicit --config composes with mock mode: load exactly
			// that file (config.Load with a non-empty path bypasses the
			// TEA_DASH_CONFIG/repo-local-.tea-dash.yml lookup chain, going
			// straight to the given path) rather than DemoConfig.
			loaded, err := config.Load(opts.configPath)
			if err != nil {
				cleanup()
				return nil, auth.Config{}, func() {}, err
			}
			cfg = loaded
		} else {
			// No explicit --config: use the built-in demo config, and
			// deliberately do NOT call config.Load("") here — that would
			// run the full lookup chain (TEA_DASH_CONFIG env, a repo-local
			// .tea-dash.yml/.tea-dash.yaml under cwd) and could pull in
			// whatever config the user happens to be sitting in, making
			// --mock's behavior depend on cwd instead of being predictable.
			cfg = mockgitea.DemoConfig(repoDir)
		}
		if err := cfg.Validate(); err != nil {
			cleanup()
			return nil, auth.Config{}, func() {}, err
		}
		return cfg, auth.Config{URL: srv.URL(), Token: "mock-token"}, cleanup, nil
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return nil, auth.Config{}, func() {}, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, auth.Config{}, func() {}, err
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
		return nil, auth.Config{}, func() {}, fmt.Errorf("authentication: %w", err)
	}
	return cfg, authCfg, func() {}, nil
}

// mockHost is the header's fixed right-side host label for --mock runs
// (spec §5's deferred item, closed in the UI-overhaul plan's Task 3).
const mockHost = "demo.gitea.local"

// launchOptions builds the UI's smart-filtering and header-host options.
// mock forces smart filtering off unconditionally: it scopes sections to
// the git repo tea-dash was launched from, and --mock must never let that
// repo (very likely the real tea-dash checkout, if that's where someone
// runs the demo from) leak into the fake teahouse data. mock runs show
// mockHost in the header; real runs show instanceURL's host.
func launchOptions(cfg *config.Config, instanceURL, cwd string, mock bool) ui.Options {
	if mock {
		return ui.Options{MockHost: mockHost}
	}
	opts := ui.Options{SmartFiltering: cfg.SmartFilteringEnabled(), InstanceHost: instanceHost(instanceURL)}
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

// instanceHost extracts the host (without scheme/port-if-default) from the
// resolved instance URL for the header's right-side label. An unparseable
// URL degrades to showing it verbatim rather than an empty header segment.
func instanceHost(instanceURL string) string {
	u, err := url.Parse(instanceURL)
	if err != nil || u.Host == "" {
		return instanceURL
	}
	return u.Host
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
  tea-dash --mock          run against built-in demo data (no Gitea needed)
  tea-dash --config <path> use a specific config file
  tea-dash --debug         append debug output to ./debug.log
  tea-dash --version       print version information
  tea-dash --help          show this help

Config lookup order:
  --config/-c, TEA_DASH_CONFIG, repo-local .tea-dash.yml/.tea-dash.yaml,
  then $XDG_CONFIG_HOME/tea-dash/config.yml.`
