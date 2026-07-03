package mockgitea

import "time"

// DemoData builds the deterministic "teahouse" dataset used by --mock and by
// the UI-overhaul demo video: a fictional company selling internet-connected
// kettles, with a Go backend (kettle), a web dashboard (steep), and
// deployment tooling (infra). Every timestamp is an offset from now, so two
// calls with the same now produce identical output — no math/rand, and every
// ID-bearing entity is registered through the store's own Add* methods (or,
// for sub-objects with no such method — reviews, statuses, job logs —
// assigned a fixed literal), so IDs come from store.id() in one fixed call
// order.
func DemoData(now time.Time) *Store {
	s := NewStore()
	d := &demoBuilder{s: s, now: now}
	d.users()
	d.repos()
	d.kettlePulls()
	d.kettleIssues()
	d.steepPulls()
	d.steepIssues()
	d.infraPulls()
	d.infraIssues()
	d.actionRuns()
	d.notifications()
	return s
}

// demoBuilder threads the store, the fixed "now", and the shared users/
// labels/milestones through the seeding steps below.
type demoBuilder struct {
	s   *Store
	now time.Time

	mei, arjun, sofia, felix *User

	kettleBug, kettleFeature, kettleUrgent, kettleGoodFirst *Label
	steepBug, steepFeature, steepUrgent, steepGoodFirst     *Label
	infraBug, infraFeature, infraUrgent, infraGoodFirst     *Label

	kettleV1, kettleV11 *Milestone
	steepV1, steepV11   *Milestone
	infraV1, infraV11   *Milestone
}

// hoursAgo and daysAgo read as offsets-from-now at each call site, e.g.
// d.hoursAgo(3) for "3 hours before the demo's fixed now".
func (d *demoBuilder) hoursAgo(h int) time.Time { return d.now.Add(-time.Duration(h) * time.Hour) }
func (d *demoBuilder) daysAgo(n int) time.Time  { return d.now.Add(-time.Duration(n) * 24 * time.Hour) }

// users registers the rest of the teahouse team. gabor (me) is already the
// store default from NewStore.
func (d *demoBuilder) users() {
	d.mei = &User{Login: "mei", FullName: "Mei Lin"}
	d.arjun = &User{Login: "arjun", FullName: "Arjun Rao"}
	d.sofia = &User{Login: "sofia", FullName: "Sofia Alvarez"}
	d.felix = &User{Login: "felix", FullName: "Felix Wagner"}
	for _, u := range []*User{d.mei, d.arjun, d.sofia, d.felix} {
		d.s.AddUser(u)
	}
}

// repos registers the three teahouse repositories and their labels/
// milestones. kettle allows every merge style; steep disallows rebase;
// infra is squash/fast-forward-only (a linear-history deploy repo) — three
// distinct capability profiles so MergeCapabilities probing has something
// real to show across repos.
func (d *demoBuilder) repos() {
	owner := &User{Login: "teahouse"}

	d.s.AddRepo(&Repo{
		Name: "kettle", Owner: owner, FullName: "teahouse/kettle",
		HTMLURL:           "https://git.teahouse.dev/teahouse/kettle",
		AllowMergeCommits: true, AllowRebase: true, AllowRebaseExplicit: true,
		AllowSquashMerge: true, AllowFastForwardOnlyMerge: true,
		DefaultMergeStyle: "merge",
	})
	d.s.AddRepo(&Repo{
		Name: "steep", Owner: owner, FullName: "teahouse/steep",
		HTMLURL:           "https://git.teahouse.dev/teahouse/steep",
		AllowMergeCommits: true, AllowRebase: false, AllowRebaseExplicit: false,
		AllowSquashMerge: true, AllowFastForwardOnlyMerge: false,
		DefaultMergeStyle: "squash",
	})
	d.s.AddRepo(&Repo{
		Name: "infra", Owner: owner, FullName: "teahouse/infra",
		HTMLURL:           "https://git.teahouse.dev/teahouse/infra",
		AllowMergeCommits: false, AllowRebase: false, AllowRebaseExplicit: false,
		AllowSquashMerge: true, AllowFastForwardOnlyMerge: true,
		DefaultMergeStyle: "squash",
	})

	d.kettleBug = d.labelDef("teahouse/kettle", "bug", "ee0000")
	d.kettleFeature = d.labelDef("teahouse/kettle", "feature", "00aa00")
	d.kettleUrgent = d.labelDef("teahouse/kettle", "urgent", "ff8800")
	d.kettleGoodFirst = d.labelDef("teahouse/kettle", "good-first-issue", "8855ff")

	d.steepBug = d.labelDef("teahouse/steep", "bug", "ee0000")
	d.steepFeature = d.labelDef("teahouse/steep", "feature", "00aa00")
	d.steepUrgent = d.labelDef("teahouse/steep", "urgent", "ff8800")
	d.steepGoodFirst = d.labelDef("teahouse/steep", "good-first-issue", "8855ff")

	d.infraBug = d.labelDef("teahouse/infra", "bug", "ee0000")
	d.infraFeature = d.labelDef("teahouse/infra", "feature", "00aa00")
	d.infraUrgent = d.labelDef("teahouse/infra", "urgent", "ff8800")
	d.infraGoodFirst = d.labelDef("teahouse/infra", "good-first-issue", "8855ff")

	d.kettleV1 = d.milestoneDef("teahouse/kettle", "v1.0")
	d.kettleV11 = d.milestoneDef("teahouse/kettle", "v1.1")
	d.steepV1 = d.milestoneDef("teahouse/steep", "v1.0")
	d.steepV11 = d.milestoneDef("teahouse/steep", "v1.1")
	d.infraV1 = d.milestoneDef("teahouse/infra", "v1.0")
	d.infraV11 = d.milestoneDef("teahouse/infra", "v1.1")
}

func (d *demoBuilder) labelDef(repo, name, color string) *Label {
	l := &Label{Name: name, Color: color}
	d.s.AddLabelDef(repo, l)
	return l
}

func (d *demoBuilder) milestoneDef(repo, title string) *Milestone {
	m := &Milestone{Title: title, State: "open"}
	d.s.AddMilestoneDef(repo, m)
	return m
}

// seedComments records n.Body's worth of comments (via Store.SeedComment,
// which stamps an explicit time instead of AddComment's wall-clock
// time.Now()) and returns the count, so a Pull/Issue literal's CommentCount
// field can be set from the same call: CommentCount: d.comments(repo, num, ...).
func (d *demoBuilder) comments(repo string, num int64, at time.Time, entries ...[2]string) int64 {
	for _, e := range entries {
		login, body := e[0], e[1]
		d.s.SeedComment(repo, num, login, body, at)
	}
	return int64(len(entries))
}

// --- kettle: the Go brewing-service backend ---------------------------

const kettlePID1Body = `Adds a proper PID control loop for kettle temperature instead of the old
on/off bang-bang controller. Includes an integral clamp to reduce overshoot
on cold starts. Still tuning gains — see the failing test.`

const kettlePID1Diff = `diff --git a/internal/kettle/pid.go b/internal/kettle/pid.go
index 3f1a2c9..8b77e21 100644
--- a/internal/kettle/pid.go
+++ b/internal/kettle/pid.go
@@ -12,7 +12,7 @@ type Controller struct {
 	Ki float64
 	Kd float64

-	integral float64
+	integral   float64
 	lastError  float64
 	lastSample time.Time
 }
@@ -40,6 +40,11 @@ func (c *Controller) Step(setpoint, actual float64, now time.Time) float64 {
 	c.integral += err * dt
 	derivative := (err - c.lastError) / dt

+	// Clamp the integral term to avoid windup while the kettle is still
+	// heating from a cold start.
+	if c.integral > c.maxIntegral {
+		c.integral = c.maxIntegral
+	}
+
 	output := c.Kp*err + c.Ki*c.integral + c.Kd*derivative
 	c.lastError = err
 	c.lastSample = now
`

const kettleDebounce2Body = `Vessels sometimes got double-POSTed from a flaky client retry, starting
two brews in the scheduler for the same vessel. Debounces StartBrew per
vessel while a start is in flight.`

const kettleDebounce2Diff = `diff --git a/internal/api/brew_handler.go b/internal/api/brew_handler.go
index 5c1f0a2..9e3d441 100644
--- a/internal/api/brew_handler.go
+++ b/internal/api/brew_handler.go
@@ -22,6 +22,7 @@ type BrewHandler struct {
 	store     *Store
 	scheduler *Scheduler
+	inFlight  sync.Map
 }
@@ -38,6 +39,11 @@ func (h *BrewHandler) StartBrew(w http.ResponseWriter, r *http.Request) {
 	vesselID := chi.URLParam(r, "vesselID")

+	if _, loaded := h.inFlight.LoadOrStore(vesselID, true); loaded {
+		http.Error(w, "brew already starting", http.StatusConflict)
+		return
+	}
+	defer h.inFlight.Delete(vesselID)
+
 	if err := h.scheduler.Start(vesselID); err != nil {
 		http.Error(w, err.Error(), http.StatusInternalServerError)
 		return
`

const kettleMultiVessel3Body = `WIP: first pass at letting the scheduler track multiple vessels instead of
just one. Not ready — no fairness policy yet, and Start() needs a vesselID
everywhere it's called.`

const kettleMultiVessel3Diff = `diff --git a/internal/scheduler/scheduler.go b/internal/scheduler/scheduler.go
index 1a9bb31..44210aa 100644
--- a/internal/scheduler/scheduler.go
+++ b/internal/scheduler/scheduler.go
@@ -10,7 +10,7 @@ import (
 )

 type Scheduler struct {
-	vessel *Vessel
+	vessels map[string]*Vessel
 	queue   chan Job
 }
@@ -25,5 +25,8 @@ func New() *Scheduler {
 // TODO: this only handles a single vessel for now; the multi-vessel
 // rework needs a per-vessel queue and a fairness policy across them.
 func (s *Scheduler) Start(vesselID string) error {
-	return s.vessel.Start()
+	v, ok := s.vessels[vesselID]
+	if !ok {
+		return fmt.Errorf("unknown vessel %q", vesselID)
+	}
+	return v.Start()
 }
`

const kettleHealth4Body = `Adds a bare /healthz endpoint for the k8s liveness/readiness probes ahead
of the blue-green rollout in teahouse/infra.`

const kettleHealth4Diff = `diff --git a/internal/api/health.go b/internal/api/health.go
new file mode 100644
index 0000000..7d21eaa
--- /dev/null
+++ b/internal/api/health.go
@@ -0,0 +1,10 @@
+package api
+
+import "net/http"
+
+// HealthHandler answers Kubernetes liveness/readiness probes.
+func HealthHandler(w http.ResponseWriter, r *http.Request) {
+	w.Header().Set("Content-Type", "application/json")
+	w.WriteHeader(http.StatusOK)
+	_, _ = w.Write([]byte(` + "`" + `{"status":"ok"}` + "`" + `))
+}
`

const kettleSteepcalc5Body = `Extracts the steep-time math into its own package so it's unit-testable
without constructing a full Brew. No behavior change.`

const kettleSteepcalc5Diff = `diff --git a/internal/brew/steep.go b/internal/brew/steep.go
index 88a1c00..c3fe912 100644
--- a/internal/brew/steep.go
+++ b/internal/brew/steep.go
@@ -14,17 +14,8 @@ func (b *Brew) Duration() time.Duration {
-	base := b.TeaType.BaseSteepSeconds()
-	if b.Strength == Strong {
-		base = base * 12 / 10
-	}
-	if b.WaterTemp < b.TeaType.IdealTemp {
-		base += int((b.TeaType.IdealTemp - b.WaterTemp) / 2)
-	}
-	return time.Duration(base) * time.Second
+	return steepcalc.Duration(b.TeaType, b.Strength, b.WaterTemp)
 }
diff --git a/internal/brew/steepcalc/steepcalc.go b/internal/brew/steepcalc/steepcalc.go
new file mode 100644
index 0000000..a441fef
--- /dev/null
+++ b/internal/brew/steepcalc/steepcalc.go
@@ -0,0 +1,15 @@
+package steepcalc
+
+import "time"
+
+// Duration computes how long to steep given tea type, strength, and water
+// temperature, extracted out of Brew.Duration so it can be unit tested
+// without constructing a full Brew.
+func Duration(tea TeaType, strength Strength, waterTemp int) time.Duration {
+	base := tea.BaseSteepSeconds()
+	if strength == Strong {
+		base = base * 12 / 10
+	}
+	if waterTemp < tea.IdealTemp {
+		base += int((tea.IdealTemp - waterTemp) / 2)
+	}
+	return time.Duration(base) * time.Second
+}
`

const kettleDescale6Body = `First pass at a snooze option for the descale reminder. Conflicts with
main's recent brewCount threshold change — will rebase once #16 lands.`

const kettleDescale6Diff = `diff --git a/internal/maintenance/descale.go b/internal/maintenance/descale.go
index 2b6e0aa..a0c9f31 100644
--- a/internal/maintenance/descale.go
+++ b/internal/maintenance/descale.go
@@ -8,6 +8,7 @@ import (
 type Reminder struct {
 	lastDescale  time.Time
 	brewCount    int
+	snoozedUntil time.Time
 }
@@ -18,5 +19,11 @@ func (r *Reminder) Due(now time.Time) bool {
+<<<<<<< HEAD
 	return now.Sub(r.lastDescale) > 30*24*time.Hour || r.brewCount > 200
+=======
+	if !r.snoozedUntil.IsZero() && now.Before(r.snoozedUntil) {
+		return false
+	}
+	return now.Sub(r.lastDescale) > 30*24*time.Hour || r.brewCount > 200
+>>>>>>> main
 }
`

const kettleHandshake7Body = `Some v2 firmware boards were timing out during handshake under load.
Bumps the deadline from 2s to 10s until we understand why the v2 boards
are slower to respond.`

const kettleHandshake7Diff = `diff --git a/internal/firmware/handshake.go b/internal/firmware/handshake.go
index 9f0a221..3d88bb0 100644
--- a/internal/firmware/handshake.go
+++ b/internal/firmware/handshake.go
@@ -30,7 +30,7 @@ func Handshake(conn net.Conn) (*Session, error) {
-	conn.SetDeadline(time.Now().Add(2 * time.Second))
+	conn.SetDeadline(time.Now().Add(10 * time.Second))
 	if _, err := conn.Write(helloFrame); err != nil {
 		return nil, fmt.Errorf("send hello: %w", err)
 	}
`

const kettleGoBump8Body = `Bumps the toolchain so we can start using range-over-func iterators.`

const kettleGoBump8Diff = `diff --git a/go.mod b/go.mod
index 7a1e3aa..b902c10 100644
--- a/go.mod
+++ b/go.mod
@@ -1,6 +1,6 @@
 module github.com/teahouse/kettle

-go 1.24
+go 1.26
`

const kettleCooldown9Body = `CooldownTimer.Stop() could be called from two goroutines (an API cancel
racing natural completion) and panic on a double close(). Wraps it in a
sync.Once.`

const kettleCooldown9Diff = `diff --git a/internal/kettle/cooldown.go b/internal/kettle/cooldown.go
index c441a0b..7fa992e 100644
--- a/internal/kettle/cooldown.go
+++ b/internal/kettle/cooldown.go
@@ -16,8 +16,10 @@ func (c *CooldownTimer) Stop() {
-	close(c.done)
+	c.once.Do(func() { close(c.done) })
 }
@@ -30,6 +32,7 @@ type CooldownTimer struct {
 	done chan struct{}
+	once sync.Once
 }
`

const kettleDocs10Body = `Docs were missing the strength parameter added a while back.`

const kettleDocs10Diff = `diff --git a/docs/api/brew.md b/docs/api/brew.md
index e001a22..44bb210 100644
--- a/docs/api/brew.md
+++ b/docs/api/brew.md
@@ -12,7 +12,7 @@ curl -X POST https://api.teahouse.dev/v1/brews \
   -H "Authorization: Bearer $TOKEN" \
-  -d '{"vesselId": "kettle-01", "tea": "sencha"}'
+  -d '{"vesselId": "kettle-01", "tea": "sencha", "strength": "strong"}'
`

const kettleHarness11Body = `Adds an integration test harness for the scheduler so the multi-vessel
work has something real to run against.`

const kettleHarness11Diff = `diff --git a/internal/scheduler/scheduler_integration_test.go b/internal/scheduler/scheduler_integration_test.go
new file mode 100644
index 0000000..1c77abc
--- /dev/null
+++ b/internal/scheduler/scheduler_integration_test.go
@@ -0,0 +1,17 @@
+package scheduler_test
+
+import (
+	"testing"
+	"time"
+)
+
+// TestSchedulerRunsQueuedJobInOrder exercises the scheduler end-to-end
+// against an in-memory vessel fake, standing in for real firmware.
+func TestSchedulerRunsQueuedJobInOrder(t *testing.T) {
+	sched := newTestScheduler(t)
+	sched.Enqueue(job("kettle-01", 30*time.Second))
+	sched.Enqueue(job("kettle-01", 45*time.Second))
+
+	got := sched.RunAll(t.Context())
+	if len(got) != 2 || got[0].VesselID != "kettle-01" {
+		t.Fatalf("ran %+v, want 2 jobs on kettle-01 in order", got)
+	}
+}
`

func (d *demoBuilder) kettlePulls() {
	const repo = "teahouse/kettle"
	me := d.s.Me()

	d.s.AddPull(&Pull{Number: 1, RepoFullName: repo,
		Title: "feat: PID control loop for kettle temperature", Body: kettlePID1Body,
		State: "open", Mergeable: true, Author: me,
		HeadRef: "feat/pid-control", HeadSHA: "a1b2c3d", BaseRef: "main",
		Diff: kettlePID1Diff, Additions: 18, Deletions: 4, ChangedFiles: 1,
		Statuses:     []*CommitStatus{{Status: "failure", Context: "ci/test", Description: "TestPIDConverges failed"}},
		CommentCount: d.comments(repo, 1, d.hoursAgo(3), [2]string{"arjun", "Did you check the anti-windup clamp during a cold start? We saw oscillation last time."}),
		Created:      d.daysAgo(1), Updated: d.hoursAgo(3), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/1",
	})

	d.s.AddPull(&Pull{Number: 2, RepoFullName: repo,
		Title: "fix: debounce brew-start double submit", Body: kettleDebounce2Body,
		State: "open", Mergeable: true, Author: me,
		HeadRef: "fix/debounce-brew-start", HeadSHA: "e4f5061", BaseRef: "main",
		Diff: kettleDebounce2Diff, Additions: 24, Deletions: 3, ChangedFiles: 1,
		Statuses: []*CommitStatus{{Status: "success", Context: "ci/build"}, {Status: "success", Context: "ci/test"}},
		// Reviews get small fixed literal IDs (501+): they're appended
		// directly to Pull.Reviews rather than going through AddReview,
		// which stamps time.Now() — wrong for deterministic, historically
		// dated seed data (same reasoning as SeedComment vs AddComment).
		// There's no Add*-style registration path for a Review on its own,
		// so unlike Pulls/Issues/Runs there's no store.id() call to draw
		// from here.
		Reviews: []*Review{{ID: 501, State: "APPROVED", Body: "LGTM once CI is green.", Reviewer: d.mei, Created: d.hoursAgo(7)}},
		CommentCount: d.comments(repo, 2, d.hoursAgo(6),
			[2]string{"mei", "LGTM once CI is green."},
			[2]string{"sofia", "Nice, this was biting us in prod during the tasting event demo."}),
		Created: d.daysAgo(2), Updated: d.hoursAgo(6), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/2",
	})

	d.s.AddPull(&Pull{Number: 3, RepoFullName: repo,
		Title: "WIP: multi-vessel scheduling", Body: kettleMultiVessel3Body,
		State: "open", Mergeable: true, Draft: true, Author: me,
		HeadRef: "wip/multi-vessel", HeadSHA: "9c0ddee", BaseRef: "main",
		Diff: kettleMultiVessel3Diff, Additions: 12, Deletions: 6, ChangedFiles: 1,
		Statuses:     []*CommitStatus{{Status: "pending", Context: "ci/build"}},
		CommentCount: d.comments(repo, 3, d.hoursAgo(1), [2]string{"mei", "Don't merge yet — still need a per-vessel fairness policy."}),
		Created:      d.hoursAgo(5), Updated: d.hoursAgo(1), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/3",
	})

	d.s.AddPull(&Pull{Number: 4, RepoFullName: repo,
		Title: "feat: expose /health endpoint for k8s probes", Body: kettleHealth4Body,
		State: "open", Mergeable: true, Author: d.mei, Reviewers: []*User{me},
		HeadRef: "feat/health-endpoint", HeadSHA: "71fa220", BaseRef: "main",
		Diff: kettleHealth4Diff, Additions: 10, Deletions: 0, ChangedFiles: 1,
		Statuses:     []*CommitStatus{{Status: "success", Context: "ci/build"}},
		CommentCount: d.comments(repo, 4, d.hoursAgo(8), [2]string{"felix", "Probe path looks right, checked it against the ingress config."}),
		Created:      d.daysAgo(1), Updated: d.hoursAgo(8), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/4",
	})

	d.s.AddPull(&Pull{Number: 5, RepoFullName: repo,
		Title: "refactor: extract steep-time calculator", Body: kettleSteepcalc5Body,
		State: "open", Mergeable: true, Author: d.arjun, Reviewers: []*User{me},
		HeadRef: "refactor/steep-calc", HeadSHA: "3bb9104", BaseRef: "main",
		Diff: kettleSteepcalc5Diff, Additions: 28, Deletions: 9, ChangedFiles: 2,
		Statuses: []*CommitStatus{{Status: "success", Context: "ci/build"}, {Status: "pending", Context: "ci/test"}},
		CommentCount: d.comments(repo, 5, d.hoursAgo(10),
			[2]string{"sofia", "Could steepcalc take a Context for cancellation later?"},
			[2]string{"felix", "Not needed yet, kept it pure for now."}),
		Created: d.daysAgo(1), Updated: d.hoursAgo(10), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/5",
	})

	d.s.AddPull(&Pull{Number: 6, RepoFullName: repo,
		Title: "feat: add descale reminder scheduler", Body: kettleDescale6Body,
		State: "open", Mergeable: false, Author: d.sofia,
		HeadRef: "feat/descale-reminder", HeadSHA: "c02fee1", BaseRef: "main",
		Diff: kettleDescale6Diff, Additions: 10, Deletions: 2, ChangedFiles: 1,
		Statuses:     []*CommitStatus{{Status: "failure", Context: "ci/build", Description: "merge conflict with main"}},
		CommentCount: d.comments(repo, 6, d.daysAgo(1), [2]string{"arjun", "This has a real conflict with main's snooze feature, needs a rebase."}),
		Created:      d.daysAgo(2), Updated: d.hoursAgo(30), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/6",
	})

	d.s.AddPull(&Pull{Number: 7, RepoFullName: repo,
		Title: "fix: kettle firmware handshake timeout", Body: kettleHandshake7Body,
		State: "closed", Merged: true, Mergeable: true, Author: d.felix,
		HeadRef: "fix/handshake-timeout", HeadSHA: "88de3a1", BaseRef: "main",
		Diff: kettleHandshake7Diff, Additions: 1, Deletions: 1, ChangedFiles: 1,
		Statuses: []*CommitStatus{{Status: "success", Context: "ci/build"}, {Status: "success", Context: "ci/test"}},
		Reviews:  []*Review{{ID: 502, State: "APPROVED", Body: "Confirmed this fixes the handshake flakiness we saw on the v2 boards.", Reviewer: d.arjun, Created: d.daysAgo(5)}},
		CommentCount: d.comments(repo, 7, d.daysAgo(5),
			[2]string{"arjun", "Confirmed this fixes the handshake flakiness we saw on the v2 boards."},
			[2]string{"sofia", "Shipping this in tonight's release."}),
		Created: d.daysAgo(6), Updated: d.daysAgo(5), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/7",
	})

	d.s.AddPull(&Pull{Number: 8, RepoFullName: repo,
		Title: "chore: bump go toolchain to 1.26", Body: kettleGoBump8Body,
		State: "closed", Merged: false, Mergeable: true, Author: d.mei,
		HeadRef: "chore/go-1.26", HeadSHA: "1204abc", BaseRef: "main",
		Diff: kettleGoBump8Diff, Additions: 1, Deletions: 1, ChangedFiles: 1,
		CommentCount: d.comments(repo, 8, d.daysAgo(4), [2]string{"felix", "SDK requires go1.26 for the new iterator support — closing this in favor of bundling it with #11."}),
		Created:      d.daysAgo(5), Updated: d.daysAgo(4), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/8",
	})

	d.s.AddPull(&Pull{Number: 9, RepoFullName: repo,
		Title: "fix: race in cooldown timer", Body: kettleCooldown9Body,
		State: "closed", Merged: true, Mergeable: true, Author: d.arjun,
		HeadRef: "fix/cooldown-race", HeadSHA: "5a9e0f2", BaseRef: "main",
		Diff: kettleCooldown9Diff, Additions: 3, Deletions: 1, ChangedFiles: 1,
		Statuses:     []*CommitStatus{{Status: "success", Context: "ci/build"}},
		Reviews:      []*Review{{ID: 503, State: "APPROVED", Body: "Nice catch, this was a rare one.", Reviewer: d.sofia, Created: d.daysAgo(6)}},
		CommentCount: d.comments(repo, 9, d.daysAgo(6), [2]string{"sofia", "Nice catch, this was a rare one."}),
		Created:      d.daysAgo(7), Updated: d.daysAgo(6), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/9",
	})

	d.s.AddPull(&Pull{Number: 10, RepoFullName: repo,
		Title: "docs: update brew API examples", Body: kettleDocs10Body,
		State: "closed", Merged: false, Mergeable: true, Author: d.sofia,
		HeadRef: "docs/brew-api-examples", HeadSHA: "0d33cab", BaseRef: "main",
		Diff: kettleDocs10Diff, Additions: 1, Deletions: 1, ChangedFiles: 1,
		CommentCount: d.comments(repo, 10, d.daysAgo(7), [2]string{"mei", "Thanks for keeping the docs in sync."}),
		Created:      d.daysAgo(8), Updated: d.daysAgo(7), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/10",
	})

	d.s.AddPull(&Pull{Number: 11, RepoFullName: repo,
		Title: "test: add integration harness for brew scheduling", Body: kettleHarness11Body,
		State: "open", Mergeable: true, Author: me,
		HeadRef: "test/brew-integration-harness", HeadSHA: "7714bea", BaseRef: "main",
		Diff: kettleHarness11Diff, Additions: 21, Deletions: 0, ChangedFiles: 1,
		Statuses:     []*CommitStatus{{Status: "success", Context: "ci/build"}, {Status: "success", Context: "ci/test"}},
		CommentCount: d.comments(repo, 11, d.hoursAgo(14), [2]string{"mei", "This will make the multi-vessel work in #3 much easier to verify."}),
		Created:      d.hoursAgo(16), Updated: d.hoursAgo(14), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/pulls/11",
	})
}

const kettleIssue11Body = `The kettle overshoots the target temperature by about 2°C before the PID
loop settles, most noticeable on cold starts.

Repro:
1. Set target to 95°C from a cold kettle (room temp).
2. Watch the temperature curve in the dashboard.
3. It peaks around 97°C before settling back down.

` + "```go" + `
// current gains, from internal/kettle/pid.go
Kp: 2.0, Ki: 0.4, Kd: 0.1
` + "```" + `

Might just need a lower ` + "`Kd`" + ` or an integral clamp — see #1 for a first attempt
at the clamp.`

const kettleIssue12Body = `Several vessels ship to US customers who expect °F. Add a per-user display
preference and convert server-side values at render time.

- [ ] Add ` + "`temperatureUnit`" + ` to user prefs (` + "`celsius`" + ` | ` + "`fahrenheit`" + `)
- [ ] Convert on the dashboard, not in the API (API stays °C)
- [ ] Round to nearest whole degree in the UI`

const kettleIssue13Body = `` + "`internal/brew/steepcalc`" + ` has no tests yet (extracted in #5). Good first
task if you want to get familiar with the brew package.

Cases worth covering:
- Green vs black tea base steep time
- ` + "`Strong`" + ` strength multiplier
- Water temp below the tea's ideal temp`

const kettleIssue14Body = `Brew history entries show the *server's* timezone (UTC) instead of the
viewer's local time, which is confusing for anyone outside UTC.

` + "```json" + `
{"event": "brew_started", "at": "2026-07-01T14:00:00Z"}
` + "```" + `

The dashboard should convert ` + "`at`" + ` client-side before rendering, the same way
steep's timer already does.`

const kettleIssue15Body = `Requesting an outbound webhook when a brew finishes, so people can hook up
notifications (Slack, etc.) without polling the API.

- [ ] ` + "`POST /v1/webhooks`" + ` to register a URL + secret
- [ ] Sign the payload the same way GitHub does (HMAC-SHA256 header)
- [ ] Retry with backoff on non-2xx, give up after 5 attempts`

const kettleIssue16Body = `Getting the descale reminder push notification twice within a minute of
each other. Looks like both the scheduled job and the on-brew-complete
check are firing for the same threshold crossing.

Steps to reproduce:
1. Get ` + "`brewCount`" + ` close to 200 (the threshold)
2. Finish a brew that crosses it
3. Two notifications arrive almost immediately`

const kettleIssue17Body = `A firmware payload with a truncated CRC crashed the whole process instead
of just rejecting that message. Saw this twice in staging today.

` + "```" + `
panic: runtime error: slice bounds out of range [:8] with capacity 5

goroutine 42 [running]:
github.com/teahouse/kettle/internal/firmware.parseFrame(...)
	/app/internal/firmware/frame.go:51
` + "```" + `

This needs a bounds check before we touch production fleets — flagging urgent.`

const kettleIssue18Body = `The ` + "`Kp`/`Ki`/`Kd`" + ` constants in ` + "`internal/kettle/pid.go`" + ` have no comment
explaining how they were chosen. Would help future tuning work (see #11) to
write down:

- What rig/vessel they were tuned against
- Whether they're per-vessel-model or global
- The overshoot/settle-time tradeoff we accepted`

func (d *demoBuilder) kettleIssues() {
	const repo = "teahouse/kettle"
	me := d.s.Me()

	d.s.AddIssue(&Issue{Number: 12, RepoFullName: repo,
		Title: "bug: kettle whistles past target temp by 2°C", Body: kettleIssue11Body,
		State: "open", Author: d.mei, Labels: []*Label{d.kettleBug}, Milestone: d.kettleV1,
		Created: d.daysAgo(3), Updated: d.hoursAgo(12), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/issues/12",
	})
	d.s.AddIssue(&Issue{Number: 13, RepoFullName: repo,
		Title: "feature: support Fahrenheit display toggle", Body: kettleIssue12Body,
		State: "open", Author: d.sofia, Labels: []*Label{d.kettleFeature}, Milestone: d.kettleV11,
		Created: d.daysAgo(4), Updated: d.hoursAgo(20), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/issues/13",
	})
	d.s.AddIssue(&Issue{Number: 14, RepoFullName: repo,
		Title: "good-first-issue: add unit tests for steep-time calculator", Body: kettleIssue13Body,
		State: "open", Author: d.arjun, Labels: []*Label{d.kettleGoodFirst},
		Created: d.daysAgo(2), Updated: d.daysAgo(2), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/issues/14",
	})
	d.s.AddIssue(&Issue{Number: 15, RepoFullName: repo,
		Title: "bug: brew log timestamps use server tz not user tz", Body: kettleIssue14Body,
		State: "open", Author: d.felix, Assignees: []*User{me},
		Labels: []*Label{d.kettleBug, d.kettleUrgent}, Milestone: d.kettleV1,
		Created: d.hoursAgo(6), Updated: d.hoursAgo(4), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/issues/15",
	})
	d.s.AddIssue(&Issue{Number: 16, RepoFullName: repo,
		Title: "feature: webhook on brew-complete event", Body: kettleIssue15Body,
		State: "open", Author: d.mei, Labels: []*Label{d.kettleFeature},
		Created: d.daysAgo(3), Updated: d.daysAgo(3), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/issues/16",
	})
	d.s.AddIssue(&Issue{Number: 17, RepoFullName: repo,
		Title: "bug: descale reminder fires twice", Body: kettleIssue16Body,
		State: "open", Author: d.sofia, Assignees: []*User{me}, Labels: []*Label{d.kettleBug},
		Created: d.hoursAgo(11), Updated: d.hoursAgo(9), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/issues/17",
	})
	d.s.AddIssue(&Issue{Number: 18, RepoFullName: repo,
		Title: "urgent: kettle-api panics on malformed firmware payload", Body: kettleIssue17Body,
		State: "open", Author: d.arjun, Labels: []*Label{d.kettleBug, d.kettleUrgent}, Milestone: d.kettleV11,
		Created: d.hoursAgo(3), Updated: d.hoursAgo(2), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/issues/18",
	})
	d.s.AddIssue(&Issue{Number: 19, RepoFullName: repo,
		Title: "chore: document PID tuning constants", Body: kettleIssue18Body,
		State: "closed", Author: d.felix, Labels: []*Label{d.kettleGoodFirst},
		Created: d.daysAgo(7), Updated: d.daysAgo(6), HTMLURL: "https://git.teahouse.dev/teahouse/kettle/issues/19",
	})
}

// --- steep: the web dashboard -------------------------------------------

const steepTimer1Body = `Timer was using a naive 1-tick-per-second counter, which drifts badly once
the browser throttles background tabs. Switches to wall-clock deltas.`

const steepTimer1Diff = `diff --git a/src/hooks/useSteepTimer.ts b/src/hooks/useSteepTimer.ts
index 44a1bb2..99cc102 100644
--- a/src/hooks/useSteepTimer.ts
+++ b/src/hooks/useSteepTimer.ts
@@ -18,7 +18,13 @@ export function useSteepTimer(durationMs: number) {
   useEffect(() => {
-    const id = setInterval(() => setRemaining(r => r - 1000), 1000)
-    return () => clearInterval(id)
+    let last = Date.now()
+    const id = setInterval(() => {
+      const now = Date.now()
+      setRemaining(r => r - (now - last))
+      last = now
+    }, 1000)
+    return () => clearInterval(id)
   }, [])
`

const steepVite2Body = `Migrates the build from webpack to Vite. Dev server starts in under a
second now.`

const steepVite2Diff = `diff --git a/package.json b/package.json
index 0a11cde..b672a90 100644
--- a/package.json
+++ b/package.json
@@ -12,7 +12,7 @@
   "scripts": {
-    "build": "webpack --mode production",
-    "dev": "webpack serve"
+    "build": "vite build",
+    "dev": "vite"
   },
`

const steepDark3Body = `Adds a dark mode variant for the brew dashboard, wired through the
existing theme context.`

const steepDark3Diff = `diff --git a/src/components/BrewDashboard.tsx b/src/components/BrewDashboard.tsx
index 5566aa1..bb02efc 100644
--- a/src/components/BrewDashboard.tsx
+++ b/src/components/BrewDashboard.tsx
@@ -4,7 +4,7 @@ import { useTheme } from '../theme'

 export function BrewDashboard() {
-  const theme = 'light'
+  const { theme } = useTheme()
   return (
-    <div className="dashboard dashboard--light">
+    <div className={` + "`dashboard dashboard--${theme}`" + `}>
`

func (d *demoBuilder) steepPulls() {
	const repo = "teahouse/steep"
	me := d.s.Me()

	d.s.AddPull(&Pull{Number: 1, RepoFullName: repo,
		Title: "fix: steep timer drift on background tabs", Body: steepTimer1Body,
		State: "closed", Merged: true, Mergeable: true, Author: d.mei,
		HeadRef: "fix/timer-drift", HeadSHA: "44d201a", BaseRef: "main",
		Diff: steepTimer1Diff, Additions: 9, Deletions: 3, ChangedFiles: 1,
		Statuses:     []*CommitStatus{{Status: "success", Context: "ci/build"}},
		Reviews:      []*Review{{ID: 504, State: "APPROVED", Body: "Confirmed the drift is gone after 10 minutes backgrounded.", Reviewer: me, Created: d.daysAgo(4)}},
		CommentCount: d.comments(repo, 1, d.daysAgo(4), [2]string{"gabor", "Confirmed the drift is gone after 10 minutes backgrounded."}),
		Created:      d.daysAgo(5), Updated: d.daysAgo(4), HTMLURL: "https://git.teahouse.dev/teahouse/steep/pulls/1",
	})

	d.s.AddPull(&Pull{Number: 2, RepoFullName: repo,
		Title: "chore: migrate build tooling to Vite", Body: steepVite2Body,
		State: "closed", Merged: false, Mergeable: true, Author: d.arjun,
		HeadRef: "chore/vite-migration", HeadSHA: "b62a910", BaseRef: "main",
		Diff: steepVite2Diff, Additions: 2, Deletions: 2, ChangedFiles: 1,
		CommentCount: d.comments(repo, 2, d.daysAgo(5), [2]string{"sofia", "Bundle size dropped by 40%, nice — re-open once the CI cache is updated for Vite."}),
		Created:      d.daysAgo(6), Updated: d.daysAgo(5), HTMLURL: "https://git.teahouse.dev/teahouse/steep/pulls/2",
	})

	d.s.AddPull(&Pull{Number: 3, RepoFullName: repo,
		Title: "feat: dark mode for brew dashboard", Body: steepDark3Body,
		State: "open", Mergeable: true, Author: d.felix, Reviewers: []*User{me},
		HeadRef: "feat/dark-mode", HeadSHA: "fd10a77", BaseRef: "main",
		Diff: steepDark3Diff, Additions: 6, Deletions: 2, ChangedFiles: 1,
		Statuses:     []*CommitStatus{{Status: "success", Context: "ci/build"}},
		CommentCount: d.comments(repo, 3, d.hoursAgo(15), [2]string{"mei", "Contrast looks a little low on the brew-in-progress card, mind checking against WCAG AA?"}),
		Created:      d.daysAgo(1), Updated: d.hoursAgo(15), HTMLURL: "https://git.teahouse.dev/teahouse/steep/pulls/3",
	})
}

const steepIssue4Body = `If the steep timer tab is backgrounded (another tab focused, or laptop
sleeps briefly), the countdown resets to the full duration instead of
continuing from where it was.

Likely ` + "`setInterval`" + ` drift compounding while the tab is throttled by the
browser — see #1 for a first fix using wall-clock deltas instead of a
naive tick counter.`

const steepIssue5Body = `Right now the timer just shows remaining ` + "`mm:ss`" + `. Add a small "ready at
14:32" label next to it so people don't have to do the math themselves.

- [ ] Compute ` + "`now + remaining`" + `
- [ ] Format in the user's locale (reuse the formatter from the brew log)
- [ ] Update live as remaining changes, not just on mount`

const steepIssue6Body = `Add basic keyboard shortcuts to the steep timer card:

- ` + "`space`" + ` — pause/resume
- ` + "`r`" + ` — reset
- ` + "`+`" + ` / ` + "`-`" + ` — adjust remaining time by 30s

Should only fire when the timer card has focus, not globally.`

func (d *demoBuilder) steepIssues() {
	const repo = "teahouse/steep"
	me := d.s.Me()

	d.s.AddIssue(&Issue{Number: 4, RepoFullName: repo,
		Title: "bug: timer resets when tab is backgrounded", Body: steepIssue4Body,
		State: "open", Author: d.mei, Assignees: []*User{me},
		Labels: []*Label{d.steepBug, d.steepUrgent}, Milestone: d.steepV1,
		Created: d.daysAgo(1), Updated: d.hoursAgo(5), HTMLURL: "https://git.teahouse.dev/teahouse/steep/issues/4",
	})
	d.s.AddIssue(&Issue{Number: 5, RepoFullName: repo,
		Title: "feature: add ETA countdown to steep timer", Body: steepIssue5Body,
		State: "open", Author: d.sofia, Labels: []*Label{d.steepFeature},
		Created: d.daysAgo(2), Updated: d.daysAgo(1), HTMLURL: "https://git.teahouse.dev/teahouse/steep/issues/5",
	})
	d.s.AddIssue(&Issue{Number: 6, RepoFullName: repo,
		Title: "good-first-issue: add keyboard shortcuts to timer", Body: steepIssue6Body,
		State: "open", Author: d.arjun, Labels: []*Label{d.steepGoodFirst}, Milestone: d.steepV11,
		Created: d.daysAgo(4), Updated: d.daysAgo(3), HTMLURL: "https://git.teahouse.dev/teahouse/steep/issues/6",
	})
}

// --- infra: deployment tooling -------------------------------------------

const infraTFLock1Body = `Terraform's S3 backend lock was timing out during concurrent applies from
CI. Bumps max_retries.`

const infraTFLock1Diff = `diff --git a/terraform/backend.tf b/terraform/backend.tf
index 33a0abc..99e1a10 100644
--- a/terraform/backend.tf
+++ b/terraform/backend.tf
@@ -4,6 +4,7 @@ terraform {
   backend "s3" {
     bucket         = "teahouse-tfstate"
     dynamodb_table = "teahouse-tflock"
+    max_retries    = 10
   }
 }
`

func (d *demoBuilder) infraPulls() {
	const repo = "teahouse/infra"

	d.s.AddPull(&Pull{Number: 1, RepoFullName: repo,
		Title: "fix: terraform state lock timeout", Body: infraTFLock1Body,
		State: "closed", Merged: true, Mergeable: true, Author: d.felix,
		HeadRef: "fix/tf-lock-timeout", HeadSHA: "3391cde", BaseRef: "main",
		Diff: infraTFLock1Diff, Additions: 1, Deletions: 0, ChangedFiles: 1,
		Statuses:     []*CommitStatus{{Status: "success", Context: "ci/plan"}},
		CommentCount: d.comments(repo, 1, d.daysAgo(3), [2]string{"arjun", "This bit us during the last state migration, thanks for tracking it down."}),
		Created:      d.daysAgo(4), Updated: d.daysAgo(3), HTMLURL: "https://git.teahouse.dev/teahouse/infra/pulls/1",
	})
}

const infraIssue2Body = `The staging deploy has hung on the DB migration step for the last two
releases. No error in the logs, it just never progresses past:

` + "```" + `
Running migration 0043_add_brew_webhooks...
` + "```" + `

Rolling back and retrying always works on the second attempt, so this
might be a lock contention issue with a long-running query rather than a
migration bug itself.`

const infraIssue3Body = `We currently deploy straight to 100% of pods. Add a canary stage that
holds at 10% for 5 minutes and checks error rate before continuing.

- [ ] New ` + "`canary`" + ` stage in the deploy pipeline
- [ ] Reuse the existing error-rate metric, just scoped to canary pods
- [ ] Auto-rollback if error rate > baseline + 2%`

func (d *demoBuilder) infraIssues() {
	const repo = "teahouse/infra"
	me := d.s.Me()

	d.s.AddIssue(&Issue{Number: 2, RepoFullName: repo,
		Title: "bug: staging deploy hangs on migration step", Body: infraIssue2Body,
		State: "open", Author: d.sofia, Assignees: []*User{me},
		Labels: []*Label{d.infraBug, d.infraUrgent}, Milestone: d.infraV1,
		Created: d.daysAgo(1), Updated: d.hoursAgo(7), HTMLURL: "https://git.teahouse.dev/teahouse/infra/issues/2",
	})
	d.s.AddIssue(&Issue{Number: 3, RepoFullName: repo,
		Title: "feature: add canary rollout stage", Body: infraIssue3Body,
		State: "open", Author: d.arjun, Labels: []*Label{d.infraFeature}, Milestone: d.infraV11,
		Created: d.daysAgo(3), Updated: d.daysAgo(2), HTMLURL: "https://git.teahouse.dev/teahouse/infra/issues/3",
	})
}

// --- action runs ----------------------------------------------------------

const kettlePIDBuildLog = "go build ./...\ngo vet ./...\nok\n"
const kettlePIDTestFailLog = `=== RUN   TestPIDConverges
    pid_test.go:41: step 118: output settled at 97.6, want 98.5 +/- 0.5
--- FAIL: TestPIDConverges (0.31s)
FAIL
FAIL	github.com/teahouse/kettle/internal/kettle	0.318s
`
const kettleDebounceBuildTestLog = "go build ./...\ngo vet ./...\ngo test ./...\nok  \tgithub.com/teahouse/kettle/internal/api\t0.211s\n"
const kettleNightlyLog = "starting integration suite...\nwaiting for vessel simulator on :9001...\nvessel simulator ready\n"

const steepBuildLintLog = "vite build\n✓ 148 modules transformed.\neslint src --ext .ts,.tsx\n0 problems\n"
const steepDeployFailLog = `Applying Terraform plan...
module.kettle_api.aws_ecs_service.this: Modifying...

Error: error waiting for ECS service (teahouse-steep) to reach desired
count 3, deadline exceeded
`
const steepPostMergeLog = "vite build\ngo test ./e2e/...\nok  \tgithub.com/teahouse/steep/e2e\t4.902s\n"

// actionRuns registers six workflow runs across kettle and steep. Run IDs
// are left at 0 for AddRun's own store.id() to assign; ActionJob has no
// equivalent Add* registration (AddRun doesn't walk run.Jobs), so job IDs
// are small fixed literals instead — still fully deterministic, just not
// drawn from the same counter.
func (d *demoBuilder) actionRuns() {
	d.s.AddRun(&ActionRun{RepoFullName: "teahouse/kettle",
		DisplayTitle: "feat: PID control loop for kettle temperature", WorkflowName: "ci.yml",
		Event: "pull_request", Status: "failure", HeadBranch: "feat/pid-control", HeadSHA: "a1b2c3d",
		Actor: d.mei, Created: d.hoursAgo(4), Updated: d.hoursAgo(3),
		HTMLURL: "https://git.teahouse.dev/teahouse/kettle/actions/runs/9001",
		Jobs: []*ActionJob{
			{ID: 90011, Name: "build", Status: "success", Started: d.hoursAgo(4), Stopped: d.hoursAgo(4), Logs: kettlePIDBuildLog},
			{ID: 90012, Name: "test", Status: "failure", Started: d.hoursAgo(4), Stopped: d.hoursAgo(3), Logs: kettlePIDTestFailLog},
		},
	})
	d.s.AddRun(&ActionRun{RepoFullName: "teahouse/kettle",
		DisplayTitle: "fix: debounce brew-start double submit", WorkflowName: "ci.yml",
		Event: "pull_request", Status: "success", HeadBranch: "fix/debounce-brew-start", HeadSHA: "e4f5061",
		Actor: d.s.Me(), Created: d.hoursAgo(7), Updated: d.hoursAgo(6),
		HTMLURL: "https://git.teahouse.dev/teahouse/kettle/actions/runs/9002",
		Jobs: []*ActionJob{
			{ID: 90021, Name: "build", Status: "success", Started: d.hoursAgo(7), Stopped: d.hoursAgo(6), Logs: kettleDebounceBuildTestLog},
		},
	})
	d.s.AddRun(&ActionRun{RepoFullName: "teahouse/kettle",
		DisplayTitle: "nightly regression", WorkflowName: "nightly.yml",
		Event: "schedule", Status: "running", HeadBranch: "main", HeadSHA: "5a9e0f2",
		Actor: d.felix, Created: d.hoursAgo(1), Updated: d.hoursAgo(1),
		HTMLURL: "https://git.teahouse.dev/teahouse/kettle/actions/runs/9003",
		Jobs: []*ActionJob{
			{ID: 90031, Name: "integration", Status: "running", Started: d.hoursAgo(1), Logs: kettleNightlyLog},
		},
	})

	d.s.AddRun(&ActionRun{RepoFullName: "teahouse/steep",
		DisplayTitle: "feat: dark mode for brew dashboard", WorkflowName: "ci.yml",
		Event: "pull_request", Status: "success", HeadBranch: "feat/dark-mode", HeadSHA: "fd10a77",
		Actor: d.felix, Created: d.hoursAgo(16), Updated: d.hoursAgo(15),
		HTMLURL: "https://git.teahouse.dev/teahouse/steep/actions/runs/9004",
		Jobs: []*ActionJob{
			{ID: 90041, Name: "build", Status: "success", Started: d.hoursAgo(16), Stopped: d.hoursAgo(15), Logs: steepBuildLintLog},
			{ID: 90042, Name: "lint", Status: "success", Started: d.hoursAgo(16), Stopped: d.hoursAgo(15), Logs: steepBuildLintLog},
		},
	})
	d.s.AddRun(&ActionRun{RepoFullName: "teahouse/steep",
		DisplayTitle: "deploy staging", WorkflowName: "deploy.yml",
		Event: "workflow_dispatch", Status: "failure", HeadBranch: "main", HeadSHA: "44d201a",
		Actor: d.sofia, Created: d.hoursAgo(9), Updated: d.hoursAgo(9),
		HTMLURL: "https://git.teahouse.dev/teahouse/steep/actions/runs/9005",
		Jobs: []*ActionJob{
			{ID: 90051, Name: "deploy", Status: "failure", Started: d.hoursAgo(9), Stopped: d.hoursAgo(9), Logs: steepDeployFailLog},
		},
	})
	d.s.AddRun(&ActionRun{RepoFullName: "teahouse/steep",
		DisplayTitle: "fix: steep timer drift on background tabs", WorkflowName: "ci.yml",
		Event: "push", Status: "success", HeadBranch: "main", HeadSHA: "44d201a",
		Actor: d.mei, Created: d.daysAgo(4), Updated: d.daysAgo(4),
		HTMLURL: "https://git.teahouse.dev/teahouse/steep/actions/runs/9006",
		Jobs: []*ActionJob{
			{ID: 90061, Name: "build", Status: "success", Started: d.daysAgo(4), Stopped: d.daysAgo(4), Logs: steepPostMergeLog},
			{ID: 90062, Name: "test", Status: "success", Started: d.daysAgo(4), Stopped: d.daysAgo(4), Logs: steepPostMergeLog},
		},
	})
}

// --- notifications ---------------------------------------------------------

// notifications seeds a mixed unread/read/pinned inbox pointing at the PRs
// and issues above. Pinned rows are seeded read: Pinned and Unread are
// independent booleans on the store's Notification type, but
// MarkAllNotificationsRead sweeps every row's Unread unconditionally, which
// only matches real single-NotifyStatus-enum Gitea semantics when no row is
// simultaneously pinned and unread (see notificationEffectiveStatus).
func (d *demoBuilder) notifications() {
	add := func(unread, pinned bool, title, typ, state, repo, url string, updated time.Time) {
		d.s.AddNotification(&Notification{
			Unread: unread, Pinned: pinned, Title: title, Type: typ, State: state,
			RepoFull: repo, URL: url, Updated: updated,
		})
	}

	add(true, false, "feat: expose /health endpoint for k8s probes", "Pull", "open",
		"teahouse/kettle", "https://git.teahouse.dev/teahouse/kettle/pulls/4", d.hoursAgo(8))
	add(true, false, "refactor: extract steep-time calculator", "Pull", "open",
		"teahouse/kettle", "https://git.teahouse.dev/teahouse/kettle/pulls/5", d.hoursAgo(10))
	add(true, false, "bug: brew log timestamps use server tz not user tz", "Issue", "open",
		"teahouse/kettle", "https://git.teahouse.dev/teahouse/kettle/issues/15", d.hoursAgo(4))
	add(false, false, "fix: debounce brew-start double submit", "Pull", "open",
		"teahouse/kettle", "https://git.teahouse.dev/teahouse/kettle/pulls/2", d.hoursAgo(6))
	add(false, false, "bug: descale reminder fires twice", "Issue", "open",
		"teahouse/kettle", "https://git.teahouse.dev/teahouse/kettle/issues/17", d.hoursAgo(9))
	add(false, true, "feat: PID control loop for kettle temperature", "Pull", "open",
		"teahouse/kettle", "https://git.teahouse.dev/teahouse/kettle/pulls/1", d.hoursAgo(3))
	add(false, true, "bug: staging deploy hangs on migration step", "Issue", "open",
		"teahouse/infra", "https://git.teahouse.dev/teahouse/infra/issues/2", d.hoursAgo(7))
	add(true, false, "feat: dark mode for brew dashboard", "Pull", "open",
		"teahouse/steep", "https://git.teahouse.dev/teahouse/steep/pulls/3", d.hoursAgo(15))
	add(false, false, "bug: timer resets when tab is backgrounded", "Issue", "open",
		"teahouse/steep", "https://git.teahouse.dev/teahouse/steep/issues/4", d.hoursAgo(5))
}
