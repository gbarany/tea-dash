package mockgitea

import (
	"context"
	"testing"
	"time"
)

func notifStore(now time.Time) *Store {
	s := NewStore()
	s.AddNotification(&Notification{ID: 1, Unread: true, Title: "fix: login flow",
		Type: "Pull", RepoFull: "teahouse/kettle", Updated: now})
	s.AddNotification(&Notification{ID: 2, Unread: false, Pinned: true,
		Title: "bug: kettle whistles", Type: "Issue", RepoFull: "teahouse/kettle",
		Updated: now.Add(-time.Hour)})
	return s
}

func TestListNotifications(t *testing.T) {
	s := notifStore(time.Now())
	c := newTestClient(t, s)
	rows, _, err := c.ListNotifications(context.Background(), 50, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 notifications, got %+v", rows)
	}
}

func TestNotificationLifecycle(t *testing.T) {
	s := notifStore(time.Now())
	c := newTestClient(t, s)
	ctx := context.Background()
	if err := c.MarkNotificationRead(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if s.NotificationByID(1).Unread {
		t.Fatal("still unread after MarkNotificationRead")
	}
	if err := c.MarkNotificationUnread(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.PinNotification(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if !s.NotificationByID(1).Pinned {
		t.Fatal("not pinned")
	}
	if err := c.UnpinNotification(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	// Regression lock: UnpinNotification(unread=true) must clear Pinned as
	// well as restoring Unread — a PATCH to-status=unread is a full
	// NotifyStatus overwrite, not a Pinned-only toggle (there is no distinct
	// "unpinned" status on the real API).
	if n := s.NotificationByID(1); n.Pinned || !n.Unread {
		t.Fatalf("want unpinned+unread after UnpinNotification(unread=true), got %+v", n)
	}
	if err := c.MarkAllNotificationsRead(ctx); err != nil {
		t.Fatal(err)
	}
	if s.NotificationByID(1).Unread {
		t.Fatal("unread after mark-all")
	}
}
