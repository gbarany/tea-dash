package context

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDefaultStylesNonZero(t *testing.T) {
	s := DefaultStyles()
	// Spinner/DimText/ErrorText must be usable (Render must not panic).
	_ = s.Spinner.Render("x")
	_ = s.DimText.Render("x")
	_ = s.ErrorText.Render("x")
}

func TestStartTaskRegistersTask(t *testing.T) {
	tasks := map[string]Task{}
	ctx := &ProgramContext{}
	ctx.StartTask = func(tk Task) tea.Cmd { tasks[tk.Id] = tk; return nil }

	cmd := ctx.StartTask(Task{Id: "abc", StartText: "loading"})
	if cmd != nil {
		t.Fatalf("StartTask cmd = %v, want nil in M1a", cmd)
	}
	if _, ok := tasks["abc"]; !ok {
		t.Fatalf("task abc was not registered: %v", tasks)
	}
}

func TestGetViewSectionsConfig(t *testing.T) {
	ctx := &ProgramContext{}
	secs := ctx.GetViewSectionsConfig()
	if len(secs) != 1 || secs[0].Title != "My Pull Requests" {
		t.Fatalf("GetViewSectionsConfig = %+v", secs)
	}
}
