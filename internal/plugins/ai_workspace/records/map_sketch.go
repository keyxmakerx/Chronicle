package records

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/mapsketch"
)

// MapSketchAPI is what a map sketch needs from the maps plugin beyond pins,
// in this package's own types (adapted in app wiring).
type MapSketchAPI interface {
	// CreateGridMap makes a blank map of width by height pixels with a square
	// grid of gridSize (measured as if the map were 1000 wide) and returns
	// its id.
	CreateGridMap(ctx context.Context, campaignID, name string, width, height, gridSize int) (string, error)
	// Draw adds one drawing through the same checks the map page's tools get.
	Draw(ctx context.Context, mapID string, a Actor, d SketchDrawing) error
}

// SketchDrawing is one drawing a sketch makes, in map percent (0 to 100).
type SketchDrawing struct {
	Type        string
	Points      []SketchPoint
	StrokeColor string
	StrokeWidth float64
	Text        *string
	FontSize    *int
	Visibility  string
}

// SketchPoint is a point in map percent.
type SketchPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// The sketch's grid bounds. A square is 1000/width pixels on a map 1000
// wide, and the maps plugin keeps a grid square between 10 and 300, so a
// sketch 4 to 100 squares across draws exactly on the map's own grid.
const (
	sketchGridMin = 4
	sketchGridMax = 100
	// sketchMapWidth is the blank map's width; the grid is measured against it.
	sketchMapWidth = 1000
)

// The look of what a sketch draws that is not a box (boxes take their look
// from the viewer, which follows the theme). Lines and labels keep their
// stored colour, so these are mid tones that read on a light or dark sheet.
const (
	sketchWallColor  = "#6b7280"
	sketchRoadColor  = "#78716c"
	sketchPathColor  = "#a8a29e"
	sketchLabelColor = "#64748b"
	sketchBoxStroke  = "#334155"
)

// sketchShape is one parsed sketch line, in grid squares. The parser lives in
// internal/mapsketch so the page map block reads the same words.
type sketchShape struct{ mapsketch.Entry }

// sketchPlan is a parsed sketch: the shapes that will be drawn and a plain
// line for each line that will not.
type sketchPlan struct {
	W, H   int
	Shapes []sketchShape
	Errs   []string
}

func (p sketchPlan) counts() (shapes, pins int) {
	for _, s := range p.Shapes {
		if s.Shape == "pin" {
			pins++
		} else {
			shapes++
		}
	}
	return shapes, pins
}

// parseSketchGrid reads "40 x 26".
func parseSketchGrid(s string) (w, h int, err error) {
	w, h, err = mapsketch.ParseGridSize(s)
	if err != nil {
		return 0, 0, badRequestf("%s", err.Error())
	}
	return w, h, nil
}

// parseSketch reads a sketch body. A line it cannot draw is reported and
// skipped; it never stops the rest of the map.
func parseSketch(body string, w, h int) sketchPlan {
	mp := mapsketch.ParseEntries(body, w, h)
	p := sketchPlan{W: w, H: h, Errs: mp.Errs}
	for _, e := range mp.Entries {
		p.Shapes = append(p.Shapes, sketchShape{e})
	}
	return p
}

// parseSketchLine reads one line, or says in plain words why it cannot.
func parseSketchLine(n int, l string) (sketchShape, string) {
	e, msg := mapsketch.ParseEntry(n, l)
	return sketchShape{e}, msg
}

// sketchGridSize is a square's size as the map's grid setting measures it.
func sketchGridSize(w int) int { return sketchMapWidth / w }

// toPercent converts a grid point to map percent. Squares are gridSize
// pixels on a map sketchMapWidth wide and h*gridSize high.
func toPercent(pt [2]float64, w, h int) SketchPoint {
	size := float64(sketchGridSize(w))
	return SketchPoint{
		X: round3(pt[0] * size / sketchMapWidth * 100),
		Y: round3(pt[1] / float64(h) * 100),
	}
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

// drawing is the map drawing one parsed shape becomes; pins are not drawings.
func (s sketchShape) drawing(w, h int, vis string) SketchDrawing {
	pts := make([]SketchPoint, len(s.Points))
	for i, pt := range s.Points {
		pts[i] = toPercent(pt, w, h)
	}
	d := SketchDrawing{Points: pts, Visibility: vis}
	name := s.Name
	switch s.Shape {
	case "box":
		d.Type, d.StrokeColor, d.StrokeWidth = s.Verb, sketchBoxStroke, 2
		if s.Verb != "wall" && name != "" {
			d.Text = &name
		}
	case "circle":
		// An ellipse is stored as its centre and a point on its edge.
		edge := toPercent([2]float64{s.Points[0][0] + s.Radius, s.Points[0][1]}, w, h)
		d.Type, d.StrokeColor, d.StrokeWidth = "ellipse", sketchWallColor, 4
		d.Points = []SketchPoint{pts[0], edge}
	case "line":
		d.Type = "freehand"
		switch s.Verb {
		case "road":
			d.StrokeColor, d.StrokeWidth = sketchRoadColor, 5
		case "path":
			d.StrokeColor, d.StrokeWidth = sketchPathColor, 3
		default:
			d.StrokeColor, d.StrokeWidth = sketchWallColor, 4
		}
	case "label":
		size := 14
		d.Type, d.StrokeColor, d.StrokeWidth, d.Text, d.FontSize = "text", sketchLabelColor, 2, &name, &size
	}
	return d
}

// MapSketchKind makes a new map from a sketch on a grid of squares: rooms,
// areas, water, danger zones, walls, paths, labels and pins, each an
// ordinary shape or pin the map page can then move or delete.
type MapSketchKind struct {
	Svc  MapSketchAPI
	Pins MapsAPI
}

func (MapSketchKind) Name() string  { return "map" }
func (MapSketchKind) Label() string { return "Map sketch" }
func (MapSketchKind) Doc() string {
	return "A new map drawn from a sketch on a grid of squares. Keys: `name`, `grid` (squares across x down, 4 to 100 each way, like `40 x 26`), optional `visibility: dm_only` (hides every shape and pin from players; the map itself is still listed). Only `action: create`: a sketch always makes a new map. Each body line is one thing, with points as `x,y` in squares from the top-left (0,0) to the bottom-right (the grid size):\n\n" +
		"- `room \"Name\" x,y to x,y` (also `area`, `water`, `danger`): a box from one corner to the other; the name is optional\n" +
		"- `wall x,y to x,y` a thick wall box, `wall x,y x,y x,y …` a wall line, `wall circle x,y radius r` a round wall\n" +
		"- `road \"Name\" x,y to x,y …` and `path \"Name\" x,y to x,y …`: lines through the points\n" +
		"- `label \"words\" x,y`: words on the map\n" +
		"- `pin \"Name\" x,y [location|danger|treasure|quest|note]: a note for the pin`\n\n" +
		"Lines starting with # are skipped. A line that cannot be drawn is shown at review and left out; the rest of the map is still made.\n\n" +
		"```\n---\nkind: map\nname: The Siege of Harrowmere\ngrid: 40 x 26\n---\nwall circle 20,13 radius 9\nroom \"The keep\" 18,11 to 22,15\narea \"Enemy camp\" 1,1 to 7,5\nwater \"The moat\" 26,2 to 30,5\npath \"Road to the gate\" 20,22 to 20,26\nlabel \"enemy fires all round\" 2,24\npin \"The banner\" 20,9 danger: Red, on the keep's tower.\npin \"The gate\" 20,22: Where the herald comes with terms at dusk.\n```"
}

// plan parses the record and checks what blocks it; bad lines are only
// warnings, so a mostly good sketch still makes its map.
func (k MapSketchKind) plan(a Actor, r Record) (sketchPlan, string) {
	if r.Action != ActionCreate {
		return sketchPlan{}, "a map sketch only makes new maps; change an existing map on its map page"
	}
	if r.Name == "" {
		return sketchPlan{}, "a map sketch needs `name`"
	}
	if r.Str("grid") == "" {
		return sketchPlan{}, "a map sketch needs `grid`, like 40 x 26"
	}
	w, h, err := parseSketchGrid(r.Str("grid"))
	if err != nil {
		return sketchPlan{}, planError(err)
	}
	vis := r.Str("visibility")
	if vis != "" && vis != "everyone" && vis != "dm_only" {
		return sketchPlan{}, "visibility must be everyone or dm_only"
	}
	if vis == "dm_only" && !a.CanAuthorDmOnly() {
		return sketchPlan{}, "only the owner or a co-DM can add hidden shapes and pins"
	}
	p := parseSketch(r.Body, w, h)
	if len(p.Shapes) == 0 {
		return p, "there is nothing on this sketch that can be drawn"
	}
	return p, ""
}

func (k MapSketchKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	p, blocked := k.plan(a, r)
	if blocked != "" {
		return Plan{Error: blocked, Warnings: p.Errs}
	}
	shapes, pins := p.counts()
	out := Plan{Summary: fmt.Sprintf("makes a map with %s and %s on a %d by %d grid",
		plural(shapes, "shape", "shapes"), plural(pins, "pin", "pins"), p.W, p.H), Warnings: p.Errs}
	if r.Str("visibility") == "dm_only" {
		out.Summary += "; its shapes and pins are hidden from players"
	}
	if ms, err := k.Pins.ListMaps(ctx, campaignID); err == nil {
		for _, m := range ms {
			if m.CampaignID == campaignID && sameName(m.Name, r.Name) {
				out.Warnings = append(out.Warnings, "A map with this name already exists; this makes another")
				break
			}
		}
	}
	return out
}

// Apply makes the map, then draws every line that planned cleanly. A shape
// the maps plugin still refuses does not undo the map: the error names what
// is missing so it can be drawn by hand.
func (k MapSketchKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, blocked := k.plan(a, r)
	if blocked != "" {
		return apperror.NewBadRequest(blocked)
	}
	size := sketchGridSize(p.W)
	mapID, err := k.Svc.CreateGridMap(ctx, campaignID, r.Name, sketchMapWidth, p.H*size, size)
	if err != nil {
		return err
	}
	vis := r.Str("visibility")
	var failed []string
	for _, s := range p.Shapes {
		if s.Shape == "pin" {
			continue
		}
		if err := k.Svc.Draw(ctx, mapID, a, s.drawing(p.W, p.H, vis)); err != nil {
			failed = append(failed, fmt.Sprintf("line %d (%s)", s.Line, planError(err)))
		}
	}
	for _, s := range p.Shapes {
		if s.Shape != "pin" {
			continue
		}
		pt := toPercent(s.Points[0], p.W, p.H)
		in := PinInput{Name: s.Name, Category: s.Category, Visibility: vis, X: &pt.X, Y: &pt.Y}
		if s.Note != "" {
			note := s.Note
			in.Description = &note
		}
		if err := k.Pins.CreatePin(ctx, mapID, a.UserID, in); err != nil {
			failed = append(failed, fmt.Sprintf("line %d (%s)", s.Line, planError(err)))
		}
	}
	if len(failed) > 0 {
		return badRequestf("made the %s map, but could not draw %s", r.Name, strings.Join(failed, "; "))
	}
	return nil
}

// Export lists nothing: a sketch only makes new maps, and pins on existing
// maps are listed by the pin kind.
func (MapSketchKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
