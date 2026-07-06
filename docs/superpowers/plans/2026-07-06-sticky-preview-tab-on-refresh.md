# Sticky Preview Tab On Refresh — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pressing `r` (refresh) — or any preview re-render — keeps the user on their selected preview tab (Comments / Checks / …) instead of snapping back to Overview.

**Architecture:** The preview pane (`internal/ui/components/sidebar`) tracks the selected tab as an index and hard-resets it to 0 in `SetTabs()`, which every re-render funnels through. Add a persistent `sticky` field holding the *title* the user last explicitly selected; `SetTabs` re-selects that title when present and falls back to index 0 otherwise — **without** overwriting `sticky`. This survives the transient "Overview-only" collapse that a refresh causes (it clears the row's detail before re-fetching, and detail-less rows render only an Overview tab).

**Tech Stack:** Go 1.26+, Bubble Tea v2 (`charm.land/bubbletea/v2`), stdlib `testing` (no testify). Spec: `docs/superpowers/specs/2026-07-05-sticky-preview-tab-on-refresh-design.md`.

---

## File structure

- **Modify** `internal/ui/components/sidebar/sidebar.go` — add `sticky` field to `Model`; add `indexOfTitle` helper; rewrite `SetTabs` to preserve by sticky title; make `NextTab` / `PrevTab` / `SelectTab` record the sticky title. This is the entire behavior change.
- **Modify** `internal/ui/components/sidebar/sidebar_test.go` — unit tests for the sticky mechanism.
- **Modify** `internal/ui/app_test.go` — one end-to-end regression test driving the real `r` async sequence.

No production callers change: every reset path already flows through `SetTabs`.

---

### Task 1: Sticky-tab mechanism in the sidebar (unit TDD)

**Files:**
- Test: `internal/ui/components/sidebar/sidebar_test.go`
- Modify: `internal/ui/components/sidebar/sidebar.go`

- [ ] **Step 1: Write the failing unit tests**

Append to `internal/ui/components/sidebar/sidebar_test.go` (the file already imports `"testing"` and the `context` package, and tests live in `package sidebar` so `Tab`, `New`, `SetTabs`, `NextTab`, `CurrentTabTitle` are all in scope — see the existing `TestTabsSwitchContent`):

```go
// TestSetTabsPreservesSelectedTabByTitle: re-rendering the preview (e.g. on a
// refresh) must keep the user's selected tab, not snap back to Overview.
func TestSetTabsPreservesSelectedTabByTitle(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token"},
		{Title: "Checks", Content: "checks-token"},
	})
	m.NextTab() // user selects Checks

	// Re-render with the same titles (what a refresh does once detail reloads).
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token-2"},
		{Title: "Checks", Content: "checks-token-2"},
	})
	if got := m.CurrentTabTitle(); got != "Checks" {
		t.Fatalf("SetTabs should preserve the selected tab by title, got %q, want Checks", got)
	}
}

// TestSetTabsRestoresTabAfterTransientCollapse reproduces the refresh shape: the
// row's detail is cleared first (so only the Overview tab renders), then the full
// set returns when the re-fetch lands. The selection must survive the round trip.
// This is the case a naive "preserve the current title" approach fails, because
// the collapse rewrites the current title to Overview.
func TestSetTabsRestoresTabAfterTransientCollapse(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token"},
		{Title: "Checks", Content: "checks-token"},
	})
	m.NextTab() // user selects Checks

	// Detail cleared: only Overview renders. Must show Overview (Checks is gone).
	m.SetTabs([]Tab{{Title: "Overview", Content: "overview-only"}})
	if got := m.CurrentTabTitle(); got != "Overview" {
		t.Fatalf("with only Overview present, want Overview, got %q", got)
	}

	// Detail re-lands: full set returns. Must snap back to Checks.
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "overview-token"},
		{Title: "Checks", Content: "checks-token"},
	})
	if got := m.CurrentTabTitle(); got != "Checks" {
		t.Fatalf("selected tab should be restored after the set returns, got %q, want Checks", got)
	}
}

// TestSetTabsDefaultsToFirstWithoutExplicitSelection: a user who never switches
// tabs stays on Overview across re-renders (today's behavior, preserved).
func TestSetTabsDefaultsToFirstWithoutExplicitSelection(t *testing.T) {
	ctx := &context.ProgramContext{
		Styles:        context.DefaultStyles(),
		PreviewOpen:   true,
		PreviewWidth:  40,
		PreviewHeight: 8,
	}
	m := New(ctx)
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "a"},
		{Title: "Checks", Content: "b"},
	})
	m.SetTabs([]Tab{
		{Title: "Overview", Content: "c"},
		{Title: "Checks", Content: "d"},
	})
	if got := m.CurrentTabTitle(); got != "Overview" {
		t.Fatalf("no explicit selection should stay on Overview, got %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/components/sidebar/ -run 'TestSetTabs' -v`
Expected: `TestSetTabsPreservesSelectedTabByTitle` and `TestSetTabsRestoresTabAfterTransientCollapse` FAIL (both report `got "Overview"` where "Checks" is wanted — the current `SetTabs` resets `m.tab = 0`). `TestSetTabsDefaultsToFirstWithoutExplicitSelection` passes already.

- [ ] **Step 3: Add the `sticky` field to `Model`**

In `internal/ui/components/sidebar/sidebar.go`, change the `Model` struct (currently lines 19–25):

```go
// Model wraps a viewport bound to the shared program context.
type Model struct {
	vp      viewport.Model
	ctx     *context.ProgramContext
	tabs    []Tab
	tab     int
	// sticky is the title of the tab the user last explicitly selected. SetTabs
	// re-selects it whenever the new tab set contains it, so re-renders (refresh,
	// enrich, resize) keep the user's tab. It is written only by explicit tab
	// changes — never by SetTabs's fall-back — so a tab that is transiently
	// absent (while a refresh reloads detail) is restored once it reappears.
	sticky  string
	content string
}
```

- [ ] **Step 4: Rewrite `SetTabs` and add `indexOfTitle`**

Replace `SetTabs` (currently lines 46–54) with:

```go
// SetTabs replaces the preview tabs, re-selecting the sticky tab by title when the
// new set contains it (else the first tab), and scrolls to the top. It never
// changes the sticky intent, so a tab that is transiently absent (e.g. while a
// refresh reloads detail) is restored once it reappears. Empty tabs clear the
// viewport.
func (m *Model) SetTabs(tabs []Tab) {
	m.tabs = compactTabs(tabs)
	m.tab = indexOfTitle(m.tabs, m.sticky)
	m.resize()
	m.syncViewport()
	m.vp.GotoTop()
}

// indexOfTitle returns the index of the first tab whose Title equals title, or 0
// when there is no match (including title == "").
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

- [ ] **Step 5: Record the sticky title in `NextTab` / `PrevTab` / `SelectTab`**

Replace `NextTab` (currently lines 67–75), `PrevTab` (77–85), and `SelectTab` (87–98) with:

```go
// NextTab selects the next sidebar tab, wrapping at the end.
func (m *Model) NextTab() {
	if len(m.tabs) <= 1 {
		return
	}
	m.tab = (m.tab + 1) % len(m.tabs)
	m.sticky = m.tabs[m.tab].Title
	m.syncViewport()
	m.vp.GotoTop()
}

// PrevTab selects the previous sidebar tab, wrapping at the beginning.
func (m *Model) PrevTab() {
	if len(m.tabs) <= 1 {
		return
	}
	m.tab = (m.tab - 1 + len(m.tabs)) % len(m.tabs)
	m.sticky = m.tabs[m.tab].Title
	m.syncViewport()
	m.vp.GotoTop()
}

// SelectTab jumps directly to tab i (Task 6's ZonePreviewTab click handler;
// NextTab/PrevTab are relative, for the keyboard's "["/"]"). Out of range is a
// no-op. It records the target as the sticky selection even when i is already the
// current tab, so a click made during a refresh flash is honored — while still
// skipping the scroll reset for the already-selected case.
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

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/ui/components/sidebar/ -run 'TestSetTabs' -v`
Expected: all three PASS.

Run the whole sidebar package to confirm no regression in the existing tab tests (`TestTabsSwitchContent`, `TestSelectTabJumpsDirectly`, `TestFocusedPreviewKeysCycleTabs`):
Run: `go test ./internal/ui/components/sidebar/`
Expected: `ok`.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/components/sidebar/sidebar.go internal/ui/components/sidebar/sidebar_test.go
git commit -m "fix(sidebar): keep selected preview tab across re-render (sticky by title)"
```

---

### Task 2: End-to-end refresh regression test (app level)

Guards that all three async re-snaps a real `r` press triggers (sync handler → `TaskFinishedMsg` → `enrichedMsg`) are cured — not just the synchronous one.

**Files:**
- Test: `internal/ui/app_test.go`

- [ ] **Step 1: Write the regression test**

Append to `internal/ui/app_test.go` (mirrors the setup in the existing `TestClickPreviewTabSwitchesTab`; `data`, `config`, `tea`, and the `fetchedMsg` helper are already imported/defined in this file):

```go
// TestRefreshPreservesSelectedPreviewTab is the end-to-end guard for the sticky
// preview tab. Pressing `r` clears the row's cached detail (collapsing the tab set
// to Overview only), refetches the list, then re-enriches — three chances to snap
// the selection back to Overview. The user's Checks tab must survive all of them.
func TestRefreshPreservesSelectedPreviewTab(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	pr := data.PullRequest{
		Number: 1, Title: "First", RepoNameWithOwner: "gbarany/tea-dash",
		Author: "me", State: "open",
	}
	detail := &data.PullDetail{Body: "body", BaseRef: "main", HeadRef: "first"}
	m = update(t, m, fetchedMsg([]data.PullRequest{pr}))
	m = update(t, m, enrichedMsg{key: m.selKey(), pull: detail})

	// Select the "Checks" tab via a real click (mirrors TestClickPreviewTabSwitchesTab).
	m = viewed(m)
	ranges := m.sidebar.TabRanges()
	if len(ranges) < 2 {
		t.Fatalf("expected at least 2 preview tab ranges, got %+v", ranges)
	}
	x := m.layout.PreviewPanel.X + 1 + ranges[1].Start
	m = update(t, m, tea.MouseClickMsg{X: x, Y: m.layout.PreviewTabsRow, Button: tea.MouseLeft})
	if got := m.sidebar.CurrentTabTitle(); got != "Checks" {
		t.Fatalf("setup: expected to be on Checks, got %q", got)
	}

	// Refresh, then drive the async results the returned command would produce:
	// the list re-fetch (TaskFinishedMsg) and the re-enrich (enrichedMsg).
	m = update(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = update(t, m, fetchedMsg([]data.PullRequest{pr}))
	m = update(t, m, enrichedMsg{key: m.selKey(), pull: detail})

	if got := m.sidebar.CurrentTabTitle(); got != "Checks" {
		t.Fatalf("refresh should keep the Checks tab selected, got %q, want Checks", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it passes with the fix**

Run: `go test ./internal/ui/ -run TestRefreshPreservesSelectedPreviewTab -v`
Expected: PASS.

- [ ] **Step 3 (optional): Confirm the test actually guards the bug**

Temporarily prove the test is meaningful by reverting the fix and watching it fail:

```bash
git stash push -- internal/ui/components/sidebar/sidebar.go
go test ./internal/ui/ -run TestRefreshPreservesSelectedPreviewTab -v   # Expected: FAIL (got "Overview")
git stash pop
```

- [ ] **Step 4: Commit**

```bash
git add internal/ui/app_test.go
git commit -m "test(ui): regression guard for sticky preview tab across refresh"
```

---

### Task 3: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Run the full project check**

Run: `make check`
Expected: gofmt-check clean, `go vet` clean, `go test -race ./...` all PASS.

- [ ] **Step 2: Manual smoke (via the `run` skill or `make run`)**

Open tea-dash, open a PR's preview, switch to the **Checks** or **Comments** tab, press **`r`**.
Expected: the preview reloads but stays on the same tab (it no longer jumps to Overview). Repeat with **`R`** (refresh-all) — same result.

- [ ] **Step 3: Final commit (if the manual smoke required any tweak; otherwise skip)**

```bash
git add -A
git commit -m "chore: sticky preview tab verification tweaks"
```

---

## Self-review notes

- **Spec coverage:** sticky-by-title (Decision) → Task 1 Steps 3–5; transient-collapse survival (spec "transient-collapse complication") → Task 1's `TestSetTabsRestoresTabAfterTransientCollapse` + Task 2; `GotoTop` unchanged (spec "Scroll behavior") → `SetTabs` keeps `m.vp.GotoTop()`; unit + regression tests (spec "Testing") → Tasks 1 & 2; `make check` (spec "Verification") → Task 3.
- **No new production callers**; the single added field is package-internal — matches spec "no caller changes outside `sidebar.go`".
- **Type consistency:** `sticky string`, `indexOfTitle(tabs []Tab, title string) int`, `Tab{Title, Content}`, `enrichedMsg{key string, pull *data.PullDetail}`, `data.PullRequest`, `fetchedMsg([]data.PullRequest)` — all used consistently across tasks and match the existing code read during planning.
