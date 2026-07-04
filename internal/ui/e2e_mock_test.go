package ui

import (
	stdctx "context"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/actionrunner"
	"github.com/gbarany/tea-dash/internal/auth"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/mockgitea"
	"github.com/gbarany/tea-dash/internal/ui/actions"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

// drain runs cmd and recursively feeds resulting msgs back into m, so a real
// async fetch chain (Init()'s section fetches, a dispatched action's result,
// its triggered refetch, ...) fully settles instead of stopping after one
// round trip like the rest of this package's tests (which inject a single
// pre-built TaskFinishedMsg and skip the real Cmd entirely). tea.BatchMsg is
// unpacked and each sub-command drained independently; spinner.TickMsg is
// dropped rather than fed back in, since the section spinner reschedules
// itself on every tick and would never let the recursion end. depth bounds
// per-chain recursion depth (each BatchMsg sub-command restarts one level
// deeper, so it is not a cap on total Cmd executions). No well-behaved chain
// comes near it — those end when a Cmd returns nil — so reaching the cap
// means a cycle or an unexpectedly deep chain, and the test fails loudly
// rather than asserting against a half-settled model.
func drain(t *testing.T, m tea.Model, cmd tea.Cmd, depth int) tea.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	if depth <= 0 {
		t.Fatalf("drain exceeded depth cap %d — cycle or unexpectedly deep chain?", drainDepth)
	}
	msg := cmd()
	switch v := msg.(type) {
	case nil:
		return m
	case tea.BatchMsg:
		for _, sub := range v {
			m = drain(t, m, sub, depth-1)
		}
		return m
	case spinner.TickMsg:
		return m
	default:
		next, nextCmd := m.Update(v)
		return drain(t, next, nextCmd, depth-1)
	}
}

// drainDepth is generous on purpose: correctness (letting a real fetch/
// action/refetch chain fully settle) matters far more here than shaving
// Cmd executions, and an in-process mock server makes every round trip fast
// regardless.
const drainDepth = 30

// drainUntilResult is drain's sibling for TestE2EMergeRoundTrip's Task 8
// assertion: it settles a real dispatch chain exactly like drain, but stops
// as soon as it sees the actions.ResultMsg the dispatched action's Cmd
// eventually produces, returning the model right after Update's
// actions.ResultMsg case has run (so its Task 8 toast — set in that same
// case — is on the model) plus whatever Cmd that case returned, unexecuted.
// Plain drain can't be used for this: it would keep going and execute that
// Cmd's own sub-commands too, including the toast's tea.Tick-driven expiry
// — racing straight past the "toast is showing" state this test wants to
// observe, to its already-expired end state, in one recursive sweep with no
// externally observable checkpoint in between.
//
// found is false if cmd's chain ran to completion (or hit depth) without
// ever producing an actions.ResultMsg — the caller should treat that as a
// test failure, not silently proceed.
func drainUntilResult(t *testing.T, m tea.Model, cmd tea.Cmd, depth int) (next tea.Model, resultCmd tea.Cmd, found bool) {
	t.Helper()
	if cmd == nil || depth <= 0 {
		return m, nil, false
	}
	msg := cmd()
	switch v := msg.(type) {
	case nil:
		return m, nil, false
	case tea.BatchMsg:
		for _, sub := range v {
			var ok bool
			m, resultCmd, ok = drainUntilResult(t, m, sub, depth-1)
			if ok {
				return m, resultCmd, true
			}
		}
		return m, nil, false
	case spinner.TickMsg:
		return m, nil, false
	case actions.ResultMsg:
		next, resultCmd = m.Update(v)
		return next, resultCmd, true
	default:
		next, nextCmd := m.Update(v)
		return drainUntilResult(t, next, nextCmd, depth-1)
	}
}

// newE2EModel builds a real root Model wired to a real gitea.Client talking
// to an in-process mock Gitea server seeded with the teahouse demo dataset —
// the same stack --mock runs, minus the action dispatcher (most e2e
// assertions are read-only; TestE2EMergeRoundTrip wires its own dispatcher,
// mirroring main.go, since it needs the client/cfg/server URL that wiring
// requires and this helper doesn't expose them). Init()'s fetch commands are
// drained for real, so the starting view's sections already hold live rows
// fetched over HTTP from the mock server, not injected test fixtures.
func newE2EModel(t *testing.T) (Model, *mockgitea.Store) {
	t.Helper()
	store := mockgitea.DemoData(time.Now())
	srv := mockgitea.NewServer(store)
	t.Cleanup(srv.Close)

	client, err := gitea.NewClient(stdctx.Background(), auth.Config{URL: srv.URL(), Token: "mock-token"})
	if err != nil {
		t.Fatalf("gitea.NewClient: %v", err)
	}

	cfg := mockgitea.DemoConfig("")
	m := NewWithOptions(cfg, client, Options{})
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = drain(t, m, m.Init(), drainDepth).(Model)
	return m, store
}

// switchView presses the view-cycle key ('s': pulls -> issues ->
// notifications -> actions -> branches -> pulls) and drains the resulting
// fetch, so the newly-switched-to view's rows are the same real fetch
// everything else in this file exercises.
func switchView(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(Model)
	return drain(t, m, cmd, drainDepth).(Model)
}

// TestE2EStartupShowsDemoPulls exercises the same path --mock's very first
// screen does: build the model, drain Init(), render. The demo dataset's
// "My Pull Requests" section (open, created by @me) is non-empty by
// construction (Task 8 guarantees >=3 gabor-authored open PRs), so real rows
// from real teahouse repos must show up.
func TestE2EStartupShowsDemoPulls(t *testing.T) {
	m, _ := newE2EModel(t)
	view := m.View().Content
	for _, want := range []string{"My Pull Requests", "teahouse/"} {
		if !strings.Contains(view, want) {
			t.Fatalf("startup view missing %q:\n%s", want, view)
		}
	}
}

// TestE2ESwitchViewsRendersAllFive cycles through every view and asserts
// each renders its demo content, the same live-fetch path as startup.
// Branches is the interesting one: DemoConfig("") passes no local repo path
// (this test isn't exercising SeedLocalRepo), so DemoConfig sets no
// BranchSections, and the view falls back to ProgramContext.
// GetViewSectionsConfig's built-in default section, titled "Local Branches"
// — the same title DemoConfig itself would use had a repo path been given,
// so this assertion holds either way.
func TestE2ESwitchViewsRendersAllFive(t *testing.T) {
	m, _ := newE2EModel(t)

	m = switchView(t, m) // issues
	if view := m.View().Content; !strings.Contains(view, "Assigned To Me") {
		t.Fatalf("issues view missing %q:\n%s", "Assigned To Me", view)
	}
	m = switchView(t, m) // notifications
	// DemoConfig's Inbox is the view's only section; tabs.Model.View now
	// renders a single section's own "Title (N)" segment too (review fix —
	// the old "hidden below two sections" rule predates the framed shell),
	// so the "Inbox" title does appear on screen. Still also assert on a
	// seeded notification title (data-driven, not just chrome) rather than
	// relying on the footer count string alone. Only a prefix: the title
	// column truncates long titles ("feat: expose /healt…" at the current
	// width), so keep the needle short enough to survive that while
	// staying distinctive.
	if view := m.View().Content; !strings.Contains(view, "Inbox") {
		t.Fatalf("notifications view missing the single-section tab title %q:\n%s", "Inbox", view)
	}
	const seededNotifTitle = "feat: expose /heal"
	if view := m.View().Content; !strings.Contains(view, seededNotifTitle) {
		t.Fatalf("notifications view missing %q:\n%s", seededNotifTitle, view)
	}
	m = switchView(t, m) // actions
	if view := m.View().Content; !strings.Contains(view, "kettle CI") {
		t.Fatalf("actions view missing %q:\n%s", "kettle CI", view)
	}
	m = switchView(t, m) // branches
	// The section's "Local Branches" title now renders in the single-section
	// tab bar (see the notifications block above), so assert on it directly.
	if view := m.View().Content; !strings.Contains(view, "Local Branches") {
		t.Fatalf("branches view missing the single-section tab title %q:\n%s", "Local Branches", view)
	}
	// Additionally (verified empirically, not assumed): with no LocalRepos
	// configured, branchsection falls back to os.Getwd() (see
	// repositoriesFromConfig in internal/ui/components/branchsection), so
	// this renders the real branches of whatever repo `go test` runs in —
	// not an empty state. In a real `tea-dash --mock` run this fallback is
	// unreachable because main.go's resolveEnvironment always passes a
	// non-empty path into DemoConfig (the seeded repo, or an isolated
	// non-repo dir when seeding fails), populating LocalRepos. This test
	// uses DemoConfig("") deliberately, so the fallback fires here.
	//
	// The old flat layout's "  local branches" subtitle line is gone with
	// the framed shell (Task 3) — the header's active view label is the
	// stable equivalent: it's always rendered (independent of section
	// count/title), and its PanelTitle styling is exactly what the header
	// package renders for the active view (see components/header).
	if want := m.ctx.Styles.PanelTitle.Render("5 Branches"); !strings.Contains(m.View().Content, want) {
		t.Fatalf("branches view missing the active header label %q:\n%s", want, m.View().Content)
	}
}

// TestE2EViewJumpAndPreviewFocus covers Task 4's keymap remap end to end
// against the real demo dataset: '3' jumps straight to Inbox (a real
// fetch, drained), '1' jumps back to Pulls, enter focuses the preview,
// j/j scrolls the (real, fetched) preview content rather than moving the
// list selection, and esc unfocuses.
func TestE2EViewJumpAndPreviewFocus(t *testing.T) {
	m, _ := newE2EModel(t)

	// '3' jumps directly to Inbox (Notifications) — a real fetch, drained
	// like switchView's 's' cycling does.
	next, cmd := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	m = next.(Model)
	m = drain(t, m, cmd, drainDepth).(Model)
	if m.ctx.View != context.NotificationsView {
		t.Fatalf("'3' should jump to NotificationsView, got %v", m.ctx.View)
	}
	const seededNotifTitle = "feat: expose /heal" // see TestE2ESwitchViewsRendersAllFive
	if view := m.View().Content; !strings.Contains(view, seededNotifTitle) {
		t.Fatalf("inbox view missing seeded notification %q:\n%s", seededNotifTitle, view)
	}

	// '1' jumps back to Pulls.
	next, cmd = m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = next.(Model)
	m = drain(t, m, cmd, drainDepth).(Model)
	if m.ctx.View != context.PullsView {
		t.Fatalf("'1' should jump back to PullsView, got %v", m.ctx.View)
	}

	before, ok := m.selectedActionTarget()
	if !ok {
		t.Fatal("no row selected in My Pull Requests")
	}

	// enter focuses the preview.
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.previewFocused {
		t.Fatal("enter should focus the preview")
	}

	// j twice scrolls the (real, already-fetched) preview content, not the
	// list: the selected row must not change.
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	after, ok := m.selectedActionTarget()
	if !ok || after != before {
		t.Fatalf("list selection changed while preview focused: before=%+v after=%+v", before, after)
	}

	// esc unfocuses.
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.previewFocused {
		t.Fatal("esc should unfocus the preview")
	}
}

// TestE2EMouseWheelOverPreviewScrollsWithoutMovingSelectionOrFocus covers
// Task 6's wheel-over-preview requirement end to end. Wheel messages are
// synchronous (no Cmd to drain, unlike the fetch chains the rest of this
// file exercises), so this is a direct Update call against the same real,
// already-drained e2e model — the preview pane holds a REAL fetched PR
// body, not an injected fixture. It intentionally doesn't assert the
// preview's rendered content visibly changed: the real seeded demo body is
// short enough that it may already fit entirely (nothing to scroll into),
// so that exact assertion — with a body deliberately made long enough to
// need it — lives in the app-level
// TestMouseWheelInPreviewScrollsRegardlessOfFocus instead. What e2e adds is
// confirming the real click-free wheel routing (zones built from a real
// render, hit-tested against a real layout) doesn't disturb selection or
// focus.
func TestE2EMouseWheelOverPreviewScrollsWithoutMovingSelectionOrFocus(t *testing.T) {
	m, _ := newE2EModel(t)
	if !m.previewVisible() {
		t.Fatal("preview should be visible at 120x40 by default")
	}
	before, ok := m.selectedActionTarget()
	if !ok {
		t.Fatal("no row selected in My Pull Requests")
	}

	m = viewed(m)
	next, cmd := m.Update(tea.MouseWheelMsg{
		X:      m.layout.PreviewInterior.X,
		Y:      m.layout.PreviewInterior.Y,
		Button: tea.MouseWheelDown,
	})
	m = next.(Model)
	m = drain(t, m, cmd, drainDepth).(Model)

	after, ok := m.selectedActionTarget()
	if !ok || after != before {
		t.Fatalf("wheel over preview should not move list selection: before=%+v after=%+v", before, after)
	}
	if m.previewFocused {
		t.Fatal("wheel over preview should scroll it, not focus it")
	}
}

// TestE2EMergeRoundTrip drives a real merge end to end: open the merge
// picker (which itself round-trips to the mock server for real merge
// capabilities), submit the plain first option, and confirm both that the
// store actually recorded the merge and that the triggered refetch drops the
// merged PR out of "My Pull Requests" (open, created by @me — a merged PR is
// neither).
//
// This builds its own model + dispatcher wiring, mirroring main.go's
// resolveEnvironment/run (mockgitea.NewServer over DemoData, a real
// gitea.Client, actionrunner.New wired via SetActionDispatcher) rather than
// reusing newE2EModel, which deliberately leaves the dispatcher unwired for
// the read-only tests above.
func TestE2EMergeRoundTrip(t *testing.T) {
	store := mockgitea.DemoData(time.Now())
	srv := mockgitea.NewServer(store)
	t.Cleanup(srv.Close)

	client, err := gitea.NewClient(stdctx.Background(), auth.Config{URL: srv.URL(), Token: "mock-token"})
	if err != nil {
		t.Fatalf("gitea.NewClient: %v", err)
	}
	cfg := mockgitea.DemoConfig("")
	m := NewWithOptions(cfg, client, Options{})
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = drain(t, m, m.Init(), drainDepth).(Model)

	runner := actionrunner.New(actionrunner.Options{
		Client:      client,
		Config:      cfg,
		InstanceURL: srv.URL(),
		CWD:         t.TempDir(),
	})
	m.SetActionDispatcher(runner.Dispatch)

	// "My Pull Requests" is the starting section; whichever row the table
	// currently has selected is — by construction of its open+created-by-@me
	// filter — a real open gabor PR. Confirm against the store rather than
	// assuming a specific row position, since cross-repo ordering depends on
	// Updated timestamps this test shouldn't need to know about.
	target, ok := m.selectedActionTarget()
	if !ok {
		t.Fatal("no row selected in My Pull Requests")
	}
	before := store.Pull(target.Repo, target.Number)
	if before == nil {
		t.Fatalf("selected target %s#%d not found in store", target.Repo, target.Number)
	}
	if before.State != "open" || before.Author == nil || before.Author.Login != "gabor" {
		t.Fatalf("selected target is not an open gabor PR: %+v", before)
	}

	// 'm' fetches real merge capabilities from the mock server before the
	// picker opens (see loadMergeCapabilitiesCmd) — drain that round trip
	// first.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = next.(Model)
	m = drain(t, m, cmd, drainDepth).(Model)
	if !m.actionPrompt.Active() {
		t.Fatalf("merge picker did not open; notice=%q", m.statusLeftSegment())
	}

	// Keep the success toast's own auto-expiry tick fast (see
	// actionfeedback.WithExpiry's doc comment) — this test fires it for
	// real below and shouldn't block ~4 real seconds to do so.
	m.actionFeedback = m.actionFeedback.WithExpiry(time.Millisecond)

	// The picker opens with option 0 selected (Focus resets the cursor), and
	// mergePromptOptions always lists the plain style first — "Merge" here,
	// since kettle (every gabor-authored open PR lives there) allows every
	// merge style. Enter submits it with no navigation needed and dispatches
	// the real merge. drainUntilResult (not plain drain — see its doc
	// comment) settles everything up through Update's actions.ResultMsg
	// case, which is where the Task 8 success toast is Set — stopping there
	// instead of also draining that case's own returned Cmd (a refetch of
	// the current section batched with the toast's expiry tick) lets this
	// test observe the toast before it expires.
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	nextModel, resultCmd, foundResult := drainUntilResult(t, m, cmd, drainDepth)
	if !foundResult {
		t.Fatal("merge dispatch never produced an actions.ResultMsg")
	}
	m = nextModel.(Model)

	merged := store.Pull(target.Repo, target.Number)
	if merged == nil || !merged.Merged {
		t.Fatalf("pull %s#%d not merged in store: %+v", target.Repo, target.Number, merged)
	}

	// Task 8: the merge's success toast is on the model right after
	// Update's actions.ResultMsg case ran — before its own refetch/expiry
	// Cmd (still held in resultCmd, not yet drained) gets a chance to run.
	wantToast := fmt.Sprintf("Merged %s#%d.", target.Repo, target.Number)
	if got := m.statusLeftSegment(); !strings.Contains(got, wantToast) {
		t.Fatalf("status bar left segment = %q, want it to contain the success toast %q", got, wantToast)
	}
	if got := m.View().Content; !strings.Contains(got, wantToast) {
		t.Fatalf("rendered status bar missing the success toast %q:\n%s", wantToast, got)
	}

	// Now settle the rest (the refetch, dropping the merged PR out of "My
	// Pull Requests", plus the toast's own now-fast expiry tick).
	m = drain(t, m, resultCmd, drainDepth).(Model)
	if got := m.statusLeftSegment(); got != "" {
		t.Fatalf("success toast should be empty once its expiry tick has drained, got %q", got)
	}

	view := m.View().Content
	if strings.Contains(view, before.Title) {
		t.Fatalf("merged PR %q (%s#%d) still rendered in My Pull Requests:\n%s",
			before.Title, target.Repo, target.Number, view)
	}
}

// TestE2EPaletteMarkReadOnInboxRow covers Task 7 Step 3 end to end: open
// the palette, filter to "read", run "Mark read" on a real (unread) Inbox
// row, drain the dispatched fetch, and confirm the mock server's store —
// not just local UI state — actually flipped the notification's Unread
// flag. This exercises the same real-client dispatch path
// TestE2EMergeRoundTrip does, but through the palette's dispatchPaletteItem
// (KindAction -> handleBuiltinKeybinding) instead of a direct key press.
func TestE2EPaletteMarkReadOnInboxRow(t *testing.T) {
	m, store := newE2EModel(t)

	next, cmd := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"}) // Inbox
	m = next.(Model)
	m = drain(t, m, cmd, drainDepth).(Model)
	if m.ctx.View != context.NotificationsView {
		t.Fatalf("'3' should jump to NotificationsView, got %v", m.ctx.View)
	}

	// Find a real, currently-unread seeded notification row to act on —
	// availableActions only offers "Mark read" for one (see
	// TestE2ESwitchViewsRendersAllFive's seeded-dataset comments for why
	// this test doesn't hard-code a row index).
	s := m.getCurrSection()
	if s == nil {
		t.Fatal("no Inbox section")
	}
	found := false
	for i := 0; i < s.NumRows(); i++ {
		s.SelectRow(i)
		if n, ok := s.GetCurrRow().(data.Notification); ok && n.Unread {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no unread notification found in the seeded Inbox to mark read")
	}
	target, ok := m.getCurrRowData().(data.Notification)
	if !ok {
		t.Fatal("selected Inbox row is not a notification")
	}
	if before := store.NotificationByID(target.ID); before == nil || !before.Unread {
		t.Fatalf("selected notification %d should be unread in the store before marking read: %+v", target.ID, before)
	}

	next, cmd = m.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	m = next.(Model)
	m = drain(t, m, cmd, drainDepth).(Model)
	if m.activeOverlay != overlayPalette {
		t.Fatal("':' should open the command palette")
	}

	m = pressRunes(t, m, "read")
	if view := m.View().Content; !strings.Contains(view, "Mark read") {
		t.Fatalf("filtering by 'read' should keep 'Mark read':\n%s", view)
	}

	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	m = drain(t, m, cmd, drainDepth).(Model)

	if m.activeOverlay != overlayNone {
		t.Fatal("running 'Mark read' should close the palette")
	}
	if after := store.NotificationByID(target.ID); after == nil || after.Unread {
		t.Fatalf("notification %d should be read in the store after running 'Mark read' via the palette: %+v", target.ID, after)
	}
}
