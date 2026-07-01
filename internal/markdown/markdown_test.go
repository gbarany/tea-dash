package markdown

import (
	"strings"
	"testing"
)

// TestRenderHeading renders a simple heading without error and returns
// non-empty output containing the heading text.
func TestRenderHeading(t *testing.T) {
	out := Render("# Hi", 80)
	if out == "" {
		t.Fatal("Render(# Hi) returned empty output")
	}
	if !strings.Contains(out, "Hi") {
		t.Errorf("rendered output should contain heading text, got %q", out)
	}
}

// TestRenderEmpty returns "" for empty and whitespace-only bodies.
func TestRenderEmpty(t *testing.T) {
	if got := Render("", 80); got != "" {
		t.Errorf("Render(\"\") = %q, want empty", got)
	}
	if got := Render("   \n\t ", 80); got != "" {
		t.Errorf("Render(whitespace) = %q, want empty", got)
	}
}

// TestRenderStripsHTMLComment removes <!-- ... --> blocks (including
// multi-line) before rendering so their content never surfaces.
func TestRenderStripsHTMLComment(t *testing.T) {
	out := Render("before<!-- secret -->after", 80)
	if strings.Contains(out, "secret") {
		t.Errorf("HTML comment content leaked into output: %q", out)
	}

	// A body that is *only* an HTML comment reduces to empty.
	if got := Render("<!-- only a comment -->", 80); got != "" {
		t.Errorf("comment-only body should render empty, got %q", got)
	}

	// Multi-line comments are stripped too.
	multi := "text\n<!--\nmulti\nline\n-->\nmore"
	out = Render(multi, 80)
	if strings.Contains(out, "multi") || strings.Contains(out, "line") {
		t.Errorf("multi-line comment leaked into output: %q", out)
	}
}
