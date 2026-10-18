package mapsketch

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseGridSize(t *testing.T) {
	tests := []struct {
		in      string
		w, h    int
		wantErr string
	}{
		{in: "40 x 26", w: 40, h: 26},
		{in: "12 by 8", w: 12, h: 8},
		{in: "3 x 10", wantErr: "4 to 100"},
		{in: "forty", wantErr: "like 40 x 26"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			w, h, err := ParseGridSize(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || w != tt.w || h != tt.h {
				t.Fatalf("got %d x %d, %v", w, h, err)
			}
		})
	}
}

func TestParse_Shapes(t *testing.T) {
	tests := []struct {
		name string
		line string
		want Shape
	}{
		{"room", `room "Keep" 18,11 to 22,15`, Shape{T: "room", X: 18, Y: 11, W: 4, H: 4, Name: "Keep"}},
		{"corners swapped", `area 9,8 to 3,4`, Shape{T: "area", X: 3, Y: 4, W: 6, H: 4}},
		{"water colour", `water "Moat" 1,1 to 3,3 color teal`, Shape{T: "water", X: 1, Y: 1, W: 2, H: 2, Name: "Moat", Color: "teal"}},
		{"danger hex", `danger 0,0 to 2,2 color #AA33CC`, Shape{T: "danger", X: 0, Y: 0, W: 2, H: 2, Color: "#aa33cc"}},
		{"wall box is closed", `wall 1,1 to 3,2`, Shape{T: "wall", Pts: [][2]float64{{1, 1}, {3, 1}, {3, 2}, {1, 2}, {1, 1}}}},
		{"flat wall box is a line", `wall 0,0 to 40,0`, Shape{T: "wall", Pts: [][2]float64{{0, 0}, {40, 0}}}},
		{"wall line", `wall 1,1 5,1 5,5`, Shape{T: "wall", Pts: [][2]float64{{1, 1}, {5, 1}, {5, 5}}}},
		{"road becomes path", `road "Old" 1,1 to 5,5 to 9,5`, Shape{T: "path", Name: "Old", Pts: [][2]float64{{1, 1}, {5, 5}, {9, 5}}}},
		{"label", `label "Fires" 2,24 color rose`, Shape{T: "label", X: 2, Y: 24, Name: "Fires", Color: "rose"}},
		{"bullet", `- path 1,1 2,2`, Shape{T: "path", Pts: [][2]float64{{1, 1}, {2, 2}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.line)
			if len(r.Problems) != 0 || len(r.Doc.Shapes) != 1 || r.Shapes != 1 {
				t.Fatalf("result = %+v", r)
			}
			got := r.Doc.Shapes[0]
			tt.want.ID = "s1"
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParse_CircleWallIsClosedPolyline(t *testing.T) {
	r := Parse(`wall circle 20,13 radius 9`)
	if len(r.Problems) != 0 || len(r.Doc.Shapes) != 1 {
		t.Fatalf("result = %+v", r)
	}
	pts := r.Doc.Shapes[0].Pts
	if len(pts) != circleSegments+1 || pts[0] != pts[len(pts)-1] || pts[0] != [2]float64{29, 13} {
		t.Fatalf("pts = %v", pts)
	}
}

func TestParse_Pins(t *testing.T) {
	tests := []struct {
		name string
		line string
		want Pin
	}{
		{"default", `pin 4,4`, Pin{X: 4, Y: 4, Style: "icon", Icon: "location-dot"}},
		{"number", `pin "Gate" 20,22 number: Terms at dusk.`, Pin{X: 20, Y: 22, Name: "Gate", Style: "number", Text: "Terms at dusk."}},
		{"icon", `pin "Banner" 20,9 icon skull`, Pin{X: 20, Y: 9, Name: "Banner", Style: "icon", Icon: "skull"}},
		{"category word", `pin "Gold" 3,3 treasure: Under the bed.`, Pin{X: 3, Y: 3, Name: "Gold", Style: "icon", Icon: "gem", Text: "Under the bed."}},
		{"explicit icon wins", `pin 3,3 danger icon crown`, Pin{X: 3, Y: 3, Style: "icon", Icon: "crown"}},
		{"page and dm and colour", `pin "Gate" 5,6 icon door-open page "Harrowmere Gate" dm color sky: note, with 1,2 numbers`,
			Pin{X: 5, Y: 6, Name: "Gate", Style: "icon", Icon: "door-open", Page: "Harrowmere Gate", DM: true, Color: "sky", Text: "note, with 1,2 numbers"}},
		{"page before name", `pin page "Keep" "Tower" 5,6`, Pin{X: 5, Y: 6, Name: "Tower", Style: "icon", Icon: "location-dot", Page: "Keep"}},
		{"colon in name", `pin "Act 1: Siege" 5,6`, Pin{X: 5, Y: 6, Name: "Act 1: Siege", Style: "icon", Icon: "location-dot"}},
		{"curly quotes", `pin “Gate” 5,6 page “The Gate”`, Pin{X: 5, Y: 6, Name: "Gate", Style: "icon", Icon: "location-dot", Page: "The Gate"}},
		{"fractional", `pin 4.5,6.25 number`, Pin{X: 4.5, Y: 6.25, Style: "number"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.line)
			if len(r.Problems) != 0 || len(r.Doc.Pins) != 1 || r.Markers != 1 {
				t.Fatalf("result = %+v", r)
			}
			tt.want.ID = "p1"
			if got := r.Doc.Pins[0]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParse_BackgroundAndGrid(t *testing.T) {
	r := Parse("grid 80 x 80\nbackground HEX\nroom 1,1 to 2,2")
	if len(r.Problems) != 0 || r.Background != "hex" || r.Doc.Background != "hex" || r.Doc.Grid != (Grid{40, 26}) {
		t.Fatalf("result = %+v", r)
	}
	if r := Parse("room 1,1 to 2,2"); r.Doc.Background != "none" {
		t.Errorf("default background = %q", r.Doc.Background)
	}
}

func TestParse_Problems(t *testing.T) {
	long := strings.Repeat("a", MaxName+1)
	tests := []struct {
		name string
		line string
		want string
	}{
		{"unknown word", `tower 1,1`, `Line 1: "tower" isn't one of background, grid, room`},
		{"background", `background lava`, "Line 1: background must be none, dots, grid or hex."},
		{"two corners", `room "Hall" 3,4`, "Line 1: a room needs two corners"},
		{"off grid", `room 1,1 to 41,4`, "Line 1: 41,4 is off the 40 by 26 grid."},
		{"flat box", `room 1,1 to 1,4`, "Line 1: a room needs some width and height"},
		{"circle off grid", `wall circle 3,3 radius 5`, "Line 1: a circle of radius 5 around 3,3 runs off"},
		{"circle shape", `wall circle 3,3`, "Line 1: a circle wall needs a centre and a radius"},
		{"label words", `label 2,2`, "Line 1: a label needs its words in quotes."},
		{"pin point", `pin "Nowhere"`, "Line 1: a pin needs a point."},
		{"bad icon", `pin 1,1 icon dragon`, `Line 1: icon "dragon" isn't one of location-dot`},
		{"icon without name", `pin 1,1 icon`, "Line 1: icon needs a name"},
		{"bad colour", `room 1,1 to 2,2 color puce`, `Line 1: color "puce" isn't one of sky`},
		{"bad hex", `room 1,1 to 2,2 color #12`, "Line 1: color"},
		{"amber hex", `room 1,1 to 2,2 color #d97706`, "Line 1: color #d97706 is too close to amber"},
		{"amber brown", `pin 1,1 color #8b6a2b`, "Line 1: color #8b6a2b is too close to amber"},
		{"long name", `room "` + long + `" 1,1 to 2,2`, "Line 1: the name is longer than 80"},
		{"long note", `pin 1,1: ` + strings.Repeat("n", MaxNote+1), "Line 1: the note is longer than 500"},
		{"long page", `pin 1,1 page "` + strings.Repeat("p", MaxPageName+1) + `"`, "Line 1: the page name is longer than 200"},
		{"gold", `pin 1,1 color #ffd700`, "Line 1: color #ffd700 is too close to amber"},
		{"orange edge of amber", `pin 1,1 color #ff7000`, "Line 1: color #ff7000 is too close to amber"},
		{"too many points", `path ` + strings.Repeat("1,1 ", MaxPoints+1), "Line 1: a line holds at most 100 points."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.line)
			if len(r.Problems) != 1 || !strings.HasPrefix(r.Problems[0], tt.want) || r.Markers+r.Shapes != 0 {
				t.Fatalf("problems = %q, want prefix %q", r.Problems, tt.want)
			}
		})
	}
}

func TestParse_MutedHueAndGreyAreAllowed(t *testing.T) {
	for _, c := range []string{"#a09080", "#808080", "#3366cc"} {
		if r := Parse("room 1,1 to 2,2 color " + c); len(r.Problems) != 0 {
			t.Errorf("%s refused: %q", c, r.Problems)
		}
	}
}

func TestParse_BadLineKeepsTheRest(t *testing.T) {
	r := Parse("# map\n\nroom \"Keep\" 1,1 to 4,4\nroom 1,1\npin 2,2\n```\ntower 1,1")
	if r.Shapes != 1 || r.Markers != 1 || len(r.Problems) != 2 {
		t.Fatalf("result = %+v", r)
	}
	if !strings.HasPrefix(r.Problems[0], "Line 4: ") || !strings.HasPrefix(r.Problems[1], "Line 7: ") {
		t.Errorf("problems = %q", r.Problems)
	}
}

func TestParse_Limits(t *testing.T) {
	var shapes, pins strings.Builder
	for i := 0; i < MaxShapes+2; i++ {
		shapes.WriteString("room 1,1 to 2,2\n")
	}
	for i := 0; i < MaxPins+1; i++ {
		pins.WriteString("pin 1,1\n")
	}
	r := Parse(shapes.String() + pins.String())
	if r.Shapes != MaxShapes || r.Markers != MaxPins || len(r.Problems) != 3 {
		t.Fatalf("shapes %d markers %d problems %d", r.Shapes, r.Markers, len(r.Problems))
	}
	if !strings.Contains(r.Problems[0], "already has 200 shapes") || !strings.Contains(r.Problems[2], "already has 200 markers") {
		t.Errorf("problems = %q", r.Problems)
	}
}

const sample = `background grid
wall circle 20,13 radius 9 color rose
wall 1,1 to 3,2
room "The keep" 18,11 to 22,15
area 1,1 to 7,5 color #3366cc
water "The moat" 26,2 to 30,5
danger 5,5 to 6,6
path "Road to the gate" 20,22 20,26 color lime
wall 0,0 to 40,0
label "enemy fires all round" 2,24
pin "The banner" 20,9 icon skull dm color violet: Red, on the keep's tower.
pin "The gate" 20,22 number page "Harrowmere Gate": Terms at dusk.
pin 4.5,6 icon crown
pin "Gold" 3,3 treasure
`

func TestFormat_RoundTrip(t *testing.T) {
	first := Parse(sample)
	if len(first.Problems) != 0 {
		t.Fatalf("problems = %q", first.Problems)
	}
	text := Format(first.Doc)
	second := Parse(text)
	if len(second.Problems) != 0 {
		t.Fatalf("reparse problems = %q\n%s", second.Problems, text)
	}
	if !reflect.DeepEqual(first.Doc, second.Doc) {
		t.Fatalf("round trip changed the map\nfirst:  %+v\nsecond: %+v\ntext:\n%s", first.Doc, second.Doc, text)
	}
	if Format(second.Doc) != text {
		t.Errorf("Format is not stable")
	}
}

func TestFormat_Details(t *testing.T) {
	d := NewDoc()
	d.Shapes = []Shape{{ID: "a", T: "room", X: 0.5, Y: 1, W: 2.25, H: 3, Name: `Say "hi"` + "\nthere"}}
	d.Pins = []Pin{{ID: "b", X: 1, Y: 2, Style: "number", Page: "Keep", Text: "line one\nline two", EntityID: "e1"}}
	got := Format(d)
	want := "room \"Say 'hi' there\" 0.5,1 to 2.75,4\npin 1,2 number page \"Keep\": line one line two\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if Format(NewDoc()) != "" {
		t.Errorf("empty map should format to nothing")
	}
}

func TestGrammarExampleParses(t *testing.T) {
	i := strings.Index(Grammar, "```\n")
	if i < 0 {
		t.Fatal("no example in Grammar")
	}
	body := strings.TrimSuffix(strings.TrimPrefix(Grammar[i:], "```\n"), "```")
	r := Parse(body)
	if len(r.Problems) != 0 || r.Shapes < 4 || r.Markers != 2 || r.Doc.Background != "grid" {
		t.Fatalf("example result = %+v", r)
	}
}

func TestNormalizeColor(t *testing.T) {
	for _, c := range ColorNames {
		if n, p := NormalizeColor(strings.ToUpper(c)); n != c || p != "" {
			t.Errorf("%s -> %q %q", c, n, p)
		}
	}
	if n, p := NormalizeColor(""); n != "" || p == "" {
		t.Errorf("empty colour should be refused by NormalizeColor, got %q %q", n, p)
	}
}

func TestNormalizeColorAmberBand(t *testing.T) {
	tests := []struct {
		hex     string
		refused bool
	}{
		{"#d97706", true},
		{"#ffd700", true},
		{"#ff7000", true},
		{"#b39a5c", false}, // muted tan, saturation under 45%
		{"#5c4a1f", false}, // dark, lightness under 30%
		{"#fdf6e3", true},  // pale cream is still bright amber in HSL
		{"#3366cc", false},
	}
	for _, tt := range tests {
		_, problem := NormalizeColor(tt.hex)
		if (problem != "") != tt.refused {
			t.Errorf("NormalizeColor(%s) problem = %q, want refused=%v", tt.hex, problem, tt.refused)
		}
	}
}
