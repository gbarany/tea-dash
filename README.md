# tea-dash

A terminal dashboard for [Gitea](https://about.gitea.com/) (and Forgejo /
Codeberg), in the spirit of [`gh-dash`](https://github.com/dlvhdr/gh-dash) — but
for Gitea instead of GitHub.

tea-dash is a keyboard-driven TUI for triaging pull requests, issues and
notifications across one or more Gitea instances, without leaving the terminal.

> **Status: early — v1.** A working single screen: a live table of
> your open pull requests across all your repos (fetched via the Gitea API
> (Go SDK + REST)). Issues, notifications and PR actions are next. See
> [`docs/architecture.md`](docs/architecture.md) for the design.

## Why

There is a rich TUI dashboard for GitHub (`gh-dash`) but nothing equivalent for
Gitea/Forgejo. tea-dash fills that gap by building on Gitea's own official Go
SDK, reusing the `tea` CLI's stored login for auth.

## How it works

tea-dash talks to Gitea directly via the official Go SDK
(`code.gitea.io/sdk/gitea`). It reuses your existing `tea` login
(`~/Library/Application Support/tea/config.yml` on macOS /
`~/.config/tea/config.yml` on Linux) for the instance URL and token, so you get
auth for free without tea-dash handling credentials itself — but `tea` is **not**
run at runtime. This means tea-dash:

- reuses your existing `tea` logins, so **auth and multi-instance support come
  for free**;
- keeps credentials out of its own hands — it only reads what `tea` already
  stored;
- works against any Gitea/Forgejo server the SDK supports.

See [`docs/architecture.md`](docs/architecture.md) for the details.

## Requirements

- [Go](https://go.dev) 1.25+ (to build)
- A `tea` login for the instance URL and token. You only need
  [`tea`](https://gitea.com/gitea/tea) **once** to create a login
  (`tea login add`) — it is not a runtime dependency, and does not need to be on
  your `PATH` when tea-dash runs.

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
(`$XDG_CONFIG_HOME/tea-dash/config.yml`) to pick a specific tea login:

```yaml
# tea login profile to use (optional; empty = your default tea login)
instance:
  login: ""
# repos: reserved — not yet wired (planned for a later milestone).
# tea-dash currently shows PRs across every repo you can access.
# repos:
#   - gitea/tea
#   - gbarany/tea-dash
```

With or without a config file, tea-dash shows the pull requests you authored
across every repo you can access on your Gitea instance.

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
internal/gitea/         Gitea Go SDK client wrapper + me-scoped PR search
internal/auth/          resolves instance URL + token from the tea config
internal/data/          TUI-agnostic domain model (PullRequest)
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
