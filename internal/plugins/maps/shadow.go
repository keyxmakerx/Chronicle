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

// MapFrame is a map's size in map units (the pixels of its image), the space
// the viewer draws in. A picture turns in this space, not in percentages, so a
// turned picture can only be placed on a map whose frame is known.
type MapFrame struct {
	W, H float64
}

func (f MapFrame) known() bool {
	return f.W > 0 && f.H > 0 && !math.IsInf(f.W, 0) && !math.IsInf(f.H, 0)
}

// viewerDefaultMapUnits is the side the map viewer gives a map whose image
// size was never recorded; the frame must match what the viewer draws.
const viewerDefaultMapUnits = 1000

// mapFrameOf is the frame the viewer draws a map of this image size in.
func mapFrameOf(imageW, imageH int) MapFrame {
	f := MapFrame{W: float64(imageW), H: float64(imageH)}
	if imageW <= 0 {
		f.W = viewerDefaultMapUnits
	}
	if imageH <= 0 {
		f.H = viewerDefaultMapUnits
	}
	return f
}

// pictureIsTurned reports whether d is a picture whose box is not axis-aligned
// on screen, so judging it against a shadow needs the map's frame.
func pictureIsTurned(d *Drawing) bool {
	return d != nil && d.DrawingType == DrawingTypeImage && math.Mod(d.Rotation, 180) != 0
}

// PictureIsTurned: see pictureIsTurned. The event publisher reads the map's
// frame only for these.
func PictureIsTurned(d *Drawing) bool { return pictureIsTurned(d) }

// shadowWithholdsImageOf reports whether the picture file of drawing d must
// not be sent to a viewer subject to shadow hiding: the picture, as drawn,
// covers part of a shadow. A picture shows the land under the shadow as
// pixels, so unlike a line it cannot "merely touch" one; the drawing still
// goes out as a placeholder wherever drawingUnderShadow lets it. A turned
// picture is placed exactly in the map's frame; with no frame, or an
// unreadable box, it is withheld, failing closed.
func shadowWithholdsImageOf(areas []ShadowArea, d *Drawing, frame MapFrame) bool {
	if len(areas) == 0 || d == nil || d.DrawingType != DrawingTypeImage {
		return false
	}
	pts, ok := parsePoints(d.Points)
	if !ok || len(pts) != 2 || math.IsNaN(d.Rotation) || math.IsInf(d.Rotation, 0) {
		return true
	}
	x0, x1 := math.Min(pts[0].X, pts[1].X), math.Max(pts[0].X, pts[1].X)
	y0, y1 := math.Min(pts[0].Y, pts[1].Y), math.Max(pts[0].Y, pts[1].Y)
	if !pictureIsTurned(d) {
		for _, a := range areas {
			// Positive-area overlap: a picture lying edge to edge with a
			// shadow shows none of the land under it.
			if x0 < a.MaxX && x1 > a.MinX && y0 < a.MaxY && y1 > a.MinY {
				return true
			}
		}
		return false
	}
	if !frame.known() {
		return true
	}
	quad := turnedBox(x0/100*frame.W, y0/100*frame.H, x1/100*frame.W, y1/100*frame.H, d.Rotation)
	for _, a := range areas {
		box := [4]pointXY{
			{a.MinX / 100 * frame.W, a.MinY / 100 * frame.H}, {a.MaxX / 100 * frame.W, a.MinY / 100 * frame.H},
			{a.MaxX / 100 * frame.W, a.MaxY / 100 * frame.H}, {a.MinX / 100 * frame.W, a.MaxY / 100 * frame.H},
		}
		if convexOverlap(quad, box) {
			return true
		}
	}
	return false
}

// turnedBox returns the corners of the box (x0,y0)-(x1,y1) turned deg
// clockwise on screen (y grows downwards) about its centre, as the viewer's
// CSS rotate() draws it.
func turnedBox(x0, y0, x1, y1, deg float64) [4]pointXY {
	cx, cy, hw, hh := (x0+x1)/2, (y0+y1)/2, (x1-x0)/2, (y1-y0)/2
	rad := deg * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	var out [4]pointXY
	for i, c := range [4][2]float64{{-hw, -hh}, {hw, -hh}, {hw, hh}, {-hw, hh}} {
		out[i] = pointXY{X: cx + c[0]*cos - c[1]*sin, Y: cy + c[0]*sin + c[1]*cos}
	}
	return out
}

// convexOverlap reports whether two convex quadrilaterals share a region of
// positive area, by the separating axis test: they do unless, along the
// normal of some edge, their shadows on that axis at most touch.
func convexOverlap(p, q [4]pointXY) bool {
	for _, poly := range [2][4]pointXY{p, q} {
		for i := range poly {
			e := poly[(i+1)%4]
			nx, ny := -(e.Y - poly[i].Y), e.X-poly[i].X
			if nx == 0 && ny == 0 {
				continue
			}
			pMin, pMax := project(p, nx, ny)
			qMin, qMax := project(q, nx, ny)
			if pMax <= qMin || qMax <= pMin {
				return false
			}
		}
	}
	return true
}

func project(poly [4]pointXY, nx, ny float64) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range poly {
		d := v.X*nx + v.Y*ny
		lo, hi = math.Min(lo, d), math.Max(hi, d)
	}
	return lo, hi
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
func ShadowWithholdsImageOf(areas []ShadowArea, d *Drawing, frame MapFrame) bool {
	return shadowWithholdsImageOf(areas, d, frame)
}
