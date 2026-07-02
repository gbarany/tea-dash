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
		Fetch: func(ctx stdctx.Context, c *gitea.Client, _ config.PrIssueFilter, limit int) ([]data.Notification, int, error) {
			return c.ListNotifications(ctx, limit)
		},
		BuildRow: notificationBuildRow,
	})
}

func notificationBuildRow(n data.Notification) table.Row {
	number := fmt.Sprintf("#%d", n.Number)
	if n.Number == 0 {
		number = fmt.Sprintf("n%d", n.ID)
	}
	return table.Row{
		number,
		n.SubjectTitle,
		n.RepoNameWithOwner,
		strings.ToLower(n.SubjectType),
		notificationState(n),
		section.HumanizeTime(n.UpdatedAt),
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
