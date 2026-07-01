package section

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/table"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/ui/context"
)

func newBase(t *testing.T) *BaseModel {
	t.Helper()
	ctx := &context.ProgramContext{Styles: context.DefaultStyles()}
	b := NewBaseModel(NewOptions{
		Id:          0,
		Type:        "pr",
		Ctx:         ctx,
		Config:      config.SectionConfig{Title: "My Pull Requests"},
		Columns:     []table.Column{{Title: "#", Width: 6}, {Title: "Title", Width: 20}},
		LoadingText: "Loading pull requests…",
		EmptyText:   "No open pull requests authored by you.",
		EmptyHint:   "This board shows PRs you created across all repos on your Gitea instance.",
	})
	return &b
}

func TestBaseModelMetadata(t *testing.T) {
	b := newBase(t)
	if b.GetId() != 0 || b.GetType() != "pr" || b.GetTitle() != "My Pull Requests" {
		t.Fatalf("metadata wrong: id=%d type=%q title=%q", b.GetId(), b.GetType(), b.GetTitle())
	}
}

func TestBaseModelViewStates(t *testing.T) {
	b := newBase(t)

	b.SetIsLoading(true)
	if !strings.Contains(b.View(), "Loading pull requests") {
		t.Fatalf("loading view: %q", b.View())
	}

	b.SetIsLoading(false)
	b.SetError(errors.New("boom"))
	v := b.View()
	if !strings.Contains(v, "Error") || !strings.Contains(v, "boom") {
		t.Fatalf("error view: %q", v)
	}

	b.SetError(nil)
	b.SetRows(nil) // zero rows -> empty state
	if !strings.Contains(b.View(), "No open pull requests") {
		t.Fatalf("empty view: %q", b.View())
	}
}
