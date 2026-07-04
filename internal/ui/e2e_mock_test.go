package ui

import (
	stdctx "context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/actionrunner"
	"github.com/gbarany/tea-dash/internal/auth"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/mockgitea"
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
	// DemoConfig's Inbox is the view's only section, and tabs.Model.View
	// renders "" for fewer than 2 sections — the "Inbox" title never appears
	// on screen at all (verified empirically, not assumed). "9 notifications"
	// is the footer count and only matches if the real seeded rows loaded.
	// Assert on a seeded notification title (data-driven) rather than the
	// footer count string, which is chrome that a UI restyle may reformat.
	// Only a prefix: the title column truncates long titles ("feat: expose
	// /healt…" at the current width), so keep the needle short enough to
	// survive that while staying distinctive.
	const seededNotifTitle = "feat: expose /heal"
	if view := m.View().Content; !strings.Contains(view, seededNotifTitle) {
		t.Fatalf("notifications view missing %q:\n%s", seededNotifTitle, view)
	}
	m = switchView(t, m) // actions
	if view := m.View().Content; !strings.Contains(view, "kettle CI") {
		t.Fatalf("actions view missing %q:\n%s", "kettle CI", view)
	}
	m = switchView(t, m) // branches
	// Same single-section tab-hiding issue as notifications above, so this
	// doesn't check for the section's "Local Branches" title either.
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
		t.Fatalf("merge picker did not open; notice=%q", m.notice)
	}

	// The picker opens with option 0 selected (Focus resets the cursor), and
	// mergePromptOptions always lists the plain style first — "Merge" here,
	// since kettle (every gabor-authored open PR lives there) allows every
	// merge style. Enter submits it with no navigation needed, dispatches
	// the real merge, and — per the actions.ResultMsg handler — triggers a
	// real refetch of the current section; drain settles all of it.
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	m = drain(t, m, cmd, drainDepth).(Model)

	merged := store.Pull(target.Repo, target.Number)
	if merged == nil || !merged.Merged {
		t.Fatalf("pull %s#%d not merged in store: %+v", target.Repo, target.Number, merged)
	}

	view := m.View().Content
	if strings.Contains(view, before.Title) {
		t.Fatalf("merged PR %q (%s#%d) still rendered in My Pull Requests:\n%s",
			before.Title, target.Repo, target.Number, view)
	}
}
