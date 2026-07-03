# tea-dash UI/UX overhaul + mock mode — design

Date: 2026-07-03
Status: approved (pending spec review)

## Goal

Make tea-dash's TUI delightful and discoverable for users coming from lazygit,
gh-dash, and yazi: a framed, edge-to-edge layout that uses the whole terminal,
first-class mouse support, a cleanly redesigned keymap, and polish features
(help overlay, command palette, icons/state colors, toasts). Add a `--mock`
mode that runs the full app against an in-process fake Gitea, both as the
backbone for end-to-end tests and as the dataset for a demo video.

Decisions locked with the user:

1. **Full visual refresh** (framed panels, numbered views, status bar,
   edge-to-edge) — not a conservative polish.
2. **Clean keybinding remap**; breaking changes allowed, documented with a
   migration table. `enter` now focuses the preview (`o` opens the browser).
3. **Mock mode = in-process fake Gitea HTTP server** behind the real client.
4. All four delight features are in scope: help overlay, command palette,
   nerd-font icons + state colors, toasts + inline progress.
5. The clickable action-bar row is **dropped**, replaced by the command
   palette (incl. right-click context) and status-bar hints.
6. Demo dataset: fictional **`teahouse`** org.

## Non-goals

- No multi-instance switching, no new Gitea features (this is UI + harness).
- No new third-party TUI dependencies; stay on Bubble Tea v2 / bubbles v2 /
  lipgloss v2 and the stdlib-only test policy.
- No PTY-level test framework; end-to-end tests drive `Model.Update/View`
  directly as today's tests do.

## 1. Layout

The root fills the terminal edge-to-edge (drop `appStyle`'s `Padding(1, 2)`).

```
┌ tea-dash ── 1 Pulls · 2 Issues · 3 Inbox · 4 CI · 5 Branches ── demo.gitea.local · gabor ┐
├─ Open (12) ── Closed (3) ── Review (2) ─────────────┬─ Overview · Checks · Comments ─────┤
│▸#42 ● fix: login flow      teahouse/kettle  mei  2h │ #42 fix: login flow                │
│ #40 ✓ feat: rate limits    teahouse/kettle  arjun 1d│ mei wants to merge fix/login → main│
│  … list panel …                                     │ … preview panel …               42%│
├─────────────────────────────────────────────────────┴────────────────────────────────────┤
│ ⠹ Merging #42… │ 12 PRs · refreshed 2m ago                    ? help · : palette · q quit│
└───────────────────────────────────────────────────────────────────────────────────────────┘
```

- **Header (1 row, in the top border line):** app name, the five views with
  their jump numbers (active view highlighted), instance host · username.
  Display names are `Pulls · Issues · Inbox · CI · Branches` (short enough to
  always fit); config identifiers (`prs`, `issues`, `notifications`,
  `actions`, `branches`) are unchanged.
- **Panels:** the list and the preview are bordered panels. Section tabs are
  embedded in the list panel's top border; preview tabs in the preview
  panel's top border. The focused panel's border uses the focus color, the
  other the faint color. Preview keeps its right-aligned scroll percentage.
- **Status bar (1 row, in the bottom border line):** left segment = toast /
  in-flight action spinner, middle = section status ("12 PRs · showing 12 of
  40 · refreshed 2m ago"), right = context-sensitive key hints ending in
  `? help · q quit`.
- **Preview split:** `defaults.preview.width` still wins; the automatic split
  stays near 50/50. `p` still toggles the preview; when closed the list panel
  takes the full width.
- **Layout module (`internal/ui/layout`):** computes every rectangle (header,
  view labels, section tabs, list rows area, preview tabs, preview body,
  status segments) from `ScreenWidth×ScreenHeight` once per resize/toggle.
  Components render into those rects, and the same rects drive mouse
  hit-testing (a `Zones` registry mapping x,y → zone id + payload, e.g.
  `zoneListRow{index}`, `zoneViewLabel{view}`, `zoneSectionTab{id}`,
  `zonePreviewTab{id}`, `zonePreviewBody`). Today's hand-computed
  `tableDataStartY()`-style offsets are removed.
- **Minimum sizes:** below 60×15 the preview auto-collapses (toggle state is
  preserved and restored when the terminal grows back); below 40×10 render a
  centered "terminal too small (WxH, need 40x10)" notice instead of panels.

## 2. Keymap (clean remap)

`internal/ui/keys.go` is restructured into grouped bindings with a category
and description per binding; the help overlay and README tables are generated
from this single source. Config `keybindings:` keeps working unchanged — same
builtin names, same rebinding semantics, only the *defaults* change.

| Group | Key | Action |
|---|---|---|
| Views | `1`–`5` | jump to Pulls / Issues / Inbox / CI / Branches |
| Views | `s` | cycle views (kept for continuity) |
| Sections | `h`/`l`, `←`/`→` | previous / next section tab |
| List | `j`/`k`, `↑`/`↓` | move selection |
| List | `g`/`G` | first / last row |
| List | `ctrl+d`/`ctrl+u` | half-page down / up in the list |
| Preview | `enter` or `tab` | focus preview (drill in) |
| Preview (focused) | `j/k/d/u/g/G` | scroll · `[`/`]` preview tabs · `esc`/`tab`/`enter` back |
| Preview | `p` | toggle pane · `e` expand body |
| Search | `/` | focus search · `enter` apply · `esc` revert |
| Overlays | `?` | help overlay |
| Overlays | `:` or `ctrl+p` | command palette |
| Global | `esc` | universal dismiss: overlay → prompt → search → preview focus |
| Global | `o` | open in browser |
| Global | `r`/`R` | refresh section / refresh all |
| Global | `y`/`Y` | copy number / URL |
| Global | `t` | toggle current-repo filter |
| Global | `q` | quit (`ctrl+c` always quits) |
| PRs | `c` comment · `a/A` assign/unassign · `L/U` labels · `m` merge · `u` update branch · `W` ready · `w` checks · `x/X` close/reopen · `v` review · `@`/`#` request/remove reviewers · `d` diff · `C`/`space` checkout |
| Issues | `c` comment · `a/A` assign · `L/U` labels · `M` milestone · `b/B` subscribe/unsubscribe · `x/X` close/reopen · `C`/`space` checkout |
| Inbox | `m` read · `u` unread · `M` all read · `b` pin/unpin · `B` unpin |
| CI | `R` rerun · `!` cancel · `L` logs |
| Branches | `C`/`space`/`enter`* checkout · `P` push · `f` fast-forward · `F` force-push · `d`/`backspace` delete |

\* In the Branches view `enter` keeps meaning checkout (its rows have no
preview drill-in target of their own; the preview focus binding falls back to
`tab`). This is the only view-specific `enter` exception.

**Migration table (README):**

| Old | New | Why |
|---|---|---|
| `enter` opened browser | `enter` focuses preview; use `o` | lazygit drill-in convention |
| `ctrl+u`/`ctrl+d` scrolled preview | scroll the **list**; preview scrolls when focused (or mouse wheel) | lazygit/vim list paging |
| `ctrl+r` refresh all | dropped; use `R` | duplicate |
| `[`/`]` preview tabs from list | only while preview is focused | `[`/`]` are panel-tab keys in lazygit |
| — | `1`–`5` view jump, `tab` focus toggle, `?` overlay, `:`/`ctrl+p` palette, `esc` dismiss | new |

## 3. Mouse

All positions resolve through the layout `Zones` registry:

| Gesture | Zone | Effect |
|---|---|---|
| Left click | view label | switch view |
| Left click | section tab | switch section |
| Left click | list row | select row |
| Double left click | list row | focus preview (same as `enter`) |
| Right click | list row | command palette scoped to that row's actions |
| Left click | preview tab | switch preview tab |
| Left click | preview body | focus preview |
| Wheel | list | move selection (existing behavior) |
| Wheel | preview | scroll preview content |
| Wheel / click | help overlay, palette | scroll / activate item; click outside dismisses |

Mouse mode stays `MouseModeCellMotion`. No drag gestures in this iteration.

## 4. Delight features

### Help overlay (`?`)
Centered modal (≈80% of screen, scrollable viewport) generated from the
grouped keymap: universal groups, then the current view's scoped bindings,
then a mouse cheatsheet. Reflects config rebinding automatically because it
reads the live keymap. `esc`/`q`/`?` close. Replaces the current one-line
`showHelp` toggle.

### Command palette (`:` / `ctrl+p`)
Centered modal with a text input + filtered list (case-insensitive
subsequence match, like fzf's default feel). Items: every builtin action
valid for the current view/row (label + current key hint, reusing the same
validity rules as `actionButtons()` today), plus "Go to <view>" and
"Section: <title>" entries, plus user-configured custom commands (by name).
`enter` dispatches through `handleBuiltinKeybinding`/`startCustomCommand` —
the palette adds no new action plumbing. Right-click opens it pre-scoped to
row actions only.

### Icons + state colors
- New `theme.icons: unicode | nerd | ascii` (default `unicode`).
  - unicode: `● ✓ ✗ ○ ◐ ↑ ↓ ◦` (safe in any modern terminal font)
  - nerd: git/CI glyphs from Nerd Fonts (opt-in)
  - ascii: `* + x o` fallback
- New `theme.colors.state.{open,draft,merged,closed,success,failure,running,neutral}`
  with gh-style defaults (open green, draft gray, merged purple, closed red,
  CI success green / failure red / running yellow).
- Applied in: list state cells (icon+word), preview headers, CI check lines,
  notification unread dots, branch ahead/behind markers, tab counts.
- Cell coloring uses per-cell styled strings if the bubbles v2 table
  truncates ANSI-styled cells correctly (verify during implementation); if
  not, only the glyph choice changes per state inside cells, and full color
  is applied in panels we render ourselves (preview, tabs, status bar).

### Toasts + inline progress
`actionfeedback` is extended, not replaced: a status-bar segment with icon +
state color, auto-expiring ~4s after a final state via a tick command
(success ✓ green, error ✗ red persists until keypress, in-flight = spinner).
Errors never expire silently. The transient `notice` field merges into the
same toast mechanism (one system, one place on screen).

## 5. Mock mode

### Activation & wiring
- `tea-dash --mock` (flag only). `main.go`: skip `auth.Resolve`, start
  `mockgitea.NewServer(mockgitea.DemoData(time.Now()))` on `127.0.0.1:0`,
  build the **real** `gitea.NewClient` against `srv.URL()` with token
  `"mock-token"`, and inject a seeded temp git repo for the Branches view.
  Everything above HTTP runs unchanged.
- Header shows `demo.gitea.local` as instance host in mock mode.
- `--mock` composes with `--config` (a config can still shape sections), but
  defaults alone must produce a full demo: all five views populated.

### `internal/mockgitea` package
- **Store:** an in-memory, mutex-guarded object graph: users, repos, labels,
  milestones, pull requests (with reviews, comments, combined status, diff,
  merge capabilities), issues, notification threads, action runs + jobs +
  logs. IDs and timestamps deterministic; timestamps expressed as offsets
  from a `now` passed to `DemoData(now)` so "2h ago" reads well in any
  recording.
- **Server:** `net/http` mux implementing the endpoints the client uses
  (appendix A). Responses are Gitea-shaped JSON; errors use Gitea's
  `{"message": …}` shape with proper status codes. Unknown paths 404 loudly
  (with the path in the body) so client/server drift is caught in tests.
- **Mutations mutate the store:** merge flips state to merged (honoring
  delete-branch), comments append, close/reopen flip, labels/milestones
  change, notifications read/pin update, rerun/cancel flip run status.
  Refetch after an action shows the change — cause-and-effect on camera.
- **Demo dataset (`teahouse` org):** repos `teahouse/kettle`,
  `teahouse/steep`, `teahouse/infra`; users `gabor` (me), `mei`, `arjun`,
  `sofia`, `felix`. ≈15 PRs covering open/draft/merged/closed, CI
  passing/failing/running, review-requested-from-me, conflicts; ≈12 issues
  with labels (`bug`, `feature`, `urgent`) and milestones (`v1.0`, `v1.1`);
  ≈10 notifications (mixed read/unread/pinned); ≈8 action runs with jobs and
  multi-line logs. Bodies are realistic markdown (lists, code blocks) to show
  off the preview rendering.
- **Local branches:** `mockgitea.SeedLocalRepo(dir)` shells out to `git` to
  create a throwaway repo (in the user cache/temp dir) with a few branches
  in ahead/behind/current states; `--mock` injects it via the config's
  `localRepos` mechanism. If `git` is missing or seeding fails, mock mode
  continues with an empty Branches view (a notice explains why).

## 6. Error handling

Unchanged in shape, relocated in presentation: fetch errors render inside the
list panel with a retry hint; action errors are red toasts that persist until
a keypress; browser-open/clipboard failures are toasts carrying the value to
copy. The mock server's error shapes let every one of these paths be
exercised in tests (e.g. a `teahouse/broken` repo that 500s on demand is part
of the test fixtures, not the demo dataset).

## 7. Testing

1. **Unit:** layout rectangles at 80×24 / 120×40 / 200×60 / tiny sizes; zone
   hit-testing; palette filtering; help content generation (every binding
   appears exactly once); icon set fallback; toast expiry; keymap config
   rebinding against the new defaults.
2. **App-level (existing pattern, stdlib only):** drive `Model.Update` with
   `tea.KeyPressMsg`/`tea.MouseClickMsg`/`tea.MouseWheelMsg`: `1–5` view
   jumps, section switching, preview focus/scroll/esc-cascade, search,
   double-click, right-click palette, wheel-over-preview, overlay open/close,
   palette dispatch.
3. **End-to-end:** full `Model` + real `gitea.Client` + `mockgitea` server:
   startup loads demo rows; navigating fetches detail; merging an open PR
   makes it leave the Open section on refetch; marking a notification read
   updates the Inbox; CI rerun flips a run to running. Frame assertions check
   rendered `View()` output contains expected structures at the three
   reference sizes.
4. **Interactive (tmux):** create pane `main:1.4`, run `./bin/tea-dash
   --mock`, drive with `tmux send-keys` (including raw SGR mouse sequences)
   and verify with `capture-pane`: full-space rendering, resize behavior,
   focus borders, overlays, and the demo flow used for the video.
5. `make check` green throughout; new code follows gofmt/vet/race gates.

## 8. Implementation order

1. `internal/mockgitea` (server + demo data + `--mock` wiring) — the harness
   everything else is verified against.
2. `internal/ui/layout` + framed panels + status bar/header (full-space).
3. Keymap remap + preview focus model + esc cascade + help overlay.
4. Mouse zones (click/double-click/right-click/wheel routing).
5. Command palette; toasts; icons + state colors + theme config.
6. Docs: README keybinding + migration tables, architecture.md, example
   config, JSON schema regeneration; changelog entry.

Each step lands with its tests; the plan (writing-plans) breaks these into
commit-sized tasks.

## Appendix A — endpoints the mock server implements

Reads: `GET /api/v1/version`, `GET /api/v1/user`,
`GET /api/v1/repos/issues/search`, `GET /api/v1/repos/{o}/{r}`,
`GET /api/v1/repos/{o}/{r}/issues`, `GET /api/v1/repos/{o}/{r}/issues/{n}`,
`GET /api/v1/repos/{o}/{r}/pulls/{n}`, `GET …/pulls/{n}.diff`,
`GET …/issues/{n}/comments`, `GET …/pulls/{n}/reviews`,
`GET …/commits/{sha}/status`, `GET …/reviewers`, `GET …/labels`,
`GET …/milestones`, `GET /api/v1/notifications`,
`GET …/actions/runs`, `GET …/actions/runs/{id}`, `GET …/actions/runs/{id}/jobs`,
`GET …/actions/jobs/{id}/logs`.

Mutations: `POST …/issues/{n}/comments`, `PATCH …/issues/{n}`,
`PATCH …/pulls/{n}`, `POST …/pulls/{n}/merge`, `POST …/pulls/{n}/update`,
`POST|DELETE …/pulls/{n}/requested_reviewers`, `POST …/pulls/{n}/reviews`,
`POST|DELETE …/issues/{n}/labels`, `PUT|DELETE …/issues/{n}/subscriptions/{user}`,
`PUT /api/v1/notifications`, `PATCH /api/v1/notifications/threads/{id}`,
notification pin/unpin raw endpoints, `POST …/actions/runs/{id}/rerun`,
`POST …/actions/runs/{id}/cancel`.

(The exact list is pinned by `internal/gitea`'s SDK calls and raw paths; the
mock 404s loudly on anything else so drift surfaces in tests.)
