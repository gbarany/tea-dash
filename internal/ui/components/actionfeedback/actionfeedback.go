// Package actionfeedback renders tea-dash's single transient-feedback
// mechanism: the status bar's left-segment "toast" (spec §4 "Toasts +
// inline progress"). It replaced the root's separate `notice` string field
// (Task 8) — every transient status message, whether a validation nudge, an
// async failure, a success confirmation, or an in-flight indicator, now
// flows through this one component so there's one place on screen and one
// code path for "tell the user something just happened."
package actionfeedback

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// Kind identifies the feedback state, which drives its icon, its color, and
// its dismissal rule (see expires and DismissError).
type Kind string

const (
	KindStart   Kind = "start"
	KindSuccess Kind = "success"
	KindError   Kind = "error"
	KindCancel  Kind = "cancel"
	KindInfo    Kind = "info"
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
func Info(text string) Message    { return Message{Kind: KindInfo, Text: text} }

// DefaultExpiry is how long a Success/Info/Cancel toast stays up before
// auto-expiring (spec §4: "~4s"). Error never auto-expires (persists until
// a keypress, see DismissError); Start is always superseded by whatever
// terminal state follows the action it announces.
const DefaultExpiry = 4 * time.Second

// ExpireMsg is delivered by the tea.Cmd Set returns, after its expiry
// duration elapses. Gen must match the Model's current generation (bumped
// by every Set) for Expire to actually clear the message — see Expire's
// doc comment for why a stale tick must be a no-op.
type ExpireMsg struct{ Gen int }

// Model stores the current feedback message plus the bookkeeping needed to
// auto-expire it safely: gen increments on every Set, so a tea.Tick
// scheduled by an earlier Set (and not yet delivered) can recognize itself
// as stale once a newer Set (or Clear) has superseded it.
type Model struct {
	msg Message
	gen int

	// expiry overrides DefaultExpiry; zero means "use DefaultExpiry". Only
	// tests set this (via WithExpiry) to exercise the real tea.Tick-driven
	// path without waiting the full production duration.
	expiry time.Duration
}

func New() Model { return Model{} }

// WithExpiry overrides the auto-expire duration used by future Set calls.
// Production code never calls this (New's zero value means DefaultExpiry);
// it exists so tests — in particular the e2e merge round-trip, which drains
// the real tea.Cmd chain rather than injecting synthetic messages — don't
// have to block for a real 4 seconds to see a toast expire.
func (m Model) WithExpiry(d time.Duration) Model {
	m.expiry = d
	return m
}

func (m Model) expiryDuration() time.Duration {
	if m.expiry > 0 {
		return m.expiry
	}
	return DefaultExpiry
}

// expires reports whether a Kind auto-dismisses on a timer. Success/Info/
// Cancel are "final, non-error" states — they've said their piece and
// should clear themselves. Error persists until the user acknowledges it
// (DismissError); Start is mid-action and gets replaced by whatever
// terminal state follows, never expiring on its own.
func expires(k Kind) bool {
	switch k {
	case KindSuccess, KindInfo, KindCancel:
		return true
	default:
		return false
	}
}

// Set replaces the current message and bumps the generation counter. For
// auto-expiring kinds it returns a tea.Cmd that delivers an ExpireMsg after
// the expiry duration — the CALLER MUST return or tea.Batch this cmd into
// whatever it hands back to the bubbletea runtime, or the toast will never
// expire (see app.go's setInfo/setError/setSuccess/setStart helpers, which
// exist specifically so call sites can't forget this). Non-expiring kinds
// (Error, Start) return a nil cmd.
func (m Model) Set(msg Message) (Model, tea.Cmd) {
	m.gen++
	m.msg = msg
	if !expires(msg.Kind) {
		return m, nil
	}
	gen := m.gen
	d := m.expiryDuration()
	return m, tea.Tick(d, func(time.Time) tea.Msg {
		return ExpireMsg{Gen: gen}
	})
}

// Expire clears the message if gen matches the generation Set scheduled it
// under. A mismatch means a newer Set (or another Expire) has already
// superseded the message this tick was scheduled for — e.g. Set(Success)
// (gen 1) immediately followed by Set(Info) (gen 2) before the first tick
// fires: that stale gen-1 tick must not clear the still-fresh gen-2
// message. This is the only reason gen exists.
func (m Model) Expire(gen int) Model {
	if gen != m.gen {
		return m
	}
	m.msg = Message{}
	return m
}

// DismissError clears the message only if it's currently an Error — the
// "any key dismisses" UX (spec §6: "action errors are red toasts that
// persist until a keypress") now scoped to just errors, since
// Success/Info/Cancel already dismiss themselves via Expire and Start is
// superseded by the action's own terminal state. Called unconditionally
// from Update's tea.KeyPressMsg case, so it must be a no-op for every other
// kind (a keypress must not eat an in-flight or not-yet-expired toast).
func (m Model) DismissError() Model {
	if m.msg.Kind == KindError {
		m.msg = Message{}
	}
	return m
}

// Clear resets to no message, without touching gen — used at call sites
// that are replacing an in-progress toast (e.g. "Loading reviewers…") with
// a state change that isn't itself a new toast (opening a picker). A stale
// tick scheduled before the Clear is harmless: it'll find nothing to clear.
func (m Model) Clear() Model {
	m.msg = Message{}
	return m
}

func (m Model) Empty() bool { return m.msg.Text == "" }

// glyphs maps each Kind to the icons.State whose glyph best represents it.
// Kept as a switch (not a map indexed by Kind) since Kind is a handful of
// known constants — a map buys nothing here.
func glyphState(k Kind) icons.State {
	switch k {
	case KindSuccess:
		return icons.Success
	case KindError:
		return icons.Failure
	case KindStart:
		// Static glyph, not an animated bubbles/spinner frame: actionfeedback
		// has no Update/ticking loop of its own today, and wiring one in just
		// for the in-flight toast is more machinery than a status-bar icon
		// warrants right now. icons.Running's "half-full circle" glyph reads
		// as "in progress" at a glance; a real animated spinner is a
		// low-risk future upgrade if this ever feels static in practice.
		return icons.Running
	default: // KindInfo, KindCancel
		return icons.Neutral
	}
}

// style maps each Kind to its status-bar style. Success/Error/Info have
// dedicated StatusToast* styles (context/styles.go, from Task 2);
// Cancel and Start reuse StatusToastInfo — neither is a success or a
// failure, and a fourth/fifth dedicated style for two low-emphasis states
// isn't worth the theme-config surface it would add.
func style(k Kind, styles context.Styles) lipgloss.Style {
	switch k {
	case KindSuccess:
		return styles.StatusToastSuccess
	case KindError:
		return styles.StatusToastError
	default: // KindInfo, KindCancel, KindStart
		return styles.StatusToastInfo
	}
}

// View renders the current message for the status bar's left segment: icon
// (from set) + text, styled by Kind, fit to width. Returns "" when there's
// no message, so the caller (statusbar.View) renders nothing rather than an
// empty styled segment. width bounds the rendered cell width (measured with
// lipgloss.Width, not len, since glyphs are multi-byte UTF-8 but
// single-column).
func (m Model) View(width int, styles context.Styles, set icons.Set) string {
	if m.msg.Text == "" {
		return ""
	}
	text := m.msg.Text
	if g := icons.Glyph(set, glyphState(m.msg.Kind)); g != "" {
		text = g + " " + text
	}
	return style(m.msg.Kind, styles).Render(fit(text, width))
}

// fit truncates s to width terminal columns (lipgloss.Width, not byte
// length — icon glyphs and any future non-ASCII message text are multi-byte
// UTF-8 but single-column), appending "..." when truncated.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 3 {
		r := []rune(s)
		if len(r) > width {
			r = r[:width]
		}
		return string(r)
	}
	r := []rune(s)
	// Trim rune-by-rune (not byte slicing) until the ellipsis-suffixed
	// result fits — a single loop rather than assuming len(r) == the
	// display width, which only holds for the ASCII-only inputs this
	// function used to see before icon glyphs were added.
	for len(r) > 0 && lipgloss.Width(string(r)+"...") > width {
		r = r[:len(r)-1]
	}
	return string(r) + "..."
}
