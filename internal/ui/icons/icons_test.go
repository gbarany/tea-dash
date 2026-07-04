package icons

import "testing"

var allSets = []Set{Unicode, Nerd, ASCII}

var allStates = []State{
	Open, Draft, Merged, Closed, Success, Failure, Running, Neutral,
	Unread, AheadArrow, BehindArrow,
}

func TestGlyphNonEmptyForEverySetAndState(t *testing.T) {
	for _, set := range allSets {
		for _, state := range allStates {
			if g := Glyph(set, state); g == "" {
				t.Fatalf("Glyph(%v, %v) = %q, want non-empty", set, state, g)
			}
		}
	}
}

func TestASCIIGlyphsArePureASCII(t *testing.T) {
	for _, state := range allStates {
		g := Glyph(ASCII, state)
		for _, r := range g {
			if r >= 128 {
				t.Fatalf("Glyph(ASCII, %v) = %q contains non-ASCII rune %q", state, g, r)
			}
		}
	}
}

func TestUnicodeAndASCIIDifferForOpenAndSuccess(t *testing.T) {
	if Glyph(Unicode, Open) == Glyph(ASCII, Open) {
		t.Fatalf("Glyph(Unicode, Open) should differ from Glyph(ASCII, Open); both = %q", Glyph(Unicode, Open))
	}
	if Glyph(Unicode, Success) == Glyph(ASCII, Success) {
		t.Fatalf("Glyph(Unicode, Success) should differ from Glyph(ASCII, Success); both = %q", Glyph(Unicode, Success))
	}
}

func TestUnicodeMergedAndClosedAreDistinct(t *testing.T) {
	if Glyph(Unicode, Merged) == Glyph(Unicode, Closed) {
		t.Fatalf("Glyph(Unicode, Merged) and Glyph(Unicode, Closed) should be distinct; both = %q", Glyph(Unicode, Merged))
	}
}

func TestParseDefaultsToUnicode(t *testing.T) {
	for _, in := range []string{"", "unicode", "UNICODE", "Unicode", "bogus", "  "} {
		if got := Parse(in); got != Unicode {
			t.Fatalf("Parse(%q) = %v, want Unicode", in, got)
		}
	}
}

func TestParseIsCaseInsensitive(t *testing.T) {
	cases := map[string]Set{
		"nerd":  Nerd,
		"NERD":  Nerd,
		"Nerd":  Nerd,
		"ascii": ASCII,
		"ASCII": ASCII,
		"AsCiI": ASCII,
	}
	for in, want := range cases {
		if got := Parse(in); got != want {
			t.Fatalf("Parse(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseTrimsWhitespace(t *testing.T) {
	if got := Parse("  nerd  "); got != Nerd {
		t.Fatalf("Parse(%q) = %v, want Nerd", "  nerd  ", got)
	}
}
