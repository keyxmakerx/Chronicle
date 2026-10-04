package maps

import (
	"context"
	"math"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// This file is the fog-of-war rules that need no I/O: the party's path, which
// hexes a move reveals, where a scribe may move the party, and the point test
// that decides whether a pin or a drawing is hiding under unexplored land.
// Everything that hides data from a viewer (the hex list, pins, drawings, live
// events, the map picture) is built from these, so there is one definition of
// "unexplored".

// MaxFogBatch caps how many hexes one reveal or hide request may name.
const MaxFogBatch = MaxHexBatch

// Who may move the party, stored in display_settings hexes.party_who.
const (
	PartyWhoOwners  = "owners"
	PartyWhoScribes = "scribes"
)

// HexLine lists the hexes from a to b inclusive, interpolating in cube space
// and rounding like JavaScript's Math.round so the server's path is the same
// line the viewer draws for its walking animation (hexLine in map_hexes.js).
func HexLine(a, b HexKey) []HexKey {
	n := HexDistance(a, b)
	p, q := OffsetToCube(a.Col, a.Row), OffsetToCube(b.Col, b.Row)
	out := make([]HexKey, 0, n+1)
	for i := 0; i <= n; i++ {
		t := 0.0
		if n > 0 {
			t = float64(i) / float64(n)
		}
		x := float64(p.X) + float64(q.X-p.X)*t
		y := float64(p.Y) + float64(q.Y-p.Y)*t
		z := float64(p.Z) + float64(q.Z-p.Z)*t
		rx, ry, rz := jsRound(x), jsRound(y), jsRound(z)
		dx, dy, dz := math.Abs(rx-x), math.Abs(ry-y), math.Abs(rz-z)
		if dx > dy && dx > dz {
			rx = -ry - rz
		} else if dy <= dz {
			rz = -rx - ry
		}
		col, row := CubeToOffset(Cube{X: int(rx), Y: int(-rx - rz), Z: int(rz)})
		out = append(out, HexKey{Col: col, Row: row})
	}
	return out
}

// RevealZone returns every hex within one step of any hex on path, each once,
// in first-seen order. The server derives it from the path it computed itself,
// so a client can never name which hexes a move reveals. cols and rows clip the
// result to the field so a move at the edge never stores a hex that cannot
// exist.
func RevealZone(path []HexKey, cols, rows int) []HexKey {
	seen := make(map[HexKey]bool, len(path)*7)
	out := make([]HexKey, 0, len(path)*4)
	add := func(k HexKey) {
		if k.Col < 0 || k.Row < 0 || k.Col >= cols || k.Row >= rows || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, k)
	}
	for _, k := range path {
		add(k)
		for _, n := range Neighbors(k.Col, k.Row) {
			add(n)
		}
	}
	return out
}

// OnExploredFrontier reports whether k is explored or touches an explored hex:
// the only places a scribe may move the party. A scribe can then follow
// ground the party has already seen but cannot teleport it into the dark.
func OnExploredFrontier(explored func(HexKey) bool, k HexKey) bool {
	if explored(k) {
		return true
	}
	for _, n := range Neighbors(k.Col, k.Row) {
		if explored(n) {
			return true
		}
	}
	return false
}

// PointInUnexplored reports whether a map point (pixels) lies inside a hex of
// the field that is not explored. A point outside the field is not hidden: the
// layer says nothing about land it does not cover.
func (g HexGeometry) PointInUnexplored(explored map[HexKey]bool, x, y float64) bool {
	col, row, ok := g.HexAtPoint(x, y)
	return ok && !explored[HexKey{Col: col, Row: row}]
}

// FogMask is what hides unexplored land from a viewer below CanSeeDmOnly: the
// field's geometry and the explored set at one layer version. It is built only
// when fog is on and the grid is hexes, so a nil mask always means "nothing is
// fogged".
type FogMask struct {
	Version  uint64
	Geo      HexGeometry
	MapW     float64
	MapH     float64
	Explored map[HexKey]bool
	// AnchorID is the picture the layer is pinned to, or "" when it covers the
	// whole map. A pinned layer hides only land inside that picture.
	AnchorID string
}

// CoversMapPicture reports whether the map's own background picture is under
// the fog. A layer pinned to a picture drawing leaves the background alone: it
// is that drawing's file that is the secret.
func (f *FogMask) CoversMapPicture() bool { return f != nil && f.AnchorID == "" }

// HidesPoint reports whether a point given in map percentages (0-100) lies in
// an unexplored hex.
func (f *FogMask) HidesPoint(xPct, yPct float64) bool {
	if f == nil {
		return false
	}
	if math.IsNaN(xPct) || math.IsNaN(yPct) || math.IsInf(xPct, 0) || math.IsInf(yPct, 0) {
		return true
	}
	return f.Geo.PointInUnexplored(f.Explored, xPct/100*f.MapW, yPct/100*f.MapH)
}

// HidesMarker reports whether a pin sits in an unexplored hex.
func (f *FogMask) HidesMarker(mk *Marker) bool {
	return f != nil && mk != nil && f.HidesPoint(mk.X, mk.Y)
}

// HidesDrawing reports whether a drawing lies wholly in unexplored hexes, the
// same rule shadows use: one that merely touches explored land stays, because
// hiding it would also hide the part players may see. An unparseable drawing is
// hidden, failing closed. Shadows pass (players need them to draw the smoke)
// and so does the picture the layer is pinned to: the client needs its box to
// place the hexes, and its file is withheld separately.
func (f *FogMask) HidesDrawing(d *Drawing) bool {
	if f == nil || d == nil || d.DrawingType == DrawingTypeShadow || (f.AnchorID != "" && d.ID == f.AnchorID) {
		return false
	}
	pts, ok := parsePoints(d.Points)
	if !ok {
		return true
	}
	if len(pts) == 0 {
		return false
	}
	for _, p := range pts {
		if !f.HidesPoint(p.X, p.Y) {
			return false
		}
	}
	return true
}

// WithholdsImageOf reports whether the picture file of drawing d must not be
// sent to a viewer: it is the picture a fogged layer is pinned to.
func (f *FogMask) WithholdsImageOf(d *Drawing) bool {
	return f != nil && f.AnchorID != "" && d != nil && d.ID == f.AnchorID
}

// fogHidingApplies is who the fog hides things from: everyone who cannot see
// DM-only content, scribes included (VisibleCells uses the same line).
func fogHidingApplies(role int) bool { return !permissions.CanSeeDmOnly(role) }

// HexFogLookup returns the fog mask of a map, or nil when nothing is fogged.
// Implemented by the hex service, which owns the layer; pins, drawings, the
// event publisher and the picture renderer only apply the mask.
type HexFogLookup interface {
	FogMask(ctx context.Context, mapID string) (*FogMask, error)
}

// fogFor returns the mask for a viewer subject to fog, or nil when the viewer
// is exempt or no lookup is wired.
func fogFor(ctx context.Context, l HexFogLookup, mapID string, role int) (*FogMask, error) {
	if l == nil || !fogHidingApplies(role) {
		return nil, nil
	}
	return l.FogMask(ctx, mapID)
}

// VisiblePartyLayer returns the layer as role may receive it. With fog on, a
// viewer below CanSeeDmOnly gets no party position while the party stands on an
// unexplored hex. A move always reveals the hexes it ends on, so this should
// never trigger; it fails closed for a reset, a hand-edited row or a race, so
// the party token can never point at land the viewer has not been shown.
func VisiblePartyLayer(layer HexLayer, cells []HexCell, role int) HexLayer {
	if !layer.FogEnabled || !fogHidingApplies(role) || layer.PartyCol == nil || layer.PartyRow == nil {
		return layer
	}
	for _, c := range cells {
		if c.Explored && c.Col == *layer.PartyCol && c.Row == *layer.PartyRow {
			return layer
		}
	}
	layer.PartyCol, layer.PartyRow = nil, nil
	return layer
}
