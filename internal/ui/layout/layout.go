// Package layout computes every rectangle of the tea-dash shell from the
// terminal size, and doubles as the mouse hit-testing registry (see
// zones.go). All coordinates are 0-based screen cells; Rect.Contains does
// hit tests.
//
// Geometry modeled (spec §1):
//
//	row 0        Header    (full width, embedded in the top border line)
//	rows 1..H-2  ListPanel [+ PreviewPanel], side by side, each a full
//	             bordered box (own top border row with tabs, own bottom
//	             border row)
//	row H-1      StatusBar (full width, embedded in the bottom border line)
//
// The two panels share the frame width exactly (no outer padding): when the
// preview is open, ListPanel.W + PreviewPanel.W == W.
package layout

// Rect is an axis-aligned, 0-based screen-cell rectangle.
type Rect struct {
	X, Y, W, H int
}

// Contains reports whether (x, y) falls inside r. A zero-value Rect (W==0 or
// H==0) contains nothing.
func (r Rect) Contains(x, y int) bool {
	if r.W <= 0 || r.H <= 0 {
		return false
	}
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Input is the terminal size plus the toggles that shape the shell.
type Input struct {
	Width, Height int
	PreviewOpen   bool
	PreviewWidth  int // 0 = automatic near-50/50 split; >0 = desired PreviewPanel total width (incl. borders)
	SectionCount  int // carried for future tab-rendering consumers; does not affect any rect computed here
	SearchOpen    bool
}

// Layout is every rectangle of the shell for one Input. Fields are the zero
// Rect{} when the corresponding element isn't shown.
type Layout struct {
	TooSmall         bool // below 40x10: everything zero except Full
	PreviewCollapsed bool // auto-collapsed by the 60x15 min-size rule (toggle state preserved by the caller)

	Full Rect // the whole terminal; only meaningful (for centering a notice) when TooSmall

	Header Rect // row 0, embedded in the top border line

	ListPanel    Rect // full bordered panel incl. borders
	ListInterior Rect // content area inside the panel borders: table header + rows
	ListRows     Rect // just the data rows (below the table header, minus the search bar row when open)

	PreviewPanel    Rect // zero when closed/collapsed
	PreviewInterior Rect // zero when closed/collapsed

	PreviewTabsRow int // y of the preview panel's top border (tabs embed there); -1 when closed/collapsed/too-small
	SectionTabsRow int // y of the list panel's top border; -1 when too-small

	StatusBar Rect // last row, embedded in the bottom border line
}

const (
	minWidthTooSmall  = 40
	minHeightTooSmall = 10
	minWidthCollapse  = 60
	minHeightCollapse = 15

	// minListInterior is the floor the list panel's interior width must
	// keep when an explicit PreviewWidth is requested (spec §1).
	minListInterior = 20

	// minPreviewTotal is the floor for an explicit PreviewWidth: 2 border
	// columns + >=10 interior columns, symmetric with minListInterior's
	// floor on the list side. Without it a tiny configured PreviewWidth
	// (e.g. 1) would produce a negative PreviewInterior.
	minPreviewTotal = 12
)

// Compute derives every rect of the shell from in. See the package doc for
// the geometry model.
func Compute(in Input) Layout {
	w, h := in.Width, in.Height

	if w < minWidthTooSmall || h < minHeightTooSmall {
		fw, fh := w, h
		if fw < 0 {
			fw = 0
		}
		if fh < 0 {
			fh = 0
		}
		return Layout{
			TooSmall:         true,
			PreviewCollapsed: true,
			Full:             Rect{X: 0, Y: 0, W: fw, H: fh},
			PreviewTabsRow:   -1,
			SectionTabsRow:   -1,
		}
	}

	l := Layout{
		Full:      Rect{X: 0, Y: 0, W: w, H: h},
		Header:    Rect{X: 0, Y: 0, W: w, H: 1},
		StatusBar: Rect{X: 0, Y: h - 1, W: w, H: 1},
	}

	collapsed := w < minWidthCollapse || h < minHeightCollapse
	l.PreviewCollapsed = collapsed
	showPreview := in.PreviewOpen && !collapsed

	panelsY := 1
	panelsH := h - 2 // rows 1..h-2 inclusive

	if !showPreview {
		l.ListPanel = Rect{X: 0, Y: panelsY, W: w, H: panelsH}
		l.PreviewTabsRow = -1
	} else {
		listW, previewW := splitWidth(w, in.PreviewWidth)
		l.ListPanel = Rect{X: 0, Y: panelsY, W: listW, H: panelsH}
		l.PreviewPanel = Rect{X: listW, Y: panelsY, W: previewW, H: panelsH}
		l.PreviewInterior = interior(l.PreviewPanel)
		l.PreviewTabsRow = l.PreviewPanel.Y
	}
	l.SectionTabsRow = l.ListPanel.Y

	l.ListInterior = interior(l.ListPanel)
	l.ListRows = listRows(l.ListInterior, in.SearchOpen)

	return l
}

// splitWidth divides total width w between the list and preview panels.
// previewWidth == 0 means automatic near-50/50 (list gets the extra column
// on odd w, since it's the primary/default-focused panel). A positive
// previewWidth is honored as the desired PreviewPanel total width (including
// its own borders), floored at minPreviewTotal and clamped so the list panel
// keeps at least minListInterior interior columns. If the two bounds ever
// conflict (pathologically narrow w), the list's minimum wins since that's
// the one the spec states explicitly.
func splitWidth(w, previewWidth int) (listW, previewW int) {
	if previewWidth <= 0 {
		listW = (w + 1) / 2 // ceil
		previewW = w - listW
		return
	}

	maxPreviewW := w - (minListInterior + 2) // list panel needs +2 for its own borders
	previewW = previewWidth
	if previewW < minPreviewTotal {
		previewW = minPreviewTotal
	}
	if previewW > maxPreviewW {
		previewW = maxPreviewW
	}
	listW = w - previewW
	return
}

// interior strips the 1-cell border on every side of a bordered panel rect.
func interior(panel Rect) Rect {
	return Rect{X: panel.X + 1, Y: panel.Y + 1, W: panel.W - 2, H: panel.H - 2}
}

// listRows strips the table-header row (always) and the search-bar row
// (when open) from the top of the list interior.
func listRows(listInterior Rect, searchOpen bool) Rect {
	dy := 1
	if searchOpen {
		dy = 2
	}
	return Rect{X: listInterior.X, Y: listInterior.Y + dy, W: listInterior.W, H: listInterior.H - dy}
}
