package actionprompt

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func testKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func TestConfirmSubmitAndCancel(t *testing.T) {
	m := New()
	m = m.Focus(Config{Mode: ModeConfirm, Title: "Merge", Message: "Merge #42?"})
	if !m.Active() {
		t.Fatal("prompt should be active after Focus")
	}

	var result Result
	m, result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Submitted || result.Canceled {
		t.Fatalf("enter should submit confirm prompt: %+v", result)
	}
	if result.Value != "confirm" {
		t.Fatalf("confirm value = %q, want confirm", result.Value)
	}
	if m.Active() {
		t.Fatal("prompt should close after submit")
	}

	m = New().Focus(Config{Mode: ModeConfirm, Title: "Close", Message: "Close #42?"})
	m, result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !result.Canceled || result.Submitted {
		t.Fatalf("esc should cancel confirm prompt: %+v", result)
	}
	if m.Active() {
		t.Fatal("prompt should close after cancel")
	}
}

func TestTextSubmitAndCancel(t *testing.T) {
	m := New().Focus(Config{Mode: ModeText, Title: "Comment", Placeholder: "Body"})
	for _, r := range "ship it" {
		var result Result
		m, result, _ = m.Update(testKey(r))
		if result.Submitted || result.Canceled {
			t.Fatalf("typing %q should not finish prompt: %+v", r, result)
		}
	}

	var result Result
	m, result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Submitted || result.Value != "ship it" {
		t.Fatalf("enter should submit typed text, got %+v", result)
	}
	if m.Active() {
		t.Fatal("prompt should close after text submit")
	}

	m = New().Focus(Config{Mode: ModeText, Title: "Comment"})
	m, _, _ = m.Update(testKey('x'))
	m, result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !result.Canceled || result.Value != "" {
		t.Fatalf("esc should cancel text prompt without payload: %+v", result)
	}
	if m.Active() {
		t.Fatal("prompt should close after text cancel")
	}
}

func TestPickerSubmitAndCancel(t *testing.T) {
	cfg := Config{
		Mode:  ModePicker,
		Title: "Review",
		Options: []Option{
			{Label: "Comment", Value: "comment"},
			{Label: "Approve", Value: "approve"},
		},
	}
	m := New().Focus(cfg)
	m, _, _ = m.Update(testKey('j'))

	var result Result
	m, result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Submitted || result.Value != "approve" || result.Label != "Approve" {
		t.Fatalf("enter should submit selected picker option, got %+v", result)
	}
	if m.Active() {
		t.Fatal("prompt should close after picker submit")
	}

	m = New().Focus(cfg)
	m, result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !result.Canceled || result.Submitted {
		t.Fatalf("esc should cancel picker prompt: %+v", result)
	}
}

func TestMultiPickerTogglesAndSubmitsCommaSeparatedValues(t *testing.T) {
	cfg := Config{
		Mode:  ModeMultiPicker,
		Title: "Request reviewers",
		Options: []Option{
			{Label: "Alice A. (alice)", Value: "alice"},
			{Label: "Bob B. (bob)", Value: "bob"},
			{Label: "Carol C. (carol)", Value: "carol"},
		},
	}
	m := New().Focus(cfg)
	m, _, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m, _, _ = m.Update(testKey('j'))
	m, _, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})

	view := m.View(120)
	for _, want := range []string{"[x] Alice A. (alice)", "[x] Bob B. (bob)", "[ ] Carol C. (carol)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("multi-picker view missing %q:\n%s", want, view)
		}
	}

	var result Result
	m, result, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !result.Submitted || result.Value != "alice,bob" || result.Label != "Alice A. (alice), Bob B. (bob)" {
		t.Fatalf("multi-picker submit = %+v, want alice/bob values and labels", result)
	}
	if m.Active() {
		t.Fatal("prompt should close after multi-picker submit")
	}
}

func TestSmallWidthRender(t *testing.T) {
	m := New().Focus(Config{
		Mode:        ModeText,
		Title:       "Comment on a long pull request title",
		Message:     "This footer should stay within the available terminal width.",
		Placeholder: "Write a useful comment",
	})

	view := m.View(12)
	if view == "" {
		t.Fatal("active prompt should render")
	}
	for _, line := range strings.Split(view, "\n") {
		if len(line) > 12 {
			t.Fatalf("line %q is wider than 12 columns in:\n%s", line, view)
		}
	}
	if !strings.Contains(view, "...") {
		t.Fatalf("small-width render should truncate long text with ..., got:\n%s", view)
	}
}
