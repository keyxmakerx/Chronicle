package pageblocks

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/keyxmakerx/chronicle/internal/mapsketch"
)

// validatePageMap decodes raw strictly and checks it as a page map.
func validatePageMap(raw json.RawMessage) (mapsketch.Doc, error) {
	var d mapsketch.Doc
	if err := decodeStrict(raw, &d); err != nil {
		return mapsketch.Doc{}, err
	}
	return checkPageMap(d)
}

// checkPageMap returns the document as it will be stored. It copies field by
// field, so anything the client sent that is not part of the stored shape (the
// read-only entityName and entityUrl, a page name) never reaches the database.
func checkPageMap(d mapsketch.Doc) (mapsketch.Doc, error) {
	if (d.Grid != mapsketch.Grid{}) && d.Grid != (mapsketch.Grid{W: mapsketch.GridW, H: mapsketch.GridH}) {
		return mapsketch.Doc{}, errInvalid(fmt.Sprintf("a page map is %d by %d squares", mapsketch.GridW, mapsketch.GridH))
	}
	bg := d.Background
	if bg == "" {
		bg = mapsketch.BackgroundNone
	}
	if !mapsketch.ValidBackground(bg) {
		return mapsketch.Doc{}, errInvalid("background must be none, dots, grid or hex")
	}
	if len(d.Shapes) > mapsketch.MaxShapes {
		return mapsketch.Doc{}, errInvalid(fmt.Sprintf("a page map holds at most %d shapes", mapsketch.MaxShapes))
	}
	if len(d.Pins) > mapsketch.MaxPins {
		return mapsketch.Doc{}, errInvalid(fmt.Sprintf("a page map holds at most %d markers", mapsketch.MaxPins))
	}
	out := mapsketch.NewDoc()
	out.Background = bg
	seen := make(map[string]bool, len(d.Shapes)+len(d.Pins))
	for _, s := range d.Shapes {
		if !validToken(s.ID) || seen[s.ID] {
			return mapsketch.Doc{}, errInvalid("shape ids must be short, unique and plain")
		}
		seen[s.ID] = true
		c, err := checkShape(s)
		if err != nil {
			return mapsketch.Doc{}, err
		}
		out.Shapes = append(out.Shapes, c)
	}
	for _, p := range d.Pins {
		if !validToken(p.ID) || seen[p.ID] {
			return mapsketch.Doc{}, errInvalid("marker ids must be short, unique and plain")
		}
		seen[p.ID] = true
		c, err := checkPin(p)
		if err != nil {
			return mapsketch.Doc{}, err
		}
		out.Pins = append(out.Pins, c)
	}
	return out, nil
}

// inGrid reports a point on the canvas. NaN and infinities fail every
// comparison, so they are refused here too.
func inGrid(x, y float64) bool {
	return x >= 0 && x <= mapsketch.GridW && y >= 0 && y <= mapsketch.GridH &&
		!math.IsNaN(x) && !math.IsNaN(y)
}

func checkColor(c string) (string, error) {
	if c == "" {
		return "", nil
	}
	norm, problem := mapsketch.NormalizeColor(c)
	if problem != "" {
		return "", errInvalid("that colour isn't allowed")
	}
	return norm, nil
}

func checkShape(s mapsketch.Shape) (mapsketch.Shape, error) {
	if !mapsketch.IsShapeKind(s.T) {
		return mapsketch.Shape{}, errInvalid("unknown shape kind")
	}
	name, err := cleanText("shape name", s.Name, mapsketch.MaxName, false)
	if err != nil {
		return mapsketch.Shape{}, err
	}
	color, err := checkColor(s.Color)
	if err != nil {
		return mapsketch.Shape{}, err
	}
	out := mapsketch.Shape{ID: s.ID, T: s.T, Name: name, Color: color}
	switch {
	case mapsketch.IsBoxShape(s.T):
		const eps = 1e-9
		if len(s.Pts) != 0 || !inGrid(s.X, s.Y) || !(s.W > 0) || !(s.H > 0) ||
			s.X+s.W > mapsketch.GridW+eps || s.Y+s.H > mapsketch.GridH+eps {
			return mapsketch.Shape{}, errInvalid("a shape runs off the map or has no size")
		}
		out.X, out.Y, out.W, out.H = s.X, s.Y, s.W, s.H
	case s.T == mapsketch.ShapeLabel:
		if len(s.Pts) != 0 || !inGrid(s.X, s.Y) {
			return mapsketch.Shape{}, errInvalid("a label sits off the map")
		}
		if name == "" {
			return mapsketch.Shape{}, errInvalid("a label needs its words")
		}
		out.X, out.Y = s.X, s.Y
	default:
		if len(s.Pts) < 2 || len(s.Pts) > mapsketch.MaxPoints {
			return mapsketch.Shape{}, errInvalid(fmt.Sprintf("a line needs 2 to %d points", mapsketch.MaxPoints))
		}
		for _, p := range s.Pts {
			if !inGrid(p[0], p[1]) {
				return mapsketch.Shape{}, errInvalid("a line runs off the map")
			}
		}
		out.Pts = append([][2]float64(nil), s.Pts...)
	}
	return out, nil
}

func checkPin(p mapsketch.Pin) (mapsketch.Pin, error) {
	if !inGrid(p.X, p.Y) {
		return mapsketch.Pin{}, errInvalid("a marker sits off the map")
	}
	name, err := cleanText("marker name", p.Name, mapsketch.MaxName, false)
	if err != nil {
		return mapsketch.Pin{}, err
	}
	text, err := cleanText("marker note", p.Text, mapsketch.MaxNote, false)
	if err != nil {
		return mapsketch.Pin{}, err
	}
	color, err := checkColor(p.Color)
	if err != nil {
		return mapsketch.Pin{}, err
	}
	if p.EntityID != "" && !validToken(p.EntityID) {
		return mapsketch.Pin{}, errInvalid("a marker links to a page that doesn't exist")
	}
	out := mapsketch.Pin{ID: p.ID, X: p.X, Y: p.Y, Name: name, Style: p.Style, EntityID: p.EntityID, Text: text, DM: p.DM, Color: color}
	switch p.Style {
	case mapsketch.StyleNumber:
		// A number marker has no icon; one sent along is dropped.
	case mapsketch.StyleIcon:
		if !mapsketch.ValidIcon(p.Icon) {
			return mapsketch.Pin{}, errInvalid("a marker icon isn't one of the allowed icons")
		}
		out.Icon = p.Icon
	default:
		return mapsketch.Pin{}, errInvalid("a marker is a number or an icon")
	}
	return out, nil
}

// pinEntityIDs lists the distinct pages the markers link to.
func pinEntityIDs(d mapsketch.Doc) []string {
	seen := map[string]bool{}
	var ids []string
	for _, p := range d.Pins {
		if p.EntityID != "" && !seen[p.EntityID] {
			seen[p.EntityID] = true
			ids = append(ids, p.EntityID)
		}
	}
	return ids
}

// hasDMPins reports a DM-only marker.
func hasDMPins(d mapsketch.Doc) bool {
	for _, p := range d.Pins {
		if p.DM {
			return true
		}
	}
	return false
}
