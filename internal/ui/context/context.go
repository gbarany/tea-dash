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
	IssuesView
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

	// Preview pane state. PreviewOpen toggles the side panel; PreviewWidth and
	// PreviewHeight are the pane's content dimensions. Layout math (splitting the
	// screen between the list and the preview) is wired in the integration stage;
	// these fields are declared here so the preview components can size to them.
	PreviewOpen   bool
	PreviewWidth  int
	PreviewHeight int

	Config *config.Config
	Client *gitea.Client // may be nil in tests
	User   string        // client.Me(); "" when client is nil
	View   ViewType      // PullsView | IssuesView
	Error  error
	Styles Styles

	// StartTask registers an async task; the root assigns it. Returns nil in M1a.
	StartTask func(task Task) tea.Cmd
}

// GetViewSectionsConfig returns the section configs for the current view,
// preferring the user's per-view config and falling back to a single me-scoped
// default section.
func (c *ProgramContext) GetViewSectionsConfig() []config.SectionConfig {
	switch c.View {
	case IssuesView:
		if c.Config != nil && len(c.Config.IssuesSections) > 0 {
			return c.Config.IssuesSections
		}
		return []config.SectionConfig{{
			Title:  "My Issues",
			Filter: config.PrIssueFilter{State: "open", CreatedBy: "@me"},
		}}
	default:
		if c.Config != nil && len(c.Config.PRSections) > 0 {
			return c.Config.PRSections
		}
		return []config.SectionConfig{{
			Title:  "My Pull Requests",
			Filter: config.PrIssueFilter{State: "open", CreatedBy: "@me"},
		}}
	}
}
