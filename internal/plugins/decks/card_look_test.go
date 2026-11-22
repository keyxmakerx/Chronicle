package decks

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
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
// default and may be drawn by the base rules alone. Corners and markers are
// drawn by the markup they choose, and turn-overs by the script, so those lists
// are checked for an effect instead (the next two tests).
func TestLookupTablesCoverEveryList(t *testing.T) {
	css := stylesheet(t)
	lists := []struct {
		name  string
		keys  []string
		table map[string]string
		rules bool
	}{
		{"shape", shapeValues, shapeClass, true}, {"border", borderValues, borderClass, true}, {"turn", turnValues, turnClass, false},
		{"window", windowValues, windowClass, true}, {"name", nameValues, nameClass, true}, {"lettering", letteringValues, letteringClass, true},
		{"corner", cornerValues, cornerClass, false}, {"marker", markerValues, markerClass, false}, {"finish", finishValues, finishClass, true},
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
				if !strings.Contains(css, "."+c) {
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

// A card with a name, words, kind, picture and id, passed alongside a face-down
// card, must reach nothing of the back. The face-down state takes no card data
// at all, so there is nothing to leak; the test proves the output and the shape
// of the signature.
func TestFaceDownCarriesNoCardData(t *testing.T) {
	l := proofLook(func(l *Look) {
		l.Kinds = append(l.Kinds, KindDef{Key: "poison-kind", Name: "Poison kind name", Colour: "rose", Sign: "drop"})
		l.Border = "bound"
	})
	secret := Face{CardID: "poison-card-id", Name: "Poison card name", KindKey: "poison-kind", Words: "Poison words", Picture: "/poison/picture.jpg", Corner: "9", Rare: true}
	_ = FaceUp(secret, "/poison/picture.jpg") // held alongside, never passed

	for _, size := range []CardSize{SizeRow, SizeHand, SizeOpen} {
		for _, sh := range shapeValues {
			l.Shape = sh
			for name, c := range map[string]templ.Component{
				"view":   CardView(l, FaceDown(), CardOpts{Size: size, DeckName: "Deck", Position: "2 of 5", Turnable: true, FaceSrc: "/face"}),
				"static": CardView(l, FaceDown(), CardOpts{Size: size, DeckName: "Deck"}),
				"back":   CardBack(l, size),
			} {
				out := renderToString(t, c)
				for _, leak := range []string{"Poison", "poison", "/poison/"} {
					if strings.Contains(out, leak) {
						t.Errorf("%s %s %s: face-down output contains %q", name, size, sh, leak)
					}
				}
				if strings.Contains(out, "dk-rare") || strings.Contains(out, "dk-plate") || strings.Contains(out, "dk-compact") {
					t.Errorf("%s: a face-down card drew a face", name)
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
	if FaceDown().Up() {
		t.Error("FaceDown reports up")
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

// The kind is named in words on the face and in the card's accessible name,
// and its sign is drawn, so no kind depends on colour alone.
func TestKindIsNamedInWords(t *testing.T) {
	l := proofLook(nil)
	for _, sh := range shapeValues {
		l.Shape = sh
		out := renderToString(t, CardView(l, proofUp("k-teal", "3", false), CardOpts{Size: SizeOpen}))
		if !strings.Contains(out, "Kind teal") {
			t.Errorf("%s: kind not named in words", sh)
		}
		if !strings.Contains(out, `aria-label="Sample card, Kind teal"`) && sh == "tall" {
			t.Errorf("tall: accessible name missing the kind")
		}
		if !strings.Contains(out, "<svg") {
			t.Errorf("%s: no kind sign drawn", sh)
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

func callLooks(cc *campaigns.CampaignContext) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/decks/looks", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	if cc != nil {
		c.Set("campaign_context", cc)
	}
	return rec, NewHandler(nil).Looks(c)
}

// The proof page is Director-only: everyone else gets the answer a missing page gets.
func TestLooksPageGates(t *testing.T) {
	ctx := func(role campaigns.Role, grant bool) *campaigns.CampaignContext {
		return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsMember: true, IsDmGranted: grant}
	}
	for name, cc := range map[string]*campaigns.CampaignContext{
		"player": ctx(campaigns.RolePlayer, false), "scribe": ctx(campaigns.RoleScribe, false),
	} {
		_, err := callLooks(cc)
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
			t.Errorf("%s: got %v, want a 404", name, err)
		}
	}
	if _, err := callLooks(nil); err == nil {
		t.Error("no campaign context must be refused")
	}
	for name, cc := range map[string]*campaigns.CampaignContext{"owner": ctx(campaigns.RoleOwner, false), "granted player": ctx(campaigns.RolePlayer, true)} {
		rec, err := callLooks(cc)
		if err != nil || rec.Code != http.StatusOK {
			t.Errorf("%s: err %v, status %d, want the page", name, err, rec.Code)
		}
	}
}

// The sheet draws every border on every shape and a card per turn-over and move.
func TestProofSectionsCoverEverything(t *testing.T) {
	secs := proofSections()
	by := map[string]proofSection{}
	for _, s := range secs {
		by[s.ID] = s
	}
	if got, want := len(by["borders"].Cells), len(borderValues)*len(shapeValues); got != want {
		t.Errorf("borders: %d cells, want %d", got, want)
	}
	if got := len(by["turns"].Cells); got != len(turnValues) {
		t.Errorf("turns: %d cells, want %d", got, len(turnValues))
	}
	if got := len(by["moves"].Cells); got != len(moveNames()) {
		t.Errorf("moves: %d cells, want %d", got, len(moveNames()))
	}
	for _, id := range []string{"windows", "names", "lettering", "corners", "markers", "finishes", "backs", "kinds", "leaf", "hand"} {
		if len(by[id].Cells) == 0 {
			t.Errorf("section %q is empty", id)
		}
	}
	page := renderToString(t, proofPageBody(secs))
	for _, p := range Palette {
		if !strings.Contains(page, "dk-pal-"+p) {
			t.Errorf("the sheet shows no %s card", p)
		}
	}
}
