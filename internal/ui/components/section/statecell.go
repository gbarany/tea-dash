package section

import (
	"strings"

	"charm.land/lipgloss/v2"

	appctx "github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// StateCell renders a row/CI/notification state word (or a gitea Actions
// "status/conclusion" pair such as "completed/success") as "glyph word",
// colored per styles.StateColors when the classified icons.State has a
// config-backed color. Unread has a glyph but no StateColors entry (T2
// review note: StateColors only covers the 8 config-backed states —
// Unread/AheadArrow/BehindArrow are drawn from explicit styles instead of
// indexing the map on a miss, which would silently render with zero
// style); it is colored via the primary-foreground ActionButton style
// instead. States that don't classify (unknown/empty backend values) are
// returned unstyled and glyph-less, so a column never goes blank on an
// unrecognized value — exported (not the plan sketch's lowercase
// "stateCell") because pullsection/issuesection/notificationsection/
// actionsection all call it from outside this package, the same way they
// already call the exported HumanizeTime.
func StateCell(state string, set icons.Set, styles appctx.Styles) string {
	st, ok := ClassifyState(state)
	if !ok {
		return state
	}
	if st == icons.Unread {
		return GlyphText(set, st, state, styles.ActionButton)
	}
	if style, ok := styles.StateColors[st]; ok {
		return GlyphText(set, st, state, style)
	}
	return icons.Glyph(set, st) + " " + state
}

// GlyphText renders text prefixed with state's glyph under an explicit
// style — for the state kinds styles.StateColors has no entry for (branch
// ahead/behind arrows, the notification unread dot), rather than indexing
// the map on a guaranteed miss.
func GlyphText(set icons.Set, state icons.State, text string, style lipgloss.Style) string {
	return style.Render(icons.Glyph(set, state) + " " + text)
}

// ClassifyState maps a state word (or a "status/conclusion" pair joined by
// "/", e.g. gitea Actions' "completed/success") to its icons.State. Composite
// values are split on "/" and classified right-to-left — the rightmost token
// (the conclusion, when Actions reports "status/conclusion") is the more
// specific/authoritative signal, mirroring prview's existing
// conclusion-then-status precedence. ok is false when no token classifies
// (an unknown/empty backend value), so callers can leave the cell plain.
func ClassifyState(state string) (icons.State, bool) {
	tokens := strings.Split(state, "/")
	for i := len(tokens) - 1; i >= 0; i-- {
		if st, ok := classifyStateWord(tokens[i]); ok {
			return st, true
		}
	}
	return 0, false
}

func classifyStateWord(word string) (icons.State, bool) {
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "open":
		return icons.Open, true
	case "draft":
		return icons.Draft, true
	case "merged":
		return icons.Merged, true
	case "closed":
		return icons.Closed, true
	case "success":
		return icons.Success, true
	case "failure", "cancelled", "timed_out", "error":
		return icons.Failure, true
	case "running", "in_progress", "queued", "waiting", "pending", "requested":
		return icons.Running, true
	case "completed", "skipped", "neutral":
		return icons.Neutral, true
	case "unread":
		return icons.Unread, true
	case "read", "pinned":
		return icons.Neutral, true
	default:
		return 0, false
	}
}
