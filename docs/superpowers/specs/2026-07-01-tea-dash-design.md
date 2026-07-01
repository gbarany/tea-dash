# tea-dash — Design

tea-dash is a keyboard-driven terminal UI dashboard for a single, personal Gitea/Forgejo instance, modeled closely on [gh-dash](https://github.com/dlvhdr/gh-dash) but retargeted from GitHub to Gitea. It reuses gh-dash's architecture (root model, section/preview components, task/async envelope, layered-YAML config, semantic theming, rebindable vim keybindings) while replacing the entire I/O layer with direct calls to the Gitea Go SDK (`code.gitea.io/sdk/gitea`) for both reads and writes — it never shells out to the `tea` CLI. It shows your PRs, Issues, CI status, Notifications, and local branches as configurable filtered sections, previews and acts on them (comment, review, merge, label, assign, checkout), and is themed and keybound entirely from YAML. It is a standalone static Go binary on the Bubble Tea v2 stack, with no extension host. Because Gitea/Forgejo version and flavor fragmentation is the defining constraint, every version-sensitive capability is runtime-probed and degrades gracefully rather than crashing.

- **Repo / module:** `github.com/gbarany/tea-dash`.
- **Status:** design/spec. A lean v1 already exists (see §0) and is pivoted to the SDK in M0.
- Go import paths use canonical module forms (e.g. `charm.land/bubbletea/v2` as pinned in the existing `go.mod`, `code.gitea.io/sdk/gitea`).

---

## 0. Current State & Reconciliation (brownfield)

This is **not** a greenfield project. A working v1 already exists in this repo and is pushed (CI green). The design below is what we grow it into; this section records exactly what is kept, grown, replaced, and added.

### 0.1 What exists today

- Go module `github.com/gbarany/tea-dash` on the **Bubble Tea v2** (`charm.land/*/v2`) stack.
- `main.go` — hand-rolled `os.Args` entrypoint.
- `internal/ui` — a **single-screen** root `Model`: one `table` of open PRs fetched concurrently across configured repos, `j/k/r/o/q` keys, spinner, empty/error states, `humanizeTime`/`prState` helpers.
- `internal/teacli` — a thin wrapper that **shells out to `tea api <endpoint>`** and JSON-decodes the raw Gitea REST response (plus hand-rolled decode structs in `types.go`, and an API-error-envelope detector).
- `internal/config` — `gopkg.in/yaml.v3`; a `repos:` list + optional `login:`; falls back to the `$PWD` repo.
- `internal/build` — version/build metadata.
- Repo scaffolding: `.github/` CI, `.goreleaser.yaml`, `Makefile`, `.golangci.yml`, `.editorconfig`, `LICENSE`, `README.md`, `docs/architecture.md` (which documents the now-superseded `tea api` stance).

### 0.2 Why we pivot from `tea api` to the SDK

The original v1 shells out to `tea api` (one subprocess per call). A head-to-head benchmark (in-process HTTP vs `exec tea api`, same mock server, fairness-checked — no hidden probe calls) showed:

- **~38 ms of pure process-startup per `tea api` call** (fork/exec + Go runtime init + config load), before any network. In-process HTTP is ~0.1–0.2 ms/call.
- Delay=0 (pure overhead): 50 repos = **5 ms (HTTP)** vs **305–377 ms (`tea api`)** → ~40–75× slower; serial 20 repos → ~190×.
- With a realistic ~30 ms server RTT (network partly masks it): 50 repos @ conc 10 = **160 ms** vs **227 ms** (1.4×); 20 repos @ conc 20 = **34 ms** vs **79 ms** (2.3×).
- The mock was plain HTTP; a real HTTPS instance makes `tea api` pay a **fresh TLS handshake per subprocess** too, widening the gap. The SDK also gives **typed models** and **context cancellation** a subprocess cannot.

Because tea-dash is enrichment-heavy (CI status, review rollup, ±lines per visible row) and auto-refreshing — exactly where per-call overhead compounds — and single-instance (so `tea api`'s "free multi-instance" perk barely applies — the SDK is the better foundation. We keep the *auth-reuse* benefit by reading `tea`'s `~/.config/tea/config.yml` ourselves (§3.2), without the per-call subprocess tax.

### 0.3 Reconciliation map

| Existing | Verdict | What happens |
|---|---|---|
| Repo scaffolding (`.github` CI, `.goreleaser`, `Makefile`, `.golangci`, `LICENSE`, `docs/`) | **KEEP** | Stays; fix module path references. Clone gh-dash's goreleaser refinements over time. |
| `internal/build` | **KEEP** | Version/build info. |
| `main.go` (hand-rolled args) | **GROW** | Becomes a thin `main()` → `cmd.Execute()` (cobra+fang) — see §2. |
| `internal/config` (yaml.v3, repos+login) | **GROW** | Evolve to koanf layered config + structured filter objects + theme/keys/`repoPaths`/`pager` (§4). The `repos:`/`login:` intent survives as `instance:` + section filters. |
| `internal/ui` (single PR-table Model) | **GROW** | Seed of the gh-dash-style architecture: `ProgramContext` DI seam + `Section`/`BaseModel` + multiple views + preview panes. Existing helpers (fan-out, `humanizeTime`, `prState`, empty states, table) carry over. This is a genuine refactor, not a coat of paint. |
| **`internal/teacli`** (shell out to `tea api`) | **REPLACE** | New `internal/gitea` (SDK client, version-probe, raw me-scoped search) + thinned `internal/data` (domain model, `RowData`, fetch/enrich, worker pool, caches). The hand-rolled decode structs in `teacli/types.go` are superseded by SDK types. |
| — | **ADD** | `internal/auth` (read `tea` config.yml → URL+token; else prompt & persist to tea-dash's own XDG config; never write back to `tea`). Plus `internal/gitea/remote.go`, `internal/git`, `internal/shell` (§2). |
| `docs/architecture.md` (documents `tea api` stance) | **UPDATE** | Rewrite to the SDK-direct architecture once M0 lands; until then it is explicitly superseded by this spec. |
| Project memory (`tea-dash-project`, `use-gh-dash-proven-stack`) | **UPDATE** | They record "core stance: shell out to `tea api`," now reversed; updated to the SDK-direct decision + benchmark evidence. |

---

## 1. Overview & Goals

### 1.1 What tea-dash is

tea-dash is a near-fork of gh-dash retargeted from GitHub to Gitea/Forgejo: a keyboard-driven TUI that shows your Pull Requests, Issues, CI status, Notifications, and local branches as configurable filtered "sections," lets you preview and act on them, and is themed and keybound entirely from layered YAML.

The load-bearing architectural difference from gh-dash is the I/O layer. gh-dash reads via GitHub GraphQL (`go-gh`) and writes by shelling out to the `gh` CLI. tea-dash instead talks **directly to Gitea via the Go SDK `code.gitea.io/sdk/gitea`** for both reads and writes. It never shells out to the `tea` CLI. It shells out only for (a) local git operations (via `git-module`) and (b) the external diff pager / user-defined custom commands.

### 1.2 Scope

- **Personal, single-instance, "me"-centric.** The default dashboard answers "my open PRs," "issues assigned to me," "PRs awaiting my review," "my notifications." One Gitea/Forgejo instance per session.
- **Auth reuse.** On startup tea-dash reads the `tea` CLI's own `~/.config/tea/config.yml` to reuse the instance URL + token; if absent it prompts and persists to its own config. It never writes back to `tea`'s config.
- **Structured filters, not a search DSL.** Sections declare typed YAML filter fields (`state`, `type`, `labels[]`, `milestone`, `createdBy`, `assignedBy`, `mentioned`, `reviewRequested`, `since`, `sort`, …) that map to Gitea SDK option structs (directly for per-repo endpoints, via me-scoped booleans on the cross-repo search endpoint — see §3.4/§4.3). The live `/` box sets only a plain keyword (`q=`).
- **Ambitious but phased MVP** (see §8): core PR/Issue browse + preview + comment + merge + external diff + CI column first; reviews, Actions detail, notifications, and local git ops layered on after.

### 1.3 Non-goals (v1)

No multi-instance switching, no admin/sudo impersonation or team-wide views, no GitHub-style search query language, and no `gh`-style extension host (tea-dash is a single static binary). See §11 for the full list.

### 1.4 Design principles

1. **Grow the built v1 into gh-dash's architecture; port it on the v2 stack.** The shape of the `internal/tui/**` tree comes from gh-dash; the transport/data boundary and auth are genuinely new. The existing lean `internal/ui` is refactored into it, not frozen.
2. **Version and flavor fragmentation is the top constraint, not an edge case.** Gitea vs Forgejo, old vs new, differ in feature availability — and can differ at the *same* reported version. Negotiate `ServerVersion` once as a hint, but decide feature availability primarily by **runtime capability probes with 404→downgrade**, and degrade gracefully — never crash.
3. **The CI status column (combined commit status) is the guaranteed baseline; everything richer (Actions runs/jobs) is capability-probed and falls back to it.**
4. **Two-tier fetch:** cheap list first (table renders immediately), enriched detail lazily on preview and per-visible-row. First paint never blocks on enrichment.

---

## 2. Architecture

### 2.1 Design stance

The root model, the `ProgramContext` DI seam, the `Section`/`BaseModel` abstraction, sections, row/preview components, tasks, keys, theme, and config loader are adopted from gh-dash with the **same structure and responsibilities**, expressed on the already-pinned Bubble Tea v2 stack. The load-bearing swaps:

- **Transport/data:** replace `internal/teacli` (`tea api` subprocess) with `internal/gitea` (SDK transport) + a thinned `internal/data` (domain model + fetch/mutate orchestration + caches).
- **Auth:** a new `internal/auth` package (gh-dash relies on the ambient `gh` login; tea-dash must own login).
- **Filters:** GitHub search-DSL strings become structured filter config → SDK option structs / raw search params.
- **Actions view:** new `actionssection`/`actionview` components for the CI/Actions-detail milestone.

### 2.2 Target package tree

```
tea-dash/
├── main.go                          # thin: calls cmd.Execute()
├── cmd/
│   └── root.go                      # cobra+fang; --config/--debug; resolve cwd git repo +
│                                    #   Gitea remote (SSH/HTTPS parse, host-match); auth bootstrap;
│                                    #   build tui.Model
├── internal/
│   ├── auth/
│   │   └── auth.go                  # resolve instance URL + token: read ~/.config/tea/config.yml,
│   │                               #   apply instance overrides, else prompt + persist to own config
│   ├── gitea/                       # NEW transport. Wraps code.gitea.io/sdk/gitea. (replaces teacli)
│   │   ├── client.go                #   Client wrapper (sdk + baseURL + token + version + me); NewClient
│   │   ├── version.go               #   ServerVersion hint + Features flags (probe-first)
│   │   ├── raw.go                   #   authenticated rawGet/rawPost for search me-booleans + endpoints
│   │   │                           #     the typed SDK lacks (Actions runs/jobs, etc.)
│   │   ├── remote.go                #   parse SSH/HTTPS git remotes; normalize host vs instance URL
│   │   ├── pulls.go                 #   ListRepoPullRequests / GetPullRequest / MergePullRequest / reviews
│   │   ├── issues.go                #   ListRepoIssues + raw cross-repo search / comments / EditIssue
│   │   ├── notifications.go         #   ListNotifications / CheckNotifications / ReadNotification(s)
│   │   ├── actions.go               #   combined status + capability-probed Actions runs/jobs
│   │   └── filters.go               #   FilterConfig -> ListRepo*Options / raw search query params
│   ├── data/                        # domain. TUI-agnostic model structs + RowData.
│   │   ├── model.go                 #   PullRequest / Issue / Notification / CIStatus / ...
│   │   ├── rowdata.go               #   RowData interface + SDK->domain adapters
│   │   ├── fetch.go                 #   two-tier fetch strategies (me-centric + repo-scoped)
│   │   ├── enrich.go                #   bounded per-visible-row enrichment (CI, review decision, lines)
│   │   ├── pool.go                  #   bounded worker pool, fan-out, pagination
│   │   └── cache.go                 #   current user, label name<->ID maps, mentionables
│   ├── config/
│   │   ├── parser.go                #   koanf layered YAML; Config/SectionConfig structs
│   │   ├── filters.go               #   structured PrIssueFilter / NotificationFilter / ActionsFilter
│   │   ├── feature_flags.go
│   │   └── defaults.go
│   ├── build/build.go               # KEEP. version/build metadata.
│   ├── git/git.go                   # NEW. Local git via git-module (branches/remotes/origin).
│   ├── shell/shell.go               # NEW. $SHELL resolution for custom cmds + external pager.
│   └── tui/                         # grown from today's internal/ui
│       ├── ui.go                    # Root Model (Init/Update/View, routing, layout).
│       ├── tasks.go
│       ├── context/
│       │   ├── context.go           # ProgramContext DI seam (Gitea fields).
│       │   └── styles.go            # InitStyles precompute (Lip Gloss v2).
│       ├── constants/               # TaskFinishedMsg, Dimensions, logo.
│       ├── keys/                    # KeyMap + per-view maps + Rebind() + custom cmds.
│       ├── theme/                   # Semantic Theme + ParseTheme(cfg) (Lip Gloss v2 colors).
│       ├── markdown/                # Glamour v2 render wrapper.
│       └── components/
│           ├── section/             # Section interface + embeddable BaseModel.
│           ├── prssection/ issuessection/ notificationssection/   # concrete sections.
│           ├── reposection/         # local branches; uses internal/git.
│           ├── actionssection/      # NEW (M3).
│           ├── prrow/ issuerow/ notificationrow/ actionrow/       # row renderers.
│           ├── prview/ issueview/ notificationview/ actionview/   # preview panes.
│           ├── sidebar/ branchsidebar/
│           ├── tasks/               # Mutations: SDK calls (not exec.Command).
│           ├── tabs/ footer/ search/ prompt/ table/ listviewport/
│           └── common/ inputbox/ fuzzyselect/ carousel/
```

### 2.3 Carried-over core abstractions

**Root `Model` (`internal/tui/ui.go`)** keeps gh-dash's shape and the preview-layout logic. It holds one `[]section.Section` slice per view (`prs`, `issues`, `notifications`, `actions`, `repo`), `currSectionId` selects the active section, and per-type preview models. Grown from today's single-table `ui.Model`.

**`ProgramContext` — the DI seam:**

```go
type ProgramContext struct {
    GiteaRepo *gitea.RepoRef     // resolved cwd repo (owner/name), if any
    GitRepo   *gitm.Repository   // local git handle (RepoView)
    Client    *gitea.Client      // shared SDK client handle
    User      string             // resolved "me" login
    ScreenWidth, ScreenHeight           int
    MainContentWidth, MainContentHeight int
    SidebarOpen bool
    Config      *config.Config
    View        config.ViewType
    StartTask   func(task Task) tea.Cmd  // async seam
    Theme       theme.Theme
    Styles      Styles
}
```

**`Section` + `BaseModel`** are carried over: the `Section` interface (`Identifier`+`Component`+`Table`+`Search`+`PromptConfirmation`+`MakeSectionCmd`) and the embeddable `BaseModel` (spinner, `search.Model`, `table.Model`, cursor pagination, `prompt.Model` confirm box, `SectionMsg` envelope) stay decoupled from Gitea specifics via the `data.RowData` interface. Concrete sections embed `BaseModel` and implement only the type-specific bits (`FetchNextPageSectionRows`, `BuildRows`, `GetCurrRow`, the fetched-msg case).

```go
const (
    PRsView           ViewType = "prs"
    IssuesView        ViewType = "issues"
    NotificationsView ViewType = "notifications"
    RepoView          ViewType = "repo"     // local branches / git ops
    ActionsView       ViewType = "actions"  // NEW (M3)
)
```

### 2.4 Async Task/Cmd + optimistic-mutation pattern

The concurrency envelope mirrors gh-dash — only the leaf call changes from GraphQL/`exec` to an in-process SDK/raw call.

**Reads (two-tier).** `ProgramContext.StartTask(task)` registers the task + returns the spinner tick; the section's `FetchNextPageSectionRows()` appends a `tea.Cmd` that fetches off the UI goroutine and wraps the result in `constants.TaskFinishedMsg`. On preview, `data.FetchPullRequest(ctx, repo, index)` lazily fetches the heavy `EnrichedPullRequestData`. Cursor pagination maps to the SDK's `Page`/`Limit` + `Response.NextPage`/`X-Total-Count`.

**Writes (optimistic mutation).** Same task registration + `TaskFinishedMsg` + optimistic-update-message shape as gh-dash, but the mutation runs the SDK call inside a plain `tea.Cmd` (in-process, no subprocess):

```go
func MergePR(ctx *context.ProgramContext, sec SectionIdentifier, pr data.RowData, opt gitea.MergePullRequestOption) tea.Cmd {
    n := pr.GetNumber()
    task := context.Task{Id: fmt.Sprintf("merge_%d", n),
        StartText: fmt.Sprintf("Merging PR #%d", n), FinishedText: fmt.Sprintf("PR #%d merged", n),
        State: context.TaskStart}
    startCmd := ctx.StartTask(task)
    return tea.Batch(startCmd, func() tea.Msg {
        err := data.MergePullRequest(ctx, pr.GetRepoNameWithOwner(), n, opt) // SDK, in-process
        merged := err == nil
        return constants.TaskFinishedMsg{
            SectionId: sec.Id, SectionType: sec.Type, TaskId: task.Id, Err: err,
            Msg: UpdatePRMsg{PrNumber: n, IsMerged: &merged},
        }
    })
}
```

The section's `Update` consumes `UpdatePRMsg`/`UpdateIssueMsg` and mutates the in-memory row immediately, with rollback on error. Because writes are now in-process, the full-screen `tea.ExecProcess` suspend/restore dance is reserved only for the **external diff pager** and **custom shell commands**.

### 2.5 The v2 stack (already in place)

The repo is already pinned to **Bubble Tea v2 / Lip Gloss v2 / Bubbles v2** (`charm.land/*/v2`) — the same stack gh-dash uses — so no v1→v2 migration is needed; components are authored directly against the v2 APIs (`View()` returns `tea.View` with `AltScreen: true`; keys via `tea.KeyPressMsg`; Lip Gloss v2 color/renderer). Glamour v2 is added for markdown body rendering; cobra+fang and koanf/v2+validator are adopted as the CLI and config layers replace the hand-rolled `os.Args`/`yaml.v3`.

---

## 3. Gitea Data Layer & Auth

`internal/gitea` owns the SDK client, version negotiation, remote parsing, and low-level primitives. `internal/data` owns the domain model, the `RowData` interface, fetch/enrichment strategies, and process-wide caches. `internal/auth` owns credential resolution.

### 3.1 The `gitea.Client` wrapper

Wraps `*gitea.Client` plus the resolved identity and negotiated server version so callers never re-derive them:

```go
type Client struct {
    sdk        *gitea.Client    // code.gitea.io/sdk/gitea
    baseURL    string           // e.g. https://gitea.example.org
    apiHost    string           // normalized host of baseURL, for remote matching
    token      string           // reused for the raw escape hatch
    httpClient *http.Client     // same transport the SDK uses (TLS/insecure/custom CA)
    version    *version.Version // negotiated server version HINT
    flavor     ServerFlavor     // best-effort {Gitea, Forgejo, Unknown}
    me         *gitea.User      // GetMyUserInfo(), cached at construction
    caps       Features         // capability flags (probe-first; see §6)
}

func NewClient(ctx context.Context, cfg auth.Config) (*Client, error) {
    hc := &http.Client{Timeout: 30 * time.Second}
    tlsCfg := &tls.Config{}
    if cfg.Insecure {
        tlsCfg.InsecureSkipVerify = true
    } else if cfg.CACertPath != "" {          // private-CA support (§3.10)
        pool, err := loadCertPool(cfg.CACertPath)
        if err != nil { return nil, err }
        tlsCfg.RootCAs = pool
    }
    hc.Transport = &http.Transport{TLSClientConfig: tlsCfg}
    sdk, err := gitea.NewClient(cfg.URL,
        gitea.SetToken(cfg.Token), gitea.SetContext(ctx), gitea.SetHTTPClient(hc))
    if err != nil { return nil, err }
    // version hint (§3.5) + GetMyUserInfo (§3.7) ...
}
```

Gitea has **no default API rate limit**, so there is no global token-bucket limiter — concurrency is bounded structurally by the worker pool (§3.6).

### 3.2 Auth resolution (`internal/auth`)

Resolution reads the `tea` CLI's own config and falls back to a prompt; tea-dash never writes back into `tea`'s config.

`~/.config/tea/config.yml` shape (decoded with `gopkg.in/yaml.v3`):

```yaml
logins:
  - name: gitea
    url: https://gitea.example.org
    token: <PAT>          # may be empty if tea stored it in the OS keyring
    default: true
    insecure: false
    user: gbarany
```

**URL resolution (first hit wins):** `instance.url` override → tea login selected by `instance.login` name → tea's `default: true` login → sole login → interactive prompt.

**Token resolution (first hit wins):** `instance.tokenCommand` (shell stdout, trimmed) → `instance.tokenEnv` env var → `TEA_DASH_TOKEN` env → tea-dash's own saved `auth.yml` → the selected tea login's `token` → interactive prompt.

**Empty-token case:** modern `tea` may keep the PAT in the OS keyring rather than the YAML. We cannot read the keyring portably, so an empty `token` falls through to the prompt, then persists to tea-dash's own `auth.yml` (`0600`).

**Where the prompt runs.** Credential *resolution* is plain, non-TUI code in `cmd/root.go` before `tea.NewProgram`. If it resolves cleanly, no prompt is shown. If it must prompt, tea-dash runs a **dedicated, minimal pre-program `tea.Program`** (its own `inputbox` sub-model) that collects URL + PAT, validates via `GetMyUserInfo`, and writes `auth.yml`. Only then is the main `tui.Model` constructed. All of the above resolve into a single `auth.Config{URL, Token, Insecure, CACertPath}`.

### 3.3 Domain model + `RowData`

Section/table/preview code stays generic across PRs, Issues, and Notifications via a `RowData` interface, as in gh-dash. SDK types (`*gitea.Issue`, `*gitea.PullRequest`, `*gitea.NotificationThread`) are adapted into thin domain structs that (a) denormalize owner/repo (cross-repo search returns rows from many repos), (b) fold PR-vs-issue distinctions, and (c) attach lazily-enriched fields.

```go
type RowData interface {
    GetTitle() string
    GetNumber() int64               // Gitea per-repo index
    GetURL() string                 // html_url, for browser/yank bindings
    GetUpdatedAt() time.Time
    GetCreatedAt() time.Time
    GetRepoNameWithOwner() string   // "owner/repo", denormalized
    GetAuthor() string              // poster login
    GetState() string               // open|closed|merged (semantic)
    GetLabels() []Label
}
```

Concrete structs (`PullRequest`, `Issue`, `Notification`, `CIStatus`, `ReviewSummary`, `Comment`, `Label`, `User`) live in `model.go`. Tier-2/lazy fields (`CI *CIStatus`, `Reviews *ReviewSummary`, `Additions/Deletions *int`, `Comments []Comment`, `Files []ChangedFile`, `DiffText string`) are nil/zero in the light list. Adapters (`fromSDKIssue`, `fromSDKPull`) in `rowdata.go` are the **only** place SDK field names leak. `State` is normalized in the adapter: a PR whose SDK `Merged == true` becomes `"merged"`. `Label` carries `ID int64` because per-repo PR label filtering needs IDs (§3.4).

### 3.4 Two-tier "me"-centric fetch strategy

**Repo-scope decision (deterministic, per section):**

1. `repos: [...]` (≥1) → **per-repo fan-out** (one call per repo, goroutines, merged + re-sorted, `X-Total-Count` summed).
2. `repo: owner/name` → single **per-repo** call.
3. neither, but `smartFilteringAtLaunch` + launched inside a matching clone → inject that repo → per-repo.
4. otherwise → **cross-repo search** against `GET /repos/issues/search`.

**Cross-repo (default personal board) — how "me" is actually scoped.** `GET /repos/issues/search` scopes to the authenticated user via **boolean** query params — `created`, `assigned`, `mentioned`, `review_requested`, and (version-gated) `reviewed` — *not* via username strings. The SDK's `ListIssueOption` me-fields (`CreatedBy`→`created_by`, etc.) are **per-repo** params that the search endpoint **silently ignores**. Therefore **the entire cross-repo me-scoped query is built through the raw client** (`raw.go`, §3.8): tea-dash emits `GET /api/v1/repos/issues/search?type=pulls&state=open&created=true…` with the me-booleans set. `@me` on any of `createdBy`/`assignedBy`/`mentioned`/`reviewRequested`/`reviewedByMe` maps to the corresponding boolean = `true`. A **non-`@me` username cannot be honored on the search endpoint at all**; tea-dash forces the section to per-repo if `repo(s)` is set, else drops the constraint with a startup warning. Because there is **no global `/pulls` endpoint**, a cross-repo PR board is this search with `type=pulls`; each returned issue whose `pull_request != nil` is adapted into a `data.PullRequest`.

> This is why `raw.go` is in **M0**: the default "My PRs" board cannot be scoped to the current user through the typed SDK.

**Repo-scoped** sections use typed `sdk.ListRepoIssues` / `sdk.ListRepoPullRequests`. Two gotchas: the per-repo PR endpoint takes `Milestone` as an **ID** and `Labels` as **IDs** (not names) — so the label name→ID map (§3.7) is mandatory; on per-repo issues, `CreatedBy`/`AssignedBy`/`MentionedBy` are **string logins** and *do* work (so a non-`@me` user filter is honored here).

**Tier 1 (light list):** populates only `RowData`-level fields; no N+1 side fetches. **Tier 2 (enriched detail):** fired on select/preview, keyed by `(repo, number)` — `GetPullRequest`/`GetIssue`, `ListIssueComments`, `GetCombinedStatus`, `ListPullReviews`, `GetPullRequestDiff`/`GetPullRequestPatch`, `ListPullRequestFiles`/`ListPullRequestCommits`.

### 3.4a Per-visible-row column enrichment (not just CI)

The default `layout.prs` renders `reviewStatus`, `ci`, and `lines`. **None exist on cross-repo search rows, and `reviewStatus`/`lines` aren't on the per-repo PR list payload either.** Per visible PR row on the default board:

- **`ci`** needs `HeadSHA` (absent on search rows → one `GetPullRequest`) then `GetCombinedStatus`.
- **`reviewStatus`** — Gitea has **no `reviewDecision` rollup**; requires `ListPullReviews` + client-side aggregation (latest non-comment review per reviewer → APPROVED / CHANGES_REQUESTED / pending).
- **`lines`** (+adds/−dels) — absent from list/search payloads; requires `GetPullRequest` or `ListPullRequestFiles`.

Up to **three extra requests per visible row**. To keep "first paint never blocks": Phase 1 renders the light list immediately with placeholders; Phase 2 runs a **bounded, lazy, viewport-only** enrichment pass over the worker pool, coalescing the shared `GetPullRequest`. Results cached: CI by `(repo, sha)`; review decision and line counts by `(repo, number, updatedAt)`. **`reviewStatus` and `lines` are hidden by default** in the shipped config; cross-repo CI enrichment is gated by `ci.enrichCrossRepo`.

### 3.5 Version negotiation (a hint, not the source of truth)

Immediately after `NewClient`, probe once and pin:

```go
raw, _, err := sdk.ServerVersion()   // GET /api/v1/version -> a semver string
if err == nil {
    sdk.SetGiteaVersion(raw)          // pins; disables per-call re-probing round-trips
    c.version, _ = version.NewVersion(raw)
    c.flavor = detectFlavor(raw, respHeaders) // BEST-EFFORT only
}
```

Two honest caveats: (1) **Forgejo's `/api/v1/version` has historically returned a Gitea-compatible semver**, so **flavor detection is best-effort** and never load-bearing; (2) **Gitea and Forgejo can diverge on a feature at the *same* version**, so a version→feature table is unsound as the sole mechanism. The version is a **hint** that pre-classifies obvious cases; **authority is runtime capability probing** (a 404/feature-error downgrades the corresponding `Features` flag for the session, §6.5). `SetGiteaVersion` still matters for latency; `CheckServerVersionConstraint(">= x")` is a fast pre-filter only.

### 3.6 Concurrency & pagination

`gitea.ListOptions{Page, PageSize}` drives paging; default page size is 30, server caps ~50 (`MAX_RESPONSE_ITEMS`), so request `PageSize: 50` and never assume more. Termination via `X-Total-Count` / `resp.NextPage` (Link header). Cross-repo boards, multi-repo fan-out, and lazy enrichment run over a fixed worker pool (default 8, configurable) on an `errgroup.Group` with a shared cancelable context:

```go
g, ctx := errgroup.WithContext(parentCtx)
sem := make(chan struct{}, cfg.Concurrency)
for _, job := range jobs {
    job := job; sem <- struct{}{}
    g.Go(func() error { defer func() { <-sem }(); return job.run(ctx) })
}
err := g.Wait()
```

Cancellation is wired to Bubble Tea's lifecycle: switching sections or superseding an in-flight fetch cancels the old batch's context. Each enrichment result returns as a `tea.Msg` keyed by `(repo, number, fetchGeneration)`; the model drops superseded generations. **Partial failure** is collect-errors: one repo's 403 attaches a non-fatal warning rather than blanking the board; auth/network errors abort via `errgroup`.

### 3.7 Caches (`internal/data/cache.go`)

Process-scoped, `sync.RWMutex`-guarded (`singleflight` for label maps), lazily populated except identity:

1. **Current user** — `GetMyUserInfo()` once at construction, stored on `Client.me`; drives "is this mine / did I review" and `@me` resolution.
2. **Repo label name↔ID map** — per-repo label mutation and per-repo PR filtering use label **IDs** while users configure **names**. Built lazily per target repo on any label need; `ListRepoLabels` → `map[string]int64` + reverse; `singleflight`-deduped; TTL/manual-refresh invalidation.
3. **Mentionable users** — for filter pickers and `@`-completion; lazy, best-effort.

### 3.8 Raw escape hatch (`raw.go`)

Two classes go through the raw client: (1) the **cross-repo me-scoped search** (§3.4) with `created/assigned/mentioned/review_requested/reviewed` booleans; (2) **endpoints absent/incomplete in the pinned typed SDK** — Actions runs/jobs/tasks, the PR `update` endpoint, draft toggling, Actions rerun/cancel/approve. `rawGet(ctx, path, out)` / `rawPost(ctx, path, body, out)` issue authenticated requests against `{baseURL}/api/v1{path}` reusing the same token and `*http.Client`, decoding into tolerant caller structs (unknown fields ignored).

### 3.9 Error handling & version gating

Classified by `*gitea.Response.StatusCode` via one `humanizeError` helper: **401/403** → auth banner + offer re-auth; **404 on a gated endpoint** → feature absent, downgrade flag, degrade; **5xx/transport** → bounded retry with backoff on idempotent GETs; **`context.Canceled`** → swallowed. Mutations never auto-retry; they report the server message verbatim.

**Empty / degraded startup states (first-class, non-error):** no config/first run → auto-write default + auth flow + empty "My PRs" board; zero results → neutral "No pull requests match this filter"; launched outside a git repo → RepoView hidden, `RepoPath` keys disabled with a hint; no `repoPaths` mapping → local-git keys disabled for that row; network down at startup → full-screen retriable panel (`r` retry / `q` quit), no re-auth for transport failure.

**SDK methods used:** `NewClient` · `SetToken` · `SetContext` · `SetHTTPClient` · `SetGiteaVersion` · `ServerVersion` · `CheckServerVersionConstraint` · `GetMyUserInfo` · `GetUserInfo` · `ListRepoIssues` · `GetIssue` · `CreateIssue` · `EditIssue` · `ListRepoPullRequests` · `GetPullRequest` · `GetPullRequestDiff` · `GetPullRequestPatch` · `ListPullRequestFiles` · `ListPullRequestCommits` · `IsPullRequestMerged` · `MergePullRequest` · `EditPullRequest` · `ListPullReviews` · `CreatePullReview` · `SubmitPullReview` · `CreateReviewRequests` · `DeleteReviewRequests` · `ListIssueComments` · `CreateIssueComment` · `EditIssueComment` · `AddIssueLabels` · `ReplaceIssueLabels` · `ClearIssueLabels` · `GetCombinedStatus` · `ListStatuses` · `ListRepoLabels` · `ListRepoMilestones` · `IssueSubscribe`/`IssueUnsubscribe` · `CreatePullRequest` · `ListNotifications` · `ListRepoNotifications` · `GetNotification` · `ReadNotification` · `ReadNotifications` · `CheckNotifications`. Plus **raw**: cross-repo `/repos/issues/search` me-booleans; Actions runs/jobs/tasks/logs/rerun/cancel/approve; `pulls/{index}/update`; draft toggle where untyped.

### 3.10 TLS / private CA

`instance.insecureSkipVerify` disables verification entirely (dev/self-signed). For a private/internal CA, `instance.caCert: <path>` (PEM bundle) is loaded into `tls.Config.RootCAs`. Precedence: explicit `caCert` honored even if `insecureSkipVerify` is false; `insecureSkipVerify: true` overrides. Default is the system trust store.

---

## 4. Config Schema & Structured Filters

Layered koanf YAML (koanf + `StrictMerge`, `include:`, per-repo override, keybinding-union / section-replace merge, `validator/v10`, published JSON schema), retargeted from GitHub's search DSL to structured filter objects.

### 4.1 Deltas from gh-dash

| gh-dash | tea-dash | Why |
|---|---|---|
| `filters: "is:open author:@me label:bug"` (string) | `filters:` is a **structured object** | Gitea has no issue-search DSL. |
| Notification `reason:*` filters | `status[]` + `subjectType[]` + `repo` + time window | **Gitea's notifications API has no `reason` field.** |
| Single global GraphQL search fans out server-side | choose per-repo REST vs raw cross-repo search per section; fan out goroutines | No global `/pulls`; cross-repo "me" filters are booleans. |
| Label list is **AND** | Label list is **OR** (`labels=a,b`) | Gitea's `labels` param is comma-OR. AND only as client-side `requireAllLabels`. |
| `PrNumber`, GraphQL node ids | `PrIndex`/`IssueIndex` + per-repo label name↔ID map | Gitea addresses by per-repo `index`. |

The live `/` box sets a single plain keyword → `q=` (`KeyWord`). No DSL.

### 4.2 Top-level schema (annotated skeleton)

```yaml
# yaml-language-server: $schema=https://tea-dash.dev/schema.json

include:
  - ~/.config/tea-dash/themes/tokyonight.yml
  - ./shared-keybindings.yml    # this file wins over includes; later includes win over earlier

instance:                       # optional; omit to auto-reuse tea's config.yml
  login: work                   # name of a logins[] entry in ~/.config/tea/config.yml
  url: https://git.example.com  # overrides the reused URL
  tokenEnv: TEA_DASH_TOKEN
  tokenCommand: "pass gitea"
  insecureSkipVerify: false
  caCert: ~/.certs/corp-ca.pem
  # token order: tokenCommand > tokenEnv > TEA_DASH_TOKEN > own auth.yml > tea login token > prompt

prSections:
  - title: My Pull Requests
    filters: { type: pulls, state: open, createdBy: "@me" }   # cross-repo => created=true
    limit: 20
issuesSections:
  - title: Assigned to me
    filters: { type: issues, state: open, assignedBy: "@me" } # cross-repo => assigned=true
notificationsSections:
  - title: Unread
    filters: { status: [unread] }
actionsSections:                # version-gated (see §6)
  - title: My repo CI
    filters: { repo: me/website, status: [failure, in_progress] }

defaults:
  view: prs                     # prs | issues | notifications | actions | repo
  prsLimit: 20
  issuesLimit: 20
  notificationsLimit: 20
  actionsLimit: 20
  refetchIntervalMinutes: 30    # 0 disables polling
  dateFormat: relative
  prApproveComment: LGTM
  preview: { open: true, width: 0.45, height: 0.60, position: auto }
  merge:
    style: squash               # merge | rebase | rebase-merge | squash | fast-forward-only
    deleteBranchAfterMerge: true
    autoMergeWhenChecksSucceed: false   # -> MergeWhenChecksSucceed (version-gated)
  layout:
    prs:
      updatedAt: { width: 6 }
      repo:      { width: 20 }
      author:    { width: 15 }
      title:     { grow: true }
      labels:    { width: 22, hidden: true }
      reviewStatus: { hidden: true }   # needs ListPullReviews per visible row -> off by default
      ci:        { }
      state:     { }
      lines:     { width: 16, hidden: true }  # +adds/-dels need GetPullRequest per row -> off by default
      numComments: { }
    issues:      { updatedAt: {width:6}, state: {}, repo: {width:15}, title: {grow:true}, comments: {} }
    notifications:
      updatedAt: { width: 6 }
      repo:      { width: 20 }
      type:      { width: 6 }
      status:    { width: 3 }
      title:     { grow: true }
    actions:     { status: {width:3}, workflow: {width:20}, branch: {width:18}, event: {width:12},
                   actor: {width:12}, createdAt: {width:6}, duration: {width:8} }

keybindings:                    # union-merged by key across layers; gh-dash-shaped
  universal:
    - { key: g, name: lazygit, command: "cd {{.RepoPath}} && lazygit" }
  prs:
    - { key: O, builtin: checkout }
    - { key: m, builtin: merge }
    - { key: d, builtin: diff }
  notifications:
    - { key: d, builtin: markAsDone }

repoPaths:
  me/*:       ~/code/me/*       # wildcard key <=> wildcard value (required pairing)
  me/website: ~/code/website    # exact match wins over wildcard

pager: { diff: diffnav }        # any $PATH command; raw diff (GetPullRequestDiff) piped to its stdin

theme:
  ui: { sectionsShowCount: true, table: { showSeparator: true, compact: false } }
  colors:
    text:       { primary: "#E2E1ED", secondary: "#666CA6", success: "#3DF294",
                  warning: "#E0AF68", error: "#F7768E", faint: "#B0B3BF" }
    background: { selected: "#1B1B33" }
    border:     { primary: "#383B5B" }
  icons:
    ci:     { success: "", pending: "", failure: "", error: "", skipped: "" }
    review: { approved: "", changesRequested: "", pending: "", commented: "" }
    state:  { open: "", closed: "", merged: "" }
    notify: { pull: "", issue: "", commit: "", repository: "", release: "" }

confirmQuit: false
showAuthorIcons: true
smartFilteringAtLaunch: true    # scope sections to the repo you launched from (host-matched)
includeReadNotifications: true
```

Sections carry a **typed** `Filters` object instead of a string:

```go
type PrsSectionConfig struct {
    Title   string          `yaml:"title" validate:"required"`
    Filters PrIssueFilter   `yaml:"filters"`
    Limit   *int            `yaml:"limit,omitempty" validate:"omitempty,gt=0"`
    Layout  PrsLayoutConfig `yaml:"layout,omitempty"`
}
```

### 4.3 `PrIssueFilter` → SDK / raw-search mapping

```go
type PrIssueFilter struct {
    // scope
    Repo   string   `yaml:"repo,omitempty"`   // "owner/name" => per-repo endpoint
    Repos  []string `yaml:"repos,omitempty"`  // multiple => per-repo fan-out; excl. w/ Repo
    Owner  string   `yaml:"owner,omitempty"`  // cross-repo search: restrict to org/user
    // filter fields
    State      string   `yaml:"state,omitempty"`      // open|closed|all
    Type       string   `yaml:"type,omitempty"`       // issues|pulls (set by view; overridable)
    Labels     []string `yaml:"labels,omitempty"`     // NAMES (Gitea semantics: OR)
    RequireAllLabels bool `yaml:"requireAllLabels,omitempty"` // client-side AND post-filter (opt-in)
    Milestone  string   `yaml:"milestone,omitempty"`  // NAME
    Milestones []string `yaml:"milestones,omitempty"` // NAMES (search endpoint only)
    CreatedBy  string   `yaml:"createdBy,omitempty"`  // username (per-repo) or "@me"
    AssignedBy string   `yaml:"assignedBy,omitempty"` // assignee username (per-repo) or "@me"
    Mentioned  string   `yaml:"mentioned,omitempty"`  // username (per-repo) or "@me"
    ReviewRequested bool `yaml:"reviewRequested,omitempty"` // ME (cross-repo boolean)
    ReviewedByMe    bool `yaml:"reviewedByMe,omitempty"`    // ME (cross-repo boolean; version-gated)
    Since  string `yaml:"since,omitempty"`   // ISO-8601 or relative "-2w"
    Before string `yaml:"before,omitempty"`
    Sort   string `yaml:"sort,omitempty"`
    Q      string `yaml:"q,omitempty"`       // keyword; normally set by the live "/" box
    // client-side post-filters (no API equivalent):
    ExcludeLabels  []string `yaml:"excludeLabels,omitempty"`
    ExcludeAuthors []string `yaml:"excludeAuthors,omitempty"`
}
```

`internal/gitea/filters.go` owns translation: `ToListPullRequestsOptions(me, labelMap)` / `ToListRepoIssuesOptions(me, labelMap)` for per-repo, `ToRawSearchParams(me)` for cross-repo (emitting me-booleans). **`@me`:** per-repo endpoints take username strings (`@me`→cached login; other usernames honored); cross-repo search takes booleans scoped to the authed user (`@me`→`true`; other usernames unsupported → force per-repo or drop with warning).

---

## 5. Views, Sections & Keybindings/Actions

Every capability is bound to a concrete SDK call, raw call, git command, or external process. Keys inherit gh-dash's vim baseline; all rebindable via `keybindings.{universal,prs,issues,notifications,actions,branches}`.

### 5.1 View & section model

`tabs.Model` renders view tabs (`prs`, `issues`, `notifications`, `actions`, `repo`). Within the active view, each configured **section** is a horizontally-paged filtered table (`carousel` + `tabs` sub-row); `currSectionId` selects the active one. `tab`/`shift+tab` move between sections; view keys move between views. The preview pane renders the two-tier enriched struct for the cursor row.

### 5.2 Conventions

- **SDK** = a method on `*gitea.Client`; **raw** = a `raw.go` call. owner/repo/index from the selected row. `me` = cached `GetMyUserInfo().UserName`.
- **Confirm:** `no` fires immediately; `yes` = blocking footer prompt (`y`/`enter` confirm, `esc`/`n` cancel); `modal` = multi-field picker overlay; `input` = text box (submit `ctrl+d`, abort `esc`).
- **CI source of truth** for the list column and `w`atch is `GetCombinedStatus(owner, repo, sha)`; the Actions detail view prefers a runs endpoint, falls back to `ListStatuses`.
- **Web URL builder** (`o`, `Y`): `{baseURL}/{owner}/{repo}/{pulls|issues}/{index}` (Gitea PR web path is `/pulls/{index}`), `{baseURL}/{owner}/{repo}/actions/runs/{id}` for runs.
- **`RepoPath`-dependent keys** are disabled with an inline hint when the row's repo has no `repoPaths` mapping, when launched outside a git repo, or when a cross-repo row is missing the needed template var — never a runtime `missingkey=error`.

### 5.3 Command-template variables

Go `text/template`, `missingkey=error`. Gitea uses per-repo **index** (`PrIndex`/`IssueIndex`).

| Variable | Available in | Source |
|---|---|---|
| `RepoName` | all rows | `owner/repo` |
| `RepoPath` | rows whose repo is mapped | `repoPaths` map → local clone dir; startup cwd repo only if its remote host-matches the instance |
| `PrIndex` | prs, branches, notif(PR) | PR per-repo index |
| `IssueIndex` | issues, notif(issue) | issue per-repo index |
| `IssueTitle` | issues | issue title |
| `HeadRefName` / `BaseRefName` | prs (enriched), branches | PR head/base branch |
| `Author` | prs, issues, branches | login |
| `Sha` | prs (enriched) | PR head SHA |
| `InstanceURL` | all | reused base URL |

Before invoking a binding whose template references an unavailable variable, tea-dash **disables the binding for that row** (greyed in help, footer hint on press); if the PR isn't yet enriched, pressing the key triggers enrichment first, then runs.

### 5.4 Universal (all list views)

| Key | Action | Implementation | Confirm |
|---|---|---|---|
| `↑`/`k`, `↓`/`j` | navigate row | local cursor move | no |
| `g`/`home`, `G`/`end` | first / last | local | no |
| `h`/`←`, `l`/`→` | prev / next section | switch active section | no |
| `ctrl+u`, `ctrl+d` | preview page up/down | local scroll | no |
| `p` / `P` | toggle preview pane / position | local | no |
| `r` / `R` | refresh section / all | re-run list query / fan-out (+visible-row enrichment) | no |
| `/` | search (live keyword) | sets `q=` (`KeyWord`), debounced — **not a DSL** | no |
| `o` | open in browser | OS opener on web URL | no |
| `y` / `Y` | copy number / URL | clipboard | no |
| `t` | toggle smart filtering | section's structured filters vs raw | no |
| `?` | help | toggle full-help overlay | no |
| `q` / `ctrl+c` | quit | teardown | `yes` if `confirmQuit` |

### 5.5 PRs view

| Key | Action | Implementation | Confirm |
|---|---|---|---|
| `[` / `]` | prev/next sidebar tab | Overview / Files (`ListPullRequestFiles`) / Commits (`ListPullRequestCommits`) / Checks (`GetCombinedStatus`) | no |
| `e` | expand description | local | no |
| `c` | comment | `input` → `CreateIssueComment(owner, repo, index, {Body})` | input |
| `d` | view **diff** via external pager | `GetPullRequestDiff` → temp file → `tea.ExecProcess` into `pager.diff` | no |
| `v` | **review** (picker) | `modal`: Approve / Request-changes / Comment → `CreatePullReview(...{State, Body, Comments})` | modal+input |
| `@` | request reviewers | `modal` → `CreateReviewRequests`; remove → `DeleteReviewRequests` | modal |
| `a` / `A` | assign / unassign | `EditIssue(...{Assignees})` (default = me) | modal |
| `L` | label | `modal` multi-select → `ReplaceIssueLabels`/`AddIssueLabels`/`ClearIssueLabels` (`[]int64`) | modal |
| `x` / `X` | close / reopen | `EditIssue(...{State})` | yes |
| `W` | toggle draft / ready | typed draft field where present, else `WIP:` title-prefix add/strip | yes |
| `m` | **merge** (style picker) | `modal` → `MergePullRequest(...MergePullRequestOption{...})` (below) | modal |
| `u` | update PR from base | raw POST `/repos/{o}/{r}/pulls/{index}/update` | yes |
| `w` | watch CI checks | poll `GetCombinedStatus(headSha)`; auto-stop on terminal | no |
| `C` / `space` | **checkout PR locally** | git in `RepoPath`: `git fetch <remote> pull/{{.PrIndex}}/head:pr-{index}` then checkout | yes |
| `V` | approve pending fork workflow runs | raw POST Actions approve endpoint **if probed present**; hidden/no-op when absent | yes |
| `s` | switch to Issues section | local | no |

**Merge modal → `MergePullRequestOption`:** Style radio over interactive styles only — `merge` / `rebase` / `rebase-merge` / `squash` / `fast-forward-only`; Delete-branch (`DeleteBranchAfterMerge`); Auto-merge (`MergeWhenChecksSucceed`); Title/Message (prefilled); Force-merge (`ForceMerge`). `fast-forward-only` and auto-merge are capability-probed and hidden when unsupported. **`manually-merged` is excluded** (records an already-completed out-of-band merge + needs a SHA — a footgun; scriptable via custom binding).

### 5.6 Issues view

| Key | Action | Implementation | Confirm |
|---|---|---|---|
| `c` | comment | `CreateIssueComment(...)` | input |
| `a` / `A` | assign / unassign | `EditIssue(...{Assignees})` (default = me) | modal |
| `L` | label | `ReplaceIssueLabels`/`AddIssueLabels`/`ClearIssueLabels` (`[]int64`) | modal |
| `x` / `X` | close / reopen | `EditIssue(...{State})` | yes |
| `M` | set milestone | `EditIssue(...{Milestone})` | modal |
| `b` | subscribe / unsubscribe | `IssueSubscribe`/`IssueUnsubscribe(...me)` | no |
| `n` | new issue | `input`/editor → `CreateIssue(owner, repo, {Title, Body})` | input |
| `C` | create-branch for issue | git in `RepoPath`: `git checkout -b issue-{{.IssueIndex}}` | yes |
| `s` | switch to PRs section | local | no |

### 5.7 Notifications view (poll only)

List = `ListNotifications({Status:[unread], SubjectType, Since, Before, All, Page})`; unread badge = `CheckNotifications()`.

| Key | Action | Implementation | Confirm |
|---|---|---|---|
| `enter` | open subject | `GetNotification(id)` → `GetPullRequest`/`GetIssue`; `ReadNotification(threadID, read)` | no |
| `o` | open in browser | web URL of subject | no |
| `m` | mark as read | `ReadNotification(threadID, read)` | no |
| `M` | mark **all** read | `ReadNotifications({Status:[unread], ToStatus:read})` | yes |
| `D` / `alt+d` | mark (all) as done | `ReadNotification`/`ReadNotifications` (read) + hide locally — Gitea has no "done" | no / yes |
| `b` | toggle pin/bookmark | `ReadNotification(threadID, pinned)` (**capability-probed**, hidden if absent) | no |
| `u` | unsubscribe | `IssueUnsubscribe(...me)` | no |
| `S` | sort by repo | local group/sort | no |
| `r` / `R` | refresh | re-poll `ListNotifications` | no |

When a subject is open, the PR/Issue action set is layered on.

### 5.8 Actions / CI view (new)

Runs list prefers a dedicated endpoint via the raw client (capability-probed); falls back to `ListStatuses`/`GetCombinedStatus` as pseudo-runs. Mutating keys (rerun/cancel/approve) bound only when capability is `rich` **and the specific endpoint is confirmed present**.

| Key | Action | Implementation | Confirm |
|---|---|---|---|
| `enter` | run detail (jobs/steps) | raw `Get` run/jobs path; fallback = `CombinedStatus.Statuses[]` | no |
| `L` | view logs | raw `Get` logs → temp file → `pager.diff`/`$PAGER`; else open `TargetURL` | no |
| `o` | open run in browser | `/actions/runs/{id}` or status `TargetURL` | no |
| `u` / `U` | rerun failed / all jobs | raw POST rerun (**probed**; disabled if absent) | yes |
| `x` | cancel run | raw POST cancel (**probed**; disabled if absent) | yes |
| `w` | watch (live status) | poll runs / `GetCombinedStatus`; auto-stop on terminal | no |
| `y` / `Y` | copy run id / URL | clipboard | no |

### 5.9 Branches view (local git ops)

Operates on the repo in `RepoPath` via `git-module`. The remote and its `(owner, repo)` are resolved by `internal/gitea/remote.go`: remote URLs parsed in **both SSH and HTTPS** forms, host normalized against the instance URL; if **no remote matches the configured instance**, the branches view and SDK-backed branch ops are disabled for that repo with a notice (prevents targeting the wrong host or a mirror).

| Key | Action | Implementation | Confirm |
|---|---|---|---|
| `C` / `space` | checkout branch | `git checkout {Name}` | yes |
| `n` | new branch | `input` → `git checkout -b {name}` | input |
| `O` | **create PR** from branch | (push if ahead, then) `CreatePullRequest(owner, repo, {Head, Base, Title, Body})` | modal |
| `P` | push | `git push {remote} {HeadRefName}` (`-u` if no upstream) | yes |
| `F` | force-push | `git push --force-with-lease {remote} {HeadRefName}` | yes |
| `f` | fast-forward | `git merge --ff-only origin/{Name}` | yes |
| `u` | update branch from base | `git fetch` + `git rebase origin/{BaseRefName}` (or raw `/pulls/{index}/update`) | yes |
| `d` / `backspace` | delete branch | `git branch -D {Name}` | yes |

### 5.10 Custom shell-command keybindings (templated)

Per-view entries `{key, command?, builtin?, name?}`. A `command` (Go `text/template`, `missingkey=error`) runs via `$SHELL` through `internal/shell`, foreground via `tea.ExecProcess`; a `builtin` rebinds a native action; `name` sets the help label. If a command references an unavailable var for the current row, the binding is disabled with a footer hint (§5.3).

Built-in commands per view — **PR:** `checkout, diff, merge, approve, requestChanges, comment, close, reopen, ready, update, viewChecks, nextSidebarTab, prevSidebarTab`; **Issue:** `comment, label, assign, unassign, close, reopen, checkout`; **Notifications:** `view, markAsDone, markAllAsDone, markAsRead, togglePin, unsubscribe`; **Actions:** `rerun, cancel, viewLogs`; **Completions:** `nextSuggestion, previousSuggestion, selectSuggestion`.

---

## 6. CI/Actions & Notifications

**Guiding principle:** the CI status column (combined commit status) is the guaranteed baseline on every Gitea/Forgejo version; everything richer (Actions runs/jobs) is capability-probed and degrades to it. Never crash; never block the list on enrichment.

### 6.1 CI status column

**Source of truth:** the combined commit status of the PR's head commit — `GET /repos/{owner}/{repo}/commits/{sha}/status` → `CombinedStatus{State, Statuses[]{Context, State, TargetURL, Description}}` via `GetCombinedStatus` (and `ListStatuses` for the per-context preview breakdown). This **predates Actions** and is honored by both Gitea and Forgejo (external CI, Actions, and API-pushed statuses all report **into** commit statuses). No version gating.

**Head SHA gotcha:** `ListRepoPullRequests` returns `Head.Sha`; the cross-repo search board does not, so the column pays one lazy `GetPullRequest` per row (batched in §3.4a, shared with `lines`, gated by `ci.enrichCrossRepo`).

**Rendering:** collapse `CombinedStatus.State` to one semantic glyph (themable), optionally with per-state counts: `success`→✓, `pending`→●/spinner, `failure`→✗, `error`→⚠, empty→– (neutral, not an error).

**Fetch (enriched tier, viewport-aware):** Phase 1 placeholder; Phase 2 fans out `GetCombinedStatus` for visible rows only (bounded, cancelable), each patching one cell. **Cache** keyed by `(owner/repo, sha)`: TTL (`ci.cacheTTL` ~30s), background refresh (`ci.refreshInterval` ~60s) eager on `pending`, `r` invalidates visible slice, merge/close/push drops the SHA entry. Terminal states live long; only `pending` polls.

### 6.2 Actions runs detail view

Lists workflow runs and jobs/tasks for a repo; PR-opened → filtered to the PR's head branch/SHA. The runs surface is **newer, version-dependent, differently named across Gitea vs Forgejo, may be absent from the pinned SDK, and may be disabled server-side** — so the view is built around capability detection + a fallback ladder, mutating endpoints individually probed.

**Capability detection** (memoized as `caps.actions ∈ {rich, statusOnly, disabled}`): version pre-filter → one cheap raw probe (`200`→rich, `404`→absent, `403`→disabled) → config override `actions.capability: auto|rich|off`. **The probe, not the version, is authoritative.**

**Fallback ladder:** `rich` → dedicated runs/jobs endpoint (run/workflow/event/actor/status/durations/log URL, PR-scoped by sha/branch; rerun/cancel/approve bound only if each probes present); `statusOnly` → combined statuses as pseudo-checks (`enter` opens `TargetURL`); `disabled` → explanatory empty state, never an error modal. A small badge shows the active tier. Raw responses decode into tolerant structs in `internal/gitea/actions.go`.

### 6.3 Notifications

**Poll-only** — no stream/websocket. Two-tier polling: cheap tick (`CheckNotifications()`, ~30s) for the unread badge; full list (`ListNotifications`, ~60s) on manual `r`, on entering the section, and after any mark action. Back off on error.

**States:** unread / read / pinned. **Optimistic UI:** mark actions patch local state immediately, reconcile on next poll, roll back + toast on error.

**Thinner than GitHub — design around it.** No `reason` field: group/sort by **repository** and **subject type** (pinned first); keep copy honest; the live `/` box doesn't apply (title filtering is client-side). **Version-gating:** `pinned` capability-probed (hide affordance if absent); server-side `SubjectType` filtering recent (filter client-side if ignored); `Before`/`Since` broadly safe for delta fetches.

### 6.4 Cross-cutting config knobs

```yaml
ci:
  refreshInterval: 60s
  cacheTTL: 30s
  showCounts: true
  enrichCrossRepo: true    # pay extra GetPullRequest to resolve head sha (+lines) on the "my PRs" board
actions:
  capability: auto         # auto (probe) | rich | off
notifications:
  pollInterval: 60s
  badgeInterval: 30s
  defaultStatus: [unread]
  subjectTypes: [pull, issue]
```

### 6.5 Feature-flag classification (probe-first)

The negotiated version *pre-seeds* a `Features` struct; each flag is confirmed/downgraded by runtime probing (Gitea and Forgejo diverge at equal versions):

```go
type Features struct {
    FastForwardMerge    bool  // MergeStyleFastForwardOnly
    AutoMerge           bool  // MergeWhenChecksSucceed
    PinnedNotifications bool
    ActionsRunsAPI      bool
    ActionsRerun        bool
    ActionsCancel       bool
    ReviewRequestSearch bool  // review_requested boolean on /repos/issues/search
    ReviewedSearch      bool
    DraftTypedToggle    bool  // typed draft field vs WIP: title-prefix convention
}
```

Graceful degradation: merge menu hides `fast-forward-only`/auto-merge when unsupported; a "Needs My Review" section, if `!ReviewRequestSearch`, falls back to per-repo `ListPullReviews` scanning for a pending review addressed to `me`; `W` uses `WIP:` prefix when `!DraftTypedToggle`; a runtime 404 downgrades the flag for the session.

### 6.6 Portability guarantees

| Feature | Portability | Mechanism |
|---|---|---|
| CI status **column** | **All** versions | `GetCombinedStatus` |
| Actions runs/jobs **detail** | Newer servers, Actions enabled | capability-probed rich endpoints |
| Actions rerun/cancel/approve | Version-dependent, often absent | per-endpoint probe; keys hidden if absent |
| Actions detail **fallback** | **All** versions | expanded combined commit statuses |
| Notifications | **All** versions (poll) | `ListNotifications`/`CheckNotifications` |
| Notification **pinned** state | Capability-probed | probe + hide affordance if absent |
| Cross-repo me-scope | **All** versions | raw `/repos/issues/search` booleans (§3.4) |

---

## 7. Theming & Distribution

**Theming.** Two layers from gh-dash: a semantic `theme.Theme` (named roles + per-state CI/review/notification colors) via `ParseTheme(cfg)`, then `context/styles.go` precomputes Lip Gloss v2 styles once per resize into `context.Styles`. Nerd Font icons are semantic (configurable glyph per role) with ASCII fallbacks; truecolor/ANSI degradation handled at the Lip Gloss layer.

**Keybindings.** Vim-style, fully rebindable `keys.KeyMap` (v2 `KeyPressMsg`) with per-view keymaps and a `Rebind()` pass that overrides builtins and registers custom shell-command bindings, executed through `internal/shell` against `$SHELL`. The external diff pager reuses the same shell-exec seam. Help/footer is generated from the keymap; capability-disabled and `RepoPath`-unavailable bindings render greyed.

**Distribution.** Single static Go binary — `CGO_ENABLED=0`, no `gh`/`tea` runtime dependency (pure SDK). goreleaser v2 (grow the existing `.goreleaser.yaml`): darwin/linux/windows/freebsd × amd64/arm64/arm/386, `ldflags -s -w` with `-X` for version/commit/date. Ship archives + Homebrew tap + `go install`. **No extension host** (unlike gh-dash). Config: XDG paths, optional per-repo `.tea-dash.yml`, `include:`; published JSON Schema with the `# yaml-language-server:` hint; validated on load. Flags `--config`/`--debug` now; env/flags for URL+token later. Pin exact module versions so churn is localized behind `internal/gitea`/`internal/shell`.

---

## 8. Phased Milestones

Each milestone ships an independently usable binary, built with `CGO_ENABLED=0`. Because the stack is already v2, there is no v1→v2 migration; the work is data-layer replacement + growing `internal/ui` into the section architecture.

### M0 — Pivot to SDK (keep the walking PR list working)
- Add `cmd/` (cobra+fang) `main()` → `Execute()`, `--config`/`--debug`; resolve cwd git repo via `internal/git` + `internal/gitea/remote.go`.
- `internal/gitea` client wrapper: `NewClient` (with CA/insecure TLS), `GetMyUserInfo`, `ServerVersion` hint.
- **`raw.go` (required, not deferred):** the cross-repo "My PRs" board = raw `/repos/issues/search?type=pulls&created=true` (me-scope is a boolean the typed SDK can't emit, §3.4). Light `RowData` adapter over search rows.
- `internal/auth`: read `~/.config/tea/config.yml`, else pre-program prompt → persist to own XDG config.
- Grow `internal/config` (koanf): global XDG YAML, one PRs section, defaults (subsuming today's `repos:`/`login:`).
- **Replace `internal/teacli`**: the existing PR table now renders end-to-end via the SDK + raw search instead of `tea api`. Keep the existing fan-out/table/spinner/empty-state code, repointed at `internal/data`.

**Exit:** launches against real Gitea/Forgejo, reads auth from `tea`'s config with zero prompts, and shows **the authed user's** open PRs across repos (me-scoped via raw search) in a refreshing scrollable table — same UX as today's v1, now SDK-backed, `tea api` removed.

### M1 — Core (the daily driver)
- Grow `internal/ui` into `internal/tui`: `ProgramContext`, `Section`/`BaseModel`, `tabs`, preview pane.
- Issues section alongside PRs (cross-repo raw search; `ListRepoIssues` when scoped). Section switching.
- Two-tier fetch: light list + lazy enriched preview (`GetPullRequest`/`GetIssue`, Glamour v2 body render).
- Structured YAML filters → per-repo SDK params / cross-repo raw booleans; live `/` sets `KeyWord` only.
- Comments: read (`ListIssueComments`) + add (`CreateIssueComment`, optimistic).
- Merge with style picker (`manually-merged` excluded), confirm-guarded.
- CI status column (`GetCombinedStatus` on head SHA, viewport-only fan-out; `reviewStatus`/`lines` implemented but hidden by default).
- External diff (`GetPullRequestDiff` → `pager.diff`).
- Refresh, cursor pagination (`X-Total-Count`), theming, base keybindings.

**Exit:** browse PRs + Issues with structured filters, preview, read/add comments, view a colored external diff, see a live CI column, and merge with a chosen style behind confirmation.

### M2 — Reviews, labels, assignment, state transitions
- PR reviews (`CreatePullReview`/`SubmitPullReview`, `ListPullReviews` + derived decision, `CreateReviewRequests`/`DeleteReviewRequests`).
- Labels (lazy per-repo name↔ID map; `Add`/`Replace`/`ClearIssueLabels`; fuzzyselect picker).
- Assign / milestone (`EditIssue`); close/reopen; `W` ready/draft (typed or `WIP:`).
- Unified confirmation + optimistic update + rollback.

**Exit:** approve / request-changes / comment, add/remove labels, (re)assign, set milestone, close/reopen — each confirmed, optimistic, reconciled.

### M3 — Actions runs detail (capability-detected)
- New Actions view; capability detection (§6.2) via authoritative probe; per-endpoint probe for rerun/cancel/approve; graceful fallback to `GetCombinedStatus`/`ListStatuses`; `TargetURL` escape hatch.

**Exit:** on a server with runs APIs, lists runs/jobs with status and (where present) rerun/cancel; on older/Forgejo servers, degrades to combined statuses without error.

### M4 — Notifications
- `notificationssection`/`row`/`view`; poll `ListNotifications` with `Status[]`/`SubjectType[]`/`Since`/`Before`; mark read, unread badge, probed pin, configurable interval, jump to subject.

**Exit:** a Notifications section lists/filters (repo + subject type), shows an unread count, marks single/all read, jumps to the underlying issue/PR.

### M5 — Local repo/branch git ops
- `internal/git` + `branchsidebar`/`branch`; remote/host-match guard; `repoPaths` mapping; checkout PR, create PR (base picker), push. Dirty-tree detection, confirmations, host-mismatch/no-clone disabling.

**Exit:** from a PR row, check it out into the mapped clone; from a local branch, open a PR and push — with confirmation, dirty-state, and remote-match guards. Hidden for repos without a `repoPaths` entry or non-matching remote.

---

## 9. Testing Strategy

Mirror gh-dash's layout: `*_test.go` colocated, `testify/require`, golden files under `testdata/`, table-driven method tests. Model tests drive `tea.Model.Update` directly.

- **Fake Gitea data layer (the big win over gh-dash).** `httptest.NewServer` speaking Gitea REST; client via `gitea.NewClient(server.URL, gitea.SetToken("test"), gitea.SetGiteaVersion(...))`. Fixtures in `internal/data/testdata/*.json`. Request-assertion tests: correct method/path/**query params** — including an **explicit test that the cross-repo board emits me-booleans (`created=true`) to `/repos/issues/search` and does NOT emit `created_by`** (C1 regression guard).
- **Filter → param mapping (highest value).** `filter_test.go`: each structured filter block → expected per-repo options *or* raw-search query string. Cover label name-vs-ID, `labels` OR + `requireAllLabels` AND, me booleans vs per-repo username strings, `Type=pulls`, `Since/Before`, cross-repo-vs-per-repo scope decision.
- **Remote parsing / host matching.** `remote_test.go`: SSH (`git@host:o/r.git`), HTTPS, port/subpath, `www.`/case normalization, mirror mismatch → usable-vs-disabled.
- **Version-gating / capability.** Table of `(version, flavor-hint, feature) → available?` asserting the **probe path is authoritative**: 404 downgrades the flag and takes the fallback (runs → combined status; `review_requested` → per-repo `ListPullReviews`; draft → `WIP:`).
- **Enrichment.** Viewport-only bounded per-visible-row enrichment; coalesced shared `GetPullRequest`; cache keys; generation-drop of superseded messages.
- **Golden/interaction TUI.** `View()` at fixed term size (regen flag); feed `tea.KeyPressMsg` into `Update`: section switching, `/`→`q=`, pagination, confirm gating of merge/close, optimistic row update, disabled-key behavior for unmapped `RepoPath`/un-enriched rows.
- **Config.** Layered load (global XDG + per-repo `.tea-dash.yml` + `include:`), `~/.config/tea/config.yml` ingestion → expected base URL + token, empty-token→prompt fallthrough.
- **Keybindings & theme.** Rebind override + custom command parsing; semantic `Theme` → Lip Gloss v2 precompute, Nerd Font vs ASCII fallback.
- **CI.** `go test ./...` + `go vet` + golangci-lint on every push; matrix a couple of pinned `SetGiteaVersion` values + a fake-server "feature-absent" mode.

---

## 10. Risks & Mitigations

| # | Risk | Impact | Likelihood | Mitigation |
|---|---|---|---|---|
| R1 | **Server-version & flavor fragmentation** (Gitea vs Forgejo, divergence at equal versions) | High | High | Treat `ServerVersion` as a hint; decide features by **probe + 404→downgrade**, not a version table; flavor best-effort. Gate every version-sensitive call; graceful-degrade; version+probe test matrix. |
| R2 | **Actions endpoint instability** (runs/rerun/cancel naming/availability differ, may be absent from SDK) | High | High | M3 capability detection + per-endpoint probe via raw `Get`; **always** fall back to `GetCombinedStatus`. Isolate behind `actions.go`. Ship reliable CI column in M1; defer risky runs-detail to M3. |
| R3 | **No-search-DSL UX gap** | Medium | High | Lean into structured YAML filters; `/` is explicitly a keyword box; ship a "GitHub search → tea-dash YAML" doc; set expectations in README. |
| R4 | **Cross-repo fan-out & request amplification** (up to 3 calls/row for `ci`/`lines`/`reviewStatus`) | Medium | Med-High | Bounded pool + cancellation; **viewport-only lazy** enrichment; coalesce shared `GetPullRequest`; `reviewStatus`/`lines` hidden by default, cross-repo CI gated; cache by SHA / (repo,number,updatedAt); respect ~50 `PageSize` cap. |
| R5 | **Label name/ID mismatch** | Medium | Medium | Lazy per-repo name↔ID map on any label need; centralize translation; dedicated test; unknown name warns and skips. |
| R6 | **Token-scope/permission errors** | Medium | Medium | Verify identity at startup; classify 401/403 vs 404; actionable toast on denied mutation; document required scopes; degrade to read-only. |
| R7 | **SDK / dependency churn** | Medium | Low-Med | Wrap SDK behind `internal/gitea`, shell/pager behind `internal/shell`; pin exact versions; golden tests catch render regressions. |
| R8 | **Auth-source drift** (`tea` config format/location, keyring logins) | Low-Med | Medium | Parse defensively (only need URL+token); on failure/multiple/empty-token, fall back to prompt + own config; never write back to `tea`. |
| R9 | **Remote/host mismatch for local ops** | Medium | Medium | Parse SSH+HTTPS remotes, normalize + match host; disable local ops / `CreatePullRequest` on non-matching remotes; `remote_test.go`. |

**Prioritization:** R1/R2 are the defining technical risks (shape M0/M3 probe-first handling). R3 is the defining product risk (docs/positioning). R4–R6/R9 contained by data-layer design + targeted tests. R7–R8 by strict pinning + defensive parsing.

---

## 11. Non-Goals / Deferred

- **Multi-instance / instance switching.** One instance per session; possible future behind the same DI seam.
- **Admin / sudo impersonation, team/org-wide views.** "me"-centric only.
- **GitHub-style search DSL.** Structured YAML fields; `/` is a plain `KeyWord`. A "GitHub search → YAML" doc bridges the gap.
- **`gh`-style extension host.** Single static binary; no plugin system.
- **Arbitrary-user cross-repo filters.** Search endpoint only scopes to the authed user via booleans; filter another user only via a per-repo section.
- **Notification `reason`-based grouping/filtering.** No `reason` field in the API.
- **Cross-repo Actions runs.** No global runs API; `actionsSections` require a `repo`.
- **Real-time streams.** Notifications and CI are poll-based; delta fetches via `Since`.
- **Env-var/flag auth as the primary path.** Wired but low-priority; primary path is reusing `tea`'s config, then prompt.
- **Reading the OS keyring** for a `tea` login that stored its PAT there — falls through to prompt (documented one-time cost).
- **Writing back to `tea`'s config** — tea-dash only writes its own config.
- **`manually-merged` as an interactive merge action** — excluded from the picker; scriptable via a custom binding only.
