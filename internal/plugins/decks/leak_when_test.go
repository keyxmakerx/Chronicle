package decks

import (
	"encoding/json"
	"strings"
	"testing"
)

// secretMarkers are the words a hidden test is made of. None may reach a player
// by any route.
var secretMarkers = []string{"Secretly-a-spy", "SECRETLAYER", "secretly", "fitsSince", "saidAt", "whenJson", "directorNote"}

// whenLeakWorld is an event deck with cards whose tests the Director kept to
// themselves, in a world where those tests hold or do not. Everything a player
// can read must be the same in both.
func whenLeakWorld(t *testing.T, holds bool) map[string]string {
	t.Helper()
	fx := newFixture(t)
	f := newFakeFacts()
	f.sheet("h-tam", map[string]float64{"hp": 2, "hpmax": 20})
	f.sheet("h-ilse", map[string]float64{"hp": 20, "hpmax": 20})
	if holds {
		f.tags["h-tam"] = []TagFact{{ID: 8, Name: "Secretly-a-spy"}}
		f.lays["h-tam"] = []LayerFact{{Name: "SECRETLAYER"}}
	} else {
		f.sheet("h-tam", map[string]float64{"hp": 20, "hpmax": 20})
	}
	newFactsFixture(t, fx, f)
	deck := fx.deck("Omens", KindEvent, &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), OthersSee: ptr("face"), Draws: ptr("players")})
	fx.choose(deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(5)}, PlayerInput{UserID: "u-ilse"})
	fx.card(deck, CreateCardInput{Name: "Plain", Words: "PLAINWORDS"})
	fx.card(deck, CreateCardInput{Name: "HiddenTag", Words: "TAGWORDS", When: WhenList{{Test: TestTag, Who: "h-tam", Tag: it(8), Has: bl(true)}}})
	fx.card(deck, CreateCardInput{Name: "HiddenLayer", Words: "LAYERWORDS", When: WhenList{{Test: TestLayer, Who: "h-tam", Name: "SECRETLAYER", On: bl(true)}}})
	fx.card(deck, CreateCardInput{Name: "HiddenLow", Words: "LOWWORDS", When: WhenList{{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lte", Of: "hpmax", Share: "quarter"}}})
	fx.card(deck, CreateCardInput{Name: "Printed", Words: "PRINTEDWORDS", When: WhenList{{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "gte", Value: fl(1), Open: true}}})

	// A Director's hidden test is judged for the Director: both the draw that
	// reaches it and the moment it raises must not show in a player's replies.
	fx.must(fx.svc.Recheck(bg, camp))
	out := map[string]string{}
	put := func(label string, v any, err error) {
		if err != nil {
			out[label] = "error " + err.Error()
			return
		}
		raw, _ := json.Marshal(v)
		out[label] = string(raw)
	}
	for name, v := range map[string]Viewer{"tam": fx.tam, "ilse": fx.ilse, "scribe": fx.scribe} {
		l, err := fx.svc.List(bg, camp, v)
		put(name+"/list", l, err)
		d, err := fx.svc.Get(bg, camp, deck, v)
		put(name+"/deck", d, err)
		tb, err := fx.svc.Table(bg, camp, v)
		put(name+"/table", tb, err)
		h, err := fx.svc.Hand(bg, camp, v)
		put(name+"/hand", h, err)
		hh, err := fx.svc.HeroHand(bg, camp, "h-tam", v)
		put(name+"/herohand", hh, err)
	}
	// Tam draws as often as the deck lets them: what comes is the same whatever
	// a hidden test says, because a player never pulls a card with one.
	for i := 0; i < 3; i++ {
		res, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
		if err == nil {
			// The Plain and Printed copies are the only ones a player can pull.
			raw, _ := json.Marshal(res)
			out["draw"+string(rune('0'+i))] = strings.ReplaceAll(string(raw), res.CopyID, "FRESH")
			continue
		}
		out["draw"+string(rune('0'+i))] = "error " + err.Error()
	}
	for k, v := range out {
		out[k] = strings.ReplaceAll(v, deck, "DECK")
	}
	return out
}

func TestTwoWorldsAHiddenTestHoldingIsInvisibleToPlayers(t *testing.T) {
	yes, no := whenLeakWorld(t, true), whenLeakWorld(t, false)
	if len(yes) == 0 || len(yes) != len(no) {
		t.Fatalf("worlds returned %d and %d replies", len(yes), len(no))
	}
	for k := range yes {
		// A copy's id is random per run and is the one thing allowed to differ.
		a, b := stripIDs(yes[k]), stripIDs(no[k])
		if a != b {
			t.Errorf("%s differs between worlds:\n holds:   %s\n does not: %s", k, a, b)
		}
		for _, bad := range secretMarkers {
			if strings.Contains(a, bad) || strings.Contains(b, bad) {
				t.Errorf("%s names %q", k, bad)
			}
		}
	}
}

// stripIDs blanks the random ids so two worlds can be compared.
func stripIDs(s string) string {
	for _, key := range []string{`"id":"`, `"copyId":"`, `"cardId":"`, `"deckId":"`} {
		for i := 0; ; {
			j := strings.Index(s[i:], key)
			if j < 0 {
				break
			}
			start := i + j + len(key)
			end := strings.IndexByte(s[start:], '"')
			if end < 0 {
				break
			}
			s = s[:start] + "X" + s[start+end:]
			i = start + 1
		}
	}
	return s
}

// A printed test is the player's to read, but not whether it holds: what a
// player is sent about the deck is the same in both worlds, and the Director's
// own view carries the sentence either way.
func TestAPrintedTestsSentenceDoesNotDependOnWhetherItHolds(t *testing.T) {
	read := func(hp float64) (director, player string) {
		fx := newFixture(t)
		f := newFakeFacts()
		f.sheet("h-tam", map[string]float64{"hp": hp})
		newFactsFixture(t, fx, f)
		deck := fx.deck("Omens", KindEvent, &SettingsInput{Knows: ptr("kinds"), Draws: ptr("players")})
		fx.choose(deck, PlayerInput{UserID: "u-tam"})
		fx.card(deck, CreateCardInput{Name: "Printed", When: WhenList{{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "gte", Value: fl(10), Open: true}}})
		d, err := fx.svc.Get(bg, camp, deck, fx.dir)
		fx.must(err)
		raw, _ := json.Marshal(d)
		p, err := fx.svc.Get(bg, camp, deck, fx.tam)
		fx.must(err)
		praw, _ := json.Marshal(p)
		return strings.ReplaceAll(string(raw), deck, "DECK"), strings.ReplaceAll(string(praw), deck, "DECK")
	}
	dHolds, pHolds := read(20)
	dFails, pFails := read(1)
	for _, d := range []string{dHolds, dFails} {
		if !strings.Contains(d, "Tam's Hit points is at or above 10") {
			t.Errorf("the Director's deck lacks the printed line: %s", d)
		}
	}
	if stripIDs(pHolds) != stripIDs(pFails) {
		t.Errorf("a player's view of the deck depends on whether a printed test holds:\n %s\n %s", pHolds, pFails)
	}
}
