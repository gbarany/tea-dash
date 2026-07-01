# tea-dash

A terminal dashboard for [Gitea](https://about.gitea.com/) (and Forgejo /
Codeberg), in the spirit of [`gh-dash`](https://github.com/dlvhdr/gh-dash) — but
for Gitea instead of GitHub.

tea-dash is a keyboard-driven TUI for triaging pull requests, issues and
notifications across one or more Gitea instances, without leaving the terminal.

> **Status: early — v1.** A working single screen: a live, sortable table of
> open pull requests across the repos you configure (fetched via `tea api`).
> Issues, notifications and PR actions are next. See
> [`docs/architecture.md`](docs/architecture.md) for the design.

## Why

There is a rich TUI dashboard for GitHub (`gh-dash`) but nothing equivalent for
Gitea/Forgejo. tea-dash fills that gap by building on Gitea's own official CLI.

## How it works

tea-dash does **not** talk to the Gitea API directly. Instead it shells out to
Gitea's official [`tea`](https://gitea.com/gitea/tea) CLI, primarily via
`tea api`, which returns raw, fully-typed Gitea REST JSON. This means tea-dash:

- reuses your existing `tea` logins (`~/.config/tea/config.yml`), so **auth and
  multi-instance support come for free**;
- stays a pure presentation layer with no credential handling of its own;
- works against any Gitea/Forgejo server `tea` supports.

See [`docs/architecture.md`](docs/architecture.md) for the details, including the
important distinction between `tea <list> -o json` (flat, all-string columns)
and `tea api` (raw typed objects).

## Requirements

- [Go](https://go.dev) 1.25+ (to build)
- [`tea`](https://gitea.com/gitea/tea) on your `PATH`, with at least one login
  configured (`tea login add`)

## Install

```sh
go install github.com/gbarany/tea-dash@latest
```

Or build from source:

```sh
git clone https://github.com/gbarany/tea-dash
cd tea-dash
make build      # -> ./bin/tea-dash
```

## Usage

```sh
tea-dash            # start the dashboard
tea-dash --version  # print version info
tea-dash --help
```

### Keys

| Key             | Action            |
| --------------- | ----------------- |
| `↑`/`↓`, `j`/`k`| move selection    |
| `o` / `enter`   | open PR in browser|
| `r`             | refresh           |
| `q` / `ctrl+c`  | quit              |

## Configuration

Optional. Create `~/.config/tea-dash/config.yml`
(`$XDG_CONFIG_HOME/tea-dash/config.yml`) to choose which repositories to watch:

```yaml
# tea login profile to use (optional; empty = your default tea login)
login: ""
# repositories to show pull requests for
repos:
  - gitea/tea
  - gbarany/tea-dash
```

If the file is absent, tea-dash falls back to the Gitea repository in your
current directory (resolved by `tea` from the git remote).

## Development

```sh
make check   # gofmt-check + go vet + tests (race)
make run     # go run .
make build   # build ./bin/tea-dash
make lint    # golangci-lint (optional; requires golangci-lint v2)
make help    # list all targets
```

Project layout:

```
main.go                 entrypoint + flag handling; loads config, starts the TUI
internal/ui/            Bubble Tea model, table, keybindings, styles
internal/teacli/        wrapper around the `tea` CLI (tea api) + typed responses
internal/config/        ~/.config/tea-dash/config.yml loading
internal/build/         version metadata (set via -ldflags)
```

## Tech stack

Go with the **Bubble Tea v2** Charm stack — `charm.land/bubbletea/v2` +
`charm.land/lipgloss/v2` + `charm.land/bubbles/v2` — the exact TUI stack
[`gh-dash`](https://github.com/dlvhdr/gh-dash) and Gitea's own `tea` CLI are
built on. Planned, to stay aligned with gh-dash: `glamour/v2` (Markdown bodies),
`cobra`+`fang` (CLI), and `koanf`+`validator` (config).

## License

[MIT](LICENSE)
