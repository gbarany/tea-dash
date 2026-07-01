# tea-dash M1a — Section Architecture Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the single-file `internal/ui` model into gh-dash's section-based architecture (`ProgramContext` + `Section`/`BaseModel` + `tabs` + a routing envelope) **without changing any user-visible behavior** — the app still shows exactly today's "my pull requests" board — so that M1b can add an Issues section by dropping in a second `Section`.

**Architecture:** Introduce `internal/ui/context` (a shared per-frame `*ProgramContext`, a `StartTask` seam, one `TaskFinishedMsg` routing envelope, and cross-package styles), `internal/ui/components/section` (a `Section` interface + embeddable `BaseModel` that owns a `bubbles/v2/table` and renders loading/error/empty/table bodies), `internal/ui/components/pullsection` (the one concrete section that fetches via `gitea.SearchMyPulls` and builds today's rows), and `internal/ui/components/tabs` (a minimal tab bar that renders `""` while there is a single section). The root `ui.Model` holds `[]section.Section`, routes async results by `(SectionId, SectionType)`, and composes the same view. `data.PullRequest` gains a `RowData` interface so section/table code stays generic.

**Tech Stack:** Go 1.26, Bubble Tea v2 (`charm.land/bubbletea/v2`), Bubbles v2 (`charm.land/bubbles/v2/{table,spinner,key}`), Lip Gloss v2 (`charm.land/lipgloss/v2`), `code.gitea.io/sdk/gitea` (via the existing `internal/gitea`). Tests use the standard library `testing` package (no testify), matching the repo.

**Behavior-preservation contract (must stay byte-identical):** title `titleStyle "tea-dash" + dim "  my pull requests"`; the 6-column PR table (`# / Title / Repo / Author / State / Updated`) with the same widths and cell formatting; the loading spinner line `"  <spinner> Loading pull requests…"`; the error body `"  Error: <err>"` + the "Check your Gitea login…" hint; the empty body `"  No open pull requests authored by you."` + hint; the status line `"%d pull requests"` (only when loaded); the help line `"↑/↓ move · r refresh · o/enter open in browser · q quit"`; keys `r`/`o`/`enter`/`q`/`ctrl+c` and table row-nav; `client.SearchMyPulls(ctx, "open")` with a 30s timeout; `View()` returns `tea.View{..., AltScreen: true}`.

**Scope note — NOT in M1a (deferred):** search DSL / filters, prompts/confirmations, pagination-on-scroll, sidebar/preview/markdown, mouse zones, full task-lifecycle footer, theme parsing, multi-view switching, a second routing envelope (`SectionMsg`/`MakeSectionCmd`), section-switch keybindings. M1b adds the Issues section + `SectionConfig.Filters`; those come then.

**Package-name gotcha:** the new local package is named `context`, which shadows the stdlib `context`. Only `pullsection.go` needs both — there, import the local one aliased as `appctx` and use stdlib `context` normally. Every other file imports the local package plainly as `context`.

---

## File Structure

```
internal/data/
  utils.go                     # NEW — RowData interface + PullRequest impl
  utils_test.go                # NEW
internal/config/
  config.go                    # MODIFY — add SectionConfig
internal/ui/
  app.go                       # REWRITE (T7) — root Model over sections
  styles.go                    # MODIFY (T7) — keep appStyle/titleStyle/helpStyle only
  keys.go                      # UNCHANGED
  open.go                      # UNCHANGED
  app_test.go                  # ADAPT (T7) — drive the real TaskFinishedMsg envelope
  context/
    context.go                 # NEW — ProgramContext, Dimensions, ViewType, Task, StartTask seam
    messages.go                # NEW — TaskFinishedMsg routing envelope
    styles.go                  # NEW — Styles + DefaultStyles()
    context_test.go            # NEW
  components/
    section/
      section.go               # NEW — Section interface + BaseModel
      section_test.go          # NEW
    pullsection/
      pullsection.go           # NEW — Model (embeds BaseModel), fetch, rows, SectionType
      pullsection_test.go      # NEW
    tabs/
      tabs.go                  # NEW — minimal tab bar (hidden for <2 sections)
      tabs_test.go             # NEW
```

**Green-at-every-commit ordering:** T1–T6 create *new* packages the root does not yet import, so `internal/ui/app.go` and its tests keep compiling and passing untouched. T7 is the single deliberate cutover.

---

## Task 1: `RowData` interface on `data.PullRequest`

**Files:**
- Create: `internal/data/utils.go`
- Test: `internal/data/utils_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/data/utils_test.go`:
```go
package data

import (
	"testing"
	"time"
)

func TestPullRequestImplementsRowData(t *testing.T) {
	var _ RowData = PullRequest{}

	pr := PullRequest{
		Number:            7,
		Title:             "Fix thing",
		RepoNameWithOwner: "acme/widgets",
		HTMLURL:           "https://x/acme/widgets/pulls/7",
		UpdatedAt:         time.Unix(1000, 0),
	}
	var rd RowData = pr
	if rd.GetNumber() != 7 || rd.GetTitle() != "Fix thing" ||
		rd.GetRepoNameWithOwner() != "acme/widgets" || rd.GetUrl() != pr.HTMLURL ||
		!rd.GetUpdatedAt().Equal(pr.UpdatedAt) {
		t.Fatalf("RowData accessors wrong: %+v", rd)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/data/ -run TestPullRequestImplementsRowData -v`
Expected: FAIL — `undefined: RowData`.

- [ ] **Step 3: Write the implementation**

Create `internal/data/utils.go`:
```go
package data

import "time"

// RowData is the minimal view a section/table needs of any listed item,
// keeping the generic table/section code decoupled from concrete types.
type RowData interface {
	GetRepoNameWithOwner() string
	GetTitle() string
	GetNumber() int64
	GetUrl() string
	GetUpdatedAt() time.Time
}

func (p PullRequest) GetRepoNameWithOwner() string { return p.RepoNameWithOwner }
func (p PullRequest) GetTitle() string             { return p.Title }
func (p PullRequest) GetNumber() int64             { return p.Number }
func (p PullRequest) GetUrl() string               { return p.HTMLURL }
func (p PullRequest) GetUpdatedAt() time.Time      { return p.UpdatedAt }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/data/ -v`
Expected: PASS (new + existing data tests).

- [ ] **Step 5: Commit**

```bash
git add internal/data/utils.go internal/data/utils_test.go
git commit -m "feat(data): add RowData interface implemented by PullRequest"
```

---

## Task 2: `context` package (ProgramContext, envelope, styles)

**Files:**
- Create: `internal/ui/context/context.go`, `internal/ui/context/messages.go`, `internal/ui/context/styles.go`
- Test: `internal/ui/context/context_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/context/context_test.go`:
```go
package context

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDefaultStylesNonZero(t *testing.T) {
	s := DefaultStyles()
	// Spinner/DimText/ErrorText must be usable (Render must not panic).
	_ = s.Spinner.Render("x")
	_ = s.DimText.Render("x")
	_ = s.ErrorText.Render("x")
}

func TestStartTaskRegistersTask(t *testing.T) {
	tasks := map[string]Task{}
	ctx := &ProgramContext{}
	ctx.StartTask = func(tk Task) tea.Cmd { tasks[tk.Id] = tk; return nil }

	cmd := ctx.StartTask(Task{Id: "abc", StartText: "loading"})
	if cmd != nil {
		t.Fatalf("StartTask cmd = %v, want nil in M1a", cmd)
	}
	if _, ok := tasks["abc"]; !ok {
		t.Fatalf("task abc was not registered: %v", tasks)
	}
}

func TestGetViewSectionsConfig(t *testing.T) {
	ctx := &ProgramContext{}
	secs := ctx.GetViewSectionsConfig()
	if len(secs) != 1 || secs[0].Title != "My Pull Requests" {
		t.Fatalf("GetViewSectionsConfig = %+v", secs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/context/ -v`
Expected: FAIL — package/types undefined.

- [ ] **Step 3: Write the implementations**

Create `internal/ui/context/context.go`:
```go
// Package context holds tea-dash's shared per-frame program state, the async
// task seam, and cross-package styles. It is passed by pointer to every
// section and component. (Named "context" like gh-dash's; shadows stdlib
// context — callers that also need stdlib context alias this one.)
package context

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/gitea"
)

// Dimensions is a width/height pair.
type Dimensions struct{ Width, Height int }

// ViewType enumerates the top-level views. M1b adds IssuesView.
type ViewType int

const (
	PullsView ViewType = iota
)

// TaskState is the lifecycle of an async task.
type TaskState int

const (
	TaskStart TaskState = iota
	TaskFinished
	TaskError
)

// Task is a registered async unit of work (spinner/footer bookkeeping).
type Task struct {
	Id           string
	StartText    string
	FinishedText string
	State        TaskState
	Error        error
	StartTime    time.Time
	FinishedTime *time.Time
}

// ProgramContext is the shared state bag threaded through the UI.
type ProgramContext struct {
	ScreenWidth, ScreenHeight           int
	MainContentWidth, MainContentHeight int

	Config *config.Config
	Client *gitea.Client // may be nil in tests
	User   string        // client.Me(); "" when client is nil
	View   ViewType      // M1a: always PullsView
	Error  error
	Styles Styles

	// StartTask registers an async task; the root assigns it. Returns nil in M1a.
	StartTask func(task Task) tea.Cmd
}

// GetViewSectionsConfig returns the section configs for the current view.
// M1b grows this into a per-view, config-driven list.
func (c *ProgramContext) GetViewSectionsConfig() []config.SectionConfig {
	return []config.SectionConfig{{Title: "My Pull Requests"}}
}
```

Create `internal/ui/context/messages.go`:
```go
package context

import tea "charm.land/bubbletea/v2"

// TaskFinishedMsg is the single routing envelope: an async result self-routes
// to the owning section by (SectionId, SectionType); Msg carries the payload.
type TaskFinishedMsg struct {
	SectionId   int
	SectionType string
	TaskId      string
	Msg         tea.Msg
}
```

Create `internal/ui/context/styles.go`:
```go
package context

import (
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

// Styles holds the precomputed lipgloss styles shared across components.
// M1a hardcodes the same colors the app used before the refactor.
type Styles struct {
	Table        table.Styles
	Spinner      lipgloss.Style
	DimText      lipgloss.Style
	ErrorText    lipgloss.Style
	Tab          lipgloss.Style
	ActiveTab    lipgloss.Style
	TabSeparator lipgloss.Style
}

// DefaultStyles returns the built-in styles (no theme parsing in M1a).
func DefaultStyles() Styles {
	tbl := table.DefaultStyles()
	tbl.Header = tbl.Header.Bold(true).Foreground(lipgloss.Color("#00ADD8")).BorderBottom(true)
	tbl.Selected = tbl.Selected.Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	return Styles{
		Table:        tbl,
		Spinner:      lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8")),
		DimText:      lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		ErrorText:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		Tab:          lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1),
		ActiveTab:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00ADD8")).Padding(0, 1),
		TabSeparator: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/context/ -v`
Expected: PASS. Also `go build ./...` stays green (root doesn't import this yet).

- [ ] **Step 5: Commit**

```bash
git add internal/ui/context/
git commit -m "feat(ui/context): add ProgramContext, task envelope, and styles"
```

---

## Task 3: `config.SectionConfig`

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:
```go
func TestSectionConfigZeroValue(t *testing.T) {
	var s SectionConfig
	s.Title = "My Pull Requests"
	if s.Title != "My Pull Requests" {
		t.Fatalf("SectionConfig.Title = %q", s.Title)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestSectionConfigZeroValue -v`
Expected: FAIL — `undefined: SectionConfig`.

- [ ] **Step 3: Write the implementation**

In `internal/config/config.go`, add (after the `Instance` type):
```go
// SectionConfig describes one dashboard section (a tab). M1b adds Filters/Limit.
type SectionConfig struct {
	Title string `yaml:"title"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add SectionConfig"
```

---

## Task 4: `section` package (interface + BaseModel)

**Files:**
- Create: `internal/ui/components/section/section.go`
- Test: `internal/ui/components/section/section_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/components/section/section_test.go`:
```go
package section

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/table"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func newBase(t *testing.T) *BaseModel {
	t.Helper()
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	b := NewBaseModel(NewOptions{
		Id:      0,
		Type:    "pr",
		Ctx:     ctx,
		Config:  config.SectionConfig{Title: "My Pull Requests"},
		Columns: []table.Column{{Title: "#", Width: 6}, {Title: "Title", Width: 20}},
	})
	return &b
}

func TestBaseModelMetadata(t *testing.T) {
	b := newBase(t)
	if b.GetId() != 0 || b.GetType() != "pr" || b.GetTitle() != "My Pull Requests" {
		t.Fatalf("metadata wrong: id=%d type=%q title=%q", b.GetId(), b.GetType(), b.GetTitle())
	}
}

func TestBaseModelViewStates(t *testing.T) {
	b := newBase(t)

	b.SetIsLoading(true)
	if !strings.Contains(b.View(), "Loading pull requests") {
		t.Fatalf("loading view: %q", b.View())
	}

	b.SetIsLoading(false)
	b.SetError(errors.New("boom"))
	v := b.View()
	if !strings.Contains(v, "Error") || !strings.Contains(v, "boom") {
		t.Fatalf("error view: %q", v)
	}

	b.SetError(nil)
	b.SetRows(nil) // zero rows -> empty state
	if !strings.Contains(b.View(), "No open pull requests") {
		t.Fatalf("empty view: %q", b.View())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/components/section/ -v`
Expected: FAIL — `undefined: NewBaseModel` / `NewOptions` / `BaseModel`.

- [ ] **Step 3: Write the implementation**

Create `internal/ui/components/section/section.go`:
```go
// Package section defines the Section contract every dashboard section
// satisfies, plus an embeddable BaseModel that owns the table/spinner and
// renders the loading/error/empty/table body.
package section

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Section is implemented by *pointer* section types (they mutate on Update).
type Section interface {
	GetId() int
	GetType() string
	GetTitle() string
	GetTotalCount() int
	GetIsLoading() bool
	SetIsLoading(bool)
	GetError() error

	NumRows() int
	CurrRow() int
	GetCurrRow() data.RowData
	BuildRows() []table.Row
	FetchRows() tea.Cmd

	Update(msg tea.Msg) (Section, tea.Cmd)
	View() string
	UpdateProgramContext(ctx *context.ProgramContext)
}

// NewOptions parameterizes NewBaseModel.
type NewOptions struct {
	Id      int
	Type    string
	Ctx     *context.ProgramContext
	Config  config.SectionConfig
	Columns []table.Column
}

// BaseModel provides the shared machinery concrete sections embed.
type BaseModel struct {
	id          int
	sectionType string
	totalCount  int
	numRows     int
	isLoading   bool
	lastFetchID string
	err         error

	Ctx     *context.ProgramContext
	Config  config.SectionConfig
	Table   table.Model
	Spinner spinner.Model
	Columns []table.Column
}

// NewBaseModel builds a BaseModel with an empty focused table and a spinner,
// starting in the loading state.
func NewBaseModel(o NewOptions) BaseModel {
	t := table.New(
		table.WithColumns(o.Columns),
		table.WithFocused(true),
	)
	t.SetStyles(o.Ctx.Styles.Table)
	sp := spinner.New(spinner.WithStyle(o.Ctx.Styles.Spinner))
	return BaseModel{
		id:          o.Id,
		sectionType: o.Type,
		Ctx:         o.Ctx,
		Config:      o.Config,
		Table:       t,
		Spinner:     sp,
		Columns:     o.Columns,
		isLoading:   true,
	}
}

func (m *BaseModel) GetId() int         { return m.id }
func (m *BaseModel) GetType() string    { return m.sectionType }
func (m *BaseModel) GetTitle() string   { return m.Config.Title }
func (m *BaseModel) GetTotalCount() int { return m.totalCount }
func (m *BaseModel) SetTotalCount(n int) { m.totalCount = n }
func (m *BaseModel) GetIsLoading() bool { return m.isLoading }
func (m *BaseModel) SetIsLoading(v bool) { m.isLoading = v }
func (m *BaseModel) GetError() error    { return m.err }
func (m *BaseModel) SetError(err error) { m.err = err }
func (m *BaseModel) LastFetchID() string   { return m.lastFetchID }
func (m *BaseModel) SetLastFetchID(id string) { m.lastFetchID = id }

func (m *BaseModel) NumRows() int { return m.numRows }
func (m *BaseModel) CurrRow() int { return m.Table.Cursor() }

// SetRows updates the table rows and records the count for the empty-state check.
func (m *BaseModel) SetRows(rows []table.Row) {
	m.Table.SetRows(rows)
	m.numRows = len(rows)
}

// UpdateProgramContext recaches the context and resizes the table to the main
// content area. Concrete sections may override to also refresh columns.
func (m *BaseModel) UpdateProgramContext(ctx *context.ProgramContext) {
	m.Ctx = ctx
	m.Table.SetWidth(ctx.MainContentWidth)
	m.Table.SetHeight(ctx.MainContentHeight)
}

// View renders the section body, preserving the pre-refactor layout exactly.
func (m *BaseModel) View() string {
	st := m.Ctx.Styles
	switch {
	case m.isLoading:
		return "\n  " + m.Spinner.View() + " Loading pull requests…"
	case m.err != nil:
		return "\n" + st.ErrorText.Render("  Error: "+m.err.Error()) + "\n\n" +
			st.DimText.Render("  Check your Gitea login (run `tea login add`) and network.")
	case m.numRows == 0:
		return "\n  No open pull requests authored by you.\n\n" +
			st.DimText.Render("  This board shows PRs you created across all repos on your Gitea instance.")
	default:
		return m.Table.View()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/components/section/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/components/section/
git commit -m "feat(ui/section): add Section interface and BaseModel"
```

---

## Task 5: `pullsection` package

**Files:**
- Create: `internal/ui/components/pullsection/pullsection.go`
- Test: `internal/ui/components/pullsection/pullsection_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/components/pullsection/pullsection_test.go`:
```go
package pullsection

import (
	"strings"
	"testing"
	"time"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func newModel(t *testing.T) *Model {
	t.Helper()
	ctx := &context.ProgramContext{Styles: context.DefaultStyles(), MainContentWidth: 100, MainContentHeight: 20}
	m := NewModel(0, ctx, config.SectionConfig{Title: "My Pull Requests"})
	return m
}

func TestImplementsSection(t *testing.T) {
	var _ section.Section = (*Model)(nil)
}

func TestFetchedMsgBuildsRows(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("t1")
	next, _ := m.Update(SectionPullRequestsFetchedMsg{
		Prs: []data.PullRequest{{
			Number: 128, Title: "Add wiki CLI", RepoNameWithOwner: "gitea/tea",
			Author: "lunny", State: "open", UpdatedAt: time.Now().Add(-2 * time.Hour),
		}},
		TotalCount: 1, TaskId: "t1",
	})
	m = next.(*Model)

	if m.GetTotalCount() != 1 || m.NumRows() != 1 {
		t.Fatalf("counts: total=%d rows=%d", m.GetTotalCount(), m.NumRows())
	}
	if m.GetCurrRow() == nil || m.GetCurrRow().GetNumber() != 128 {
		t.Fatalf("GetCurrRow = %+v", m.GetCurrRow())
	}
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	for _, want := range []string{"#128", "Add wiki CLI", "gitea/tea", "@lunny", "open"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("row %q missing %q", joined, want)
		}
	}
}

func TestStaleFetchIgnored(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("t2") // an in-flight fetch t2 is expected
	next, _ := m.Update(SectionPullRequestsFetchedMsg{
		Prs: []data.PullRequest{{Number: 1}}, TotalCount: 1, TaskId: "t1", // stale
	})
	m = next.(*Model)
	if m.NumRows() != 0 {
		t.Fatalf("stale fetch was applied: rows=%d", m.NumRows())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/components/pullsection/ -v`
Expected: FAIL — `undefined: NewModel` / `Model` / `SectionPullRequestsFetchedMsg`.

- [ ] **Step 3: Write the implementation**

Create `internal/ui/components/pullsection/pullsection.go`:
```go
// Package pullsection is the pull-requests dashboard section.
package pullsection

import (
	stdctx "context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

// SectionType is the routing type tag for pull-request sections.
const SectionType = "pr"

const fetchTimeout = 30 * time.Second

// Model is the pull-requests section.
type Model struct {
	section.BaseModel
	prs []data.PullRequest
}

// compile-time interface assertion
var _ section.Section = (*Model)(nil)

// SectionPullRequestsFetchedMsg is the fetch payload carried in TaskFinishedMsg.Msg.
type SectionPullRequestsFetchedMsg struct {
	Prs        []data.PullRequest
	TotalCount int
	TaskId     string
	Err        error
}

// NewModel builds a pull-requests section.
func NewModel(id int, ctx *appctx.ProgramContext, cfg config.SectionConfig) *Model {
	m := &Model{}
	m.BaseModel = section.NewBaseModel(section.NewOptions{
		Id:      id,
		Type:    SectionType,
		Ctx:     ctx,
		Config:  cfg,
		Columns: columns(ctx.MainContentWidth),
	})
	return m
}

// FetchRows fetches the current user's open PRs across all repos.
func (m *Model) FetchRows() tea.Cmd {
	taskId := fmt.Sprintf("fetch-pulls-%d-%d", m.GetId(), time.Now().UnixNano())
	m.SetLastFetchID(taskId)
	m.SetIsLoading(true)
	client := m.Ctx.Client
	id, sType := m.GetId(), m.GetType()
	start := m.Ctx.StartTask(appctx.Task{Id: taskId, StartText: "Loading pull requests…", State: appctx.TaskStart})
	fetch := func() tea.Msg {
		ctx, cancel := stdctx.WithTimeout(stdctx.Background(), fetchTimeout)
		defer cancel()
		prs, err := client.SearchMyPulls(ctx, "open")
		return appctx.TaskFinishedMsg{
			SectionId: id, SectionType: sType, TaskId: taskId,
			Msg: SectionPullRequestsFetchedMsg{Prs: prs, TotalCount: len(prs), TaskId: taskId, Err: err},
		}
	}
	return tea.Batch(start, m.Spinner.Tick, fetch)
}

// Update applies fetched rows (behind a stale-fetch guard), advances the
// spinner while loading, and otherwise delegates to the table (row nav).
func (m *Model) Update(msg tea.Msg) (section.Section, tea.Cmd) {
	switch msg := msg.(type) {
	case SectionPullRequestsFetchedMsg:
		if m.LastFetchID() != "" && m.LastFetchID() != msg.TaskId {
			return m, nil // stale/superseded fetch
		}
		m.SetIsLoading(false)
		if msg.Err != nil {
			m.SetError(msg.Err)
			return m, nil
		}
		m.prs = msg.Prs
		m.SetTotalCount(msg.TotalCount)
		m.SetError(nil)
		// SetRows clamps but does not reset the cursor, so a refresh keeps the
		// selected row (intentional; better UX than the pre-refactor reset-to-top).
		m.SetRows(m.BuildRows())
		return m, nil
	case spinner.TickMsg:
		if m.GetIsLoading() {
			var cmd tea.Cmd
			m.Spinner, cmd = m.Spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	// Only forward navigation/other messages to the table when ready — matches the
	// pre-refactor gating, which ignored keys while loading or in the error state.
	if m.GetIsLoading() || m.GetError() != nil {
		return m, nil
	}
	var cmd tea.Cmd
	m.Table, cmd = m.Table.Update(msg)
	return m, cmd
}

// UpdateProgramContext resizes the table and refreshes columns for the new width.
func (m *Model) UpdateProgramContext(ctx *appctx.ProgramContext) {
	m.BaseModel.UpdateProgramContext(ctx)
	m.Columns = columns(ctx.MainContentWidth)
	m.Table.SetColumns(m.Columns)
}

// GetCurrRow returns the selected PR, or nil when there are no rows.
func (m *Model) GetCurrRow() data.RowData {
	if len(m.prs) == 0 {
		return nil
	}
	i := m.CurrRow()
	if i < 0 || i >= len(m.prs) {
		return nil
	}
	return m.prs[i]
}

// BuildRows maps the PRs into the table's 6-cell rows (unchanged formatting).
func (m *Model) BuildRows() []table.Row {
	rows := make([]table.Row, len(m.prs))
	for i, pr := range m.prs {
		author := ""
		if pr.Author != "" {
			author = "@" + pr.Author
		}
		rows[i] = table.Row{
			fmt.Sprintf("#%d", pr.Number),
			pr.Title,
			pr.RepoNameWithOwner,
			author,
			prState(pr),
			humanizeTime(pr.UpdatedAt),
		}
	}
	return rows
}

// columns reproduces the pre-refactor column widths and title-grow formula.
func columns(mainWidth int) []table.Column {
	const (
		numW     = 6
		repoW    = 22
		authorW  = 16
		stateW   = 8
		updatedW = 10
	)
	titleW := mainWidth - (numW + repoW + authorW + stateW + updatedW) - 6
	if titleW < 20 {
		titleW = 20
	}
	return []table.Column{
		{Title: "#", Width: numW},
		{Title: "Title", Width: titleW},
		{Title: "Repo", Width: repoW},
		{Title: "Author", Width: authorW},
		{Title: "State", Width: stateW},
		{Title: "Updated", Width: updatedW},
	}
}

func prState(pr data.PullRequest) string {
	if pr.Draft {
		return "draft"
	}
	return pr.State
}

func humanizeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/components/pullsection/ -v`
Expected: PASS (interface assertion + fetched-rows + stale-fetch tests).

- [ ] **Step 5: Commit**

```bash
git add internal/ui/components/pullsection/
git commit -m "feat(ui/pullsection): add pull-requests section over BaseModel"
```

---

## Task 6: `tabs` package

**Files:**
- Create: `internal/ui/components/tabs/tabs.go`
- Test: `internal/ui/components/tabs/tabs_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/components/tabs/tabs_test.go`:
```go
package tabs

import (
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func TestTabBarHiddenForSingleSection(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{pullsection.NewModel(0, ctx, config.SectionConfig{Title: "PRs"})})
	if got := tb.View(); got != "" {
		t.Fatalf("single-section tab bar = %q, want empty", got)
	}
}

func TestTabBarShowsTwoSections(t *testing.T) {
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	tb := New(ctx)
	tb.SetSections([]Sectioner{
		pullsection.NewModel(0, ctx, config.SectionConfig{Title: "PRs"}),
		pullsection.NewModel(1, ctx, config.SectionConfig{Title: "Issues"}),
	})
	v := tb.View()
	if !strings.Contains(v, "PRs") || !strings.Contains(v, "Issues") {
		t.Fatalf("two-section tab bar = %q", v)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/components/tabs/ -v`
Expected: FAIL — `undefined: New` / `Sectioner`.

- [ ] **Step 3: Write the implementation**

Create `internal/ui/components/tabs/tabs.go`:
```go
// Package tabs renders the section tab bar. It is hidden (empty) while there is
// fewer than two sections, so a single-section view looks unchanged.
package tabs

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Sectioner is the minimal view of a section the tab bar needs. (section.Section
// satisfies it; declared here to avoid importing section and risking a cycle.)
type Sectioner interface {
	GetTitle() string
	GetTotalCount() int
	GetIsLoading() bool
}

// Model is the tab bar.
type Model struct {
	ctx      *context.ProgramContext
	sections []Sectioner
	cursor   int
}

// New builds a tab bar bound to ctx.
func New(ctx *context.ProgramContext) Model {
	return Model{ctx: ctx}
}

// SetSections replaces the sections the bar renders.
func (m *Model) SetSections(s []Sectioner) { m.sections = s }

// SetCurrSectionId marks the active tab.
func (m *Model) SetCurrSectionId(i int) { m.cursor = i }

// CurrSectionId returns the active tab index.
func (m Model) CurrSectionId() int { return m.cursor }

// UpdateProgramContext recaches the shared context.
func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) { m.ctx = ctx }

// View renders the tab bar, or "" when there are fewer than two sections.
func (m Model) View() string {
	if len(m.sections) < 2 {
		return ""
	}
	st := m.ctx.Styles
	tabs := make([]string, len(m.sections))
	for i, s := range m.sections {
		label := fmt.Sprintf("%s (%d)", s.GetTitle(), s.GetTotalCount())
		if i == m.cursor {
			tabs[i] = st.ActiveTab.Render(label)
		} else {
			tabs[i] = st.Tab.Render(label)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/components/tabs/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/components/tabs/
git commit -m "feat(ui/tabs): add tab bar (hidden for a single section)"
```

---

## Task 7: Cutover — rewrite the root model, slim styles, adapt tests

This is the single integration commit; every dependency (T1–T6) is already tested and committed.

**Files:**
- Rewrite: `internal/ui/app.go`
- Modify: `internal/ui/styles.go`
- Adapt: `internal/ui/app_test.go`

- [ ] **Step 1: Adapt the tests first (they define the new wiring)**

Replace the whole file `internal/ui/app_test.go` with:
```go
package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func fetchedMsg(prs []data.PullRequest) context.TaskFinishedMsg {
	return context.TaskFinishedMsg{
		SectionId:   0,
		SectionType: pullsection.SectionType,
		TaskId:      "t1",
		Msg: pullsection.SectionPullRequestsFetchedMsg{
			Prs: prs, TotalCount: len(prs), TaskId: "t1",
		},
	}
}

func TestModelRendersLoadedPulls(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, fetchedMsg([]data.PullRequest{{
		Number: 128, Title: "Add wiki CLI", RepoNameWithOwner: "gitea/tea",
		Author: "lunny", State: "open", UpdatedAt: time.Now().Add(-2 * time.Hour),
	}}))

	view := m.View().Content
	for _, want := range []string{"#128", "Add wiki CLI", "gitea/tea", "@lunny", "1 pull requests"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q\n---\n%s", want, view)
		}
	}
}

func TestModelRendersError(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = update(t, m, context.TaskFinishedMsg{
		SectionId: 0, SectionType: pullsection.SectionType, TaskId: "t1",
		Msg: pullsection.SectionPullRequestsFetchedMsg{TaskId: "t1", Err: errBoom},
	})

	view := m.View().Content
	if !strings.Contains(view, "Error") || !strings.Contains(view, "boom") {
		t.Fatalf("expected an error view, got:\n%s", view)
	}
}

func TestQuitKeyStopsProgram(t *testing.T) {
	m := New(&config.Config{}, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected a quit command, got nil")
	}
}

func TestUnknownSectionIsNoOp(t *testing.T) {
	m := New(&config.Config{}, nil)
	// A TaskFinishedMsg for a section/type that doesn't exist must not panic.
	_, _ = m.Update(context.TaskFinishedMsg{SectionId: 99, SectionType: "nope", TaskId: "x"})
}

var errBoom = errBoomType("boom")

type errBoomType string

func (e errBoomType) Error() string { return string(e) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestModelRendersLoadedPulls -v`
Expected: FAIL — build errors (`New` signature/fields changed, `context`/`pullsection` unused by old app.go, `pullsLoadedMsg` gone).

- [ ] **Step 3: Rewrite `internal/ui/app.go`**

Replace the whole file `internal/ui/app.go` with:
```go
// Package ui contains the Bubble Tea models that make up the tea-dash TUI.
package ui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/pullsection"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	"github.com/gbarany/tea-dash/internal/ui/components/tabs"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

// Model is the root tea-dash model: a set of sections rendered as tabs.
type Model struct {
	ctx           *context.ProgramContext
	keys          keyMap
	tabs          tabs.Model
	tasks         map[string]context.Task
	currSectionId int
	sections      []section.Section
}

// New builds the root model. client may be nil in tests (only FetchRows uses it).
func New(cfg *config.Config, client *gitea.Client) Model {
	tasks := map[string]context.Task{}
	user := ""
	if client != nil {
		user = client.Me()
	}
	ctx := &context.ProgramContext{
		Config: cfg,
		Client: client,
		User:   user,
		View:   context.PullsView,
		Styles: context.DefaultStyles(),
	}
	ctx.StartTask = func(t context.Task) tea.Cmd {
		tasks[t.Id] = t
		return nil
	}

	sections := []section.Section{
		pullsection.NewModel(0, ctx, ctx.GetViewSectionsConfig()[0]),
	}

	tb := tabs.New(ctx)
	tb.SetSections(toSectioners(sections))

	return Model{
		ctx:      ctx,
		keys:     defaultKeyMap(),
		tabs:     tb,
		tasks:    tasks,
		sections: sections,
	}
}

// Init starts the initial fetch for the current section.
func (m Model) Init() tea.Cmd {
	return m.getCurrSection().FetchRows()
}

// Update routes messages: layout, async results, keys, then generic fallthrough.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.ctx.ScreenWidth = msg.Width
		m.ctx.ScreenHeight = msg.Height
		m.syncMainContentDimensions()
		m.syncProgramContext()
		return m, nil

	case context.TaskFinishedMsg:
		delete(m.tasks, msg.TaskId)
		return m, m.updateSection(msg.SectionId, msg.SectionType, msg.Msg)

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Refresh):
			if !m.getCurrSection().GetIsLoading() {
				return m, m.getCurrSection().FetchRows()
			}
			return m, nil
		case key.Matches(msg, m.keys.Open):
			return m, m.openSelected()
		}
	}

	return m, m.updateCurrentSection(msg)
}

// View composes the same shell as before: title, (tab bar), section body,
// status line, help.
func (m Model) View() tea.View {
	title := titleStyle.Render("tea-dash") + m.ctx.Styles.DimText.Render("  my pull requests")

	parts := []string{title}
	if tv := m.tabs.View(); tv != "" {
		parts = append(parts, tv)
	}
	parts = append(parts, m.getCurrSection().View(), m.statusLine(),
		helpStyle.Render("↑/↓ move · r refresh · o/enter open in browser · q quit"))

	content := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return tea.View{Content: content, AltScreen: true}
}

func (m Model) getCurrSection() section.Section { return m.sections[m.currSectionId] }

func (m Model) getCurrRowData() data.RowData { return m.getCurrSection().GetCurrRow() }

func (m Model) openSelected() tea.Cmd {
	row := m.getCurrRowData()
	if row == nil {
		return nil
	}
	url := row.GetUrl()
	return func() tea.Msg {
		_ = openURL(url)
		return nil
	}
}

func (m *Model) updateSection(id int, sType string, msg tea.Msg) tea.Cmd {
	for i, s := range m.sections {
		if s.GetId() == id && s.GetType() == sType {
			var cmd tea.Cmd
			m.sections[i], cmd = s.Update(msg)
			return cmd
		}
	}
	return nil
}

func (m *Model) updateCurrentSection(msg tea.Msg) tea.Cmd {
	s := m.getCurrSection()
	return m.updateSection(s.GetId(), s.GetType(), msg)
}

func (m *Model) syncProgramContext() {
	for _, s := range m.sections {
		s.UpdateProgramContext(m.ctx)
	}
	m.tabs.UpdateProgramContext(m.ctx)
}

func (m *Model) syncMainContentDimensions() {
	m.ctx.MainContentWidth = m.ctx.ScreenWidth - 4
	h := m.ctx.ScreenHeight - 6
	if h < 3 {
		h = 3
	}
	m.ctx.MainContentHeight = h
}

func (m Model) statusLine() string {
	s := m.getCurrSection()
	if s.GetIsLoading() || s.GetError() != nil {
		return ""
	}
	return m.ctx.Styles.DimText.Render(fmt.Sprintf("%d pull requests", s.GetTotalCount()))
}

// toSectioners adapts sections to the tab bar's minimal interface.
func toSectioners(sections []section.Section) []tabs.Sectioner {
	out := make([]tabs.Sectioner, len(sections))
	for i, s := range sections {
		out[i] = s
	}
	return out
}
```

- [ ] **Step 4: Slim `internal/ui/styles.go`**

Replace the whole file `internal/ui/styles.go` with (shell-only styles; table/spinner/dim/error styles now live in `context.Styles`):
```go
package ui

import "charm.land/lipgloss/v2"

var (
	appStyle   = lipgloss.NewStyle().Padding(1, 2)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00ADD8"))
	helpStyle  = lipgloss.NewStyle().MarginTop(1).Foreground(lipgloss.Color("241"))
)
```

- [ ] **Step 5: Run the UI tests and the whole suite**

Run: `go test ./internal/ui/... -v && go build ./...`
Expected: PASS — all four+ UI tests (loaded, error, quit, unknown-section no-op) plus the section/pullsection/tabs/context package tests; `go build ./...` and `main.go` compile unchanged (`ui.New(cfg, client)` signature preserved).

- [ ] **Step 6: Full check + behavior spot-check**

Run: `make check && make build`
Expected: `make check` (fmt-check + vet + test) green; binary builds. (If `gofmt` flags files, run `make fmt` and re-run.)

- [ ] **Step 7: Commit**

```bash
git add internal/ui/app.go internal/ui/styles.go internal/ui/app_test.go
git commit -m "refactor(ui): move to section-based architecture (ProgramContext + Section + tabs)"
```

---

## Done — M1a Exit Criteria

- [ ] The app renders exactly today's "my pull requests" board (title, 6-column table, loading/error/empty bodies, status + help lines) — behavior unchanged.
- [ ] `internal/ui/context` (ProgramContext + `TaskFinishedMsg` + `DefaultStyles`), `internal/ui/components/section` (Section + BaseModel), `internal/ui/components/pullsection`, and `internal/ui/components/tabs` exist and are tested.
- [ ] The root `ui.Model` holds `[]section.Section`, routes fetch results by `(SectionId, SectionType)`, and an unknown `(id,type)` is a safe no-op.
- [ ] `ui.New(cfg, client)` / `Init` / `Update` / `View` signatures unchanged (main.go untouched).
- [ ] The tab bar renders `""` for a single section (so M1b's second section lights it up automatically).
- [ ] `make check` passes.

**Next:** M1b — add the Issues section + `SectionConfig.Filters` (structured → SDK/raw params) + section-switch keybindings + live `/` keyword; the tab bar becomes visible with two sections.
