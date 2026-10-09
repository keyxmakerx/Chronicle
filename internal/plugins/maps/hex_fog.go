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

// HidesToken reports whether a token sits in an unexplored hex, by the same
// point rule as a pin.
func (f *FogMask) HidesToken(t *Token) bool {
	return f != nil && t != nil && f.HidesPoint(t.X, t.Y)
}

// WithholdsImageOf reports whether the picture file of drawing d must not be
// sent to a viewer: it is the picture a fogged layer is pinned to, or a
// picture whose box reaches into any unexplored hex. A picture shows land as
// pixels, so unlike a line it cannot "merely touch" the fog; the drawing itself
// still goes out (as a placeholder) wherever HidesDrawing lets it. A picture
// whose box cannot be read is withheld, failing closed.
func (f *FogMask) WithholdsImageOf(d *Drawing) bool {
	if f == nil || d == nil || d.DrawingType != DrawingTypeImage {
		return false
	}
	if f.AnchorID != "" && d.ID == f.AnchorID {
		return true
	}
	pts, ok := parsePoints(d.Points)
	if !ok || len(pts) != 2 || math.IsNaN(d.Rotation) || math.IsInf(d.Rotation, 0) {
		return true
	}
	// Corners in map pixels, then the box the picture covers once turned about
	// its centre, so a rotated picture is judged by all the land it can show.
	x0, x1 := minFloat(pts[0].X, pts[1].X)/100*f.MapW, maxFloat(pts[0].X, pts[1].X)/100*f.MapW
	y0, y1 := minFloat(pts[0].Y, pts[1].Y)/100*f.MapH, maxFloat(pts[0].Y, pts[1].Y)/100*f.MapH
	cx, cy, hw, hh := (x0+x1)/2, (y0+y1)/2, (x1-x0)/2, (y1-y0)/2
	rad := d.Rotation * math.Pi / 180
	cos, sin := math.Abs(math.Cos(rad)), math.Abs(math.Sin(rad))
	ex, ey := hw*cos+hh*sin, hw*sin+hh*cos
	return f.boxTouchesUnexplored(cx-ex, cy-ey, cx+ex, cy+ey)
}

// boxTouchesUnexplored reports whether the box (map pixels) overlaps any
// unexplored hex of the field with a non-zero area. Only the hexes whose
// rows and columns can reach the box are tested, each exactly (the hex is a
// hexagon, not its bounding box), so a picture lying in explored land next to
// the fog keeps its file.
func (f *FogMask) boxTouchesUnexplored(x0, y0, x1, y1 float64) bool {
	g := f.Geo
	if g.R <= 0 || g.Cols <= 0 || g.Rows <= 0 || !(x1 > x0) || !(y1 > y0) {
		return false
	}
	w := g.Width()
	r0 := clampIndex((y0-g.Oy-g.R)/(1.5*g.R)-1, g.Rows)
	r1 := clampIndex((y1-g.Oy+g.R)/(1.5*g.R)+1, g.Rows)
	c0 := clampIndex((x0-g.Ox-w)/w-1, g.Cols)
	c1 := clampIndex((x1-g.Ox+w)/w+1, g.Cols)
	for row := r0; row <= r1; row++ {
		for col := c0; col <= c1; col++ {
			if f.Explored[HexKey{Col: col, Row: row}] {
				continue
			}
			hx, hy := g.Center(col, row)
			if hexOverlapsBox(hx, hy, g.R, x0, y0, x1, y1) {
				return true
			}
		}
	}
	return false
}

// clampIndex turns a fractional hex index into one inside [0, n-1], safe for
// any finite or infinite input.
func clampIndex(v float64, n int) int {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > float64(n-1) {
		return n - 1
	}
	return int(v)
}

// hexAxes are the separating axes of a pointy-top hexagon and a box: the box's
// own two and the hexagon's three edge normals.
var hexAxes = [4][2]float64{{1, 0}, {0, 1}, {0.5, math.Sqrt(3) / 2}, {-0.5, math.Sqrt(3) / 2}}

// hexOverlapsBox is the separating-axis test between the pointy-top hexagon of
// corner radius r centred on (hx, hy) and the box: they overlap unless some
// axis separates them. Touching edges do not count as overlap.
func hexOverlapsBox(hx, hy, r, x0, y0, x1, y1 float64) bool {
	var hv [6][2]float64
	for i := range hv {
		a := (float64(i)*60 - 90) * math.Pi / 180
		hv[i] = [2]float64{hx + r*math.Cos(a), hy + r*math.Sin(a)}
	}
	bv := [4][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	for _, ax := range hexAxes {
		hmin, hmax := math.Inf(1), math.Inf(-1)
		for _, v := range hv {
			p := v[0]*ax[0] + v[1]*ax[1]
			hmin, hmax = math.Min(hmin, p), math.Max(hmax, p)
		}
		bmin, bmax := math.Inf(1), math.Inf(-1)
		for _, v := range bv {
			p := v[0]*ax[0] + v[1]*ax[1]
			bmin, bmax = math.Min(bmin, p), math.Max(bmax, p)
		}
		if hmax <= bmin || bmax <= hmin {
			return false
		}
	}
	return true
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
