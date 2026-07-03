package mockgitea

import (
	"fmt"
	"net/http"
	"sort"
)

// notificationRoutes registers the notification list and read/pin lifecycle
// endpoints internal/gitea/notifications.go consumes: GET /notifications
// (list), PUT /notifications (mark-all-unread-as-read), and PATCH
// /notifications/threads/{id} (read/unread/pin). PinNotification and
// UnpinNotification are thin wrappers around that same PATCH endpoint with a
// different to-status value — Read/PinNotification aren't separate raw-POST
// paths, they all funnel through c.sdk.ReadNotification.
func (s *Server) notificationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/notifications", s.handleListNotifications)
	mux.HandleFunc("PUT /api/v1/notifications", s.handleMarkAllNotificationsRead)
	mux.HandleFunc("PATCH /api/v1/notifications/threads/{id}", s.handleMarkNotification)
}

// handleListNotifications serves GET /notifications. status-types (repeated)
// selects which of pinned/unread/read buckets to include; limit/page page the
// (Updated-descending) result the same way the repo-scoped list endpoints do.
func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	statusTypes := q["status-types"]
	limit, page := paginationParams(q)
	respondList(s, w, func() (rows []map[string]any, total int) {
		matched := filterNotifications(s.store.notificationsLocked(), statusTypes)
		return pageRows(matched, limit, page, notificationRow)
	})
}

// handleMarkAllNotificationsRead serves PUT /notifications. Every real caller
// (internal/gitea's MarkAllNotificationsRead) sends status-types=unread&
// to-status=read, so this always marks every unread thread read rather than
// interpreting the query generically. Gitea's SDK decodes the response into
// []*NotificationThread once the mock's declared server version (>= 1.16.0,
// see GET /version) is in play, so the body must be a JSON array, not empty.
func (s *Server) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	s.store.MarkAllNotificationsRead()
	writeJSON(w, []map[string]any{})
}

// handleMarkNotification serves PATCH /notifications/threads/{id}?to-status=X,
// the single endpoint backing MarkNotificationRead/Unread, PinNotification,
// and UnpinNotification alike (only the to-status value differs). An unknown
// thread id 404s loudly; an unsupported to-status value (none of the real
// client's read/unread/pinned) is a 400, distinct from "unknown resource".
//
// The existence check, the mutation, and the re-fetch-for-response are three
// separate WithLock/self-locking critical sections rather than one — this is
// deliberate, not an oversight. It's what lets "unknown id" (404) and "known
// id, bad status" (400) return different responses without duplicating the
// existence check inside SetNotificationStatus's own error path. It's safe
// here because nothing in this package ever deletes a notification once
// added and --mock never has two goroutines racing a write to the same id,
// so no thread can vanish between the three sections. Tasks 6 and 7 reuse
// this same check-then-mutate-then-respond shape for the same reasons; if a
// future task adds deletion or concurrent same-id writes, this pattern needs
// re-examining (e.g. re-check existence after the mutation, not just before).
func (s *Server) handleMarkNotification(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathInt64(r.PathValue("id"))
	if !ok {
		notFound(w, r)
		return
	}
	var exists bool
	s.store.WithLock(func() {
		exists = s.store.notificationByIDLocked(id) != nil
	})
	if !exists {
		notFound(w, r)
		return
	}

	status := r.URL.Query().Get("to-status")
	if status == "" {
		status = "read"
	}
	// SetNotificationStatus self-locks; it must be called outside WithLock
	// (the mutex is non-reentrant) rather than from inside a locked closure.
	if err := s.store.SetNotificationStatus(id, status); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	respondOr404(s, w, r, func() *map[string]any {
		n := s.store.notificationByIDLocked(id)
		if n == nil {
			return nil
		}
		row := notificationRow(n)
		return &row
	})
}

// filterNotifications keeps notifications whose effective status is one of
// statusTypes, sorted by Updated descending (matching the other list
// endpoints' newest-first convention). A nil/empty statusTypes matches
// nothing — every real caller sends at least one bucket.
func filterNotifications(all []*Notification, statusTypes []string) []*Notification {
	want := make(map[string]bool, len(statusTypes))
	for _, st := range statusTypes {
		want[st] = true
	}
	out := make([]*Notification, 0, len(all))
	for _, n := range all {
		if want[notificationEffectiveStatus(n)] {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

// notificationEffectiveStatus collapses a Notification's independent
// Unread/Pinned fields into Gitea's single NotifyStatus enum (a real
// notification's status is exactly one of pinned/unread/read): pinned takes
// precedence over unread, which takes precedence over read.
func notificationEffectiveStatus(n *Notification) string {
	switch {
	case n.Pinned:
		return "pinned"
	case n.Unread:
		return "unread"
	default:
		return "read"
	}
}

// notificationRow marshals a Notification into the shape internal/gitea's
// mapNotificationThread reads: id/unread/pinned/updated_at at the top level,
// plus nested repository.full_name and subject.{title,type,state,url,
// html_url}. The store keeps a single bookkeeping URL (Notification.URL);
// mapNotificationThread's Number extraction tries subject.url then falls back
// to subject.html_url, so using the same value for both keeps that working
// without needing two separate URL fields on the store type.
func notificationRow(n *Notification) map[string]any {
	return map[string]any{
		"id":         n.ID,
		"unread":     n.Unread,
		"pinned":     n.Pinned,
		"updated_at": n.Updated,
		"url":        fmt.Sprintf("/api/v1/notifications/threads/%d", n.ID),
		"repository": map[string]any{"full_name": n.RepoFull},
		"subject": map[string]any{
			"title":                   n.Title,
			"type":                    n.Type,
			"state":                   n.State,
			"url":                     n.URL,
			"html_url":                n.URL,
			"latest_comment_html_url": "",
		},
	}
}
