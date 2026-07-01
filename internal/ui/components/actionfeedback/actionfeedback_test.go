package actionfeedback

import (
	"strings"
	"testing"
)

func TestFeedbackRendersKinds(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want string
	}{
		{name: "start", msg: Start("Starting merge"), want: "Starting merge"},
		{name: "success", msg: Success("Merged"), want: "Merged"},
		{name: "error", msg: Error("Merge failed"), want: "Merge failed"},
		{name: "cancel", msg: Cancel("Action cancelled"), want: "Action cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := New().Set(tt.msg).View(80)
			if !strings.Contains(view, tt.want) {
				t.Fatalf("feedback view missing %q:\n%s", tt.want, view)
			}
		})
	}
}

func TestFeedbackSmallWidthRender(t *testing.T) {
	view := New().Set(Error("This message is too long for the footer")).View(10)
	if view == "" {
		t.Fatal("feedback with a message should render")
	}
	for _, line := range strings.Split(view, "\n") {
		if len(line) > 10 {
			t.Fatalf("line %q is wider than 10 columns in:\n%s", line, view)
		}
	}
	if !strings.Contains(view, "...") {
		t.Fatalf("small-width render should truncate with ..., got:\n%s", view)
	}
}
