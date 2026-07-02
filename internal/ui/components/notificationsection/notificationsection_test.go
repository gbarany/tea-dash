package notificationsection

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
	return NewModel(0, ctx, config.SectionConfig{Title: "Notifications"})
}

func TestImplementsSection(t *testing.T) {
	var _ section.Section = (*Model)(nil)
}

func TestFetchedMsgBuildsRows(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("n1")
	next, _ := m.Update(SectionNotificationsFetchedMsg{
		Rows: []data.Notification{{
			ID: 9, Number: 42, SubjectTitle: "Fix notifications",
			RepoNameWithOwner: "gbarany/tea-dash", SubjectType: "Pull",
			SubjectState: "open", Unread: true, UpdatedAt: time.Now().Add(-time.Hour),
		}},
		TotalCount: 1, TaskId: "n1",
	})
	m = next.(*Model)

	if m.GetTotalCount() != 1 || m.NumRows() != 1 {
		t.Fatalf("counts: total=%d rows=%d", m.GetTotalCount(), m.NumRows())
	}
	if m.GetCurrRow() == nil || m.GetCurrRow().GetNumber() != 42 {
		t.Fatalf("GetCurrRow = %+v", m.GetCurrRow())
	}
	row := m.BuildRows()[0]
	joined := strings.Join([]string(row), "|")
	for _, want := range []string{"#42", "Fix notifications", "gbarany/tea-dash", "pull", "unread"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("row %q missing %q", joined, want)
		}
	}
}

func TestFetchedMsgUsesThreadIDWhenSubjectHasNoNumber(t *testing.T) {
	m := newModel(t)
	m.SetLastFetchID("n1")
	next, _ := m.Update(SectionNotificationsFetchedMsg{
		Rows: []data.Notification{{
			ID: 91, SubjectTitle: "Repository notification",
			RepoNameWithOwner: "gbarany/tea-dash", SubjectType: "Repository",
			UpdatedAt: time.Now().Add(-time.Hour),
		}},
		TotalCount: 1, TaskId: "n1",
	})
	m = next.(*Model)

	row := m.BuildRows()[0]
	if !strings.Contains(row[0], "n91") {
		t.Fatalf("number cell = %q, want notification thread id fallback", row[0])
	}
}
