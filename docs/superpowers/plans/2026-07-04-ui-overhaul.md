# UI/UX Overhaul Implementation Plan (Plan 2 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The framed, edge-to-edge, mouse-first, lazygit-familiar TUI from the approved spec — full-space layout, clean keymap remap with preview focus, help overlay, command palette, icons/state colors, toasts — verified end-to-end against the Plan-1 mockgitea harness and live in tmux.

**Architecture:** A new `internal/ui/layout` package computes every screen rectangle once per resize and doubles as the mouse hit-testing zone registry; `app.go` renders into those rects (bordered panels, header, status bar) and routes all mouse events through zone lookups. The keymap is restructured into grouped bindings that generate the help overlay. New overlay components (help, palette) intercept input before section routing; a focus flag routes keys to the preview. All features are exercised three ways: unit tests, app-level `Update/View` tests, and the Plan-1 e2e harness (`internal/ui/e2e_mock_test.go`'s `drain` + mockgitea) plus interactive tmux verification with `./bin/tea-dash --mock`.

**Tech Stack:** Go 1.26, Bubble Tea v2 / bubbles v2 / lipgloss v2 (no new deps), stdlib testing only. Spec: `docs/superpowers/specs/2026-07-03-ui-ux-overhaul-mock-mode-design.md` §1–4, 6–8. Branch: `feat/ui-overhaul-mock-mode`.

**Ground rules (every task):**
- TDD; contract through behavior, not mocks. `make check` green before every commit.
- Visual tasks additionally verify in tmux: `make build && tmux split-window -t main:1 -h -d`, find the pane index via `tmux list-panes -t main:1`, run `./bin/tea-dash --mock`, drive with `tmux send-keys`, capture with `tmux capture-pane -p`, kill the pane after. Capture at the pane's natural size AND after `tmux resize-pane` shrink/grow (resize correctness is a spec requirement).
- The e2e harness settles async chains via `drain` (`internal/ui/e2e_mock_test.go`) — reuse it; never sleep.
- When behavior/keys change, update the same task's tests — never leave a failing test for a later task.
- Keybinding config compatibility is binding: `config.Keybindings` builtin names keep working (keys.go `rebindBuiltin` names are the contract).

---

### Task 0: Carry-forwards from Plan 1's final review

**Files:**
- Modify: `internal/mockgitea/store.go`, `internal/mockgitea/server_detail.go`, `internal/git/git_test.go` + `internal/git/branches_test.go` (test helpers only)

- [ ] **Step 1:** store.go dead exported getters (`SetMe`, `Users`, `Runs`, `RunByID`, `LabelDefs`, `MilestoneDefs`): KEEP them (they are the Plan-2 harness API — palette/e2e tests will drive them), but add one doc line each marking them as harness API so a future cleanup doesn't trim them blind.
- [ ] **Step 2:** `handleIssueComments` in server_detail.go: add the `X-Total-Count` header (use `writeList`) so a future paginated-comments client doesn't silently get no total.
- [ ] **Step 3:** internal/git test helpers that create commits must isolate git config exactly as `mockgitea.SeedLocalRepo` does (`GIT_CONFIG_GLOBAL=os.DevNull`, `GIT_CONFIG_NOSYSTEM=1`, pinned identity env) — during the 1Password outage these tests failed on the machine's global `gpgsign=true`; that's an environment leak in the tests. Find the commit-creating helper (grep `exec.Command("git"` in internal/git tests) and pin its env.
- [ ] **Step 4:** Run `go test ./internal/git/ ./internal/mockgitea/ -race -count=1` → green even with signing broken. Commit: `test: isolate git config in test repos; mockgitea polish from final review`.

---

### Task 1: `internal/ui/layout` — rectangles + zones

**Files:**
- Create: `internal/ui/layout/layout.go`, `internal/ui/layout/zones.go`
- Test: `internal/ui/layout/layout_test.go`

- [ ] **Step 1: failing tests first.** Golden-rectangle tests for `Compute(Input{Width, Height, PreviewOpen, PreviewWidth, SectionCount, SearchOpen})` at 80×24, 120×40, 200×60, plus the min-size rules: below 60×15 → `PreviewCollapsed=true`; below 40×10 → `TooSmall=true`. Assert non-overlap and full coverage: header row = y0, status row = y(H-1), panel interiors sum to the remaining height, list+preview widths+borders sum to W.
- [ ] **Step 2: implement.**

```go
// Package layout computes every rectangle of the tea-dash shell from the
// terminal size, and doubles as the mouse hit-testing registry. All
// coordinates are 0-based screen cells; Rect.Contains does hit tests.
package layout

type Rect struct{ X, Y, W, H int }

func (r Rect) Contains(x, y int) bool

type Input struct {
	Width, Height   int
	PreviewOpen     bool
	PreviewWidth    int // 0 = automatic near-50/50
	SectionCount    int
	SearchOpen      bool
}

type Layout struct {
	TooSmall         bool
	PreviewCollapsed bool // auto-collapsed by min-size rule (toggle state preserved by caller)
	Header           Rect // row 0, embedded in the top border line
	ListPanel        Rect // full bordered panel incl. borders
	ListInterior     Rect // content area: table header + rows
	ListRows         Rect // just the data rows (below table header, minus search bar when open)
	PreviewPanel     Rect
	PreviewInterior  Rect
	PreviewTabsRow   int // y of the preview panel's top border (tabs embed there); -1 when closed
	SectionTabsRow   int // y of the list panel's top border
	StatusBar        Rect // last row, embedded in the bottom border line
}

func Compute(in Input) Layout
```

- [ ] **Step 3: zones.** A zone registry rebuilt on every `Compute`-consuming render:

```go
type ZoneKind int
const (
	ZoneNone ZoneKind = iota
	ZoneViewLabel   // Payload = int (view index 0-4)
	ZoneSectionTab  // Payload = int (section id)
	ZoneListRow     // Payload = int (row index)
	ZonePreviewTab  // Payload = int (tab index)
	ZonePreviewBody
	ZoneListBody
	ZoneStatusBar
)

type Zone struct { Kind ZoneKind; Rect Rect; Payload int }

type Zones struct{ /* ordered slice; later registrations win */ }
func (z *Zones) Add(kind ZoneKind, r Rect, payload int)
func (z *Zones) Hit(x, y int) (Zone, bool)
```

Zone tests: register overlapping zones, assert later-wins; assert `Hit` misses outside all rects.
- [ ] **Step 4:** `go test ./internal/ui/layout/ -v` → green. Commit: `feat(ui): add layout package (rects, min-size rules, mouse zones)`.

---

### Task 2: Theme expansion — state colors, icons, focus borders

**Files:**
- Modify: `internal/ui/context/styles.go`, `internal/config/config.go` (Theme), `schema.json` (regenerate — check how `schema_test.go`/`internal/config` produce it)
- Create: `internal/ui/icons/icons.go` + test
- Test: `internal/ui/context/context_test.go` additions, `internal/config/config_test.go` additions

- [ ] **Step 1: config first (failing tests).** `theme.icons: unicode|nerd|ascii` (default unicode; validation rejects others) and `theme.colors.state.{open,draft,merged,closed,success,failure,running,neutral}` string colors. Test YAML round-trip + validation error message.
- [ ] **Step 2: icons package.**

```go
// Package icons maps row/CI/notification states to glyphs for the configured
// icon set. unicode is the default (safe in modern monospace fonts); nerd is
// opt-in; ascii is the lowest common denominator.
package icons

type Set int
const (Unicode Set = iota; Nerd; ASCII)
func Parse(s string) Set // "" -> Unicode

type State int
const (Open State = iota; Draft; Merged; Closed; Success; Failure; Running; Neutral; Unread; AheadArrow; BehindArrow)

func Glyph(set Set, s State) string
```

Table test: every (set, state) returns non-empty; ASCII returns pure ASCII; unicode/nerd sets differ where intended (`● ○ ⇥ ✓ ✗ ◐ ↑ ↓ ·` for unicode; nerd uses ``-family git glyphs).
- [ ] **Step 3: styles.** Extend `context.Styles` with: `BorderFocused`, `BorderBlurred lipgloss.Style` (border color only), `PanelTitle`, `StatusBar`, `StatusToastSuccess/Error/Info`, `StateColors map[icons.State]lipgloss.Style` built from theme config with gh-convention defaults (open green #2da44e, draft gray, merged purple #8250df, closed red #cf222e, success green, failure red, running yellow, neutral dim). `StylesForConfig` wires overrides. Unit-test that a configured override lands in the right style and defaults hold otherwise.
- [ ] **Step 4:** regenerate/extend `schema.json` for the new theme keys (follow whatever `schema_test.go` asserts — read it first; keep it green). Commit: `feat(theme): state colors, icon sets, focus border styles`.

---

### Task 3: Framed shell — header, bordered panels, status bar, full-space

This is the visual core. It replaces the flat stack in `app.go View()` (title line, tab row, body, action bar, status, help line) with the spec §1 shell, and deletes the action-bar row (spec decision #5).

**Files:**
- Modify: `internal/ui/app.go` (View, statusLine, syncMainContentDimensions → layout.Compute; delete `appStyle` padding via `internal/ui/styles.go`; delete actionBar\* funcs and their mouse hooks), `internal/ui/components/sidebar/sidebar.go` (render inside a bordered panel; tabs move to the border line), `internal/ui/components/tabs/tabs.go` (render embedded in a border line), `internal/ui/components/section/section.go` (table height budget comes from layout)
- Create: `internal/ui/components/header/header.go` + test (view labels `1 Pulls · 2 Issues · 3 Inbox · 4 CI · 5 Branches`, active highlighted, host · user on the right; host label is `demo.gitea.local` when mock — add `ui.Options.MockHost string` wired from main.go, closing the spec §5 deferred item), `internal/ui/components/statusbar/statusbar.go` + test (left toast segment / middle counts / right key hints)
- Test: update `internal/ui/app_test.go` layout-dependent assertions + `e2e_mock_test.go` (the "local branches" breadcrumb assertion moves to the header/status equivalents)

- [ ] **Step 1: failing app-level test.** At 120×40 with demo data: first rendered row contains `tea-dash` AND `1 Pulls` AND the mock host; last row contains `? help`; a `│` border column exists between list and preview; row count of `View().Content` == 40 (full-space, no outer padding).
Geometry (adjudicated in T1 review — render to this): row 0 = header AND
top border line; row H-1 = status bar AND bottom border line (corners/rule
drawn with the status content); panels occupy rows 1..H-2 carrying their own
top border (tabs) and bottom border (row H-2); interiors are H-4 tall. The
spec mockup's trailing `└──┘` is box-art closure, not an extra row.

- [ ] **Step 2: implement** — `Model.View()` builds: `header.View(l.Header, ...)` on the top border line; list panel borders with section tabs embedded in `l.SectionTabsRow` (`├─ Open (12) ── Closed (3) ─...`); preview panel with its tabs in the border; `statusbar.View(l.StatusBar, ...)` on the bottom border line. Focused panel (list vs preview — focus flag arrives in Task 4; until then list is always focused) gets `BorderFocused`. `syncMainContentDimensions` becomes a thin wrapper over `layout.Compute` storing the `Layout` on the Model; `MainContentWidth/Height` keep feeding sections (interior dims). Delete the action-bar (`actionButtons`, `actionBarView`, `actionBarY`, `actionButtonAt`, `handleActionButton` and the `msg.Y == m.actionBarY()` branch).
- [ ] **Step 3:** `TooSmall` renders the centered notice; `PreviewCollapsed` hides the preview but preserves `ctx.PreviewOpen` (restore on grow — test both by sending small/large `WindowSizeMsg` sequences).
- [ ] **Step 4:** run full suite; fix every layout-coupled test in app_test.go (expect several — they encode the old Y offsets).
- [ ] **Step 5: tmux verification** (per ground rules): capture at full size, then `tmux resize-pane -x 55 -y 20` (preview auto-collapses: threshold is width<60 or height<15), `-x 39 -y 9` (too-small notice), restore. Confirm borders join cleanly (no gaps/overdraw), all five views, search open state.
- [ ] **Step 6:** Commit: `feat(ui): framed full-space shell — header, bordered panels, status bar`.

---

### Task 4: Keymap remap + preview focus + esc cascade

**Files:**
- Modify: `internal/ui/keys.go` (restructure), `internal/ui/app.go` (routing), `internal/ui/components/sidebar/sidebar.go` (focused-scroll keys `j/k/d/u/g/G`, `[`/`]` tabs)
- Test: `internal/ui/keys_test.go` (new), app_test.go updates, e2e additions

- [ ] **Step 1: keymap structure (failing tests first).** Rework `keyMap` into groups that carry help metadata:

```go
type BindingGroup struct {
	Title    string        // "Views", "List", "Preview", ...
	Bindings []key.Binding // help text on every binding
}
func (k keyMap) Groups(view context.ViewType) []BindingGroup // universal + view-scoped
```

New defaults per spec §2 table: `1`–`5` direct view jump; `s` cycles; `h/l` sections; `enter`/`tab` focus preview; focused-preview keys; `ctrl+d/u` = list half-page (forward to table); `esc` cascade (overlay → prompt → search → preview focus — implement as one `dismissTop()` helper with its own unit test); `o` browser only (drop `enter` from `Open`); drop `ctrl+r` from RefreshAll; `[`/`]` only act when preview focused. `rebindBuiltin` keeps ALL existing builtin names working against the new fields (test: rebinding `open`, `scrollDown`, `nextSection` still lands).
- [ ] **Step 2: preview focus.** `Model.previewFocused bool`; when true, keys route to `m.sidebar` before section routing (sidebar.Update grows the scroll/tabs keys); `enter`/`tab` toggles (Branches view: `enter` stays checkout — guard on `m.ctx.View == context.BranchesView`); focus drives which panel renders `BorderFocused`. Tests: focus toggle, scroll while focused doesn't move list selection, esc unfocuses, `1`–`5` still switch views while focused.
- [ ] **Step 3: migration table** — add to README `### Keys` a `#### Changed in vNEXT` table exactly per spec §2 migration rows.
- [ ] **Step 4:** e2e: `TestE2EViewJumpAndPreviewFocus` — `3` lands on Inbox (assert seeded notification), `1` back to pulls, `enter` focuses preview (border presence in the right panel region), `j` twice scrolls preview not list (list selection unchanged via selectedActionTarget), `esc` unfocuses.
- [ ] **Step 5:** tmux pass: drive `1..5`, `enter`, `j/k`, `esc`, `?` (still the old inline help until Task 5 — fine). Commit: `feat(keys)!: lazygit-style remap — view numbers, preview focus, esc cascade`.

---

### Task 5: Help overlay

**Files:**
- Create: `internal/ui/components/helpoverlay/helpoverlay.go` + test
- Modify: `internal/ui/app.go` (replace `showHelp` line-toggle with the overlay; `?` opens, `esc`/`q`/`?` close; overlay intercepts all keys while open)

- [ ] **Step 1: component (failing tests first).** Centered modal ≈80% of screen (from `layout.Layout`), viewport-scrollable, content generated from `keyMap.Groups(view)` + a static mouse cheatsheet section. Test: every binding in every group appears exactly once (walk Groups and assert `strings.Count(content, help.Key) >= 1`); a config-rebound key shows the rebound key (build a cfg with `keybindings.universal: [{key: F1, builtin: help}]` and assert "F1" renders).
- [ ] **Step 2: wire in app.go** — new `overlay` field (nil | help | palette later); overlay gets first routing priority (before search/prompt); `dismissTop()` closes it first. Render: overlay composited centered over the shell (lipgloss.Place over the frame; simple full replacement of the interior region is acceptable — decide by what renders cleanly, note the choice).
- [ ] **Step 3:** app tests: `?` opens (view contains group titles "Views"/"List"), `j` scrolls overlay not list, `esc` closes, help reflects current view's scoped group ("Inbox" group only in notifications view). tmux pass on all five views. Commit: `feat(ui): full help overlay generated from the keymap`.

---

### Task 6: Mouse — everything through zones

**Files:**
- Modify: `internal/ui/app.go` (`handleMouseClick`/`handleMouseWheel` rewritten over `layout.Zones`; delete `tableDataStartY`, `tabBarY`, `inMainListPane`, `rowIndexAtY`, `selectRowFromMouse` Y-math), `internal/ui/components/header/header.go` (view-label hit ranges registered as zones), tabs/sidebar zone registration
- Test: app_test.go mouse suite rewritten to click zone coordinates taken from `m.layout` (never hard-coded), plus new cases

- [ ] **Step 1: failing tests.** Click view label → view switches; click section tab → section switches; click list row → selection; double-click row (two `tea.MouseClickMsg` within the double-click window — implement `lastClick time.Time + position`, threshold 400ms, monotonic-safe) → preview focus; right-click row (`tea.MouseRight`) → opens palette scoped to row actions (palette lands in Task 7 — for this task assert the intent: a `pendingPaletteScope` field set, or defer the right-click case to Task 7 and note it); click preview tab → tab switch; click preview body → focus; wheel over list → selection moves; wheel over preview → preview scrolls (selection unchanged); wheel over overlay → overlay scrolls; click outside overlay → dismiss.
- [ ] **Step 2: implement** — one `zoneAt(msg.X, msg.Y)` dispatch in `handleMouseClick`; zones registered during View (store `*layout.Zones` on Model, rebuilt per render).
- [ ] **Step 3:** e2e wheel test through drain (wheel messages are sync — direct Update calls). tmux: manual mouse verification is limited — send SGR sequences: `printf '\e[<0;10;5M\e[<0;10;5m'` via `tmux send-keys -H` if workable, else note keyboard-verified + unit-covered. Commit: `feat(ui): first-class mouse — zone-based routing, double-click, preview wheel`.

---

### Task 7: Command palette

**Files:**
- Create: `internal/ui/components/palette/palette.go` + test
- Modify: `internal/ui/app.go` (`:`/`ctrl+p` open; right-click opens row-scoped; enter dispatches through `handleBuiltinKeybinding`/`startCustomCommand`)

- [ ] **Step 1: component (failing tests first).** Text input + filtered list; case-insensitive subsequence match (`"mrg"` matches "Merge"; test ranking: prefix matches sort before scattered ones, stable otherwise); items carry `{Label, Builtin, KeyHint, Kind}`. Item sources: builtin actions valid for current view/row (reuse the validity logic that `actionButtons()` used — resurrect it as `availableActions(view, row)` returning builtins list; it was deleted with the action bar in Task 3, so extract it THERE into a shared func instead of deleting), "Go to <view>"×5, "Section: <title>" per section, custom commands by Name.
- [ ] **Step 2: wiring + tests.** Open via keys; type-to-filter; enter runs (app test: palette-run "Merge" on a demo PR opens the merge prompt exactly like pressing `m`); right-click scope = actions only; esc closes (cascade order with help overlay: whichever is open closes first — both never open at once, assert that).
- [ ] **Step 3:** e2e: open palette, filter "read", run "Mark read" on Inbox row, drain, assert store notification read. tmux pass. Commit: `feat(ui): command palette with fuzzy filtering`.

---

### Task 8: Toasts + unified transient feedback

**Files:**
- Modify: `internal/ui/components/actionfeedback/actionfeedback.go` (+test), `internal/ui/app.go` (merge `notice` into it; statusbar left segment renders it), `internal/ui/components/statusbar/statusbar.go`

- [ ] **Step 1: failing tests.** `actionfeedback` gains states with icons/colors + expiry: `Success/Info` auto-expire ~4s (a `tea.Tick`-driven `expireMsg` keyed by a generation counter — test: Set → tick before deadline keeps it, after clears it); `Error` persists until any keypress; in-flight shows spinner frame. `notice` field deleted — every `m.notice = ...` site becomes `m.actionFeedback.Set(actionfeedback.Info/Error(...))` (grep shows ~30 sites; mechanical).
- [ ] **Step 2:** statusbar renders the segment with `StatusToast*` styles + state glyph via icons package.
- [ ] **Step 3:** e2e: merge round-trip asserts a success toast appears then the section refetches (drain the tick manually — drain drops spinner ticks only; the expire tick must flow). tmux: run a merge in --mock, watch toast. Commit: `feat(ui): toast feedback with auto-expiry replacing transient notices`.

---

### Task 9: Icons + state colors in rows, preview, tabs

**Files:**
- Modify: `internal/ui/components/pullsection/pullsection.go`, `issuesection`, `notificationsection`, `actionsection`, `branchsection` (state cells: glyph + word, colored), `internal/ui/components/prview/prview.go` (headers, check lines, unread dots), `internal/ui/components/tabs/tabs.go` (counts colored when a section has failures — only if cheap; otherwise skip, YAGNI)
- Test: per-section BuildRow tests assert glyph presence per state and ASCII fallback; prview golden-ish substring tests

- [ ] **Step 1 (spike, 15 min): verify bubbles v2 table handles ANSI-styled cell strings** (styled state cell + width truncation). Write a throwaway test rendering a table with a lipgloss-colored cell at narrow width; if truncation breaks (miscounted width), fall back to glyph-only-differentiation in cells (colors only in self-rendered areas: preview/tabs/status), per the spec's stated fallback. Record the outcome in the commit message.
- [ ] **Step 2:** implement per the spike result: `stateCell(state string, set icons.Set, styles)` helper shared by sections (put it in `section` package next to `HumanizeTime`).
- [ ] **Step 3:** icons set comes from `ctx.Config.Theme.Icons` via `icons.Parse`; thread through `ProgramContext` (add `Icons icons.Set` resolved once in `NewWithOptions`). Note (T2 review): `StateColors` only covers the 8 config-backed states — `Unread`/`AheadArrow`/`BehindArrow` have glyphs but no map entry (zero-style on miss); color those from explicit styles, don't index the map. Also: T2's Nerd codepoints are best-effort — do a sighted tmux check with a Nerd Font and correct via the documented name+codepoint comments.
- [ ] **Step 4:** tmux passes with `theme.icons: unicode` (default), then a temp config with `ascii` via `--mock --config` (write the temp config under the scratchpad, not the repo). Commit: `feat(ui): state icons and colors across sections and preview`.

---

### Task 10: Full-space QA sweep + docs + schema

**Files:**
- Modify: `README.md` (Keys tables regenerated from the final keymap + mouse table + migration table check), `docs/architecture.md` (internal/ui description update: layout/zones/overlays), `examples/tea-dash.yml` (theme.icons + state colors example), `schema.json`, `CLAUDE.md` (architecture orientation lines that mention the old flat UI)
- Test: `schema_test.go` green; README keys cross-checked against `keyMap.Groups` (manual, but list every group)

- [ ] **Step 1:** docs updates above (keep hygiene: placeholders only, `make check`'s public-hygiene green).
- [ ] **Step 2: final interactive QA in tmux** — scripted sweep at 3 sizes: all views (`1`–`5`), search, preview focus + scroll, help overlay, palette open/run, a merge with toast, notifications mark-read, CI logs open, branches checkout. Capture each; if anything renders broken, STOP and fix before commit.
- [ ] **Step 3:** `make check` + full e2e. Commit: `docs: UI overhaul — keys, migration table, architecture, schema`.

---

## Task → spec traceability

| Spec section | Tasks |
|---|---|
| §1 layout/full-space | 1, 3, 10 |
| §2 keymap remap | 4, 10(docs) |
| §3 mouse | 1(zones), 6 |
| §4 help overlay / palette / icons / toasts | 5, 7, 9, 8 |
| §5 mock-host header label (deferred item) | 3 |
| §6 error handling (toasts) | 8 |
| §7 testing (unit/app/e2e/tmux) | every task's steps |
| §8 order | task order above |

## Self-review (done at plan time)

- **Spec coverage:** every §1–§4 feature maps to a task; §5's deferred header label closes in Task 3; the action-bar removal (decision #5) is Task 3; migration table (decision #2) in Task 4+10.
- **Placeholders:** none — each step names exact files, code shapes for novel pieces (layout/zones/icons/groups), and the authoritative in-repo pattern to mirror for the rest; the one genuine unknown (bubbles table ANSI cells) is an explicit time-boxed spike with a decided fallback.
- **Type consistency:** `layout.Compute/Layout/Zones` (T1) are what T3/T5/T6 consume; `keyMap.Groups` (T4) is what T5 renders; `icons.Set/Glyph` (T2) is what T8/T9 consume; `availableActions` extracted in T3 is what T7 consumes — checked task-to-task.
