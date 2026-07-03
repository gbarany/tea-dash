package mockgitea

import "testing"

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
}

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
}
