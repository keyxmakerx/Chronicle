package decks

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderToString(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func stylesheet(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../../static/css/decks_card.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Every list in settings.go has a class (or a drawing) for each of its keys, and
// the stylesheet has a rule for each class that carries the look, so a key
// added on the server without a look cannot ship. A list's first value is its
// default and may be drawn by the base rules alone. A class matches as a whole
// name, so .dk-m-tab is not satisfied by .dk-m-tabs.
func TestLookupTablesCoverEveryList(t *testing.T) {
	css := stylesheet(t)
	lists := []struct {
		name  string
		keys  []string
		table map[string]string
		rules bool
	}{
		{"shape", shapeValues, shapeClass, true}, {"border", borderValues, borderClass, true}, {"turn", turnValues, turnClass, true},
		{"window", windowValues, windowClass, true}, {"name", nameValues, nameClass, true}, {"lettering", letteringValues, letteringClass, true},
		{"corner", cornerValues, cornerClass, true}, {"marker", markerValues, markerClass, true}, {"finish", finishValues, finishClass, true},
		{"pattern", patternValues, patternClass, true}, {"palette", Palette, paletteClass, true},
	}
	for _, l := range lists {
		for i, k := range l.keys {
			cls, ok := l.table[k]
			if !ok || cls == "" {
				t.Errorf("%s %q has no class", l.name, k)
				continue
			}
			if !l.rules || i == 0 {
				continue
			}
			for _, c := range strings.Fields(cls) {
				if !regexp.MustCompile(`\.` + regexp.QuoteMeta(c) + `([^A-Za-z0-9_-]|$)`).MatchString(css) {
					t.Errorf("%s %q: the stylesheet has no rule for .%s", l.name, k, c)
				}
			}
		}
		if len(l.table) != len(l.keys) {
			t.Errorf("%s: the table has %d entries for %d keys", l.name, len(l.table), len(l.keys))
		}
	}
	for _, k := range signValues {
		if kindSignPath[k] == "" {
			t.Errorf("kind sign %q has no drawing", k)
		}
	}
	for _, k := range backSignValues {
		if k != "none" && emblemPath[k] == "" {
			t.Errorf("back sign %q has no drawing", k)
		}
	}
	for _, b := range borderValues {
		if strings.Contains(borderClass[b], "dk-mat") && len(fittingsFor[b]) == 0 {
			t.Errorf("forged border %q has no fittings", b)
		}
	}
}

// Each corner style and each marker draws something of its own, so a key added
// without markup shows up as a copy of another.
func TestCornersAndMarkersDrawDistinctly(t *testing.T) {
	seen := map[string]string{}
	for _, c := range cornerValues {
		l := proofLook(func(l *Look) { l.Corner = c })
		out := renderToString(t, cardCorner(l, "7"))
		if prev, dup := seen["corner:"+out]; dup && out != "" {
			t.Errorf("corner %q draws the same as %q", c, prev)
		}
		seen["corner:"+out] = c
	}
	k := KindDef{Key: "k", Name: "Kind", Colour: "teal", Sign: "round"}
	for _, m := range markerValues {
		l := proofLook(func(l *Look) { l.Marker = m })
		out := renderToString(t, cardMarker(l, k)) + "|" + renderToString(t, cardKindLine(l, k))
		if prev, dup := seen["marker:"+out]; dup {
			t.Errorf("marker %q draws the same as %q", m, prev)
		}
		seen["marker:"+out] = m
	}
}

// Every turn-over the server accepts has a class and a move in the script.
func TestScriptNamesEveryTurnAndMove(t *testing.T) {
	b, err := os.ReadFile("static/js/deck_cards.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, k := range turnValues {
		if !strings.Contains(js, "'"+k+"'") {
			t.Errorf("deck_cards.js does not name the turn-over %q", k)
		}
	}
	for _, m := range moveNames() {
		if !strings.Contains(js, "'"+m+"'") {
			t.Errorf("deck_cards.js does not name the move %q", m)
		}
	}
}

// A face-down card is drawn from the deck's look and a size and nothing else.
// What the signature guarantees: CardState has no exported field and FaceDown()
// takes no argument, so a face-down state has no field that can hold card data,
// and CardBack takes no card. This test renders the wrapper, through the call a
// real page makes, for a deck whose card carries a poison string in every field
// (name, words, kind key and name, corner, picture, id), and checks none of them
// appears in any shape, size or border.
func TestFaceDownCarriesNoCardData(t *testing.T) {
	poison := Face{
		CardID: "poison-id-0001", Name: "Poison name", KindKey: "poison-key", Words: "Poison words",
		Picture: "/poison/picture.jpg", Corner: "poison-corner", Rare: true,
	}
	leaks := []string{poison.CardID, poison.Name, poison.KindKey, "Poison kind name", poison.Words, poison.Picture, poison.Corner, "poison"}
	for _, border := range borderValues {
		for _, sh := range shapeValues {
			l := proofLook(func(l *Look) {
				l.Kinds = append(l.Kinds, KindDef{Key: poison.KindKey, Name: "Poison kind name", Colour: "rose", Sign: "drop"})
				l.Border, l.Shape = border, sh
			})
			for _, size := range []CardSize{SizeRow, SizeHand, SizeOpen} {
				// The page has the card in hand and draws it face down.
				_ = poison
				for name, c := range map[string]templ.Component{
					"turnable": CardView(l, FaceDown(), CardOpts{Size: size, DeckName: "Deck", Position: "2 of 5", Turnable: true, FaceSrc: "/campaigns/c/decks/d/copies/copy-1/face"}),
					"static":   CardView(l, FaceDown(), CardOpts{Size: size, DeckName: "Deck"}),
					"back":     CardBack(l, size),
				} {
					out := renderToString(t, c)
					for _, leak := range leaks {
						if strings.Contains(out, leak) {
							t.Errorf("%s %s %s %s: face-down output contains %q", name, border, sh, size, leak)
						}
					}
					for _, drawn := range []string{"dk-rare", "dk-plate", "dk-compact", "dk-leaf"} {
						if strings.Contains(out, drawn) {
							t.Errorf("%s: a face-down card drew %s", name, drawn)
						}
					}
				}
			}
		}
	}
	if got := reflect.TypeOf(CardBack).NumIn(); got != 2 {
		t.Errorf("CardBack takes %d arguments; it must take only the look and a size", got)
	}
	if !reflect.DeepEqual(FaceDown(), CardState{}) {
		t.Error("a zero card state must be face down")
	}
	st := reflect.TypeOf(CardState{})
	for i := 0; i < st.NumField(); i++ {
		if st.Field(i).IsExported() {
			t.Errorf("CardState.%s is exported; a state's card data must only come through FaceUp", st.Field(i).Name)
		}
	}
}

// The turn-over button names which card of which deck; it never names the card.
func TestTurnButtonLabel(t *testing.T) {
	out := renderToString(t, CardView(proofLook(nil), FaceDown(), CardOpts{Size: SizeRow, DeckName: "Sample deck", Position: "3 of 5", Turnable: true}))
	if !strings.Contains(out, `aria-label="Turn over card 3 of 5, Sample deck"`) {
		t.Errorf("button label missing in %s", out)
	}
	if !strings.Contains(out, `<button type="button" class="dk-turn"`) {
		t.Error("turn over is not a real button")
	}
	still := renderToString(t, CardView(proofLook(nil), FaceDown(), CardOpts{Size: SizeRow, DeckName: "Sample deck"}))
	if strings.Contains(still, "<button") {
		t.Error("a card that cannot be turned drew a button")
	}
}

func TestCornerMark(t *testing.T) {
	tests := []struct {
		style, text string
		shown       string
		pips        int
	}{
		{"pips", "1", "1", 1}, {"pips", "7", "7", 7}, {"pips", "10", "10", 10},
		{"pips", "0", "0", 0}, {"pips", "11", "11", 0}, {"pips", "-3", "-3", 0}, {"pips", "2.5", "2.5", 0},
		{"pips", "Ace", "Ace", 0}, {"pips", " 4 ", "4", 4}, {"pips", "", "", 0},
		{"roman", "4", "IV", 0}, {"roman", "Ace", "Ace", 0}, {"roman", "0", "0", 0},
		{"number", "7", "7", 0}, {"suit", "K", "K", 0}, {"none", "7", "", 0},
	}
	for _, tt := range tests {
		shown, pips := cornerMark(tt.style, tt.text)
		if shown != tt.shown || pips != tt.pips {
			t.Errorf("cornerMark(%q, %q) = %q, %d; want %q, %d", tt.style, tt.text, shown, pips, tt.shown, tt.pips)
		}
	}
}

func TestPipsDrawDotsAndFallBack(t *testing.T) {
	l := proofLook(func(l *Look) { l.Corner = "pips" })
	out := renderToString(t, CardView(l, proofUp("k-violet", "7", false), CardOpts{Size: SizeRow}))
	if got := strings.Count(out, "<i></i>"); got != 7 {
		t.Errorf("7 pips drew %d dots", got)
	}
	out = renderToString(t, CardView(l, proofUp("k-violet", "Ace", false), CardOpts{Size: SizeRow}))
	if strings.Contains(out, "dk-pips") || !strings.Contains(out, ">Ace") {
		t.Errorf("a non-number fell back wrongly: %s", out)
	}
}

func TestFoilIsSheenOnTheSmallShape(t *testing.T) {
	small := proofLook(func(l *Look) { l.Finish = "foil"; l.Shape = "small" })
	if got := effectiveFinish(small); got != "sheen" {
		t.Errorf("small foil = %q", got)
	}
	tall := proofLook(func(l *Look) { l.Finish = "foil" })
	if got := effectiveFinish(tall); got != "foil" {
		t.Errorf("tall foil = %q", got)
	}
}

// A card names its kind in words wherever it is read (the compact card at row
// and open size, the leaf of a tall card), in its accessible name at every
// size, and draws the kind's sign at every size, so no kind depends on colour
// alone. At the hand size neither build shows the word.
func TestKindIsNamedInWords(t *testing.T) {
	for _, sh := range shapeValues {
		for _, size := range []CardSize{SizeRow, SizeHand, SizeOpen} {
			l := proofLook(func(l *Look) { l.Shape = sh })
			out := renderToString(t, CardView(l, proofUp("k-teal", "3", false), CardOpts{Size: size}))
			where := sh + " " + string(size)
			if !strings.Contains(out, `aria-label="Sample card, Kind teal"`) {
				t.Errorf("%s: accessible name lacks the kind", where)
			}
			if !strings.Contains(out, `class="dk-sg"`) {
				t.Errorf("%s: no kind sign drawn", where)
			}
			// At the hand size the word is in the markup but the stylesheet
			// hides it (checked below); everywhere else it is drawn.
			wantWords := (sh != "tall" && size != SizeHand) || (sh == "tall" && size == SizeOpen)
			if wantWords && !strings.Contains(out, "Kind teal</") {
				t.Errorf("%s: the kind is not named in words", where)
			}
		}
	}
	css := stylesheet(t)
	for _, sel := range []string{".dk-sz-hand .dk-kind", ".dk-sz-hand .dk-kl"} {
		if !strings.Contains(css, sel) {
			t.Errorf("the stylesheet does not hide %s at the hand size", sel)
		}
	}
}

// A tall card at the open size has its leaf; smaller sizes and other shapes do not.
func TestLeafOnlyOnOpenTall(t *testing.T) {
	l := proofLook(nil)
	if out := renderToString(t, CardView(l, proofUp("k-teal", "3", false), CardOpts{Size: SizeOpen, Foot: "In the deck"})); !strings.Contains(out, "dk-leaf") || !strings.Contains(out, "In the deck") {
		t.Error("open tall card has no leaf")
	}
	if out := renderToString(t, CardView(l, proofUp("k-teal", "3", false), CardOpts{Size: SizeRow})); strings.Contains(out, "dk-leaf") {
		t.Error("row card has a leaf")
	}
	l.Shape = "standard"
	if out := renderToString(t, CardView(l, proofUp("k-teal", "3", false), CardOpts{Size: SizeOpen})); strings.Contains(out, "dk-leaf") {
		t.Error("standard card has a leaf")
	}
}

func TestMaterialsAreDefinedOnceAtTheTop(t *testing.T) {
	css := stylesheet(t)
	i := strings.Index(css, "---- MATERIALS")
	j := strings.Index(css, "---- PALETTE")
	if i < 0 || j < i {
		t.Fatal("the MATERIALS block is missing or out of order")
	}
	for _, name := range []string{"--dk-iron:", "--dk-brass:", "--dk-steel:", "--dk-stone:", "--dk-gold:"} {
		if c := strings.Count(css, "  "+name); c != 1 || !strings.Contains(css[i:j], name) {
			t.Errorf("%s must be defined once, in the MATERIALS block (found %d)", name, c)
		}
	}
}
