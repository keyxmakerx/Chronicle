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
	body := "# the keep\n\nroom \"Keep\" 1,1 to 4,4\nroom \"Off\" 1,1 to 50,4\nroom \"Flat\" 1,1 to 1,4\ntower 1,1\n```\npin \"Gate\" 2,2"
	p := parseSketch(body, 40, 26)
	if len(p.Shapes) != 2 {
		t.Fatalf("shapes = %+v, want the room and the pin", p.Shapes)
	}
	want := []string{
		"Line 4: 50,4 is off the 40 by 26 grid.",
		"Line 5: a room needs some width and height; its corners are in line.",
		`Line 6: "tower" isn't one of`,
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
}

func (f *fakeSketchMaps) CreateGridMap(_ context.Context, _, name string, w, h, size int) (string, error) {
	f.created = append(f.created, name)
	f.grid = [3]int{w, h, size}
	return "new-map", nil
}

func (f *fakeSketchMaps) Draw(_ context.Context, mapID string, _ Actor, d SketchDrawing) error {
	if mapID != "new-map" {
		return errors.New("wrong map")
	}
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
		{name: "update refused", a: owner, r: rec("map", ActionUpdate, "M", grid, "room 1,1 to 2,2"), wantErr: "only makes new maps"},
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
