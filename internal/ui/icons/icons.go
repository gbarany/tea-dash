// Package icons maps row/CI/notification states to glyphs for the configured
// icon set. unicode is the default (safe in modern monospace fonts); nerd is
// opt-in and requires a Nerd Font installed in the terminal; ascii is the
// lowest common denominator for fonts/terminals with no Unicode glyph
// coverage at all.
package icons

import "strings"

// Set selects which glyph family Glyph draws from.
type Set int

const (
	// Unicode is the default set: plain Unicode symbols available in any
	// modern monospace font, no special font required.
	Unicode Set = iota
	// Nerd uses Nerd Font glyphs (Private Use Area codepoints). Opt-in via
	// theme.icons: nerd; requires a patched ("Nerd Font") terminal font.
	Nerd
	// ASCII uses only 7-bit ASCII characters, for terminals/fonts with no
	// Unicode glyph coverage.
	ASCII
)

// Parse maps a theme.icons config string to a Set. An empty or unrecognized
// value defaults to Unicode. Matching is case-insensitive and trims
// surrounding whitespace.
func Parse(s string) Set {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "nerd":
		return Nerd
	case "ascii":
		return ASCII
	default:
		return Unicode
	}
}

// State enumerates the row/CI/notification states tea-dash draws a glyph for.
type State int

const (
	Open State = iota
	Draft
	Merged
	Closed
	Success
	Failure
	Running
	Neutral
	Unread
	AheadArrow
	BehindArrow
)

// unicodeGlyphs are safe in any modern monospace font.
var unicodeGlyphs = map[State]string{
	Open:        "●", // U+25CF BLACK CIRCLE
	Draft:       "○", // U+25CB WHITE CIRCLE
	Merged:      "⇥", // U+21E5 RIGHTWARDS ARROW TO BAR
	Closed:      "✗", // U+2717 BALLOT X
	Success:     "✓", // U+2713 CHECK MARK
	Failure:     "✗", // U+2717 BALLOT X
	Running:     "◐", // U+25D0 CIRCLE WITH LEFT HALF BLACK
	Neutral:     "·", // U+00B7 MIDDLE DOT
	Unread:      "●", // U+25CF BLACK CIRCLE
	AheadArrow:  "↑", // U+2191 UPWARDS ARROW
	BehindArrow: "↓", // U+2193 DOWNWARDS ARROW
}

// nerdGlyphs use Nerd Font (Private Use Area) codepoints. Each is documented
// by its Nerd Fonts cheat-sheet name (https://www.nerdfonts.com/cheat-sheet)
// and exact codepoint in a trailing comment, since the glyphs themselves
// render as blank/tofu without a Nerd Font installed in the viewing
// terminal or editor font. A future font-mapping fix is a one-line lookup
// against that name rather than a re-derivation.
var nerdGlyphs = map[State]string{
	Open:        "", // U+F407 nf-oct-git_pull_request
	Draft:       "", // U+F418 nf-oct-git_branch
	Merged:      "", // U+F419 nf-oct-git_merge
	Closed:      "", // U+F057 nf-fa-times_circle
	Success:     "", // U+F00C nf-fa-check
	Failure:     "", // U+F00D nf-fa-times
	Running:     "", // U+F1CE nf-fa-circle_o_notch (spinner)
	Neutral:     "", // U+F10C nf-fa-circle_o
	Unread:      "", // U+F111 nf-fa-circle
	AheadArrow:  "", // U+F062 nf-fa-arrow_up
	BehindArrow: "", // U+F063 nf-fa-arrow_down
}

// asciiGlyphs use only 7-bit ASCII, adjusted from the raw */o/>/x/+/x/~/./*/^/v
// suggestion for readability while keeping every value distinct enough to
// tell states apart in a plain terminal.
var asciiGlyphs = map[State]string{
	Open:        "*",
	Draft:       "o",
	Merged:      ">",
	Closed:      "x",
	Success:     "+",
	Failure:     "x",
	Running:     "~",
	Neutral:     ".",
	Unread:      "*",
	AheadArrow:  "^",
	BehindArrow: "v",
}

// Glyph returns the glyph for state in the given set. Every (set, state) pair
// is defined; an unrecognized state returns "" (there are no such states
// today, this is just defensive).
func Glyph(set Set, s State) string {
	switch set {
	case Nerd:
		return nerdGlyphs[s]
	case ASCII:
		return asciiGlyphs[s]
	default:
		return unicodeGlyphs[s]
	}
}
