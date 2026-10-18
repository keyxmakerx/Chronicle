package records

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// harrowmere is the signed mockup's sample sketch.
const harrowmere = `wall circle 20,13 radius 9
room "The keep" 18,11 to 22,15
area "Enemy camp" 1,1 to 7,5
area "Enemy camp" 33,19 to 39,25
water "The moat" 26,2 to 30,5
path "Road to the gate" 20,22 to 20,26
label "enemy fires all round" 2,24
pin "The banner" 20,9 danger: Red, on the keep's tower. The hidden rule is about this.
pin "The gate" 20,22: Where the herald comes with terms at dusk.
pin "The ramparts" 12,8: Day one's fight.
pin "Edge of the world" 1,13 note: Ride out far enough and you come back in.`

func TestParseSketchGrid(t *testing.T) {
	tests := []struct {
		in         string
		w, h       int
		wantErrHas string
	}{
		{in: "40 x 26", w: 40, h: 26},
		{in: "40x26", w: 40, h: 26},
		{in: "12 by 8", w: 12, h: 8},
		{in: "40 × 26", w: 40, h: 26},
		{in: "3 x 10", wantErrHas: "4 to 100"},
		{in: "101 x 10", wantErrHas: "4 to 100"},
		{in: "forty", wantErrHas: "like 40 x 26"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			w, h, err := parseSketchGrid(tt.in)
			if tt.wantErrHas != "" {
				if err == nil || !strings.Contains(planError(err), tt.wantErrHas) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErrHas)
				}
				return
			}
			if err != nil || w != tt.w || h != tt.h {
				t.Fatalf("got %d x %d, %v; want %d x %d", w, h, err, tt.w, tt.h)
			}
		})
	}
}

func TestParseSketchLine(t *testing.T) {
	tests := []struct {
		line      string
		shape     string
		name      string
		points    int
		category  string
		note      string
		wantErr   string
		wantNoErr bool
	}{
		{line: `room "The keep" 18,11 to 22,15`, shape: "box", name: "The keep", points: 2},
		{line: `danger "Pit" 1,1 to 3,3`, shape: "box", name: "Pit", points: 2},
		{line: `water 1,1 to 3,3`, shape: "box", points: 2},
		{line: `wall 0,0 to 40,1`, shape: "box", points: 2},
		{line: `wall 0,0 to 40,0`, shape: "line", points: 2},
		{line: `wall 1,1 5,1 5,5`, shape: "line", points: 3},
		{line: `wall circle 20,13 radius 9`, shape: "circle", points: 1},
		{line: `road "North road" 1,1 to 5,5 to 9,5`, shape: "line", name: "North road", points: 3},
		{line: `label "enemy fires" 2,24`, shape: "label", name: "enemy fires", points: 1},
		{line: `pin "The banner" 20,9 danger: Red, on 3,4 the tower.`, shape: "pin", name: "The banner", points: 1, category: "danger", note: "Red, on 3,4 the tower."},
		{line: `pin "The gate" 20,22: Terms at dusk.`, shape: "pin", name: "The gate", points: 1, category: "location", note: "Terms at dusk."},
		{line: `pin 4,4`, shape: "pin", name: "Pin", points: 1, category: "location"},
		{line: `room "Hall" 3,4`, wantErr: "a room needs two corners, like 3,4 to 9,8."},
		{line: `wall circle 20,13`, wantErr: "a circle wall needs a centre and a radius"},
		{line: `path 1,1`, wantErr: "a path needs at least two points."},
		{line: `label 2,2`, wantErr: "a label needs its words in quotes."},
		{line: `pin "Nowhere"`, wantErr: "a pin needs a point."},
		{line: `tower "Keep" 1,1`, wantErr: `"tower" isn't one of room, area, water, danger, wall, path, road, label, pin.`},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			s, err := parseSketchLine(1, tt.line)
			if tt.wantErr != "" {
				if !strings.Contains(err, tt.wantErr) {
					t.Fatalf("err = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != "" {
				t.Fatalf("unexpected err %q", err)
			}
			if s.Shape != tt.shape || s.Name != tt.name || len(s.Points) != tt.points || s.Category != tt.category || s.Note != tt.note {
				t.Fatalf("got %+v", s)
			}
		})
	}
}

func TestParseSketch_BadLinesAreReportedNotFatal(t *testing.T) {
	body := "# the keep\n\nroom \"Keep\" 1,1 to 4,4\nroom \"Off\" 1,1 to 50,4\nroom \"Flat\" 1,1 to 1,4\ntower 1,1\n```\npin \"Gate\" 2,2\nwall circle 3,3 radius 5"
	p := parseSketch(body, 40, 26)
	if len(p.Shapes) != 2 {
		t.Fatalf("shapes = %+v, want the room and the pin", p.Shapes)
	}
	want := []string{
		"Line 4: 50,4 is off the 40 by 26 grid.",
		"Line 5: a room needs some width and height; its corners are in line.",
		`Line 6: "tower" isn't one of`,
		"Line 9: a circle of radius 5 around 3,3 runs off the 40 by 26 grid.",
	}
	if len(p.Errs) != len(want) {
		t.Fatalf("errs = %q", p.Errs)
	}
	for i, w := range want {
		if !strings.HasPrefix(p.Errs[i], w) {
			t.Errorf("errs[%d] = %q, want prefix %q", i, p.Errs[i], w)
		}
	}
}

func TestSketchDrawing_GridToPercent(t *testing.T) {
	// 40 squares across: 25 px squares on a 1000 px map, 26 squares down.
	tests := []struct {
		line string
		want SketchDrawing
	}{
		{`room "Keep" 18,11 to 22,15`, SketchDrawing{Type: "room", Points: []SketchPoint{{45, 42.308}, {55, 57.692}}}},
		{`wall circle 20,13 radius 9`, SketchDrawing{Type: "ellipse", Points: []SketchPoint{{50, 50}, {72.5, 50}}}},
		{`road 0,0 to 40,26`, SketchDrawing{Type: "freehand", Points: []SketchPoint{{0, 0}, {100, 100}}}},
		{`label "Fires" 2,24`, SketchDrawing{Type: "text", Points: []SketchPoint{{5, 92.308}}}},
		{`wall 0,0 to 40,1`, SketchDrawing{Type: "wall", Points: []SketchPoint{{0, 0}, {100, 3.846}}}},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			s, err := parseSketchLine(1, tt.line)
			if err != "" {
				t.Fatal(err)
			}
			d := s.drawing(40, 26, "")
			if d.Type != tt.want.Type || len(d.Points) != len(tt.want.Points) {
				t.Fatalf("got %+v", d)
			}
			for i := range d.Points {
				if d.Points[i] != tt.want.Points[i] {
					t.Errorf("point %d = %+v, want %+v", i, d.Points[i], tt.want.Points[i])
				}
			}
			if d.StrokeColor == "" || d.StrokeWidth <= 0 {
				t.Errorf("drawing has no stroke: %+v", d)
			}
		})
	}
	// A wall box carries no name; the map refuses text on a wall.
	s, _ := parseSketchLine(1, `wall "Rampart" 0,0 to 40,1`)
	if d := s.drawing(40, 26, ""); d.Text != nil {
		t.Errorf("wall has text %q", *d.Text)
	}
}

type fakeSketchMaps struct {
	created  []string
	grid     [3]int
	drawings []SketchDrawing
	failType string

	// Update-path state: the target's size and earlier sketch, and the calls
	// made against it.
	target  SketchTarget
	cleared []string
	order   []string
}

func (f *fakeSketchMaps) Inspect(context.Context, string, Actor) (SketchTarget, error) {
	return f.target, nil
}

func (f *fakeSketchMaps) SketchLayer(_ context.Context, mapID string) (string, error) {
	f.order = append(f.order, "layer")
	return "layer-" + mapID, nil
}

func (f *fakeSketchMaps) ClearSketch(_ context.Context, mapID, layerID string, _ Actor) (int, error) {
	f.order = append(f.order, "clear")
	f.cleared = append(f.cleared, mapID+"/"+layerID)
	return f.target.Sketched, nil
}

func (f *fakeSketchMaps) CreateGridMap(_ context.Context, _, name string, w, h, size int) (string, error) {
	f.created = append(f.created, name)
	f.grid = [3]int{w, h, size}
	return "new-map", nil
}

func (f *fakeSketchMaps) Draw(_ context.Context, mapID string, _ Actor, d SketchDrawing) error {
	if mapID != "new-map" && mapID != "m1" {
		return errors.New("wrong map")
	}
	f.order = append(f.order, "draw")
	if d.Type == f.failType {
		return badRequestf("the map refused it")
	}
	f.drawings = append(f.drawings, d)
	return nil
}

func TestMapSketchKind_PlanAndApply(t *testing.T) {
	grid := map[string]any{"grid": "40 x 26"}
	tests := []struct {
		name        string
		a           Actor
		r           Record
		wantErr     string
		wantSummary string
		wantWarn    string
	}{
		{name: "harrowmere", a: owner, r: rec("map", ActionCreate, "The Siege of Harrowmere", grid, harrowmere),
			wantSummary: "makes a map with 7 shapes and 4 pins on a 40 by 26 grid"},
		{name: "same name warns", a: owner, r: rec("map", ActionCreate, "Grimvale", grid, "room 1,1 to 2,2"),
			wantSummary: "1 shape and 0 pins", wantWarn: "already exists"},
		{name: "bad line warns", a: owner, r: rec("map", ActionCreate, "M", grid, "room 1,1 to 2,2\nroom 1,1"),
			wantSummary: "1 shape", wantWarn: "Line 2: a room needs two corners"},
		{name: "delete refused", a: owner, r: rec("map", ActionDelete, "M", grid, "room 1,1 to 2,2"), wantErr: "can only create a map or update one"},
		{name: "update of a missing map", a: owner, r: rec("map", ActionUpdate, "Nowhere", grid, "room 1,1 to 2,2"), wantErr: `no map called "Nowhere" to sketch onto`},
		{name: "update cannot reach another campaign's map", a: owner, r: rec("map", ActionUpdate, "M", grid, "room 1,1 to 2,2"), wantErr: "no map called"},
		{name: "update adds", a: owner, r: rec("map", ActionUpdate, "Grimvale", grid, "room 1,1 to 2,2\npin \"P\" 3,3"),
			wantSummary: "adds 1 shape and 1 pin onto the Grimvale map; nothing is replaced"},
		{name: "no grid", a: owner, r: rec("map", ActionCreate, "M", nil, "room 1,1 to 2,2"), wantErr: "needs `grid`"},
		{name: "no name", a: owner, r: rec("map", ActionCreate, "", grid, "room 1,1 to 2,2"), wantErr: "needs `name`"},
		{name: "nothing drawable", a: owner, r: rec("map", ActionCreate, "M", grid, "tower 1,1"), wantErr: "nothing on this sketch", wantWarn: "tower"},
		{name: "hidden needs a DM", a: player, r: rec("map", ActionCreate, "M", map[string]any{"grid": "40 x 26", "visibility": "dm_only"}, "room 1,1 to 2,2"),
			wantErr: "only the owner or a co-DM"},
		{name: "hidden by a DM", a: owner, r: rec("map", ActionCreate, "M", map[string]any{"grid": "40 x 26", "visibility": "dm_only"}, "room 1,1 to 2,2"),
			wantSummary: "hidden from players"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pins := &fakeMaps{maps: []MapRef{{ID: "m1", CampaignID: camp, Name: "Grimvale"}, {ID: "m2", CampaignID: "other", Name: "M"}}}
			k := MapSketchKind{Svc: &fakeSketchMaps{}, Pins: pins}
			p := k.Plan(context.Background(), camp, tt.a, tt.r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("Error = %q, want %q", p.Error, tt.wantErr)
				}
			} else if p.Error != "" || !strings.Contains(p.Summary, tt.wantSummary) {
				t.Fatalf("plan = %+v, want summary with %q", p, tt.wantSummary)
			}
			if tt.wantWarn != "" && !strings.Contains(strings.Join(p.Warnings, "|"), tt.wantWarn) {
				t.Errorf("warnings = %q, want %q", p.Warnings, tt.wantWarn)
			}
			if tt.wantWarn == "" && len(p.Warnings) > 0 {
				t.Errorf("unexpected warnings %q", p.Warnings)
			}
		})
	}
}

func TestMapSketchKind_ApplyMakesTheMap(t *testing.T) {
	svc := &fakeSketchMaps{}
	pins := &fakeMaps{}
	k := MapSketchKind{Svc: svc, Pins: pins}
	r := rec("map", ActionCreate, "The Siege of Harrowmere", map[string]any{"grid": "40 x 26", "visibility": "dm_only"}, harrowmere)
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if len(svc.created) != 1 || svc.grid != [3]int{1000, 650, 25} {
		t.Fatalf("created %q with grid %v, want one 1000x650 map with 25 px squares", svc.created, svc.grid)
	}
	var types []string
	for _, d := range svc.drawings {
		types = append(types, d.Type)
		if d.Visibility != "dm_only" {
			t.Errorf("%s drawn %q, want dm_only", d.Type, d.Visibility)
		}
	}
	if got := strings.Join(types, ","); got != "ellipse,room,area,area,water,freehand,text" {
		t.Errorf("drawings = %s", got)
	}
	if len(pins.created) != 4 {
		t.Fatalf("pins = %+v", pins.created)
	}
	banner := pins.created[0]
	if banner.Name != "The banner" || banner.Category != "danger" || banner.Visibility != "dm_only" ||
		banner.Description == nil || !strings.HasPrefix(*banner.Description, "Red, on the keep") ||
		*banner.X != 50 || *banner.Y != 34.615 {
		t.Errorf("banner = %+v", banner)
	}
}

func TestMapSketchKind_ApplyKeepsTheMapWhenAShapeFails(t *testing.T) {
	svc := &fakeSketchMaps{failType: "water"}
	k := MapSketchKind{Svc: svc, Pins: &fakeMaps{}}
	r := rec("map", ActionCreate, "Harrowmere", map[string]any{"grid": "40 x 26"}, harrowmere)
	err := k.Apply(context.Background(), camp, owner, r)
	if err == nil || !strings.Contains(planError(err), "made the Harrowmere map, but could not draw line 5 (the map refused it)") {
		t.Fatalf("err = %v", err)
	}
	if len(svc.drawings) != 6 {
		t.Errorf("drew %d shapes, want the other 6", len(svc.drawings))
	}
}

func updateKit(sketched int, w, h int) (*fakeSketchMaps, *fakeMaps, MapSketchKind) {
	svc := &fakeSketchMaps{target: SketchTarget{Width: w, Height: h, Sketched: sketched}}
	pins := &fakeMaps{
		maps: []MapRef{{ID: "m1", CampaignID: camp, Name: "Imix"}, {ID: "m2", CampaignID: "other", Name: "Elsewhere"}},
		pins: []PinRef{{ID: "p1", Name: "Old ferry"}},
	}
	return svc, pins, MapSketchKind{Svc: svc, Pins: pins}
}

func TestMapSketchKind_UpdatePlan(t *testing.T) {
	body := "room 1,1 to 2,2\nlabel \"x\" 4,4\npin \"Old ferry\" 3,3\npin \"New dock\" 5,5"
	tests := []struct {
		name     string
		sketched int
		w, h     int
		grid     string
		want     []string
		warn     []string
	}{
		{name: "first sketch, matching picture", w: 1500, h: 1000, grid: "39 x 26",
			want: []string{"adds 2 shapes and 1 pin onto the Imix map", "nothing is replaced"},
			warn: []string{"Left out, already pinned on this map: Old ferry"}},
		{name: "replaces the earlier sketch", sketched: 5, w: 1500, h: 1000, grid: "39 x 26",
			want: []string{"replaces the 5 shapes on its AI sketch layer, whoever drew them"}},
		{name: "stretch is called out", w: 1500, h: 1000, grid: "40 x 40",
			want: []string{"adds 2 shapes"}, warn: []string{"The map's picture is 1500 x 1000, so a 40 x 40 grid stretches the sketch; 40 x 27 keeps squares square"}},
		{name: "unknown picture size is not nagged", w: 0, h: 0, grid: "40 x 40",
			want: []string{"adds 2 shapes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, k := updateKit(tt.sketched, tt.w, tt.h)
			r := rec("map", ActionUpdate, "imix", map[string]any{"grid": tt.grid}, body)
			p := k.Plan(context.Background(), camp, owner, r)
			if p.Error != "" {
				t.Fatalf("plan error %q", p.Error)
			}
			for _, w := range tt.want {
				if !strings.Contains(p.Summary, w) {
					t.Errorf("summary %q lacks %q", p.Summary, w)
				}
			}
			joined := strings.Join(p.Warnings, "|")
			for _, w := range tt.warn {
				if !strings.Contains(joined, w) {
					t.Errorf("warnings %q lack %q", p.Warnings, w)
				}
			}
			if strings.Contains(joined, "stretches") && !strings.Contains(strings.Join(tt.warn, ""), "stretches") {
				t.Errorf("unexpected stretch warning: %q", p.Warnings)
			}
		})
	}
}

func TestMapSketchKind_UpdateApply(t *testing.T) {
	body := "room 1,1 to 3,2\nlabel \"x\" 4,4\npin \"Old ferry\" 3,3\npin \"New dock\" 39,13"
	t.Run("clears the old sketch, draws on its layer, adds only new pins", func(t *testing.T) {
		svc, pins, k := updateKit(4, 1500, 1000)
		r := rec("map", ActionUpdate, "Imix", map[string]any{"grid": "39 x 26", "visibility": "dm_only"}, body)
		if err := k.Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if len(svc.created) != 0 {
			t.Fatalf("an update made a map: %v", svc.created)
		}
		if strings.Join(svc.cleared, ",") != "m1/layer-m1" {
			t.Errorf("cleared %v", svc.cleared)
		}
		if got := strings.Join(svc.order, ","); got != "layer,clear,draw,draw" {
			t.Errorf("call order = %s: the old sketch must go before the new shapes are drawn", got)
		}
		for _, d := range svc.drawings {
			if d.LayerID != "layer-m1" || d.Visibility != "dm_only" {
				t.Errorf("drawing %+v not on the sketch layer or not hidden", d)
			}
		}
		// Exact spread: x 1..3 of 39, y 1..2 of 26.
		if p := svc.drawings[0].Points; p[0].X != 2.564 || p[1].X != 7.692 || p[1].Y != 7.692 {
			t.Errorf("room points = %+v", p)
		}
		if len(pins.created) != 1 || pins.created[0].Name != "New dock" || *pins.created[0].X != 100 || *pins.created[0].Y != 50 {
			t.Errorf("pins = %+v, want only New dock at 100,50", pins.created)
		}
	})
	t.Run("first sketch clears nothing", func(t *testing.T) {
		svc, _, k := updateKit(0, 0, 0)
		r := rec("map", ActionUpdate, "Imix", map[string]any{"grid": "39 x 26"}, body)
		if err := k.Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if len(svc.cleared) != 0 {
			t.Errorf("cleared %v with nothing sketched", svc.cleared)
		}
	})
	t.Run("missing map writes nothing", func(t *testing.T) {
		svc, pins, k := updateKit(0, 0, 0)
		r := rec("map", ActionUpdate, "Nowhere", map[string]any{"grid": "39 x 26"}, body)
		err := k.Apply(context.Background(), camp, owner, r)
		if err == nil || !strings.Contains(planError(err), `no map called "Nowhere"`) {
			t.Fatalf("err = %v", err)
		}
		if len(svc.order)+len(svc.created)+len(pins.created) != 0 {
			t.Errorf("wrote despite refusal: %v %v %v", svc.order, svc.created, pins.created)
		}
	})
	t.Run("map: key names the map", func(t *testing.T) {
		svc, _, k := updateKit(0, 0, 0)
		r := rec("map", ActionUpdate, "Whatever", map[string]any{"grid": "39 x 26", "map": "Imix"}, "room 1,1 to 3,2")
		if err := k.Apply(context.Background(), camp, owner, r); err != nil || len(svc.drawings) != 1 {
			t.Fatalf("err=%v drawings=%d", err, len(svc.drawings))
		}
	})
	t.Run("create puts its shapes on the sketch layer", func(t *testing.T) {
		svc := &fakeSketchMaps{}
		k := MapSketchKind{Svc: svc, Pins: &fakeMaps{}}
		r := rec("map", ActionCreate, "N", map[string]any{"grid": "40 x 26"}, "room 1,1 to 2,2")
		if err := k.Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if svc.drawings[0].LayerID != "layer-new-map" {
			t.Errorf("layer = %q", svc.drawings[0].LayerID)
		}
	})
}
