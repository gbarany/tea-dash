# tea-dash

A terminal dashboard for [Gitea](https://about.gitea.com/) (and Forgejo /
Codeberg), in the spirit of [`gh-dash`](https://github.com/dlvhdr/gh-dash) — but
for Gitea instead of GitHub.

tea-dash is a keyboard-driven TUI for triaging pull requests, issues,
notifications, and local branches across one or more Gitea instances, without
leaving the terminal.

> **Status: early — v1.** A working multi-view dashboard: live tables of your
> pull requests, issues, unread notifications, Actions runs, and read-only local
> branch status (fetched via the Gitea API (Go SDK + REST) and local `git`),
> with view switching (`s`), configurable sections you page through with `h`/`l`,
> live keyword search (`/`), progressive PR/issue loading, a default-open
> preview, and PR / issue actions. See [`docs/architecture.md`](docs/architecture.md)
> for the design.

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
| mouse click      | select row              |
| action button click | run common row action |
| mouse wheel      | move selection          |
| `g` / `G`       | first / last row        |
| `s`             | switch view (PRs/issues/notifications/actions/branches)|
| `h` / `l`       | prev / next section     |
| `/`             | search by keyword       |
| `t`             | toggle current-repo smart filtering (when launched from a matching git checkout) |
| `p`             | show / hide preview panel |
| `e`             | expand / fold preview body |
| `ctrl+u` / `ctrl+d` | scroll preview       |
| `o` / `enter`   | open in browser         |
| `y` / `Y`       | copy row number / URL   |
| `c`             | add comment             |
| `a` / `A`       | assign / unassign yourself |
| `L` / `U`       | add / remove labels     |
| `M`             | set issue milestone     |
| `m`             | merge PR                |
| `u`             | update PR branch from its base branch |
| `W`             | mark draft PR ready for review |
| `m` / `u` / `M` | mark notification read / unread / all read |
| `x` / `X`       | close / reopen          |
| `v`             | submit PR review        |
| `d` / `ctrl+t`  | open PR diff in external pager |
| `C` / `space`   | checkout PR locally; switch branch in Branches view |
| `R` / `!`       | rerun / cancel Actions run |
| `r` / `R`       | refresh section / all sections (`ctrl+r` refreshes all in Actions view) |
| `?`             | show / hide full help   |
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
  # token:        ""                                      # a literal token (not recommended in plaintext)
  # tokenCommand: "<command that prints the token>"        # e.g. pass, gopass, 1Password CLI, etc.
  # tokenEnv:     TEA_DASH_TOKEN                           # name of an env var holding the token

smartFilteringAtLaunch: true # when launched inside a matching git checkout, blank PR/issue sections scope to that repo

defaults:
  view: prs              # startup view: "prs", "issues", "notifications", "actions", or "branches"
  prsLimit: 50           # PR page size; more rows load when you reach the bottom (0 -> 50)
  issuesLimit: 50        # issue page size; more rows load when you reach the bottom (0 -> 50)
  notificationsLimit: 50 # rows fetched per notifications section (0 -> 50)
  actionsLimit: 50       # rows fetched per Actions section (0 -> 50)
  branchesLimit: 0       # local branches shown (0 -> all)

repos:
  - acme/widgets         # PR/issue sections without repo: fan out across these repos
  - acme/api             # omit repos to use the instance-wide cross-repo search endpoint

localRepos:
  - name: tea-dash
    path: ~/src/tea-dash

pager:
  diff: diffnav     # command that receives PR diff bytes on stdin (falls back to $PAGER, then less -R)

repoPaths:
  "example/*": "~/src/{{.Repo}}"  # used by C checkout; exact repo names and wildcards both work

git:
  remote: origin
  prBranchTemplate: "pr-{{.PrIndex}}"

# Each section becomes a tab you page through with h/l. A section-level repo:
# overrides global repos: for that tab. Omit prSections to get two
# "@me"-authored PR defaults: open and closed pull requests. Omit issuesSections
# to get one open "@me"-authored issues section.
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
    limit: 25              # per-section page size; overrides defaults.prsLimit
  - title: Alice in Widgets
    repo: acme/widgets     # repo-scoped sections can use plain login filters
    filter:
      state: open
      createdBy: alice

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

keybindings:
  universal:
    - key: tab
      builtin: nextSection
    - key: H
      builtin: help
  prs:
    - key: O
      builtin: checkout
    - key: a
      builtin: assign
    - key: A
      builtin: unassign
    - key: L
      builtin: addLabel
    - key: U
      builtin: removeLabel
    - key: u
      builtin: update
    - key: W
      builtin: ready
    - key: g
      name: lazygit
      command: cd {{.RepoPath}} && lazygit
  issues:
    - key: a
      builtin: assign
    - key: A
      builtin: unassign
    - key: L
      builtin: addLabel
    - key: U
      builtin: removeLabel
    - key: M
      builtin: setMilestone
    - key: i
      command: echo issue {{.IssueNumber}} in {{.RepoName}}
  notifications:
    - key: D
      builtin: markAllRead
  actions:
    - key: a
      command: echo run {{.RunID}} in {{.RepoName}}
  branches:
    - key: B
      command: git -C {{.RepoPath}} status
```

`filter` fields: `state`, `labels` (AND-ed), `milestone`, `createdBy`,
`assignedBy`, `mentioned`, `reviewRequested` (PRs only), `since` (RFC3339),
`sort`. PR and issue sections fetch one page at a time; reaching the loaded
bottom automatically requests the next page until the server total is loaded.
When `repos:` is configured, sections without their own `repo:` use the
repo-scoped endpoint for each listed repo and merge the results by updated time;
sections with `repo:` query only that repo. With neither `repos:` nor `repo:`,
tea-dash uses the instance-wide cross-repo search endpoint. `reviewRequested`
is the one exception: Gitea exposes it only on instance-wide PR search, so those
sections ignore `repos:` and stay cross-repo. The page size follows section
`limit` -> per-view default -> 50.

If `smartFilteringAtLaunch` is enabled (the default) and you start `tea-dash`
from a local git checkout whose configured remote host matches the selected
Gitea/Forgejo instance, PR and issue sections without an explicit `repo:` are
scoped to that current repository. Press `t` to toggle between current-repo and
all-repositories mode. Sections with explicit `repo:` always keep their
configured repository.

`keybindings` follows gh-dash's shape: each entry has a `key`, optional `name`,
and exactly one of `builtin` or `command`. Built-ins remap implemented tea-dash
actions; commands run through your shell with row template fields such as
`RepoName`, `RepoPath`, `PrIndex`/`PrNumber`, `IssueIndex`/`IssueNumber`,
`RunID`, `Title`, `Author`, `Sha`, `InstanceURL`, and `Url`/`URL`.

> **Note:** the me-scoped author fields (`createdBy`, `assignedBy`, `mentioned`,
> `reviewRequested`) support the sentinel `"@me"` on cross-repo sections.
> Plain login filters such as `createdBy: alice` require either global `repos:`
> or section-level `repo: owner/name`, because Gitea's cross-repo search
> endpoint has no per-login author filter.

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
