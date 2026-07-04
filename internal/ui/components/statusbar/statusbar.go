// Package statusbar renders tea-dash's one-row status bar, embedded in the
// shell's bottom border line (spec §1): left is transient action feedback,
// middle is section status counts, right is context-sensitive key hints.
// Like header, it hand-draws the box-art corners/fill and colors them via
// context.Styles.BorderBlurred (border styles are foreground-only by
// design — see context/styles.go).
package statusbar

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

// View renders the status row into exactly w cells. left, middle, and right
// are plain (unstyled) text; the assembled content is styled once with
// styles.StatusBar. Narrow widths drop middle first, then left; right is
// never dropped (it ends in the help/quit hint) and is only hard-truncated,
// from the front so its tail survives, as an absolute last resort.
func View(w int, left, middle, right string, styles context.Styles) string {
	border := styles.BorderBlurred
	leftCorner := border.Render("└")
	rightCorner := border.Render("┘")
	corners := lipgloss.Width(leftCorner) + lipgloss.Width(rightCorner)
	if w < 0 {
		w = 0
	}
	inner := w - corners
	if inner < 0 {
		inner = 0
	}
	// Below 2 there's no room even for the row's outer padding spaces, let
	// alone content; render just the frame. This never happens in
	// production (layout.Compute zeroes StatusBar below 40 columns), but
	// stays exact-width and panic-free regardless.
	if inner < 2 {
		return leftCorner + strings.Repeat(" ", inner) + rightCorner
	}

	l, mid, r := left, middle, right
	if contentWidth(l, mid, r) > inner {
		mid = ""
	}
	if contentWidth(l, mid, r) > inner {
		l = ""
	}
	if contentWidth(l, mid, r) > inner {
		budget := inner - 2 // outer padding spaces
		r = truncateHead(r, budget)
	}

	content := strings.Join(nonEmpty(l, mid, r), " ─ ")
	styled := styles.StatusBar.Render(content)
	pad := inner - 2 - lipgloss.Width(content)
	if pad < 0 {
		pad = 0
	}
	return leftCorner + " " + styled + strings.Repeat(" ", pad) + " " + rightCorner
}

// contentWidth is the cell width of l/mid/r joined with " ─ " separators
// plus the row's outer padding spaces (one on each side of the joined
// content, before the corners).
func contentWidth(l, mid, r string) int {
	parts := nonEmpty(l, mid, r)
	w := 2 // outer padding spaces
	for i, s := range parts {
		if i > 0 {
			w += 3 // " ─ " separator
		}
		w += lipgloss.Width(s)
	}
	return w
}

func nonEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// truncateHead keeps the tail of s — the part that matters (e.g. "? help ·
// q quit") — within budget runes, dropping characters from the front.
func truncateHead(s string, budget int) string {
	if budget <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= budget {
		return s
	}
	return string(r[len(r)-budget:])
}
