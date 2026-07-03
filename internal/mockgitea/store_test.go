package mockgitea

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestMergePullFlipsState(t *testing.T) {
	s := NewStore()
	s.AddRepo(&Repo{FullName: "teahouse/kettle", Name: "kettle", Owner: &User{Login: "teahouse"}})
	s.AddPull(&Pull{Number: 1, RepoFullName: "teahouse/kettle", Title: "fix", State: "open"})
	if err := s.MergePull("teahouse/kettle", 1, "merge", false); err != nil {
		t.Fatalf("MergePull: %v", err)
	}
	p := s.Pull("teahouse/kettle", 1)
	if p == nil || p.State != "closed" || !p.Merged {
		t.Fatalf("want merged closed pull, got %+v", p)
	}
}

func TestAddCommentAppendsAndBumpsCount(t *testing.T) {
	s := NewStore()
	s.AddRepo(&Repo{FullName: "teahouse/kettle", Name: "kettle", Owner: &User{Login: "teahouse"}})
	s.AddIssue(&Issue{Number: 2, RepoFullName: "teahouse/kettle", Title: "bug", State: "open"})
	c := s.AddComment("teahouse/kettle", 2, "gabor", "on it")
	if c.ID == 0 || len(s.Comments("teahouse/kettle", 2)) != 1 {
		t.Fatalf("comment not stored: %+v", c)
	}
	if issue := s.Issue("teahouse/kettle", 2); issue == nil || issue.CommentCount != 1 {
		t.Fatalf("want issue comment count 1, got %+v", issue)
	}
}

// TestAddIssueInitializesSubscribers guards against a nil-map panic in the
// (later) subscription handler: AddIssue must hand back an Issue whose
// Subscribers map is always writable.
func TestAddIssueInitializesSubscribers(t *testing.T) {
	s := NewStore()
	s.AddRepo(&Repo{FullName: "teahouse/kettle", Name: "kettle", Owner: &User{Login: "teahouse"}})
	s.AddIssue(&Issue{Number: 3, RepoFullName: "teahouse/kettle", Title: "bug", State: "open"})

	issue := s.Issue("teahouse/kettle", 3)
	if issue == nil {
		t.Fatal("issue not stored")
	}
	issue.Subscribers["x"] = true // must not panic on a nil map
	if !issue.Subscribers["x"] {
		t.Fatal("subscriber not recorded")
	}
}

// TestNotificationReadPin exercises the full read/unread/pinned lifecycle,
// including unpinning: Gitea's NotifyStatus is a single enum with no distinct
// "unpinned" value, so a real client unpins by sending to-status=unread or
// to-status=read (UnpinNotification), which must clear Pinned as a side
// effect of that full status overwrite.
func TestNotificationReadPin(t *testing.T) {
	s := NewStore()
	s.AddNotification(&Notification{ID: 7, Unread: true})
	if err := s.SetNotificationStatus(7, "read"); err != nil {
		t.Fatal(err)
	}
	if n := s.NotificationByID(7); n.Unread {
		t.Fatal("still unread")
	}
	if err := s.SetNotificationStatus(7, "pinned"); err != nil {
		t.Fatal(err)
	}
	if n := s.NotificationByID(7); !n.Pinned {
		t.Fatal("not pinned")
	}

	// Unpin via "unread" (UnpinNotification(id, unread=true)): Pinned clears
	// and the unread state is restored.
	if err := s.SetNotificationStatus(7, "unread"); err != nil {
		t.Fatal(err)
	}
	if n := s.NotificationByID(7); n.Pinned || !n.Unread {
		t.Fatalf("want unpinned+unread after to-status=unread, got %+v", n)
	}

	// Pin again, then unpin via "read" (UnpinNotification(id, unread=false)).
	if err := s.SetNotificationStatus(7, "pinned"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotificationStatus(7, "read"); err != nil {
		t.Fatal(err)
	}
	if n := s.NotificationByID(7); n.Pinned || n.Unread {
		t.Fatalf("want unpinned+read after to-status=read, got %+v", n)
	}
}

func TestMergePullUnknownPullErrors(t *testing.T) {
	s := NewStore()
	s.AddRepo(&Repo{FullName: "teahouse/kettle", Name: "kettle", Owner: &User{Login: "teahouse"}})
	if err := s.MergePull("teahouse/kettle", 404, "merge", false); err == nil {
		t.Fatal("want error merging unknown pull, got nil")
	}
}

func TestSetNotificationStatusUnknownIDErrors(t *testing.T) {
	s := NewStore()
	if err := s.SetNotificationStatus(404, "read"); err == nil {
		t.Fatal("want error for unknown notification id, got nil")
	}
}

func TestSetNotificationStatusUnknownStatusErrors(t *testing.T) {
	s := NewStore()
	s.AddNotification(&Notification{ID: 8, Unread: true})
	if err := s.SetNotificationStatus(8, "bogus"); err == nil {
		t.Fatal("want error for unknown status, got nil")
	}
}

// TestAllPullsAndAllIssuesSortedByID guards the ordering guarantee later
// determinism tests rely on (identically-seeded stores must compare equal
// element-by-element; Go map iteration order is randomized).
func TestAllPullsAndAllIssuesSortedByID(t *testing.T) {
	s := NewStore()
	s.AddRepo(&Repo{FullName: "teahouse/kettle", Name: "kettle", Owner: &User{Login: "teahouse"}})
	s.AddRepo(&Repo{FullName: "teahouse/pot", Name: "pot", Owner: &User{Login: "teahouse"}})

	// IDs are assigned in call order, but stash across two repos so map
	// iteration order (which is randomized) would shuffle an unsorted result.
	s.AddPull(&Pull{ID: 3, Number: 1, RepoFullName: "teahouse/pot"})
	s.AddPull(&Pull{ID: 1, Number: 1, RepoFullName: "teahouse/kettle"})
	s.AddPull(&Pull{ID: 2, Number: 2, RepoFullName: "teahouse/kettle"})

	s.AddIssue(&Issue{ID: 6, Number: 1, RepoFullName: "teahouse/pot"})
	s.AddIssue(&Issue{ID: 4, Number: 1, RepoFullName: "teahouse/kettle"})
	s.AddIssue(&Issue{ID: 5, Number: 2, RepoFullName: "teahouse/kettle"})

	pulls := s.AllPulls()
	for i := 1; i < len(pulls); i++ {
		if pulls[i-1].ID > pulls[i].ID {
			t.Fatalf("AllPulls not sorted by ID: %+v", pulls)
		}
	}

	issues := s.AllIssues()
	for i := 1; i < len(issues); i++ {
		if issues[i-1].ID > issues[i].ID {
			t.Fatalf("AllIssues not sorted by ID: %+v", issues)
		}
	}
}

// TestConcurrentMarshalWithLock is the regression test for the confirmed data
// race: a writer goroutine mutates a pull in place while a reader goroutine
// marshals it. Wrapping the read+marshal in WithLock (and reading via the
// lock-assuming pullLocked, not the self-locking Pull) must make this race
// free under -race.
func TestConcurrentMarshalWithLock(t *testing.T) {
	s := NewStore()
	s.AddRepo(&Repo{FullName: "teahouse/kettle", Name: "kettle", Owner: &User{Login: "teahouse"}})
	s.AddPull(&Pull{Number: 1, RepoFullName: "teahouse/kettle", Title: "fix", State: "open"})

	const iterations = 300
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = s.SetPullState("teahouse/kettle", 1, "open")
			_ = s.MergePull("teahouse/kettle", 1, "merge", false)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			s.WithLock(func() {
				p := s.pullLocked("teahouse/kettle", 1)
				if p == nil {
					return
				}
				if _, err := json.Marshal(p); err != nil {
					t.Errorf("marshal pull: %v", err)
				}
			})
		}
	}()

	wg.Wait()
}
