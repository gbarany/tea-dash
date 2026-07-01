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
}
