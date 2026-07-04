// Package notificationsection is the notifications dashboard section: thin
// wiring over the generic section.Model parameterized for data.Notification.
package notificationsection

import (
	stdctx "context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
)

// SectionType is the routing type tag for notification sections.
const SectionType = "notification"

// Model is the notifications section (the generic section specialized for
// notifications).
type Model = section.Model[data.Notification]

// SectionNotificationsFetchedMsg is the fetch payload carried in TaskFinishedMsg.Msg.
type SectionNotificationsFetchedMsg = section.RowsFetchedMsg[data.Notification]

// NewModel builds a notifications section.
func NewModel(id int, ctx *appctx.ProgramContext, cfg config.SectionConfig) *Model {
	return section.New(section.Options[data.Notification]{
		Id:           id,
		Ctx:          ctx,
		Config:       cfg,
		Type:         SectionType,
		FilterKind:   "",
		LoadingText:  "Loading notifications…",
		EmptyText:    "No notifications.",
		EmptyHint:    "This board shows notification threads from your Gitea instance.",
		SingularForm: "notification",
		PluralForm:   "notifications",
		Limit:        func(c *config.Config) int { return c.Defaults.NotificationsLimit },
		Fetch: func(fetchCtx stdctx.Context, c *gitea.Client, _ config.PrIssueFilter, limit, _ int) ([]data.Notification, int, error) {
			includeRead := true
			if ctx.Config != nil {
				includeRead = ctx.Config.Defaults.IncludeReadNotificationsEnabled()
			}
			return c.ListNotifications(fetchCtx, limit, includeRead)
		},
		BuildRow: func(n data.Notification) table.Row {
			// Column-name-driven (not a fixed 6-cell literal): Columns
			// falls back to the shared section.DefaultColumns, which
			// responsively drops columns per SixColumnSpec.Fit, so the
			// row's cell count/order must track whatever that yields for
			// the CURRENT width, recomputed on every call (not frozen at
			// construction — see pullsection.NewModel's identical comment).
			columnNames := section.ColumnNamesFromConfig(nil, section.DefaultColumnDefinitions(ctx.MainContentWidth))
			return notificationBuildRowWithColumns(n, columnNames, ctx)
		},
	})
}

func notificationBuildRowWithColumns(n data.Notification, columns []string, ctx *appctx.ProgramContext) table.Row {
	row := make(table.Row, 0, len(columns))
	for _, column := range columns {
		row = append(row, notificationColumnValue(n, column, ctx))
	}
	return row
}

func notificationColumnValue(n data.Notification, column string, ctx *appctx.ProgramContext) string {
	switch column {
	case "number":
		if n.Number == 0 {
			return fmt.Sprintf("n%d", n.ID)
		}
		return fmt.Sprintf("#%d", n.Number)
	case "title":
		return n.SubjectTitle
	case "repo":
		return n.RepoNameWithOwner
	case "author":
		// Pre-existing choice (kept as-is): this section shows the
		// notification's subject type (pull/issue) under the shared
		// "Author"-named column slot rather than an actual author.
		return strings.ToLower(n.SubjectType)
	case "state":
		return section.StateCell(notificationState(n), ctx.Icons, ctx.Styles)
	case "updated":
		return section.HumanizeTime(n.UpdatedAt)
	default:
		return ""
	}
}

func notificationState(n data.Notification) string {
	switch {
	case n.Unread:
		return "unread"
	case n.Pinned:
		return "pinned"
	case n.SubjectState != "":
		return n.SubjectState
	default:
		return "read"
	}
}
