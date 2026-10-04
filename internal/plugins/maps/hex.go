package maps

import (
	"math"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// This file holds the hex layer's types and its pure rules: the hex maths and
// the one function that decides which painted hexes a viewer may receive.
// Keeping both free of I/O lets the service, the handler and the tests share a
// single definition, and lets static/js/map_hexes.js be checked against the
// same golden vectors.

// Terrain kinds a hex may carry. The set is an allowlist enforced here (the
// column is a plain VARCHAR) so a new kind is a code change, not a migration.
// It matches the viewer's palette; "road" is drawn as a line through its hexes
// rather than as a fill.
const (
	TerrainPlains   = "plains"
	TerrainForest   = "forest"
	TerrainHills    = "hills"
	TerrainMountain = "mountain"
	TerrainWater    = "water"
	TerrainSwamp    = "swamp"
	TerrainDesert   = "desert"
	TerrainSnow     = "snow"
	TerrainTown     = "town"
	TerrainRoad     = "road"
)

// hexTerrains is the closed set of terrain kinds, in palette order.
var hexTerrains = []string{
	TerrainPlains, TerrainForest, TerrainHills, TerrainMountain, TerrainWater,
	TerrainSwamp, TerrainDesert, TerrainSnow, TerrainTown, TerrainRoad,
}

// IsValidTerrain reports whether t is one of the allowed terrain kinds.
func IsValidTerrain(t string) bool {
	for _, k := range hexTerrains {
		if k == t {
			return true
		}
	}
	return false
}

// Limits on what a hex may hold and what one request may change. The cell cap
// bounds a layer to roughly a 200 x 200 field so one map cannot grow without
// limit. MaxHexCoord is the highest valid column or row index, so a field is at
// most MaxHexCoord+1 (400) hexes along either axis; static/js/map_hexes.js
// clamps its field to the same MAX_HEX_AXIS so the client never offers a hex
// the server would refuse. A field of that size can hold more hexes than the
// cell cap, which only limits how many are painted.
const (
	MaxHexBatch       = 500
	MaxHexNameRunes   = 120
	MaxHexNotesRunes  = 2000
	MaxHexCoord       = 399
	MaxHexCellsPerMap = 40000
)

// HexLayer is the per-map hex settings row. A map without a row has an
// implicit layer at version 0 (see DefaultHexLayer); the row appears on the
// first write. Fields past the cells are carried now so later slices (fog,
// party, travel, pinning to a picture) need no schema change.
type HexLayer struct {
	MapID           string    `json:"map_id"`
	AnchorDrawingID *string   `json:"anchor_drawing_id"`
	FogEnabled      bool      `json:"fog_enabled"`
	PartyCol        *int      `json:"party_col"`
	PartyRow        *int      `json:"party_row"`
	MilesPerHex     int       `json:"miles_per_hex"`
	MilesPerDay     int       `json:"miles_per_day"`
	Version         uint64    `json:"version"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DefaultHexLayer is the layer of a map nobody has painted yet.
func DefaultHexLayer(mapID string) HexLayer {
	return HexLayer{MapID: mapID, MilesPerHex: 6, MilesPerDay: 24}
}

// HexCell is one painted or annotated hex. Only hexes someone has touched have
// a row. Name and Notes are plain text: they are stored as typed and escaped
// by whatever renders them.
type HexCell struct {
	Col       int       `json:"col"`
	Row       int       `json:"row"`
	Terrain   *string   `json:"terrain"`
	Piece     *int      `json:"piece"`
	Name      string    `json:"name"`
	Notes     *string   `json:"notes"`
	Explored  bool      `json:"explored"`
	UpdatedBy *string   `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

// HexKey identifies a hex by its offset coordinates.
type HexKey struct{ Col, Row int }

// VisibleCells returns the cells role may receive. role is the viewer's
// VisibilityRole, so a co-DM grant already arrives promoted to owner.
//
// With fog off everyone sees everything. With fog on, only whoever passes
// CanSeeDmOnly sees every row; everyone else, scribes included, gets explored
// rows only and nothing that hints at the rest. Scribes are filtered on
// purpose: an unexplored hex is a secret from the whole table, and a scribe
// can write on the map without being the DM.
func VisibleCells(layer HexLayer, cells []HexCell, role int) []HexCell {
	out := make([]HexCell, 0, len(cells))
	if !layer.FogEnabled || permissions.CanSeeDmOnly(role) {
		return append(out, cells...)
	}
	for _, c := range cells {
		if c.Explored {
			out = append(out, c)
		}
	}
	return out
}

// UsableAnchor reports whether d can still carry a layer on mapID: it must
// exist, sit on this map and be a picture. The anchor column has no foreign
// key (so deleting a picture never cascades into painted terrain), which means
// a stale value is normal and is read as "the whole map", never as an error.
func UsableAnchor(mapID string, d *Drawing) bool {
	return d != nil && d.MapID == mapID && d.DrawingType == DrawingTypeImage
}

// AnchorHidesLayer reports whether the layer must be withheld from role
// because its picture is hidden from them. A hex field pinned to a DM-only
// picture would otherwise show players the outline, and any painted terrain, of
// something they are not meant to know exists.
func AnchorHidesLayer(anchor *Drawing, role int) bool {
	return anchor != nil && anchor.Visibility == "dm_only" && !permissions.CanSeeDmOnly(role)
}

// --- Hex maths ---
//
// Hexes are pointy-top with odd rows shifted half a hex to the right ("odd-r"
// offset coordinates), exactly as the plain hex grid in map_viewer.js draws
// them. Distances and neighbours go through cube coordinates (x+y+z = 0),
// where they are simple.

// Cube is a hex in cube coordinates.
type Cube struct{ X, Y, Z int }

// OffsetToCube converts offset (col, row) to cube coordinates.
func OffsetToCube(col, row int) Cube {
	x := col - (row-(row&1))/2
	return Cube{X: x, Y: -x - row, Z: row}
}

// CubeToOffset converts cube coordinates back to offset (col, row).
func CubeToOffset(c Cube) (col, row int) {
	return c.X + (c.Z-(c.Z&1))/2, c.Z
}

// cubeDirs are the six neighbours' cube offsets, east first and turning
// clockwise on screen.
var cubeDirs = [6]Cube{{1, -1, 0}, {0, -1, 1}, {-1, 0, 1}, {-1, 1, 0}, {0, 1, -1}, {1, 0, -1}}

// Neighbors returns the six hexes around (col, row), east first and turning
// clockwise. Results may have negative coordinates at the field's edge; the
// caller decides whether those exist.
func Neighbors(col, row int) [6]HexKey {
	c := OffsetToCube(col, row)
	var out [6]HexKey
	for i, d := range cubeDirs {
		oc, or := CubeToOffset(Cube{c.X + d.X, c.Y + d.Y, c.Z + d.Z})
		out[i] = HexKey{oc, or}
	}
	return out
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// HexDistance is the number of steps between two hexes.
func HexDistance(a, b HexKey) int {
	p, q := OffsetToCube(a.Col, a.Row), OffsetToCube(b.Col, b.Row)
	d := absInt(p.X - q.X)
	if v := absInt(p.Y - q.Y); v > d {
		d = v
	}
	if v := absInt(p.Z - q.Z); v > d {
		d = v
	}
	return d
}

// HexGeometry places the hex field on the map in map pixels. R is the
// corner radius; hex width is sqrt(3)*R and rows are 1.5*R apart. (Ox, Oy) is
// the centre of hex (0, 0). Cols and Rows bound the field.
type HexGeometry struct {
	R, Ox, Oy  float64
	Cols, Rows int
}

// NewHexGeometry lays the field over the whole map picture. gridSize is the
// stored grid.size, measured as if the map were 1000 wide, so the same number
// looks the same on any picture; the field starts with hex (0, 0) centred on
// the map's top-left corner and covers the same range the plain hex grid
// draws.
func NewHexGeometry(gridSize float64, mapW, mapH float64) HexGeometry {
	r := gridSize * mapW / 1000 / 2
	w := math.Sqrt(3) * r
	return HexGeometry{
		R:    r,
		Cols: int(math.Ceil(mapW/w)) + 1,
		Rows: int(math.Ceil((mapH + r) / (1.5 * r))),
	}
}

// NewAnchoredHexGeometry lays the field inside a picture's box (boxX, boxY,
// boxW, boxH in map pixels) instead of over the whole map. gridSize is
// measured as if the picture were 1000 wide, so every length scales with the
// box: resize the picture and the hexes grow with it, with the same columns,
// rows and keys. Hex (0, 0) is the first hex that fits whole at the box's
// top-left, and the field stops before a hex would cross the right or bottom
// edge, so nothing spills off the picture. static/js/map_hexes.js builds the
// same field (anchoredGeometry) in a box 1000 wide and scales it.
func NewAnchoredHexGeometry(gridSize, boxX, boxY, boxW, boxH float64) HexGeometry {
	r := gridSize * boxW / 1000 / 2
	if r <= 0 || boxW <= 0 || boxH <= 0 {
		return HexGeometry{}
	}
	w := math.Sqrt(3) * r
	return HexGeometry{
		R:    r,
		Ox:   boxX + w/2,
		Oy:   boxY + r,
		Cols: maxInt(1, int(math.Floor((boxW-1.5*w)/w))+1),
		Rows: maxInt(1, int(math.Floor((boxH-2*r)/(1.5*r)))+1),
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Width is a hex's width.
func (g HexGeometry) Width() float64 { return math.Sqrt(3) * g.R }

// Center is the centre of hex (col, row) in map pixels.
func (g HexGeometry) Center(col, row int) (x, y float64) {
	w := g.Width()
	x = g.Ox + float64(col)*w
	if row&1 == 1 {
		x += w / 2
	}
	return x, g.Oy + float64(row)*1.5*g.R
}

// jsRound rounds halves toward +infinity like JavaScript's Math.round, so a
// point exactly on a hex border lands in the same hex here as in the viewer.
func jsRound(v float64) float64 { return math.Floor(v + 0.5) }

// HexAtPoint returns the hex under a map point, or ok=false when the point
// falls outside the field. It rounds in cube space so the result is exact on
// hex borders rather than approximated with rectangles.
func (g HexGeometry) HexAtPoint(x, y float64) (col, row int, ok bool) {
	if g.R <= 0 {
		return 0, 0, false
	}
	x -= g.Ox
	y -= g.Oy
	q := (math.Sqrt(3)/3*x - y/3) / g.R
	rr := (2.0 / 3 * y) / g.R
	cx, cz := q, rr
	cy := -cx - cz
	rx, ry, rz := jsRound(cx), jsRound(cy), jsRound(cz)
	dx, dy, dz := math.Abs(rx-cx), math.Abs(ry-cy), math.Abs(rz-cz)
	if dx > dy && dx > dz {
		rx = -ry - rz
	} else if dy <= dz {
		rz = -rx - ry
	}
	row = int(rz)
	col = int(rx) + (row-(row&1))/2
	if col < 0 || row < 0 || col >= g.Cols || row >= g.Rows {
		return 0, 0, false
	}
	return col, row, true
}
