package maps

import (
	"encoding/json"
	"math"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// DrawingTypeShadow marks a drawing that players see as a drifting dark
// shadow instead of a shape. Pins and drawings lying under one are withheld
// from players on the server, so the shadow is a real secret, not a cover.
const DrawingTypeShadow = "shadow"

// The two strengths a shadow can have, stored in the drawing's fill_alpha.
const (
	ShadowAlphaHint   = 0.5  // "A hint": something is there, hard to make out.
	ShadowAlphaHidden = 0.85 // "Almost nothing".
)

// normalizeShadowAlpha maps any stored or submitted value onto one of the two
// offered strengths, so the viewer never has to guess what an odd number means.
func normalizeShadowAlpha(a float64) float64 {
	if a == ShadowAlphaHidden {
		return ShadowAlphaHidden
	}
	return ShadowAlphaHint
}

// ShadowArea is a shadow's box in map percentages (0-100), already ordered so
// Min <= Max. Corners are normalised on parse because a person can drag a box
// from any corner to its opposite. Strength is one of the two normalised
// strengths; the player copy of the map picture needs it to know how dark to
// smudge the area.
type ShadowArea struct {
	MinX, MinY, MaxX, MaxY float64
	Strength               float64
}

// Contains reports whether the point lies inside the box. Edges count as
// inside: a pin sitting exactly on the border must not be left showing.
func (a ShadowArea) Contains(x, y float64) bool {
	return x >= a.MinX && x <= a.MaxX && y >= a.MinY && y <= a.MaxY
}

// pointXY is one stored coordinate pair.
type pointXY struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// parsePoints decodes a drawing's points. ok is false for anything that is not
// an array of finite coordinate pairs.
func parsePoints(raw json.RawMessage) ([]pointXY, bool) {
	var pts []pointXY
	if err := json.Unmarshal(raw, &pts); err != nil {
		return nil, false
	}
	for _, p := range pts {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
			return nil, false
		}
	}
	return pts, true
}

// shadowAreaFromDrawing builds the box of a shadow drawing. ok is false when
// the drawing is not a shadow or does not carry exactly two valid corners.
func shadowAreaFromDrawing(d Drawing) (ShadowArea, bool) {
	if d.DrawingType != DrawingTypeShadow {
		return ShadowArea{}, false
	}
	pts, ok := parsePoints(d.Points)
	if !ok || len(pts) != 2 {
		return ShadowArea{}, false
	}
	return ShadowArea{
		MinX:     math.Min(pts[0].X, pts[1].X),
		MinY:     math.Min(pts[0].Y, pts[1].Y),
		MaxX:     math.Max(pts[0].X, pts[1].X),
		MaxY:     math.Max(pts[0].Y, pts[1].Y),
		Strength: normalizeShadowAlpha(d.FillAlpha),
	}, true
}

// pointInAnyShadow is the one rule behind hiding: is (x, y) under any shadow.
func pointInAnyShadow(areas []ShadowArea, x, y float64) bool {
	for _, a := range areas {
		if a.Contains(x, y) {
			return true
		}
	}
	return false
}

// drawingUnderShadow reports whether a non-shadow drawing is hidden: it has
// points and every one of them lies inside some shadow. A drawing that merely
// touches a shadow stays visible, because hiding it would also hide the part
// the players can legitimately see. An unparseable drawing is treated as
// hidden, failing closed.
func drawingUnderShadow(areas []ShadowArea, d Drawing) bool {
	if len(areas) == 0 || d.DrawingType == DrawingTypeShadow {
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
		if !pointInAnyShadow(areas, p.X, p.Y) {
			return false
		}
	}
	return true
}

// shadowWithholdsImageOf reports whether the picture file of drawing d must
// not be sent to a viewer subject to shadow hiding: its box overlaps a shadow.
// A picture shows the land under the shadow as pixels, so unlike a line it
// cannot "merely touch" one; the drawing still goes out as a placeholder
// wherever drawingUnderShadow lets it. Shadows are boxes in map percentages
// and the map's aspect is not known here, so a turned picture's box cannot be
// placed exactly: any turn other than a half turn is withheld while the map
// has a shadow at all, and an unreadable box is withheld, failing closed.
func shadowWithholdsImageOf(areas []ShadowArea, d *Drawing) bool {
	if len(areas) == 0 || d == nil || d.DrawingType != DrawingTypeImage {
		return false
	}
	pts, ok := parsePoints(d.Points)
	if !ok || len(pts) != 2 || math.IsNaN(d.Rotation) || math.IsInf(d.Rotation, 0) {
		return true
	}
	if math.Mod(d.Rotation, 180) != 0 {
		return true
	}
	x0, x1 := math.Min(pts[0].X, pts[1].X), math.Max(pts[0].X, pts[1].X)
	y0, y1 := math.Min(pts[0].Y, pts[1].Y), math.Max(pts[0].Y, pts[1].Y)
	for _, a := range areas {
		// Positive-area overlap: a picture lying edge to edge with a shadow
		// shows none of the land under it.
		if x0 < a.MaxX && x1 > a.MinX && y0 < a.MaxY && y1 > a.MinY {
			return true
		}
	}
	return false
}

// shadowHidingApplies reports whether a viewer is subject to shadow hiding.
// Only owners and DM-equivalents (a co-DM grant is promoted to the owner role
// by the visibility role) see under a shadow; a scribe is a diligent player
// and is hidden like one. role must be the promoted visibility role.
func shadowHidingApplies(role int) bool {
	return !permissions.CanSeeDmOnly(role)
}

// requireShadowAuthor refuses anyone who is not owner/DM-equivalent. Shadows
// decide what players may know, so authoring one is a DM act even though a
// scribe may draw everything else.
func requireShadowAuthor(isDM bool) error {
	if !isDM {
		return apperror.NewForbidden("only the owner or a co-DM can hide areas under a shadow")
	}
	return nil
}

// filterMarkersByShadow drops markers lying under a shadow.
func filterMarkersByShadow(areas []ShadowArea, markers []Marker) []Marker {
	if len(areas) == 0 {
		return markers
	}
	out := make([]Marker, 0, len(markers))
	for _, mk := range markers {
		if !pointInAnyShadow(areas, mk.X, mk.Y) {
			out = append(out, mk)
		}
	}
	return out
}

// filterDrawingsByShadow drops non-shadow drawings that lie wholly under a
// shadow. Shadow drawings pass through: players need them to draw the shadow.
func filterDrawingsByShadow(areas []ShadowArea, drawings []Drawing) []Drawing {
	if len(areas) == 0 {
		return drawings
	}
	out := make([]Drawing, 0, len(drawings))
	for _, d := range drawings {
		if !drawingUnderShadow(areas, d) {
			out = append(out, d)
		}
	}
	return out
}

// MarkerUnderShadow and DrawingUnderShadow are the single-item forms of the
// rule for callers that gate one resource at a time (the event publisher and
// the by-id reads).
func MarkerUnderShadow(areas []ShadowArea, mk *Marker) bool {
	return mk != nil && pointInAnyShadow(areas, mk.X, mk.Y)
}

// DrawingUnderShadow: see MarkerUnderShadow.
func DrawingUnderShadow(areas []ShadowArea, d *Drawing) bool {
	return d != nil && drawingUnderShadow(areas, *d)
}

// ShadowWithholdsImageOf reports whether a shadow withholds the picture file of
// d, for the event publisher, whose payload carries the file id.
func ShadowWithholdsImageOf(areas []ShadowArea, d *Drawing) bool {
	return shadowWithholdsImageOf(areas, d)
}
