# Sticky preview tab across refresh — design

**Date:** 2026-07-05
**Status:** Approved, ready for implementation planning
**Area:** `internal/ui/components/sidebar`, `internal/ui`

## Problem

Pressing **`r`** (refresh) while parked on a non-default preview tab throws the user
back to the **Overview** tab. If you are reading the **Comments** tab or the Forgejo/Gitea
**Checks** ("action status") tab of the selected pull request, refreshing snaps the preview
back to Overview. This is disruptive during normal review flow.

## Root cause

The preview pane (right-hand panel) renders tabs — **Overview / Comments / Checks / Files /
Reviews**, depending on the row kind. The selected tab is `sidebar.Model.tab` (an `int`).

`sidebar.SetTabs()` (`internal/ui/components/sidebar/sidebar.go:48`) unconditionally resets
`m.tab = 0` every time it is called. `syncSidebar()` (`internal/ui/app.go:1469`) funnels the
per-row render (`RenderPullTabs` / `RenderIssueTabs` / `RenderActionTabs`) into `SetTabs()`,
and `SetContent()` does too. So any re-render of the preview snaps the selection to Overview.

The `r` handler (`internal/ui/app.go:704`) rebuilds the preview:

```go
case !m.scopedBuiltinOverridden("refresh") && key.Matches(msg, m.keys.Refresh):
    if s := m.getCurrSection(); s != nil && !s.GetIsLoading() {
        if m.ctx.PreviewOpen {
            m.clearSelectedPreviewCache()
            m.syncSidebar()      // reset #1 (synchronous)
        }
        return m, s.FetchRows()
    }
```

A single `r` press resets the tab **up to three times**:

1. **#1** synchronously in the refresh handler (`app.go:708`).
2. **#2** when `FetchRows()` completes → `TaskFinishedMsg` → `syncSidebar()` (`app.go:472`).
3. **#3** when the re-fetched detail lands → `enrichedMsg` → `syncSidebar()` (`app.go:528`),
   because step 1's `clearSelectedPreviewCache()` wiped the cached detail and the handler
   re-issues `enrichCurrRow()`.

Consequence: a fix that only patches the `r` key handler is insufficient — the async
completions re-snap the tab. The fix must live at the reset point (`SetTabs`).

### The transient-collapse complication

`clearSelectedPreviewCache()` runs **before** the re-render, and `RenderPullTabs` /
`RenderIssueTabs` / `RenderActionTabs` return **only** `[{Title: "Overview"}]` when their
detail is `nil` (`internal/ui/components/prview/prview.go:79`). So during a refresh the tab set
**transiently collapses to Overview-only**, and the full set (Overview / Checks / Reviews /
Comments) only returns once the re-fetched detail lands (reset #3).

This rules out the naive "preserve the currently-selected tab's title across `SetTabs`": the
collapse (resets #1/#2) rewrites the selected title to "Overview", so when the full set returns
there is nothing left pointing at "Checks". The preserved value must be a **persistent sticky
intent** that survives the collapse — see Implementation.

We must keep clearing the cache: it is what forces `enrichCurrRow()` to actually re-fetch the
detail (it early-returns when the detail is already cached), so `r` genuinely refreshes the
preview's comments/checks. The collapse is therefore inherent to refresh; the fix tolerates it.

The same latent snap exists on `R` (refresh-all), action completions
(`actions.ResultMsg` → `syncSidebar`), and the optional background auto-refresh — all funnel
through `SetTabs`, so fixing `SetTabs` cures them together.

Note: the reported "thrown back to the default page" refers to the preview **tab**, not the
top-level view (pulls/issues/…). The top-level view is unaffected by refresh.

## Decision

**Sticky tab by title.** `SetTabs()` preserves the currently-selected tab by matching its
**title** in the new tab set, rather than hard-resetting to index 0. When the title is absent
from the new set (or there was no previous tab), it falls back to index 0 (Overview).

This was the explicit UX choice: when the cursor moves to a *different* item while parked on
"Comments", the new item opens on its "Comments" tab too (if it has one), like browser/editor
tabs. It also happens to be the simplest, single-point fix.

### Rejected alternative

*Reset-on-navigation, preserve-only-on-same-item-re-render.* Would keep the tab only when
re-rendering the same row and reset to Overview when the selection changes. More faithful to a
literal reading of the complaint, but requires tracking a "which item is the sidebar currently
showing" key and classifying every `syncSidebar` call site. Rejected in favor of the simpler,
more predictable sticky behavior.

## Implementation

All in `internal/ui/components/sidebar/sidebar.go`. Add a persistent `sticky` field holding the
title of the tab the user last **explicitly** selected. `SetTabs` consults it; only explicit
tab actions (`NextTab` / `PrevTab` / `SelectTab`, and the `[`/`]` keys, which route through
those) write to it. Crucially, `SetTabs`'s fallback-to-index-0 does **not** overwrite `sticky`,
so a transient collapse to Overview-only doesn't erase the intent.

```go
type Model struct {
    vp      viewport.Model
    ctx     *context.ProgramContext
    tabs    []Tab
    tab     int
    sticky  string // title of the last explicitly-selected tab; preserved across
                   // SetTabs re-renders even when transiently absent from the set
    content string
}

// SetTabs replaces the preview tabs, re-selecting the sticky tab by title when the
// new set contains it (else the first tab), and scrolls to the top. It never
// changes the sticky intent, so a tab that is transiently absent (e.g. while a
// refresh reloads detail) is restored once it reappears. Empty tabs clear the viewport.
func (m *Model) SetTabs(tabs []Tab) {
    m.tabs = compactTabs(tabs)
    m.tab = indexOfTitle(m.tabs, m.sticky) // 0 when not found or sticky == ""
    m.resize()
    m.syncViewport()
    m.vp.GotoTop()
}

// indexOfTitle returns the index of the first tab whose Title equals title, or
// 0 when there is no match (including title == "").
func indexOfTitle(tabs []Tab, title string) int {
    if title == "" {
        return 0
    }
    for i, t := range tabs {
        if t.Title == title {
            return i
        }
    }
    return 0
}
```

`NextTab` / `PrevTab` record the new selection as sticky (after moving `m.tab`):

```go
m.sticky = m.tabs[m.tab].Title
```

`SelectTab` records sticky for any in-range index — even the redundant "already on it" case —
so a click during the refresh flash is honored, while keeping the existing no-op scroll guard:

```go
func (m *Model) SelectTab(i int) {
    if i < 0 || i >= len(m.tabs) {
        return
    }
    m.sticky = m.tabs[i].Title
    if i == m.tab {
        return // already showing: don't reset scroll
    }
    m.tab = i
    m.syncViewport()
    m.vp.GotoTop()
}
```

- Title matching runs against the **compacted** tab set (`compactTabs` may drop empty tabs),
  so the restored index is always valid.
- No caller changes outside `sidebar.go`. The one added field is package-internal.
- `SetContent()` (single "Overview" tab) is unaffected in practice: a non-Overview `sticky`
  title won't be found, so it collapses to 0 — the only tab present — but `sticky` is retained.
- Default `sticky == ""` reproduces today's behavior for users who never switch tabs (always
  Overview).

### Scroll behavior — unchanged (`GotoTop()` kept)

Scroll still resets to the top on every `SetTabs`. With sticky tabs, `SetTabs` cannot
distinguish "refresh of the same item" from "moved to a different item that also has a
Comments tab" — and for the different-item case the scroll **must** restart (you don't want
item B's Comments opening at item A's scroll offset). The consistent, honest rule is therefore
**keep the tab, restart the scroll**. Preserving scroll on same-item refresh would require
tracking item identity — extra state for a marginal gain, out of scope.

## Testing

**Unit — `internal/ui/components/sidebar/sidebar_test.go`:**
- Preserve-by-title: `SetTabs([Overview, Checks])` → `NextTab()` (→ Checks) → `SetTabs(...)`
  again with the same titles ⇒ `CurrentTabTitle() == "Checks"` (pre-fix: "Overview").
- Transient collapse & restore (the refresh shape): `SetTabs([Overview, Checks])` →
  `NextTab()` (→ Checks) → `SetTabs([Overview])` ⇒ shows `"Overview"` (only tab present) →
  `SetTabs([Overview, Checks])` again ⇒ back on `"Checks"`. This is the case the naive
  approach fails.
- First-ever `SetTabs` with no explicit selection (`sticky == ""`) ⇒ index 0 (Overview),
  and re-render keeps Overview.

**Regression — `internal/ui/app_test.go`:**
- Reproduce the real bug end-to-end: preview open on a pull with detail loaded (so the
  Checks/Comments tab exists), switch to a non-Overview tab, press `r`, drive the resulting
  `TaskFinishedMsg` + `enrichedMsg`, and assert `CurrentTabTitle()` is unchanged. This guards
  all three async re-snaps, not just the synchronous one.

## Scope guard (YAGNI)

Out of scope: config option to toggle stickiness, per-view tab memory, scroll-position
persistence across refresh. The change is: re-rendering the preview keeps you on your tab.

## Verification

- `make check` (gofmt-check + go vet + `go test -race ./...`) green.
- Manual: open tea-dash, open a PR preview, switch to Checks/Comments, press `r`,
  confirm the tab stays selected.
