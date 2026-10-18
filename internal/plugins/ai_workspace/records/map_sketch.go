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
	// Inspect reads, without changing anything, an existing map's picture
	// size in pixels (0 when unknown) and how many drawings an earlier sketch
	// left on it.
	Inspect(ctx context.Context, mapID string, a Actor) (SketchTarget, error)
	// SketchLayer returns the id of the map's sketch layer, making it when
	// the map has none. Every drawing a sketch makes goes on it, which is the
	// only way a later sketch can tell its own shapes from the ones people
	// drew.
	SketchLayer(ctx context.Context, mapID string) (string, error)
	// ClearSketch deletes every drawing on the sketch layer, whoever drew it,
	// and returns how many it removed.
	ClearSketch(ctx context.Context, mapID, layerID string, a Actor) (int, error)
}

// SketchTarget is what a sketch update needs to know about its map.
type SketchTarget struct {
	Width, Height int // picture size in pixels; 0 when the map has none
	Sketched      int // drawings an earlier sketch left on the sketch layer
}

// SketchDrawing is one drawing a sketch makes, in map percent (0 to 100).
type SketchDrawing struct {
	LayerID     string // the sketch layer; empty draws on no layer
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

// sketchMapWidth is the blank map's width; the grid is measured against it. A
// square is 1000/width pixels, and the maps plugin keeps a grid square between
// 10 and 300, so a sketch mapsketch.GridMin to GridMax squares across draws
// exactly on the map's own grid.
const sketchMapWidth = 1000

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
	return s.drawingWith(w, h, vis, toPercent)
}

// toExactPercent spreads the grid over the whole map: an existing map keeps
// its own grid and picture, so the sketch's squares are only a coordinate
// system, with no pixel rounding to match.
func toExactPercent(pt [2]float64, w, h int) SketchPoint {
	return SketchPoint{X: round3(pt[0] / float64(w) * 100), Y: round3(pt[1] / float64(h) * 100)}
}

// drawingWith is drawing with the grid-to-percent conversion chosen by the
// caller: new maps use their own rounded grid size, existing ones the exact
// spread.
func (s sketchShape) drawingWith(w, h int, vis string, conv func([2]float64, int, int) SketchPoint) SketchDrawing {
	toPercent := conv
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

// SketchLayerName is the layer a sketch's shapes live on. A later sketch
// replaces whatever is on this layer: it cannot tell a shape the AI drew from
// one a person drew onto the same layer, so people should keep their own
// shapes on other layers.
const SketchLayerName = "AI sketch"

// MapSketchKind makes a new map from a sketch on a grid of squares, or adds
// one to an existing map: rooms, areas, water, danger zones, walls, paths,
// labels and pins, each an ordinary shape or pin the map page can then move
// or delete.
type MapSketchKind struct {
	Svc  MapSketchAPI
	Pins MapsAPI
}

func (MapSketchKind) Name() string  { return "map" }
func (MapSketchKind) Label() string { return "Map sketch" }
func (MapSketchKind) Doc() string {
	return "A map drawn from a sketch on a grid of squares: a new map (`action: create`), or shapes and pins added onto a map that exists (`action: update`). Keys: `name`, `grid` (squares across x down, 4 to 100 each way, like `40 x 26`), optional `visibility: dm_only` (hides every shape and pin from players; the map itself is still listed). `action: update` names the map in `name` (or `map`); it never changes the map's picture, which can only be uploaded by hand. On an update the grid is spread over the whole map, so pick one with the picture's proportions (a 1500 x 1000 picture suits `39 x 26`); the review row warns when the squares would stretch. Everything on that map's \"AI sketch\" layer is replaced by the new shapes, including a shape a person drew on that layer; shapes on other layers and pins are not touched. If a shape cannot be drawn, the earlier sketch is already gone and is not restored. A pin whose name is already on the map is left out, so repeating a sketch does not stack pins. Each body line is one thing, with points as `x,y` in squares from the top-left (0,0) to the bottom-right (the grid size):\n\n" +
		"- `room \"Name\" x,y to x,y` (also `area`, `water`, `danger`): a box from one corner to the other; the name is optional\n" +
		"- `wall x,y to x,y` a thick wall box, `wall x,y x,y x,y …` a wall line, `wall circle x,y radius r` a round wall\n" +
		"- `road \"Name\" x,y to x,y …` and `path \"Name\" x,y to x,y …`: lines through the points\n" +
		"- `label \"words\" x,y`: words on the map\n" +
		"- `pin \"Name\" x,y [location|danger|treasure|quest|note]: a note for the pin`\n\n" +
		"Lines starting with # are skipped. A line that cannot be drawn is shown at review and left out; the rest of the map is still made.\n\n" +
		"```\n---\nkind: map\nname: The Siege of Harrowmere\ngrid: 40 x 26\n---\nwall circle 20,13 radius 9\nroom \"The keep\" 18,11 to 22,15\narea \"Enemy camp\" 1,1 to 7,5\nwater \"The moat\" 26,2 to 30,5\npath \"Road to the gate\" 20,22 to 20,26\nlabel \"enemy fires all round\" 2,24\npin \"The banner\" 20,9 danger: Red, on the keep's tower.\npin \"The gate\" 20,22: Where the herald comes with terms at dusk.\n```\n\n" +
		"To sketch onto a map that already exists:\n\n```\n---\nkind: map\naction: update\nname: Imix\ngrid: 39 x 26\n---\nlabel \"Dragon's rest\" 12,8\npin \"Old ferry\" 30,20 location: Closed since the flood.\n```"
}

// sketchName is the map a record names: `map:` on an update, else `name`.
func sketchName(r Record) string {
	if m := r.Str("map"); m != "" {
		return m
	}
	return r.Name
}

// plan parses the record and checks what blocks it; bad lines are only
// warnings, so a mostly good sketch still makes its map.
func (k MapSketchKind) plan(a Actor, r Record) (sketchPlan, string) {
	if r.Action != ActionCreate && r.Action != ActionUpdate {
		return sketchPlan{}, "a map sketch can only create a map or update one with `action: update`; delete a map on its map page"
	}
	if sketchName(r) == "" {
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

// existingTarget is the map an update sketches onto, with what is on it.
type existingTarget struct {
	ref  MapRef
	info SketchTarget
	pins []PinRef
}

// findTarget finds the named map in this campaign; a map that is not there is
// a plain refusal, never a new map.
func (k MapSketchKind) findTarget(ctx context.Context, campaignID string, a Actor, r Record) (*existingTarget, error) {
	name := sketchName(r)
	ms, err := k.Pins.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, badRequestf("could not read the maps")
	}
	var hits []MapRef
	for _, m := range ms {
		if m.CampaignID == campaignID && sameName(m.Name, name) {
			hits = append(hits, m)
		}
	}
	switch len(hits) {
	case 0:
		return nil, badRequestf("no map called %s to sketch onto", quote(name))
	case 1:
	default:
		return nil, badRequestf("more than one map is called %s", quote(name))
	}
	t := &existingTarget{ref: hits[0]}
	if t.info, err = k.Svc.Inspect(ctx, hits[0].ID, a); err != nil {
		return nil, badRequestf("could not read the map %s", quote(name))
	}
	if t.pins, err = k.Pins.ListPins(ctx, campaignID, hits[0].ID, a.Role, a.UserID); err != nil {
		return nil, badRequestf("could not read the pins on %s", quote(name))
	}
	return t, nil
}

// stretchWarning says so when the sketch's squares would not be square on the
// map's picture: the grid is spread over the whole map.
func stretchWarning(w, h int, t SketchTarget) string {
	if t.Width <= 0 || t.Height <= 0 {
		return ""
	}
	// How much taller than wide a square comes out, 1 being square.
	ratio := (float64(t.Height) / float64(h)) / (float64(t.Width) / float64(w))
	if ratio > 0.85 && ratio < 1.18 {
		return ""
	}
	fit := int(math.Round(float64(w) * float64(t.Height) / float64(t.Width)))
	return fmt.Sprintf("The map's picture is %d x %d, so a %d x %d grid stretches the sketch; %d x %d keeps squares square",
		t.Width, t.Height, w, h, w, fit)
}

// newPins are the sketch's pins that the map does not already have.
func newPins(p sketchPlan, have []PinRef) (add []sketchShape, skipped []string) {
	for _, s := range p.Shapes {
		if s.Shape != "pin" {
			continue
		}
		dup := false
		for _, e := range have {
			if sameName(e.Name, s.Name) {
				dup = true
				break
			}
		}
		if dup {
			skipped = append(skipped, s.Name)
		} else {
			add = append(add, s)
		}
	}
	return add, skipped
}

func (k MapSketchKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	p, blocked := k.plan(a, r)
	if blocked != "" {
		return Plan{Error: blocked, Warnings: p.Errs}
	}
	shapes, pins := p.counts()
	if r.Action == ActionUpdate {
		t, err := k.findTarget(ctx, campaignID, a, r)
		if err != nil {
			return Plan{Error: planError(err), Warnings: p.Errs}
		}
		_, skipped := newPins(p, t.pins)
		pins -= len(skipped)
		out := Plan{Summary: fmt.Sprintf("adds %s and %s onto the %s map",
			plural(shapes, "shape", "shapes"), plural(pins, "pin", "pins"), t.ref.Name), Warnings: p.Errs}
		if t.info.Sketched > 0 {
			out.Summary += fmt.Sprintf("; replaces the %s on its AI sketch layer, whoever drew them, and the old ones are not restored if a new shape fails",
				plural(t.info.Sketched, "shape", "shapes"))
		} else {
			out.Summary += "; nothing is replaced, as no earlier sketch is on it"
		}
		if r.Str("visibility") == "dm_only" {
			out.Summary += "; the new shapes and pins are hidden from players"
		}
		if w := stretchWarning(p.W, p.H, t.info); w != "" {
			out.Warnings = append(out.Warnings, w)
		}
		if len(skipped) > 0 {
			out.Warnings = append(out.Warnings, "Left out, already pinned on this map: "+strings.Join(skipped, ", "))
		}
		return out
	}
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

// Apply makes the map (or finds the one to sketch onto), then draws every
// line that planned cleanly. A shape the maps plugin still refuses does not
// undo the map: the error names what is missing so it can be drawn by hand.
func (k MapSketchKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, blocked := k.plan(a, r)
	if blocked != "" {
		return apperror.NewBadRequest(blocked)
	}
	var (
		mapID  string
		conv   = toPercent
		pins   []sketchShape
		layer  string
		update = r.Action == ActionUpdate
	)
	if update {
		t, err := k.findTarget(ctx, campaignID, a, r)
		if err != nil {
			return err
		}
		mapID, conv = t.ref.ID, toExactPercent
		pins, _ = newPins(p, t.pins)
		// The layer is cleared before drawing because ClearSketch removes the
		// whole layer and would take the new shapes with it afterwards. A
		// failed draw therefore leaves no old sketch; only a failure to
		// clear leaves it whole.
		if layer, err = k.Svc.SketchLayer(ctx, mapID); err != nil {
			return badRequestf("could not prepare the %s map for a sketch: %s", t.ref.Name, planError(err))
		}
		if t.info.Sketched > 0 {
			if _, err := k.Svc.ClearSketch(ctx, mapID, layer, a); err != nil {
				return badRequestf("could not clear the earlier sketch on %s: %s", t.ref.Name, planError(err))
			}
		}
	} else {
		size := sketchGridSize(p.W)
		id, err := k.Svc.CreateGridMap(ctx, campaignID, r.Name, sketchMapWidth, p.H*size, size)
		if err != nil {
			return err
		}
		mapID = id
		// A layer that cannot be made leaves the shapes unlayered: they still
		// draw, only a later sketch cannot tell them apart.
		layer, _ = k.Svc.SketchLayer(ctx, mapID)
		for _, s := range p.Shapes {
			if s.Shape == "pin" {
				pins = append(pins, s)
			}
		}
	}
	vis := r.Str("visibility")
	var failed []string
	for _, s := range p.Shapes {
		if s.Shape == "pin" {
			continue
		}
		d := s.drawingWith(p.W, p.H, vis, conv)
		d.LayerID = layer
		if err := k.Svc.Draw(ctx, mapID, a, d); err != nil {
			failed = append(failed, fmt.Sprintf("line %d (%s)", s.Line, planError(err)))
		}
	}
	for _, s := range pins {
		pt := conv(s.Points[0], p.W, p.H)
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
		verb := "made the %s map, but could not draw %s"
		if update {
			verb = "sketched onto the %s map, but could not draw %s"
		}
		return badRequestf(verb, sketchName(r), strings.Join(failed, "; "))
	}
	return nil
}

// Export lists nothing: sketches are written, not read back, and pins on
// existing maps are listed by the pin kind.
func (MapSketchKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
