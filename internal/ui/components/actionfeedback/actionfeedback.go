// Package actionfeedback renders compact footer feedback for action results.
package actionfeedback

// Kind identifies the feedback state.
type Kind string

const (
	KindStart   Kind = "start"
	KindSuccess Kind = "success"
	KindError   Kind = "error"
	KindCancel  Kind = "cancel"
)

// Message is the root-owned feedback value rendered by this component.
type Message struct {
	Kind Kind
	Text string
}

func Start(text string) Message   { return Message{Kind: KindStart, Text: text} }
func Success(text string) Message { return Message{Kind: KindSuccess, Text: text} }
func Error(text string) Message   { return Message{Kind: KindError, Text: text} }
func Cancel(text string) Message  { return Message{Kind: KindCancel, Text: text} }

// Model stores the last action feedback message.
type Model struct {
	msg Message
}

func New() Model { return Model{} }

func (m Model) Set(msg Message) Model {
	m.msg = msg
	return m
}

func (m Model) Clear() Model {
	m.msg = Message{}
	return m
}

func (m Model) Empty() bool { return m.msg.Text == "" }

func (m Model) View(width int) string {
	if m.msg.Text == "" {
		return ""
	}
	return fit(m.msg.Text, width)
}

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}
