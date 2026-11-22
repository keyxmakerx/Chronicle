package decks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// cursedTest is a test on whether Tam has the tag Cursed.
func cursedTest(has, open bool) WhenList {
	return WhenList{{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(has), Open: open}}
}

// whenWorld is an event deck whose Director draws, with Tam holding the tag
// Cursed, a plain card, a card that fits, and one that does not.
type whenWorld struct {
	fx                 *fixture
	f                  *fakeFacts
	deck               string
	plain, fits, unfit string
}

func newWhenWorld(t *testing.T, settings *SettingsInput) *whenWorld {
	t.Helper()
	fx := newFixture(t)
	f := newFakeFacts()
	f.tags["h-tam"] = []TagFact{{ID: 7, Name: "Cursed"}}
	newFactsFixture(t, fx, f)
	if settings == nil {
		settings = &SettingsInput{}
	}
	w := &whenWorld{fx: fx, f: f, deck: fx.deck("Omens", KindEvent, settings)}
	w.plain = fx.card(w.deck, CreateCardInput{Name: "Plain", Words: "PLAINWORDS"})
	w.fits = fx.card(w.deck, CreateCardInput{Name: "Fits", Words: "FITSWORDS", When: cursedTest(true, false)})
	w.unfit = fx.card(w.deck, CreateCardInput{Name: "Unfit", Words: "UNFITWORDS", When: cursedTest(false, false)})
	return w
}

func (w *whenWorld) pileOf(card string) int {
	n := 0
	for _, c := range w.fx.repo.copies {
		if c.CardID == card && c.Zone == ZonePile {
			n++
		}
	}
	return n
}

func TestADirectorsDrawPicksOnlyAmongCardsThatFit(t *testing.T) {
	w := newWhenWorld(t, nil)
	// Draw everything that can come: the unfit card must stay in the pile.
	if _, err := w.fx.svc.Draw(bg, camp, w.deck, w.fx.dir, DrawInput{Count: 5}); err != nil {
		t.Fatal(err)
	}
	if w.pileOf(w.unfit) != 1 || w.pileOf(w.plain) != 0 || w.pileOf(w.fits) != 0 {
		t.Errorf("pile after the draw: plain %d, fits %d, unfit %d", w.pileOf(w.plain), w.pileOf(w.fits), w.pileOf(w.unfit))
	}
}

func TestNothingFittingIsAConflictAndReshufflesNothing(t *testing.T) {
	fx := newFixture(t)
	f := newFakeFacts()
	newFactsFixture(t, fx, f)
	deck := fx.deck("Omens", KindEvent, nil)
	unfit := fx.card(deck, CreateCardInput{Name: "Unfit", When: cursedTest(true, false)})
	used := fx.card(deck, CreateCardInput{Name: "Used"})
	fx.place(fx.copiesOf(used)[0], ZoneUsed, "", true)
	before := fx.copy(fx.copiesOf(used)[0])

	_, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{})
	if code(err) != http.StatusConflict || !strings.Contains(err.Error(), "No card in the pile fits") {
		t.Fatalf("err = %v (%d)", err, code(err))
	}
	if got := fx.copy(fx.copiesOf(used)[0]); got.Zone != ZoneUsed || got.MovedAt != before.MovedAt {
		t.Errorf("the used copy was reshuffled: %+v", got)
	}
	if got := fx.copy(fx.copiesOf(unfit)[0]); got.Zone != ZonePile {
		t.Errorf("the unfit copy moved: %+v", got)
	}
}

func TestDealByNameIgnoresConditions(t *testing.T) {
	w := newWhenWorld(t, nil)
	if _, err := w.fx.svc.DealCard(bg, camp, w.deck, w.fx.dir, DealCardInput{CardID: w.unfit, To: "hand", HeroID: "h-tam"}); err != nil {
		t.Fatalf("a deal by name was refused: %v", err)
	}
	if w.pileOf(w.unfit) != 0 {
		t.Error("the named copy is still in the pile")
	}
}

func TestADirectorsAnotherDrawsAmongFittingCards(t *testing.T) {
	w := newWhenWorld(t, nil)
	res, err := w.fx.svc.Draw(bg, camp, w.deck, w.fx.dir, DrawInput{})
	if err != nil {
		t.Fatal(err)
	}
	copyID := res.CopyID
	for i := 0; i < 4; i++ {
		next, err := w.fx.svc.Act(bg, camp, w.deck, copyID, "another", w.fx.dir, ActInput{})
		if err != nil {
			t.Fatal(err)
		}
		if got := w.fx.copy(next.CopyID); got.CardID == w.unfit {
			t.Fatalf("draw another brought a card whose test fails")
		}
		copyID = next.CopyID
	}
}

// A player's own draw only pulls a card with no test or with printed tests that
// all hold.
func TestAPlayersOwnDrawPullsOnlyOpenCardsThatHold(t *testing.T) {
	players := &SettingsInput{Knows: ptr("count"), Arrives: ptr("down"), Draws: ptr("players")}
	setup := func(t *testing.T, cards map[string]WhenList) (*fixture, string, map[string]string) {
		fx := newFixture(t)
		f := newFakeFacts()
		f.tags["h-tam"] = []TagFact{{ID: 7, Name: "Cursed"}}
		newFactsFixture(t, fx, f)
		deck := fx.deck("Omens", KindEvent, players)
		fx.choose(deck, PlayerInput{UserID: "u-tam"})
		ids := map[string]string{}
		for name, when := range cards {
			ids[name] = fx.card(deck, CreateCardInput{Name: name, When: when})
		}
		return fx, deck, ids
	}
	t.Run("an open card that holds can be drawn", func(t *testing.T) {
		fx, deck, ids := setup(t, map[string]WhenList{"Open": cursedTest(true, true)})
		res, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
		if err != nil || fx.copy(res.CopyID).CardID != ids["Open"] {
			t.Fatalf("draw = %+v, %v", res, err)
		}
	})
	t.Run("an open card that fails is not eligible", func(t *testing.T) {
		fx, deck, _ := setup(t, map[string]WhenList{"Open": cursedTest(false, true)})
		_, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
		if code(err) != http.StatusConflict {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a hidden test keeps the card out whether or not it holds", func(t *testing.T) {
		for _, holds := range []bool{true, false} {
			fx, deck, _ := setup(t, map[string]WhenList{"Hidden": cursedTest(holds, false)})
			_, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
			if code(err) != http.StatusConflict {
				t.Errorf("hidden test holding=%v: err = %v", holds, err)
			}
		}
	})
	t.Run("a plain card is drawn past a hidden one", func(t *testing.T) {
		fx, deck, ids := setup(t, map[string]WhenList{"Hidden": cursedTest(true, false), "Plain": nil})
		for i := 0; i < 1; i++ {
			res, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
			if err != nil || fx.copy(res.CopyID).CardID != ids["Plain"] {
				t.Fatalf("draw = %+v, %v", res, err)
			}
		}
	})
	t.Run("nothing eligible spends no draw", func(t *testing.T) {
		fx, deck, _ := setup(t, map[string]WhenList{"Hidden": cursedTest(true, false)})
		fx.choose(deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(2)})
		if _, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"}); err == nil {
			t.Fatal("drew")
		}
		left := fx.repo.decks[deck].Players[0].DrawsLeft
		if left == nil || *left != 2 {
			t.Errorf("draws left = %v", left)
		}
	})
}

// A player cannot tell "nothing is eligible" from "the pile is empty": the
// reply is byte for byte the same.
func TestAPlayersNothingEligibleReplyIsTheEmptyPileReply(t *testing.T) {
	reply := func(t *testing.T, build func(fx *fixture, deck string)) string {
		fx := newFixture(t)
		f := newFakeFacts()
		f.tags["h-tam"] = []TagFact{{ID: 7}}
		newFactsFixture(t, fx, f)
		deck := fx.deck("Omens", KindEvent, &SettingsInput{Draws: ptr("players")})
		fx.choose(deck, PlayerInput{UserID: "u-tam"})
		build(fx, deck)
		_, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
		if err == nil {
			t.Fatal("drew")
		}
		b, _ := json.Marshal(map[string]any{"code": code(err), "msg": err.Error()})
		return string(b)
	}
	empty := reply(t, func(*fixture, string) {})
	for name, build := range map[string]func(fx *fixture, deck string){
		"a hidden test that holds": func(fx *fixture, deck string) {
			fx.card(deck, CreateCardInput{Name: "H", When: cursedTest(true, false)})
		},
		"a hidden test that fails": func(fx *fixture, deck string) {
			fx.card(deck, CreateCardInput{Name: "H", When: cursedTest(false, false)})
		},
		"a printed test that fails": func(fx *fixture, deck string) {
			fx.card(deck, CreateCardInput{Name: "H", When: cursedTest(false, true)})
		},
	} {
		if got := reply(t, build); got != empty {
			t.Errorf("%s: the reply differs from the empty pile's:\n got  %s\n want %s", name, got, empty)
		}
	}
}

func TestAnUnsaidSayCardDoesNotFitUntilSaid(t *testing.T) {
	fx := newFixture(t)
	newFactsFixture(t, fx, newFakeFacts())
	deck := fx.deck("Omens", KindEvent, nil)
	card := fx.card(deck, CreateCardInput{Name: "Say", When: WhenList{{Test: TestSay}}})
	if _, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{}); code(err) != http.StatusConflict {
		t.Fatalf("an unsaid card was drawn: %v", err)
	}
	if _, err := fx.svc.Say(bg, camp, deck, card, fx.dir, true); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{}); err != nil {
		t.Fatalf("a said card was refused: %v", err)
	}
	if fx.repo.cards[card].SaidAt != nil {
		t.Error("saying must be cleared when a copy leaves the pile")
	}
}

func TestOnlyADirectorSays(t *testing.T) {
	fx := newFixture(t)
	newFactsFixture(t, fx, newFakeFacts())
	deck := fx.deck("Omens", KindEvent, nil)
	card := fx.card(deck, CreateCardInput{Name: "Say", When: WhenList{{Test: TestSay}}})
	for name, v := range map[string]Viewer{"player": fx.tam, "scribe": fx.scribe, "visitor": fx.visitor, "former": fx.former} {
		if _, err := fx.svc.Say(bg, camp, deck, card, v, true); code(err) != http.StatusForbidden && code(err) != http.StatusNotFound {
			t.Errorf("%s: %v", name, err)
		}
		if fx.repo.cards[card].SaidAt != nil {
			t.Fatalf("%s said so", name)
		}
	}
}
