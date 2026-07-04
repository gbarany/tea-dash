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

// TestFilterNotificationsByStatusType is a discriminating test for the
// read-side status-types filter: each bucket must exclude the others, not
// just "include the right one" (a filter that matched everything would also
// pass a same-bucket-only assertion).
func TestFilterNotificationsByStatusType(t *testing.T) {
	all := []*Notification{
		{ID: 1, Unread: true, Title: "unread one"},
		{ID: 2, Pinned: true, Title: "pinned one"},
		{ID: 3, Title: "read one"}, // Unread: false, Pinned: false -> "read"
	}

	pinned := filterNotifications(all, []string{"pinned"})
	if len(pinned) != 1 || pinned[0].ID != 2 {
		t.Fatalf("status-types=[pinned] = %+v, want only #2", pinned)
	}

	unread := filterNotifications(all, []string{"unread"})
	if len(unread) != 1 || unread[0].ID != 1 {
		t.Fatalf("status-types=[unread] = %+v, want only #1 (pinned/read excluded)", unread)
	}

	read := filterNotifications(all, []string{"read"})
	if len(read) != 1 || read[0].ID != 3 {
		t.Fatalf("status-types=[read] = %+v, want only #3 (pinned/unread excluded)", read)
	}
}

// TestSetNotificationStatusWriteReadRoundTrip pins that the write side
// (SetNotificationStatus) and read side (notificationEffectiveStatus) agree
// on the same three-bucket encoding: writing status X and then computing the
// effective status of the result must yield X again, for every X.
func TestSetNotificationStatusWriteReadRoundTrip(t *testing.T) {
	for _, status := range []string{"read", "unread", "pinned"} {
		t.Run(status, func(t *testing.T) {
			s := NewStore()
			s.AddNotification(&Notification{ID: 42})
			if err := s.SetNotificationStatus(42, status); err != nil {
				t.Fatal(err)
			}
			n := s.NotificationByID(42)
			if got := notificationEffectiveStatus(n); got != status {
				t.Fatalf("wrote status %q, effective status = %q (n=%+v)", status, got, n)
			}
		})
	}
}
