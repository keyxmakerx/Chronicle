package hexsketch

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		in         string
		cols, rows int
		wantErr    string
	}{
		{in: "30 x 20", cols: 30, rows: 20},
		{in: "30x20", cols: 30, rows: 20},
		{in: "4 by 100", cols: 4, rows: 100},
		{in: "30 × 20", cols: 30, rows: 20},
		{in: "3 x 20", wantErr: "4 to 100"},
		{in: "30 x 101", wantErr: "4 to 100"},
		{in: "thirty", wantErr: "like 30 x 20"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			c, r, err := ParseSize(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || c != tt.cols || r != tt.rows {
				t.Fatalf("got %d x %d, %v", c, r, err)
			}
		})
	}
}

func TestLand(t *testing.T) {
	tests := []struct{ in, want string }{
		{"forest", "forest"}, {"Forest", "forest"}, {"MOUNTAINS", "mountain"}, {"scrubland", "scrub"},
		{"Wasteland", "waste"}, {"deep sea", "deep"}, {"Deep-Sea", "deep"}, {"deepsea", "deep"},
		{"caverns", "cavern"}, {"cave", "cavern"}, {"sea", "water"}, {"lake", "water"},
		{"city", "town"}, {"village", "town"}, {"jungle", "jungle"}, {"badlands", "badlands"},
		{"road", ""}, {"river", ""}, {"tower", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := Land(tt.in)
			if got != tt.want || ok != (tt.want != "") {
				t.Fatalf("Land(%q) = %q, %v", tt.in, got, ok)
			}
		})
	}
	if len(Lands) != 19 {
		t.Errorf("%d lands, want the 19 terrains other than road", len(Lands))
	}
}

func hexes(pairs ...int) []Hex {
	var out []Hex
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Hex{pairs[i], pairs[i+1]})
	}
	return out
}

func TestParse_Verbs(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		cols  int
		rows  int
		want  Part
		wantN int // hexes in the part
	}{
		{name: "fill", body: "fill plains", cols: 6, rows: 5, want: Part{Kind: PartLand, Word: "plains", Line: 1}, wantN: 30},
		{name: "fill friendly name", body: "fill Deep sea", cols: 6, rows: 5, want: Part{Kind: PartLand, Word: "deep", Line: 1}, wantN: 30},
		{name: "one hex", body: "forest 2,3", cols: 6, rows: 5, want: Part{Kind: PartLand, Word: "forest", Line: 1, Hexes: hexes(2, 3)}, wantN: 1},
		{name: "one hex named", body: `town "Oakhold" 2,3`, cols: 6, rows: 5, want: Part{Kind: PartLand, Word: "town", Name: "Oakhold", Line: 1, Hexes: hexes(2, 3)}, wantN: 1},
		{name: "box", body: "hills 1,1 to 3,2", cols: 6, rows: 5,
			want: Part{Kind: PartLand, Word: "hills", Line: 1, Hexes: hexes(1, 1, 2, 1, 3, 1, 1, 2, 2, 2, 3, 2)}, wantN: 6},
		{name: "box corners either way round", body: "hills 3,2 to 1,1", cols: 6, rows: 5, wantN: 6,
			want: Part{Kind: PartLand, Word: "hills", Line: 1, Hexes: hexes(1, 1, 2, 1, 3, 1, 1, 2, 2, 2, 3, 2)}},
		{name: "box clipped", body: "forest -2,-2 to 1,1", cols: 6, rows: 5, wantN: 4,
			want: Part{Kind: PartLand, Word: "forest", Line: 1, Hexes: hexes(0, 0, 1, 0, 0, 1, 1, 1)}},
		{name: "box clipped far side", body: "forest 4,3 to 99,99", cols: 6, rows: 5, wantN: 4,
			want: Part{Kind: PartLand, Word: "forest", Line: 1, Hexes: hexes(4, 3, 5, 3, 4, 4, 5, 4)}},
		{name: "around radius 0", body: "snow around 2,2 radius 0", cols: 6, rows: 5, wantN: 1,
			want: Part{Kind: PartLand, Word: "snow", Line: 1, Hexes: hexes(2, 2)}},
		{name: "around radius 1 on an even row", body: "swamp around 2,2 radius 1", cols: 6, rows: 5, wantN: 7,
			want: Part{Kind: PartLand, Word: "swamp", Line: 1,
				Hexes: hexes(1, 1, 2, 1, 1, 2, 2, 2, 3, 2, 1, 3, 2, 3)}},
		{name: "around radius 1 on an odd row", body: "swamp around 2,3 radius 1", cols: 6, rows: 5, wantN: 7,
			want: Part{Kind: PartLand, Word: "swamp", Line: 1,
				Hexes: hexes(2, 2, 3, 2, 1, 3, 2, 3, 3, 3, 2, 4, 3, 4)}},
		{name: "around clipped at the corner", body: "ice around 0,0 radius 2", cols: 6, rows: 5, wantN: 7,
			want: Part{Kind: PartLand, Word: "ice", Line: 1, Hexes: hexes(0, 0, 1, 0, 2, 0, 0, 1, 1, 1, 0, 2, 1, 2)}},
		{name: "around a centre off the map still clips", body: "ice around -1,0 radius 1", cols: 6, rows: 5, wantN: 1,
			want: Part{Kind: PartLand, Word: "ice", Line: 1, Hexes: hexes(0, 0)}},
		{name: "along a straight row", body: "mountains along 0,0 4,0", cols: 6, rows: 5, wantN: 5,
			want: Part{Kind: PartLand, Word: "mountain", Line: 1, Hexes: hexes(0, 0, 1, 0, 2, 0, 3, 0, 4, 0)}},
		{name: "along bent with to", body: "mountain along 0,0 to 2,0 to 2,2", cols: 6, rows: 5, wantN: 5,
			want: Part{Kind: PartLand, Word: "mountain", Line: 1, Hexes: hexes(0, 0, 1, 0, 2, 0, 2, 1, 2, 2)}},
		{name: "road line", body: `road "King's Road" 0,0 to 3,0`, cols: 6, rows: 5, wantN: 4,
			want: Part{Kind: PartLine, Word: "road", Name: "King's Road", Line: 1, Hexes: hexes(0, 0, 1, 0, 2, 0, 3, 0)}},
		{name: "river line", body: "RIVER 0,0 3,0", cols: 6, rows: 5, wantN: 4,
			want: Part{Kind: PartLine, Word: "river", Line: 1, Hexes: hexes(0, 0, 1, 0, 2, 0, 3, 0)}},
		{name: "trail one point", body: "trail 2,2", cols: 6, rows: 5, wantN: 1,
			want: Part{Kind: PartLine, Word: "trail", Line: 1, Hexes: hexes(2, 2)}},
		{name: "place with notes", body: `place "Oakhold" 4,4: A market town, famous for 3 things: ale, cheese, wool.`, cols: 6, rows: 5, wantN: 1,
			want: Part{Kind: PartPlace, Name: "Oakhold", Notes: "A market town, famous for 3 things: ale, cheese, wool.", Line: 1, Hexes: hexes(4, 4)}},
		{name: "place without notes, spaced point", body: `place "Oakhold" 4 , 4`, cols: 6, rows: 5, wantN: 1,
			want: Part{Kind: PartPlace, Name: "Oakhold", Line: 1, Hexes: hexes(4, 4)}},
		{name: "curly quotes and bullets", body: `- town “Oakhold” 2,3`, cols: 6, rows: 5, wantN: 1,
			want: Part{Kind: PartLand, Word: "town", Name: "Oakhold", Line: 1, Hexes: hexes(2, 3)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Parse(tt.body, tt.cols, tt.rows)
			if len(res.Problems) != 0 || len(res.Parts) != 1 {
				t.Fatalf("parts = %+v, problems = %v", res.Parts, res.Problems)
			}
			got := res.Parts[0]
			if len(got.Hexes) != tt.wantN {
				t.Fatalf("%d hexes, want %d: %v", len(got.Hexes), tt.wantN, got.Hexes)
			}
			if tt.want.Hexes == nil { // fill: only the count is pinned
				got.Hexes = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParse_BadLinesAreReportedNotFatal(t *testing.T) {
	body := strings.Join([]string{
		"# a comment",
		"",
		"forest 1,1 to 2,2",          // 3: fine
		"tower 1,1",                  // 4
		"forest 1,1 5,5",             // 5
		"forest 1,1 3,3 4,4",         // 6
		"forest 40,40",               // 7
		"forest along 0,0",           // 8
		"road 1,1 40,40",             // 9
		"road around 3,3 radius 2",   // 10
		"fill road",                  // 11
		"fill",                       // 12
		"water around 3,3",           // 13
		"water around 3,3 radius 31", // 14
		"water -9,-9 to -5,-5",       // 15
		"place Oakhold 3,3",          // 16
		`place "Oakhold"`,            // 17
		`place "Oakhold" 40,40`,      // 18
		`place "" 3,3`,               // 19
		"river",                      // 20
		"hills 3,x",                  // 21
		"plains",                     // 22 (no hex)
		"```",                        // skipped
		"fill plains",                // 24: fine
	}, "\n")
	res := Parse(body, 10, 10)
	if len(res.Parts) != 2 {
		t.Fatalf("parts = %+v, want the box and the fill", res.Parts)
	}
	want := map[int]string{
		4:  `"tower" isn't a kind of land`,
		5:  "write a box as forest 1,1 to 5,5",
		6:  "or along with several points",
		7:  "40,40 is outside the 10 by 10 map",
		8:  "along needs at least two points",
		9:  "40,40 is outside the 10 by 10 map. The line is left out.",
		10: "road is a line, not land: write road c,r c,r",
		11: "road is a line, not land: write road c,r c,r",
		12: "fill needs a kind of land",
		13: "around needs a hex and a radius",
		14: "too wide",
		15: "all outside the 10 by 10 map",
		16: "needs its name in quotes",
		17: "needs one hex after its name",
		18: "The place is left out.",
		19: "needs a name",
		20: "a river needs points",
		21: `I could not read "3,x"`,
		22: "plains needs a hex",
	}
	if len(res.Problems) != len(want) {
		t.Fatalf("problems = %v", res.Problems)
	}
	for _, p := range res.Problems {
		w, ok := want[p.Line]
		if !ok || !strings.Contains(p.Text, w) {
			t.Errorf("line %d: %q, want it to contain %q", p.Line, p.Text, w)
		}
		if !strings.HasPrefix(p.String(), "Line ") {
			t.Errorf("String() = %q", p.String())
		}
	}
}

func TestParse_TextLimits(t *testing.T) {
	long := strings.Repeat("a", MaxNameRunes+1)
	res := Parse(`place "`+long+`" 1,1`+"\n"+`place "ok" 1,1: `+strings.Repeat("n", MaxNotesRunes+1)+"\n"+"place \"ok\" 2,2: tab\there", 5, 5)
	if len(res.Parts) != 0 || len(res.Problems) != 3 {
		t.Fatalf("parts %+v problems %v", res.Parts, res.Problems)
	}
	for _, w := range []string{"the name is too long", "the notes is too long", "characters that cannot be used"} {
		found := false
		for _, p := range res.Problems {
			found = found || strings.Contains(p.Text, w)
		}
		if !found {
			t.Errorf("no problem says %q: %v", w, res.Problems)
		}
	}
}

func cellAt(t *testing.T, cells []Cell, c, r int) Cell {
	t.Helper()
	for _, x := range cells {
		if x.Col == c && x.Row == r {
			return x
		}
	}
	t.Fatalf("no cell at %d,%d in %+v", c, r, cells)
	return Cell{}
}

func ptrVal(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

func TestFold_LaterLinesPaintOver(t *testing.T) {
	body := "fill plains\nforest 1,1 to 2,2\nwater 2,2\nplace \"Keep\" 2,2: first\nplace \"Fort\" 2,2: second\nplace \"Hut\" 0,0"
	res := Parse(body, 4, 4)
	if len(res.Cells) != 16 {
		t.Fatalf("%d cells, want 16", len(res.Cells))
	}
	if got := cellAt(t, res.Cells, 2, 2); got.Terrain != "water" || got.Name != "Fort" || got.Notes != "second" {
		t.Errorf("2,2 = %+v", got)
	}
	if got := cellAt(t, res.Cells, 1, 1); got.Terrain != "forest" || got.Name != "" {
		t.Errorf("1,1 = %+v", got)
	}
	if got := cellAt(t, res.Cells, 3, 3); got.Terrain != "plains" {
		t.Errorf("3,3 = %+v", got)
	}
	if got := cellAt(t, res.Cells, 0, 0); got.Name != "Hut" || got.Notes != "" || got.Terrain != "plains" {
		t.Errorf("0,0 = %+v", got)
	}
	// Row by row, left to right.
	for i := 1; i < len(res.Cells); i++ {
		a, b := res.Cells[i-1], res.Cells[i]
		if a.Row > b.Row || (a.Row == b.Row && a.Col >= b.Col) {
			t.Fatalf("cells out of order at %d: %+v then %+v", i, a, b)
		}
	}
}

func TestFold_LineMasks(t *testing.T) {
	// Bits are numbered in neighbour order, east first and clockwise: 0 E, 1 SE,
	// 2 SW, 3 W, 4 NW, 5 NE. On an even row SE is the same column one row down;
	// on an odd row SE is one column to the right.
	tests := []struct {
		name string
		body string
		want map[Hex]int
	}{
		{name: "straight east", body: "road 2,2 to 5,2",
			want: map[Hex]int{{2, 2}: 1, {3, 2}: 1 | 8, {4, 2}: 1 | 8, {5, 2}: 8}},
		{name: "one step south-east from an even row", body: "river 3,2 3,3",
			want: map[Hex]int{{3, 2}: 1 << 1, {3, 3}: 1 << 4}},
		{name: "one step south-west from an odd row", body: "river 3,3 3,4",
			want: map[Hex]int{{3, 3}: 1 << 2, {3, 4}: 1 << 5}},
		{name: "bent path", body: "trail 2,2 4,2 4,4",
			want: map[Hex]int{{2, 2}: 1, {3, 2}: 1 | 8, {4, 2}: 8 | 1<<1, {4, 3}: 1<<4 | 1<<2, {4, 4}: 1 << 5}},
		{name: "one point has no exits", body: "road 5,5", want: map[Hex]int{{5, 5}: 0}},
		{name: "the same hex twice has no exits", body: "road 5,5 5,5", want: map[Hex]int{{5, 5}: 0}},
		{name: "west to east and back is the same line", body: "road 5,2 2,2",
			want: map[Hex]int{{5, 2}: 1 << 3, {4, 2}: 1<<3 | 1, {3, 2}: 1<<3 | 1, {2, 2}: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			word := strings.Fields(tt.body)[0]
			res := Parse(tt.body, 10, 10)
			if len(res.Problems) != 0 {
				t.Fatal(res.Problems)
			}
			if len(res.Cells) != len(tt.want) {
				t.Fatalf("%d cells, want %d: %+v", len(res.Cells), len(tt.want), res.Cells)
			}
			for h, mask := range tt.want {
				c := cellAt(t, res.Cells, h.Col, h.Row)
				got := map[string]*int{"road": c.Road, "river": c.River, "trail": c.Trail}
				if ptrVal(got[word]) != mask {
					t.Errorf("%v %s mask = %d, want %d", h, word, ptrVal(got[word]), mask)
				}
				for other, p := range got {
					if other != word && p != nil {
						t.Errorf("%v has a stray %s line", h, other)
					}
				}
			}
		})
	}
}

func TestFold_LinesAddUp(t *testing.T) {
	// A road and a river may share a hex, and two roads that meet keep both
	// sets of exits.
	res := Parse("road 2,2 to 4,2\nriver 3,2 3,3\nroad 3,2 3,3", 10, 10)
	c := cellAt(t, res.Cells, 3, 2)
	if ptrVal(c.Road) != 1|8|1<<1 {
		t.Errorf("road mask on 3,2 = %d", ptrVal(c.Road))
	}
	if c.River == nil {
		t.Errorf("3,2 lost its river: %+v", c)
	}
	if got := cellAt(t, res.Cells, 3, 3); got.Road == nil || ptrVal(got.Road) != 1<<4 {
		t.Errorf("3,3 road = %+v", got)
	}
}

func TestRun_BothEndsAndJoined(t *testing.T) {
	// Consecutive hexes on a run are always neighbours, whatever the angle.
	pts := []Hex{{0, 0}, {9, 3}, {2, 9}, {7, 7}, {0, 9}, {9, 0}}
	for i := 1; i < len(pts); i++ {
		run := Run(pts[i-1], pts[i])
		if run[0] != pts[i-1] || run[len(run)-1] != pts[i] {
			t.Fatalf("run %v..%v = %v", pts[i-1], pts[i], run)
		}
		for j := 1; j < len(run); j++ {
			if sideTo(run[j-1], run[j]) < 0 {
				t.Fatalf("run %v..%v: %v and %v are not neighbours", pts[i-1], pts[i], run[j-1], run[j])
			}
		}
	}
}

func TestResult_JSONShape(t *testing.T) {
	res := Parse("forest 1,1\nroad 1,1 2,1\nplace \"Keep\" 1,1: x", 5, 5)
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"parts"`, `"cells"`, `"kind":"land"`, `"word":"forest"`, `"hexes":[{"col":1,"row":1}]`, `"terrain":"forest"`, `"road":`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("JSON lacks %s: %s", key, b)
		}
	}
}
