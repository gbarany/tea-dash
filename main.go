// Command tea-dash is a terminal dashboard for Gitea, built on top of the
// official `tea` CLI.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/build"
	"github.com/gbarany/tea-dash/internal/config"
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
	p := tea.NewProgram(ui.New(cfg))
	_, err = p.Run()
	return err
}

const usage = `tea-dash — a terminal dashboard for Gitea

Usage:
  tea-dash            start the dashboard
  tea-dash --version  print version information
  tea-dash --help     show this help

tea-dash shells out to Gitea's official ` + "`tea`" + ` CLI, so make sure tea is
installed and you have run ` + "`tea login add`" + ` at least once.`
