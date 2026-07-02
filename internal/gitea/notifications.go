package gitea

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	sdk "code.gitea.io/sdk/gitea"

	"github.com/gbarany/tea-dash/internal/data"
)

// ListNotifications returns the authenticated user's notification threads.
// Gitea's notification list response does not expose X-Total-Count through the
// SDK response, so total is the number of rows returned for this page.
func (c *Client) ListNotifications(_ context.Context, limit int) ([]data.Notification, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var threads []*sdk.NotificationThread
	err := c.call(func() error {
		var e error
		threads, _, e = c.sdk.ListNotifications(sdk.ListNotificationOptions{
			ListOptions: sdk.ListOptions{PageSize: limit},
			Status:      []sdk.NotifyStatus{sdk.NotifyStatusUnread},
		})
		return e
	})
	if err != nil {
		return nil, 0, err
	}
	rows := make([]data.Notification, 0, len(threads))
	for _, thread := range threads {
		rows = append(rows, mapNotificationThread(thread))
	}
	return rows, len(rows), nil
}

func mapNotificationThread(thread *sdk.NotificationThread) data.Notification {
	if thread == nil {
		return data.Notification{}
	}
	n := data.Notification{
		ID:        thread.ID,
		Unread:    thread.Unread,
		Pinned:    thread.Pinned,
		UpdatedAt: thread.UpdatedAt,
	}
	if thread.Repository != nil {
		n.RepoNameWithOwner = thread.Repository.FullName
	}
	if thread.Subject != nil {
		n.Number = notificationSubjectNumber(thread.Subject)
		n.SubjectTitle = thread.Subject.Title
		n.SubjectType = string(thread.Subject.Type)
		n.SubjectState = string(thread.Subject.State)
		n.HTMLURL = thread.Subject.HTMLURL
		n.LatestCommentURL = thread.Subject.LatestCommentHTMLURL
	}
	return n
}

func notificationSubjectNumber(subject *sdk.NotificationSubject) int64 {
	if subject == nil {
		return 0
	}
	for _, raw := range []string{subject.URL, subject.HTMLURL} {
		if n := trailingIndex(raw); n != 0 {
			return n
		}
	}
	return 0
}

func trailingIndex(raw string) int64 {
	if raw == "" {
		return 0
	}
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		raw = u.Path
	}
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return 0
	}
	parts := strings.Split(raw, "/")
	tail := parts[len(parts)-1]
	n, err := strconv.ParseInt(tail, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
