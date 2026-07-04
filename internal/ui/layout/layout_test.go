package layout

import "testing"

// Geometry model under test (derived from spec §1 "Layout" and confirmed
// with the plan's Task 1 contract):
//
//	row 0        : Header                         (full width, 1 row)
//	rows 1..H-2  : ListPanel [+ PreviewPanel]      (H-2 rows, bordered boxes)
//	row H-1      : StatusBar                       (full width, 1 row)
//
// Each panel is a full bordered box: its own top border row (row 1, carries
// section/preview tabs) and its own bottom border row (row H-2, directly
// above the status bar — there is no extra gap row). So:
//
//	PanelInterior.Y = PanelPanel.Y + 1 = 2
//	PanelInterior.H = PanelPanel.H - 2 = (H-2) - 2 = H-4
//
// Width: when the preview is open the two panels abut exactly (no shared or
// skipped column) so ListPanel.W + PreviewPanel.W == W. The automatic
// (PreviewWidth==0) split gives the list panel the extra column on odd W
// (list is primary/default-focused): listW = ceil(W/2), previewW = floor(W/2).

func mustNoOverlap(t *testing.T, name string, a, b Rect) {
	t.Helper()
	if a.W == 0 || a.H == 0 || b.W == 0 || b.H == 0 {
		return // zero rects never overlap anything
	}
	// Two axis-aligned rects overlap iff they overlap on both axes.
	xOverlap := a.X < b.X+b.W && b.X < a.X+a.W
	yOverlap := a.Y < b.Y+b.H && b.Y < a.Y+a.H
	if xOverlap && yOverlap {
		t.Errorf("%s: rects overlap: %+v vs %+v", name, a, b)
	}
}

func TestCompute_Golden_80x24_PreviewClosed(t *testing.T) {
	for _, sc := range []int{1, 4} {
		for _, search := range []bool{false, true} {
			in := Input{Width: 80, Height: 24, PreviewOpen: false, SectionCount: sc, SearchOpen: search}
			l := Compute(in)

			want := Layout{
				Header:       Rect{0, 0, 80, 1},
				ListPanel:    Rect{0, 1, 80, 22}, // H-2 = 22
				ListInterior: Rect{1, 2, 78, 20}, // W-2=78, H-4=20
				StatusBar:    Rect{0, 23, 80, 1}, // y = H-1
			}
			wantRows := Rect{1, 3, 78, 19} // interior minus 1 header row
			if search {
				wantRows = Rect{1, 4, 78, 18} // minus 1 more for the search bar
			}

			if l.TooSmall || l.PreviewCollapsed {
				t.Fatalf("80x24 must not be small/collapsed: %+v", l)
			}
			if l.Header != want.Header {
				t.Errorf("Header = %+v, want %+v", l.Header, want.Header)
			}
			if l.ListPanel != want.ListPanel {
				t.Errorf("ListPanel = %+v, want %+v", l.ListPanel, want.ListPanel)
			}
			if l.ListInterior != want.ListInterior {
				t.Errorf("ListInterior = %+v, want %+v", l.ListInterior, want.ListInterior)
			}
			if l.ListRows != wantRows {
				t.Errorf("ListRows(search=%v) = %+v, want %+v", search, l.ListRows, wantRows)
			}
			if l.StatusBar != want.StatusBar {
				t.Errorf("StatusBar = %+v, want %+v", l.StatusBar, want.StatusBar)
			}
			if l.PreviewPanel != (Rect{}) {
				t.Errorf("PreviewPanel should be zero when closed, got %+v", l.PreviewPanel)
			}
			if l.PreviewInterior != (Rect{}) {
				t.Errorf("PreviewInterior should be zero when closed, got %+v", l.PreviewInterior)
			}
			if l.PreviewTabsRow != -1 {
				t.Errorf("PreviewTabsRow = %d, want -1 when closed", l.PreviewTabsRow)
			}
			if l.SectionTabsRow != l.ListPanel.Y {
				t.Errorf("SectionTabsRow = %d, want ListPanel.Y = %d", l.SectionTabsRow, l.ListPanel.Y)
			}
		}
	}
}

func TestCompute_Golden_80x24_PreviewOpen(t *testing.T) {
	in := Input{Width: 80, Height: 24, PreviewOpen: true, SectionCount: 1}
	l := Compute(in)

	// Auto split of W=80: listW=ceil(80/2)=40, previewW=40.
	wantList := Rect{0, 1, 40, 22}
	wantPreview := Rect{40, 1, 40, 22}
	wantListInterior := Rect{1, 2, 38, 20}
	wantPreviewInterior := Rect{41, 2, 38, 20}

	if l.ListPanel != wantList {
		t.Errorf("ListPanel = %+v, want %+v", l.ListPanel, wantList)
	}
	if l.PreviewPanel != wantPreview {
		t.Errorf("PreviewPanel = %+v, want %+v", l.PreviewPanel, wantPreview)
	}
	if l.ListInterior != wantListInterior {
		t.Errorf("ListInterior = %+v, want %+v", l.ListInterior, wantListInterior)
	}
	if l.PreviewInterior != wantPreviewInterior {
		t.Errorf("PreviewInterior = %+v, want %+v", l.PreviewInterior, wantPreviewInterior)
	}
	if l.PreviewTabsRow != l.PreviewPanel.Y {
		t.Errorf("PreviewTabsRow = %d, want PreviewPanel.Y = %d", l.PreviewTabsRow, l.PreviewPanel.Y)
	}
	if l.SectionTabsRow != l.ListPanel.Y {
		t.Errorf("SectionTabsRow = %d, want ListPanel.Y = %d", l.SectionTabsRow, l.ListPanel.Y)
	}
	if l.ListPanel.W+l.PreviewPanel.W != in.Width {
		t.Errorf("panel widths %d+%d != W %d", l.ListPanel.W, l.PreviewPanel.W, in.Width)
	}
	mustNoOverlap(t, "list/preview", l.ListPanel, l.PreviewPanel)
}

func TestCompute_Golden_120x40(t *testing.T) {
	closed := Compute(Input{Width: 120, Height: 40, PreviewOpen: false})
	if want := (Rect{0, 0, 120, 1}); closed.Header != want {
		t.Errorf("Header = %+v, want %+v", closed.Header, want)
	}
	if want := (Rect{0, 39, 120, 1}); closed.StatusBar != want {
		t.Errorf("StatusBar = %+v, want %+v", closed.StatusBar, want)
	}
	if want := (Rect{0, 1, 120, 38}); closed.ListPanel != want { // H-2=38
		t.Errorf("ListPanel = %+v, want %+v", closed.ListPanel, want)
	}
	if want := (Rect{1, 2, 118, 36}); closed.ListInterior != want { // W-2=118, H-4=36
		t.Errorf("ListInterior = %+v, want %+v", closed.ListInterior, want)
	}
	if want := (Rect{1, 3, 118, 35}); closed.ListRows != want {
		t.Errorf("ListRows = %+v, want %+v", closed.ListRows, want)
	}

	open := Compute(Input{Width: 120, Height: 40, PreviewOpen: true, SearchOpen: true})
	// Auto split of W=120: listW=60, previewW=60 (even, exact half).
	if want := (Rect{0, 1, 60, 38}); open.ListPanel != want {
		t.Errorf("ListPanel = %+v, want %+v", open.ListPanel, want)
	}
	if want := (Rect{60, 1, 60, 38}); open.PreviewPanel != want {
		t.Errorf("PreviewPanel = %+v, want %+v", open.PreviewPanel, want)
	}
	if want := (Rect{1, 2, 58, 36}); open.ListInterior != want {
		t.Errorf("ListInterior = %+v, want %+v", open.ListInterior, want)
	}
	if want := (Rect{61, 2, 58, 36}); open.PreviewInterior != want {
		t.Errorf("PreviewInterior = %+v, want %+v", open.PreviewInterior, want)
	}
	// search open: ListRows loses 2 rows total off ListInterior (1 table
	// header + 1 search bar) instead of 1.
	if want := (Rect{1, 4, 58, 34}); open.ListRows != want {
		t.Errorf("ListRows(search) = %+v, want %+v", open.ListRows, want)
	}
}

func TestCompute_Golden_200x60(t *testing.T) {
	closed := Compute(Input{Width: 200, Height: 60, PreviewOpen: false})
	if want := (Rect{0, 0, 200, 1}); closed.Header != want {
		t.Errorf("Header = %+v, want %+v", closed.Header, want)
	}
	if want := (Rect{0, 59, 200, 1}); closed.StatusBar != want {
		t.Errorf("StatusBar = %+v, want %+v", closed.StatusBar, want)
	}
	if want := (Rect{0, 1, 200, 58}); closed.ListPanel != want { // H-2=58
		t.Errorf("ListPanel = %+v, want %+v", closed.ListPanel, want)
	}
	if want := (Rect{1, 2, 198, 56}); closed.ListInterior != want { // W-2=198, H-4=56
		t.Errorf("ListInterior = %+v, want %+v", closed.ListInterior, want)
	}

	open := Compute(Input{Width: 200, Height: 60, PreviewOpen: true})
	// Auto split of W=200: listW=100, previewW=100.
	if want := (Rect{0, 1, 100, 58}); open.ListPanel != want {
		t.Errorf("ListPanel = %+v, want %+v", open.ListPanel, want)
	}
	if want := (Rect{100, 1, 100, 58}); open.PreviewPanel != want {
		t.Errorf("PreviewPanel = %+v, want %+v", open.PreviewPanel, want)
	}
	if want := (Rect{1, 2, 98, 56}); open.ListInterior != want {
		t.Errorf("ListInterior = %+v, want %+v", open.ListInterior, want)
	}
	if want := (Rect{101, 2, 98, 56}); open.PreviewInterior != want {
		t.Errorf("PreviewInterior = %+v, want %+v", open.PreviewInterior, want)
	}
}

// TestCompute_PreviewWidth_ClampedForMinList exercises the explicit-width
// path: PreviewWidth is honored as the desired PreviewPanel total width
// (including its own borders), but clamped so ListPanel keeps >=20 interior
// columns, i.e. ListPanel.W >= 22.
func TestCompute_PreviewWidth_ClampedForMinList(t *testing.T) {
	// W=80: max allowed previewW = 80-22 = 58.
	l := Compute(Input{Width: 80, Height: 24, PreviewOpen: true, PreviewWidth: 70})
	if l.PreviewPanel.W != 58 {
		t.Errorf("PreviewPanel.W = %d, want clamped 58", l.PreviewPanel.W)
	}
	if l.ListPanel.W != 22 {
		t.Errorf("ListPanel.W = %d, want 22 (>=20 interior cols)", l.ListPanel.W)
	}
	if l.ListInterior.W < 20 {
		t.Errorf("ListInterior.W = %d, want >=20", l.ListInterior.W)
	}

	// A small requested width well under the clamp is honored exactly.
	l2 := Compute(Input{Width: 80, Height: 24, PreviewOpen: true, PreviewWidth: 30})
	if l2.PreviewPanel.W != 30 {
		t.Errorf("PreviewPanel.W = %d, want honored 30", l2.PreviewPanel.W)
	}
	if l2.ListPanel.W != 50 {
		t.Errorf("ListPanel.W = %d, want 50", l2.ListPanel.W)
	}
}

// TestCompute_PreviewWidth_FlooredForTinyRequest: a tiny requested
// PreviewWidth (e.g. 1) must not produce negative/zero interior dims — it's
// floored at minPreviewTotal (12: 2 border cols + >=10 interior cols).
func TestCompute_PreviewWidth_FlooredForTinyRequest(t *testing.T) {
	l := Compute(Input{Width: 80, Height: 24, PreviewOpen: true, PreviewWidth: 1})

	if l.PreviewPanel.W != 12 {
		t.Errorf("PreviewPanel.W = %d, want floored to 12", l.PreviewPanel.W)
	}
	if l.ListPanel.W != 68 {
		t.Errorf("ListPanel.W = %d, want 68 (80-12)", l.ListPanel.W)
	}
	rects := []Rect{l.Header, l.ListPanel, l.ListInterior, l.ListRows, l.PreviewPanel, l.PreviewInterior, l.StatusBar}
	for _, r := range rects {
		if r.W < 0 || r.H < 0 {
			t.Errorf("rect has negative dimension: %+v", r)
		}
	}
	if l.PreviewInterior.W != 10 {
		t.Errorf("PreviewInterior.W = %d, want 10 (12-2 borders)", l.PreviewInterior.W)
	}
}

// TestCompute_SectionCountDoesNotAffectRects: SectionCount is carried in
// Input for future tab-rendering consumers, but this package's geometry
// does not depend on it — it never computes per-tab rects.
func TestCompute_SectionCountDoesNotAffectRects(t *testing.T) {
	base := Compute(Input{Width: 120, Height: 40, PreviewOpen: true, SectionCount: 1})
	other := Compute(Input{Width: 120, Height: 40, PreviewOpen: true, SectionCount: 4})
	if base != other {
		t.Errorf("SectionCount affected Layout: %+v vs %+v", base, other)
	}
}

// TestCompute_SearchOpen_ShrinksListRowsByExactlyOne.
func TestCompute_SearchOpen_ShrinksListRowsByExactlyOne(t *testing.T) {
	without := Compute(Input{Width: 100, Height: 30, SearchOpen: false})
	with := Compute(Input{Width: 100, Height: 30, SearchOpen: true})
	if without.ListRows.H-with.ListRows.H != 1 {
		t.Errorf("ListRows.H delta = %d, want 1 (without=%+v with=%+v)",
			without.ListRows.H-with.ListRows.H, without.ListRows, with.ListRows)
	}
	if without.ListRows.Y+1 != with.ListRows.Y {
		t.Errorf("ListRows.Y should move down by exactly 1 row, got %d -> %d",
			without.ListRows.Y, with.ListRows.Y)
	}
}

// TestCompute_MinSize_PreviewCollapsed: below 60x15 (either dimension) the
// preview auto-collapses; the list takes the full width; preview rects are
// zero. 59x24 trips on width, 80x14 trips on height.
func TestCompute_MinSize_PreviewCollapsed(t *testing.T) {
	cases := []Input{
		{Width: 59, Height: 24, PreviewOpen: true},
		{Width: 80, Height: 14, PreviewOpen: true},
	}
	for _, in := range cases {
		l := Compute(in)
		if l.TooSmall {
			t.Errorf("%+v: TooSmall=true, want false", in)
		}
		if !l.PreviewCollapsed {
			t.Errorf("%+v: PreviewCollapsed=false, want true", in)
		}
		if l.PreviewPanel != (Rect{}) {
			t.Errorf("%+v: PreviewPanel = %+v, want zero", in, l.PreviewPanel)
		}
		if l.PreviewInterior != (Rect{}) {
			t.Errorf("%+v: PreviewInterior = %+v, want zero", in, l.PreviewInterior)
		}
		if l.PreviewTabsRow != -1 {
			t.Errorf("%+v: PreviewTabsRow = %d, want -1", in, l.PreviewTabsRow)
		}
		if l.ListPanel.W != in.Width {
			t.Errorf("%+v: ListPanel.W = %d, want full width %d", in, l.ListPanel.W, in.Width)
		}
		if l.ListPanel.H != in.Height-2 {
			t.Errorf("%+v: ListPanel.H = %d, want %d", in, l.ListPanel.H, in.Height-2)
		}
	}
}

// TestCompute_MinSize_ExactlyAtCollapseThreshold_NotCollapsed: 60x15 is the
// smallest size that must NOT collapse (the rule is strictly "<", not "<=")
// — the preview must still render at exactly the threshold.
func TestCompute_MinSize_ExactlyAtCollapseThreshold_NotCollapsed(t *testing.T) {
	l := Compute(Input{Width: 60, Height: 15, PreviewOpen: true})
	if l.PreviewCollapsed {
		t.Errorf("60x15 must NOT be collapsed, got PreviewCollapsed=true: %+v", l)
	}
	if l.PreviewPanel == (Rect{}) {
		t.Errorf("60x15 with PreviewOpen: PreviewPanel should render, got zero rect")
	}
	if l.PreviewTabsRow == -1 {
		t.Errorf("60x15 with PreviewOpen: PreviewTabsRow should not be -1")
	}
}

// TestCompute_MinSize_TooSmall: below 40x10 (either dimension), everything
// zeroes out except Full. 39x20 trips on width, 80x9 trips on height.
func TestCompute_MinSize_TooSmall(t *testing.T) {
	cases := []Input{
		{Width: 39, Height: 20},
		{Width: 80, Height: 9},
	}
	for _, in := range cases {
		l := Compute(in)
		if !l.TooSmall {
			t.Errorf("%+v: TooSmall=false, want true", in)
		}
		zero := Rect{}
		if l.Header != zero || l.ListPanel != zero || l.ListInterior != zero || l.ListRows != zero ||
			l.PreviewPanel != zero || l.PreviewInterior != zero || l.StatusBar != zero {
			t.Errorf("%+v: expected all rects zero except Full, got %+v", in, l)
		}
		if l.PreviewTabsRow != -1 || l.SectionTabsRow != -1 {
			t.Errorf("%+v: expected tab rows -1, got preview=%d section=%d", in, l.PreviewTabsRow, l.SectionTabsRow)
		}
		want := Rect{0, 0, in.Width, in.Height}
		if l.Full != want {
			t.Errorf("%+v: Full = %+v, want %+v", in, l.Full, want)
		}
	}
}

// TestCompute_MinSize_ExactlyAtTooSmallThreshold_NotTooSmall: 40x10 is the
// smallest size that must NOT be too-small (the rule is strictly "<", not
// "<="). It's still below the 60x15 collapse threshold, so the preview
// collapses, but the list panel must still render with non-negative rects.
func TestCompute_MinSize_ExactlyAtTooSmallThreshold_NotTooSmall(t *testing.T) {
	l := Compute(Input{Width: 40, Height: 10, PreviewOpen: true})
	if l.TooSmall {
		t.Errorf("40x10 must NOT be TooSmall, got true: %+v", l)
	}
	if l.ListPanel == (Rect{}) {
		t.Errorf("40x10: ListPanel should render, got zero rect")
	}
	if l.ListPanel.W != 40 {
		t.Errorf("40x10: ListPanel.W = %d, want 40 (full width, collapsed)", l.ListPanel.W)
	}
	rects := []Rect{l.Header, l.ListPanel, l.ListInterior, l.ListRows, l.StatusBar}
	for _, r := range rects {
		if r.W < 0 || r.H < 0 {
			t.Errorf("40x10: rect has negative dimension: %+v", r)
		}
	}
}

// TestCompute_FullCoverage: header + panels + status bar tile every row of
// H exactly, for both preview states, across the three reference sizes.
func TestCompute_FullCoverage(t *testing.T) {
	sizes := []struct{ w, h int }{{80, 24}, {120, 40}, {200, 60}}
	for _, sz := range sizes {
		for _, preview := range []bool{false, true} {
			l := Compute(Input{Width: sz.w, Height: sz.h, PreviewOpen: preview})
			if l.Header.H != 1 || l.Header.Y != 0 {
				t.Fatalf("%dx%d preview=%v: bad Header %+v", sz.w, sz.h, preview, l.Header)
			}
			if l.StatusBar.H != 1 || l.StatusBar.Y != sz.h-1 {
				t.Fatalf("%dx%d preview=%v: bad StatusBar %+v", sz.w, sz.h, preview, l.StatusBar)
			}
			if l.ListPanel.Y != l.Header.Y+l.Header.H {
				t.Fatalf("%dx%d preview=%v: ListPanel.Y=%d should directly follow header", sz.w, sz.h, preview, l.ListPanel.Y)
			}
			if l.ListPanel.Y+l.ListPanel.H != l.StatusBar.Y {
				t.Fatalf("%dx%d preview=%v: panels bottom (%d) should directly precede status bar (%d)",
					sz.w, sz.h, preview, l.ListPanel.Y+l.ListPanel.H, l.StatusBar.Y)
			}
			// 1 (header) + panelsH + 1 (status) == H
			if 1+l.ListPanel.H+1 != sz.h {
				t.Fatalf("%dx%d preview=%v: rows don't tile H: %d", sz.w, sz.h, preview, 1+l.ListPanel.H+1)
			}
			sumW := l.ListPanel.W + l.PreviewPanel.W
			if sumW != sz.w {
				t.Fatalf("%dx%d preview=%v: widths sum to %d, want %d", sz.w, sz.h, preview, sumW, sz.w)
			}
			mustNoOverlap(t, "list/preview", l.ListPanel, l.PreviewPanel)
			mustNoOverlap(t, "header/listpanel", l.Header, l.ListPanel)
			mustNoOverlap(t, "statusbar/listpanel", l.StatusBar, l.ListPanel)
		}
	}
}

func TestRect_Contains(t *testing.T) {
	r := Rect{X: 2, Y: 3, W: 4, H: 5} // covers x in [2,6), y in [3,8)
	inside := []struct{ x, y int }{{2, 3}, {5, 7}, {2, 7}, {5, 3}}
	for _, p := range inside {
		if !r.Contains(p.x, p.y) {
			t.Errorf("Contains(%d,%d) = false, want true for %+v", p.x, p.y, r)
		}
	}
	outside := []struct{ x, y int }{{1, 3}, {2, 2}, {6, 3}, {2, 8}, {0, 0}}
	for _, p := range outside {
		if r.Contains(p.x, p.y) {
			t.Errorf("Contains(%d,%d) = true, want false for %+v", p.x, p.y, r)
		}
	}
	zero := Rect{}
	if zero.Contains(0, 0) {
		t.Errorf("zero Rect must contain nothing")
	}
}

// --- Zones ---

func TestZones_HitOnRegisteredRect(t *testing.T) {
	var z Zones
	r := Rect{X: 0, Y: 0, W: 10, H: 5}
	z.Add(ZoneListRow, r, 3)

	got, ok := z.Hit(2, 2)
	if !ok {
		t.Fatalf("Hit inside registered rect: ok=false")
	}
	if got.Kind != ZoneListRow || got.Payload != 3 || got.Rect != r {
		t.Errorf("Hit = %+v, want Kind=ZoneListRow Payload=3 Rect=%+v", got, r)
	}
}

func TestZones_MissOutsideAllRects(t *testing.T) {
	var z Zones
	z.Add(ZoneListRow, Rect{0, 0, 10, 5}, 1)
	z.Add(ZoneSectionTab, Rect{20, 0, 10, 1}, 2)

	if _, ok := z.Hit(15, 0); ok {
		t.Errorf("Hit outside all registered rects should miss")
	}
}

func TestZones_OverlappingRegistration_LaterWins(t *testing.T) {
	var z Zones
	overlap := Rect{X: 5, Y: 5, W: 10, H: 10}
	z.Add(ZoneListBody, overlap, 1)
	z.Add(ZonePreviewBody, overlap, 2) // registered later, same rect

	got, ok := z.Hit(7, 7)
	if !ok {
		t.Fatalf("Hit in overlapping region: ok=false")
	}
	if got.Kind != ZonePreviewBody || got.Payload != 2 {
		t.Errorf("Hit = %+v, want the LATER registration (ZonePreviewBody, payload 2)", got)
	}
}

func TestZones_Reset_Empties(t *testing.T) {
	var z Zones
	z.Add(ZoneStatusBar, Rect{0, 0, 5, 1}, 0)
	if _, ok := z.Hit(1, 0); !ok {
		t.Fatalf("expected a hit before Reset")
	}
	z.Reset()
	if _, ok := z.Hit(1, 0); ok {
		t.Errorf("expected no hit after Reset")
	}
}

func TestZones_ReusableAfterReset(t *testing.T) {
	var z Zones
	z.Add(ZoneViewLabel, Rect{0, 0, 5, 1}, 9)
	z.Reset()
	z.Add(ZonePreviewTab, Rect{0, 0, 5, 1}, 7)

	got, ok := z.Hit(1, 0)
	if !ok || got.Kind != ZonePreviewTab || got.Payload != 7 {
		t.Errorf("Hit after Reset+Add = %+v ok=%v, want ZonePreviewTab payload 7", got, ok)
	}
}
