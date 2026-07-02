# tea-dash

A terminal dashboard for [Gitea](https://about.gitea.com/) (and Forgejo /
Codeberg), in the spirit of [`gh-dash`](https://github.com/dlvhdr/gh-dash) — but
for Gitea instead of GitHub.

tea-dash is a keyboard-driven TUI for triaging pull requests, issues,
notifications, and local branches across one or more Gitea instances, without
leaving the terminal.

> **Status: early — v1.** A working multi-view dashboard: live tables of your
> pull requests, issues, unread notifications across all your repos (fetched via
> the Gitea API (Go SDK + REST)), and read-only local branch status for
> configured git checkouts, with view switching (`s`), configurable sections you
> page through with `h`/`l`, preview pane support, and live keyword search (`/`).
> PR actions are in progress. See
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

| Key             | Action                  |
| --------------- | ----------------------- |
| `↑`/`↓`, `j`/`k`| move selection          |
| `s`             | switch view (PRs/issues/notifications/branches)|
| `h` / `l`       | prev / next section     |
| `/`             | search by keyword       |
| `p`             | toggle preview pane     |
| `e`             | expand preview body     |
| `ctrl+u/d`      | scroll preview          |
| `o` / `enter`   | open in browser         |
| `r`             | refresh                 |
| `q` / `ctrl+c`  | quit                    |

## Configuration

Optional. Create `~/.config/tea-dash/config.yml`
(`$XDG_CONFIG_HOME/tea-dash/config.yml`) to pick a tea login, choose the startup
view, and define your own sections:

```yaml
instance:
  login: ""          # tea login profile to use (empty = your default tea login)
  # url:   ""        # override the instance URL (else taken from the tea login)
  # Token source (first non-empty wins): token > tokenCommand > tokenEnv > TEA_DASH_TOKEN > tea login.
  # tea-dash reads the token from tea's config file. If tea stored yours in the OS
  # keychain (so the config's token is empty), give tea-dash one of these instead:
  # token:        ""                                   # a literal token (not recommended in plaintext)
  # tokenCommand: op read "op://Private/tea-dash/credential"   # e.g. 1Password, pass, gopass
  # tokenEnv:     TEA_DASH_TOKEN                        # name of an env var holding the token

defaults:
  view: prs              # startup view: "prs", "issues", "notifications", or "branches"
  prsLimit: 50           # rows fetched per PR section (0 -> 50)
  issuesLimit: 50        # rows fetched per issue section (0 -> 50)
  notificationsLimit: 50 # rows fetched per notifications section (0 -> 50)
  branchesLimit: 0       # local branches shown (0 -> all)

localRepos:
  - name: tea-dash
    path: /Users/gaborbarany/dev/sandbox/tea-dash

# Each section becomes a tab you page through with h/l. Omit prSections to get
# two "@me"-authored PR defaults: open and closed pull requests. Omit
# issuesSections to get one open "@me"-authored issues section.
prSections:
  - title: Open PRs
    filter:
      state: open          # open | closed | all (default open)
      createdBy: "@me"     # me-scoped author fields accept "@me" only
  - title: Closed PRs
    filter:
      state: closed        # closed includes merged PRs; merged rows show "merged"
      createdBy: "@me"
  - title: Review Requested
    filter:
      reviewRequested: "@me"
    limit: 25              # per-section cap; overrides defaults.prsLimit

issuesSections:
  - title: My Issues
    filter:
      state: open
      assignedBy: "@me"
      labels: [bug, urgent]  # AND-ed
      milestone: v2

notificationsSections:
  - title: Unread
    limit: 50

branchSections:
  - title: Local Branches
```

`filter` fields: `state`, `labels` (AND-ed), `milestone`, `createdBy`,
`assignedBy`, `mentioned`, `reviewRequested` (PRs only), `since` (RFC3339),
`sort`. The row cap follows section `limit` -> per-view default -> 50.

> **Note:** the me-scoped author fields (`createdBy`, `assignedBy`, `mentioned`,
> `reviewRequested`) support the sentinel `"@me"` only — a plain login is
> rejected at load, because Gitea's cross-repo search endpoint has no per-login
> author filter.

With or without a config file, tea-dash shows the pull requests and issues you
authored, plus unread notifications, across every repo you can access on your
Gitea instance. The default PR view has separate open and closed-history tabs;
sections and filters let you tailor what each tab shows. Notification sections
currently support title/limit configuration and default to unread threads. The
branches view shells out to local `git` for configured `localRepos` only and is
read-only; with no `localRepos`, it falls back to the current working directory.

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
internal/ui/            Bubble Tea model, table, preview, keybindings, styles
internal/gitea/         Gitea Go SDK client wrapper + PR/issue/notification APIs
internal/git/           read-only local git branch status
internal/auth/          resolves instance URL + token from the tea config
internal/data/          TUI-agnostic domain models
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
