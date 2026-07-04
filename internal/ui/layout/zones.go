package layout

// ZoneKind identifies what kind of shell element a Zone represents, for
// mouse hit-testing dispatch.
type ZoneKind int

const (
	ZoneNone       ZoneKind = iota
	ZoneViewLabel           // Payload = int (view index 0-4)
	ZoneSectionTab          // Payload = int (section id)
	ZoneListRow             // Payload = int (row index)
	ZonePreviewTab          // Payload = int (tab index)
	ZonePreviewBody
	ZoneListBody
	ZoneStatusBar
)

// Zone is one hit-testable region of the shell.
type Zone struct {
	Kind    ZoneKind
	Rect    Rect
	Payload int
}

// Zones is an ordered registry of Zone rects, rebuilt on every render.
// Registrations may overlap; Hit resolves overlaps by returning the most
// recently registered match (later registrations win, so components can
// register a broad body zone first and narrower row/tab zones on top of it).
type Zones struct {
	zones []Zone
}

// Add registers a new hit-testable zone.
func (z *Zones) Add(kind ZoneKind, r Rect, payload int) {
	z.zones = append(z.zones, Zone{Kind: kind, Rect: r, Payload: payload})
}

// Hit returns the most recently registered zone containing (x, y), if any.
func (z *Zones) Hit(x, y int) (Zone, bool) {
	for i := len(z.zones) - 1; i >= 0; i-- {
		if z.zones[i].Rect.Contains(x, y) {
			return z.zones[i], true
		}
	}
	return Zone{}, false
}

// Reset empties the registry so it can be rebuilt for the next render,
// reusing the underlying storage.
func (z *Zones) Reset() {
	z.zones = z.zones[:0]
}
