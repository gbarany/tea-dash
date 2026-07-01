package context

import (
	"testing"
)

func TestDefaultStylesNonZero(t *testing.T) {
	s := DefaultStyles()
	// Spinner/DimText/ErrorText must be usable (Render must not panic).
	_ = s.Spinner.Render("x")
	_ = s.DimText.Render("x")
	_ = s.ErrorText.Render("x")
}

func TestGetViewSectionsConfig(t *testing.T) {
	ctx := &ProgramContext{}
	secs := ctx.GetViewSectionsConfig()
	if len(secs) != 1 || secs[0].Title != "My Pull Requests" {
		t.Fatalf("GetViewSectionsConfig = %+v", secs)
	}
	// The default PR section is filter-driven now: me-scoped, open state.
	if secs[0].Filter.CreatedBy != "@me" || secs[0].Filter.State != "open" {
		t.Fatalf("default section filter = %+v, want CreatedBy=@me State=open", secs[0].Filter)
	}
}

func TestGetViewSectionsConfigIssues(t *testing.T) {
	ctx := &ProgramContext{View: IssuesView}
	secs := ctx.GetViewSectionsConfig()
	if len(secs) != 1 || secs[0].Title != "My Issues" {
		t.Fatalf("GetViewSectionsConfig(IssuesView) = %+v", secs)
	}
	if secs[0].Filter.CreatedBy != "@me" || secs[0].Filter.State != "open" {
		t.Fatalf("default issues filter = %+v, want CreatedBy=@me State=open", secs[0].Filter)
	}
}
