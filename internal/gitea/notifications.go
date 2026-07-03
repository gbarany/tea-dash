package gitea

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	sdk "code.gitea.io/sdk/gitea"

	"github.com/gbarany/tea-dash/internal/data"
)

// ListNotifications returns the authenticated user's notification threads.
// Gitea's notification list response does not expose X-Total-Count through the
// SDK response, so total is the number of rows returned for this page.
func (c *Client) ListNotifications(_ context.Context, limit int, includeRead bool) ([]data.Notification, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var threads []*sdk.NotificationThread
	err := c.call(func() error {
		var e error
		threads, _, e = c.sdk.ListNotifications(sdk.ListNotificationOptions{
			ListOptions: sdk.ListOptions{PageSize: limit},
			Status:      notificationStatuses(includeRead),
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

func notificationStatuses(includeRead bool) []sdk.NotifyStatus {
	if includeRead {
		return []sdk.NotifyStatus{sdk.NotifyStatusUnread, sdk.NotifyStatusRead, sdk.NotifyStatusPinned}
	}
	return []sdk.NotifyStatus{sdk.NotifyStatusUnread, sdk.NotifyStatusPinned}
}

// MarkNotificationRead marks one notification thread as read.
func (c *Client) MarkNotificationRead(_ context.Context, threadID int64) error {
	return c.markNotification(threadID, sdk.NotifyStatusRead, "read")
}

// MarkNotificationUnread marks one notification thread as unread.
func (c *Client) MarkNotificationUnread(_ context.Context, threadID int64) error {
	return c.markNotification(threadID, sdk.NotifyStatusUnread, "unread")
}

// PinNotification pins one notification thread.
func (c *Client) PinNotification(_ context.Context, threadID int64) error {
	return c.markNotification(threadID, sdk.NotifyStatusPinned, "pinned")
}

// UnpinNotification unpins one notification thread by restoring its normal
// read/unread status. Gitea models pinned as a notification status rather than
// a boolean patch, so the caller must pass the row's current unread state.
func (c *Client) UnpinNotification(_ context.Context, threadID int64, unread bool) error {
	status := sdk.NotifyStatusRead
	label := "unpinned read"
	if unread {
		status = sdk.NotifyStatusUnread
		label = "unpinned unread"
	}
	return c.markNotification(threadID, status, label)
}

// MarkAllNotificationsRead marks all unread notification threads as read.
func (c *Client) MarkAllNotificationsRead(_ context.Context) error {
	err := c.call(func() error {
		_, _, e := c.sdk.ReadNotifications(sdk.MarkNotificationOptions{
			Status:   []sdk.NotifyStatus{sdk.NotifyStatusUnread},
			ToStatus: sdk.NotifyStatusRead,
		})
		return e
	})
	if err != nil {
		return fmt.Errorf("marking all unread notifications read: %w", err)
	}
	return nil
}

func (c *Client) markNotification(threadID int64, status sdk.NotifyStatus, label string) error {
	err := c.call(func() error {
		_, _, e := c.sdk.ReadNotification(threadID, status)
		return e
	})
	if err != nil {
		return fmt.Errorf("marking notification %d %s: %w", threadID, label, err)
	}
	return nil
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
