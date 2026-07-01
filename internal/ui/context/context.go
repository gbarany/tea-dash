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
	return []config.SectionConfig{{
		Title:  "My Pull Requests",
		Filter: config.PrIssueFilter{State: "open", CreatedBy: "@me"},
	}}
}
