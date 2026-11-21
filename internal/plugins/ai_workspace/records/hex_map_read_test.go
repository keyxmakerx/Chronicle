package records

import (
	"context"
	"math/rand"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/hexsketch"
)

// bodyOf is the lines between the block's front matter and its closing fence.
func bodyOf(t *testing.T, text string) string {
	t.Helper()
	parts := strings.SplitN(text, "---\n", 3)
	if len(parts) != 3 {
		t.Fatalf("no front matter in:\n%s", text)
	}
	return strings.SplitN(parts[2], "```", 2)[0]
}

func intp(v int) *int { return &v }

// sameCells reports the first difference between what was stored and what the
// written block reads back as.
func sameCells(t *testing.T, want []HexCellState, got []hexsketch.Cell) {
	t.Helper()
	byHex := map[[2]int]hexsketch.Cell{}
	for _, c := range got {
		byHex[[2]int{c.Col, c.Row}] = c
	}
	if len(got) != len(want) {
		t.Fatalf("read back %d hexes, stored %d", len(got), len(want))
	}
	eq := func(a, b *int) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	for _, w := range want {
		g, ok := byHex[[2]int{w.Col, w.Row}]
		if !ok {
			t.Fatalf("hex %d,%d was not written", w.Col, w.Row)
		}
		if g.Terrain != w.Terrain || !eq(g.Road, w.Road) || !eq(g.River, w.River) || !eq(g.Trail, w.Trail) || g.Name != w.Name || g.Notes != w.Notes {
			t.Fatalf("hex %d,%d read back as %+v, stored %+v", w.Col, w.Row, g, w)
		}
	}
}

func TestHexMapRead_RoundTripsRandomMaps(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for n := 0; n < 200; n++ {
		cols, rows := 4+rng.Intn(14), 4+rng.Intn(14)
		// Build the stored state the way a map gets it: by painting.
		var parts []hexsketch.Part
		for i := 0; i < 3+rng.Intn(12); i++ {
			a := hexsketch.Hex{Col: rng.Intn(cols), Row: rng.Intn(rows)}
			b := hexsketch.Hex{Col: rng.Intn(cols), Row: rng.Intn(rows)}
			switch rng.Intn(4) {
			case 0, 1:
				var hs []hexsketch.Hex
				for r := min(a.Row, b.Row); r <= max(a.Row, b.Row); r++ {
					for c := min(a.Col, b.Col); c <= max(a.Col, b.Col); c++ {
						hs = append(hs, hexsketch.Hex{Col: c, Row: r})
					}
				}
				parts = append(parts, hexsketch.Part{Kind: hexsketch.PartLand, Word: hexsketch.Lands[rng.Intn(len(hexsketch.Lands))], Hexes: hs})
			case 2:
				var hs []hexsketch.Hex
				for _, h := range hexsketch.Run(a, b) {
					if h.Col >= 0 && h.Col < cols {
						hs = append(hs, h)
					}
				}
				parts = append(parts, hexsketch.Part{Kind: hexsketch.PartLine, Word: hexsketch.Lines[rng.Intn(3)], Hexes: hs})
			case 3:
				parts = append(parts, hexsketch.Part{Kind: hexsketch.PartPlace, Name: "Place " + string(rune('A'+i)), Notes: []string{"", "A note."}[rng.Intn(2)], Hexes: []hexsketch.Hex{a}})
			}
		}
		var st HexMapState
		st.Cols, st.Rows = cols, rows
		for _, c := range hexsketch.Fold(parts) {
			st.Cells = append(st.Cells, HexCellState{Col: c.Col, Row: c.Row, Terrain: c.Terrain, Road: c.Road, River: c.River, Trail: c.Trail, Name: c.Name, Notes: c.Notes})
		}
		text, shown, more := hexMapRead("Test", st, nil)
		if more || shown != len(st.Cells) {
			t.Fatalf("map %d: shown %d of %d, more %v", n, shown, len(st.Cells), more)
		}
		res := hexsketch.Parse(bodyOf(t, text), cols, rows)
		if len(res.Problems) > 0 {
			t.Fatalf("map %d: the block does not read back: %v\n%s", n, res.Problems, text)
		}
		sameCells(t, st.Cells, res.Cells)
	}
}

func TestHexMapRead_OneSidedExitIsNotWrittenAsAJoin(t *testing.T) {
	// A reader under fog is sent 2,2 with its exit east cleared, and never
	// 3,2. Here the exit is left set, as a stored row would have it, to pin
	// that the block still joins nothing to a hex that was not sent.
	st := HexMapState{Cols: 6, Rows: 6, Cells: []HexCellState{
		{Col: 1, Row: 2, Terrain: "plains", River: intp(1)},
		{Col: 2, Row: 2, Terrain: "plains", River: intp(1 | 8)},
	}}
	text, _, _ := hexMapRead("Fog", st, nil)
	body := bodyOf(t, text)
	if strings.Contains(body, "3,2") {
		t.Fatalf("the block names a hex that was not sent:\n%s", body)
	}
	if !strings.Contains(body, "river 1,2 2,2\n") {
		t.Fatalf("the seen stretch of river is missing:\n%s", body)
	}
}

func TestHexMapRead_TextAPlaceLineCannotCarryIsWrittenAsNotes(t *testing.T) {
	st := HexMapState{Cols: 6, Rows: 6, Cells: []HexCellState{
		{Col: 1, Row: 1, Terrain: "town", Name: "Oakhold", Notes: "Line one.\nLine two."},
		{Col: 2, Row: 1, Notes: "No name here."},
		{Col: 3, Row: 1, Name: `The "Old" Mill`},
	}}
	text, _, _ := hexMapRead("Notes", st, nil)
	body := bodyOf(t, text)
	for _, want := range []string{"place \"Oakhold\" 1,1\n", "#   Line one.\n", "#   Line two.\n", "#   No name here.\n", "# 3,1 is named"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	res := hexsketch.Parse(body, 6, 6)
	if len(res.Problems) > 0 {
		t.Fatalf("problems: %v", res.Problems)
	}
	for _, c := range res.Cells {
		if c.Notes != "" {
			t.Errorf("sending the block back would replace the notes on %d,%d", c.Col, c.Row)
		}
	}
}

func TestHexMapRead_ABigMapComesInBandsOfRows(t *testing.T) {
	var st HexMapState
	st.Cols, st.Rows = 100, 100
	for r := 0; r < 100; r++ {
		for c := 0; c < 100; c++ {
			st.Cells = append(st.Cells, HexCellState{Col: c, Row: r, Terrain: hexsketch.Lands[(c*7+r*3+c*r)%len(hexsketch.Lands)]})
		}
	}
	seen := 0
	text, shown, more := hexMapRead("Big", st, nil)
	if !more || len(text) > hexReadBudget+1024 || !strings.Contains(text, "Ask again with `rows`") {
		t.Fatalf("a 10,000 hex map came as %d bytes, more %v", len(text), more)
	}
	seen += shown
	next := shown / 100
	for next < 100 {
		band := hexRows{next, min(next+9, 99)}
		_, shown, _ = hexMapRead("Big", st, &band)
		seen += shown
		next = band.to + 1
	}
	if seen != 10000 {
		t.Fatalf("the bands cover %d hexes, want 10000", seen)
	}
}

func TestLookupPins_AHexMapComesWithItsHexesReadAsTheAsker(t *testing.T) {
	hx := &fakeHexMaps{cols: 8, rows: 8, notHex: map[string]bool{"sq": true}, cells: []HexCellState{{Col: 1, Row: 1, Terrain: "forest"}}}
	l := &Lookups{Maps: &fakeMaps{maps: []MapRef{{ID: "m1", CampaignID: camp, Name: "Imix"}, {ID: "sq", CampaignID: camp, Name: "Square"}}}, Hexes: hx}
	player := Actor{UserID: "p1", Role: 1}
	ans := l.Answer(context.Background(), camp, player, []Record{lookup("pins", map[string]any{"map": "Imix"})})
	if len(ans) != 1 || ans[0].Error != "" || !strings.Contains(ans[0].Text, "kind: hex-map") || !strings.Contains(ans[0].Text, "forest 1,1") {
		t.Fatalf("answer: %+v", ans)
	}
	if len(hx.actors) != 1 || hx.actors[0] != player {
		t.Fatalf("the hexes were read as %+v, want the asker", hx.actors)
	}
	ans = l.Answer(context.Background(), camp, player, []Record{lookup("pins", map[string]any{"map": "Square"})})
	if strings.Contains(ans[0].Text, "Hexes") {
		t.Fatalf("a square-grid map answered with hexes: %s", ans[0].Text)
	}
	ans = l.Answer(context.Background(), camp, player, []Record{lookup("pins", map[string]any{"map": "Imix", "rows": "soon"})})
	if ans[0].Error == "" {
		t.Fatal("rows that cannot be read were not refused")
	}
}
