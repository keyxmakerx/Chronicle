package records

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/hexsketch"
)

// hexPreview is the picture data of a hex-map row, read by the hex map preview
// widget (static/js/widgets/hex_map_preview.js). Hexes are short arrays, not
// objects, because a 100 by 100 map has ten thousand of them and the data
// sits in the row's markup: [col, row, land, road, river, trail, name], where
// land is an index into Lands (-1 for none), a line is its exit mask or null,
// and name is trailing and left off when the hex has none.
type hexPreview struct {
	Cols, Rows int
	// Art is the map's own art style on an update and "detailed" for a new map.
	Art    string
	Update bool
	Lands  []string
	// Cells is the whole block on a new map, and the hexes already stored on an
	// update (the Before picture).
	Cells [][]any
	// After holds, on an update, each hex the block touches as it will be once
	// the block is applied; every other hex is as stored.
	After [][]any
	// Ring lists the stored hexes that would change.
	Ring [][2]int
}

func (p hexPreview) MarshalJSON() ([]byte, error) {
	type wire struct {
		Cols   int      `json:"cols"`
		Rows   int      `json:"rows"`
		Art    string   `json:"art"`
		Update bool     `json:"update,omitempty"`
		Lands  []string `json:"lands"`
		Cells  [][]any  `json:"cells"`
		After  [][]any  `json:"after,omitempty"`
		Ring   [][2]int `json:"ring,omitempty"`
	}
	return json.Marshal(wire{p.Cols, p.Rows, p.Art, p.Update, p.Lands, p.Cells, p.After, p.Ring})
}

func previewCell(c HexCellState) []any {
	land := -1
	if c.Terrain != "" {
		land = slices.Index(hexsketch.Lands, c.Terrain)
	}
	out := []any{c.Col, c.Row, land, c.Road, c.River, c.Trail}
	if c.Name != "" {
		out = append(out, c.Name)
	}
	return out
}

func (p hexPreview) encode() string {
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// stateOf is a block's hex as the shape a stored hex has.
func stateOf(c hexsketch.Cell) HexCellState {
	return HexCellState{Col: c.Col, Row: c.Row, Terrain: c.Terrain, Road: c.Road, River: c.River, Trail: c.Trail, Name: c.Name, Notes: c.Notes}
}

// createPreview is the picture of a new map: the block alone, in the default
// art, over the size the block names.
func createPreview(res hexsketch.Result) string {
	p := hexPreview{Cols: res.Cols, Rows: res.Rows, Art: "detailed", Lands: hexsketch.Lands, Cells: make([][]any, 0, len(res.Cells))}
	for _, c := range res.Cells {
		p.Cells = append(p.Cells, previewCell(stateOf(c)))
	}
	return p.encode()
}

// updatePreview is the picture of a map being painted on: what is stored, what
// the block makes of each hex it touches, and the stored hexes it would
// change. A map whose field is unknown (state.Cols is 0) is drawn over the
// size the block names.
func updatePreview(res hexsketch.Result, have map[hexsketch.Hex]HexCellState, state HexMapState) string {
	p := hexPreview{Cols: state.Cols, Rows: state.Rows, Art: state.Art, Update: true, Lands: hexsketch.Lands}
	if p.Cols == 0 || p.Rows == 0 {
		p.Cols, p.Rows = res.Cols, res.Rows
		for _, c := range state.Cells {
			p.Cols, p.Rows = max(p.Cols, c.Col+1), max(p.Rows, c.Row+1)
		}
	}
	if p.Art == "" {
		p.Art = "detailed"
	}
	p.Cells = make([][]any, 0, len(state.Cells))
	for _, c := range state.Cells {
		p.Cells = append(p.Cells, previewCell(c))
	}
	for _, c := range res.Cells {
		old, stored := have[hexsketch.Hex{Col: c.Col, Row: c.Row}]
		p.After = append(p.After, previewCell(merged(old, c)))
		if stored && wouldChange(old, c) {
			p.Ring = append(p.Ring, [2]int{c.Col, c.Row})
		}
	}
	return p.encode()
}

// mergeLine is a hex's line once the block's exits are added to the stored
// ones; a line the block does not draw is left as stored.
func mergeLine(added, have *int) *int {
	if added == nil {
		return have
	}
	return orMask(added, have)
}

// merged is a stored hex after the block paints it, by the rules Apply writes
// with: land replaces land, lines add their exits, a place renames the hex and
// replaces its notes only when it carries some.
func merged(old HexCellState, c hexsketch.Cell) HexCellState {
	out := old
	out.Col, out.Row = c.Col, c.Row
	if c.Terrain != "" {
		out.Terrain = c.Terrain
	}
	out.Road, out.River, out.Trail = mergeLine(c.Road, old.Road), mergeLine(c.River, old.River), mergeLine(c.Trail, old.Trail)
	if c.Name != "" {
		out.Name = c.Name
		if c.Notes != "" {
			out.Notes = c.Notes
		}
	}
	return out
}

// wouldChange says whether painting the block's hex over a stored one takes
// something away: other land, or a name or notes a place replaces with
// different words. A hex that only gains a line or a name, or gets the land it
// already has, loses nothing.
func wouldChange(old HexCellState, c hexsketch.Cell) bool {
	if old.Terrain != "" && c.Terrain != "" && old.Terrain != c.Terrain {
		return true
	}
	if c.Name == "" {
		return false
	}
	if old.Name != "" && old.Name != c.Name {
		return true
	}
	return c.Notes != "" && old.Notes != "" && old.Notes != c.Notes
}

// ringedLine says how many hexes the picture rings.
func ringedLine(n int) string {
	if n == 1 {
		return "The 1 hex that would change is ringed on the map."
	}
	return fmt.Sprintf("The %d hexes that would change are ringed on the map.", n)
}

// outsideField warns when the block names hexes beyond the map's own hex field,
// which are saved but never drawn. "" when none, or when the field is not known.
func outsideField(res hexsketch.Result, state HexMapState) string {
	if state.Cols == 0 || state.Rows == 0 {
		return ""
	}
	n := 0
	for _, c := range res.Cells {
		if c.Col >= state.Cols || c.Row >= state.Rows {
			n++
		}
	}
	switch n {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("1 hex in this block lies outside the map's own hex field (%d across by %d down) and will not show on the map", state.Cols, state.Rows)
	}
	return fmt.Sprintf("%d hexes in this block lie outside the map's own hex field (%d across by %d down) and will not show on the map", n, state.Cols, state.Rows)
}
