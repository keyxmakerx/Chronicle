package decks

import (
	"encoding/json"
	"strings"
	"testing"
)

// kindNames lists the kind names a player's copy of a deck's look carries.
func kindNames(l Look) string {
	names := make([]string, 0, len(l.Kinds))
	for _, k := range l.Kinds {
		names = append(names, k.Name)
	}
	return strings.Join(names, ",")
}

// The names of a deck's kinds say what a Director built it from, so a player
// is sent only the kinds of the faces they are sent, unless the deck lets
// players know its kinds.
func TestAPlayerIsSentOnlyTheKindsOfFacesTheyCanRead(t *testing.T) {
	for _, tc := range []struct {
		knows                 string
		faceDown, afterTurned string
	}{
		{"nothing", "", "Bane"},
		{"count", "", "Bane"},
		{"kinds", "Boon,Bane", "Boon,Bane"},
		{"faces", "Boon,Bane", "Boon,Bane"},
	} {
		t.Run(tc.knows, func(t *testing.T) {
			fx := newFixture(t)
			deck := fx.deck("Omens", KindEvent, &SettingsInput{Knows: ptr(tc.knows), Arrives: ptr("down"), OthersSee: ptr("face")})
			_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
				Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}, {Key: "bane", Name: "Bane", Colour: "rose"}}})})
			fx.must(err)
			fx.choose(deck, PlayerInput{UserID: "u-tam"}, PlayerInput{UserID: "u-ilse"})
			fx.card(deck, CreateCardInput{Name: "Gift", KindKey: "boon", Copies: ptr(2)})
			curse := fx.card(deck, CreateCardInput{Name: "Curse", KindKey: "bane"})
			res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: curse, To: "hand", HeroID: "h-tam"})
			fx.must(err)

			look := func(v Viewer) Look { return mustGet(t, fx, deck, v).(PlayerDeck).Look }
			if got := kindNames(look(fx.tam)); got != tc.faceDown {
				t.Errorf("holder, card face down: kinds %q, want %q", got, tc.faceDown)
			}
			if got := kindNames(look(fx.ilse)); got != tc.faceDown {
				t.Errorf("other player, card face down: kinds %q, want %q", got, tc.faceDown)
			}
			if _, err := fx.svc.Act(bg, camp, deck, res.CopyID, "turn", fx.tam, ActInput{}); err != nil {
				t.Fatal(err)
			}
			if got := kindNames(look(fx.tam)); got != tc.afterTurned {
				t.Errorf("holder, card turned: kinds %q, want %q", got, tc.afterTurned)
			}
			// A private turn changes nothing for anyone else.
			if got := kindNames(look(fx.ilse)); got != tc.faceDown {
				t.Errorf("other player, card only turned: kinds %q, want %q", got, tc.faceDown)
			}
			if _, err := fx.svc.Act(bg, camp, deck, res.CopyID, "show", fx.tam, ActInput{}); err != nil {
				t.Fatal(err)
			}
			if got := kindNames(look(fx.ilse)); got != tc.afterTurned {
				t.Errorf("other player, card shown: kinds %q, want %q", got, tc.afterTurned)
			}
			// The Director's own copy of the deck is never trimmed.
			if got := kindNames(mustGet(t, fx, deck, fx.dir).(DirectorDeck).Look); got != "Boon,Bane" {
				t.Errorf("Director's kinds = %q", got)
			}
		})
	}
}

// Any member reads the table, so a deck's look there names only the kinds of
// the cards lying on it.
func TestTheTableNamesOnlyTheKindsOfItsCards(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Hidden events", KindEvent, &SettingsInput{Fate: ptr(FatePlay)})
	_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
		Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}, {Key: "bane", Name: "SECRETKIND", Colour: "rose"}}})})
	fx.must(err)
	gift := fx.card(deck, CreateCardInput{Name: "Gift", KindKey: "boon"})
	fx.card(deck, CreateCardInput{Name: "Curse", KindKey: "bane"})
	_, err = fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: gift, To: "table"})
	fx.must(err)
	tv, err := fx.svc.Table(bg, camp, fx.ilse)
	fx.must(err)
	if len(tv.Decks) != 1 || kindNames(tv.Decks[0].Look) != "Boon" {
		t.Fatalf("table decks = %+v", tv.Decks)
	}
	if raw, _ := json.Marshal(tv); strings.Contains(string(raw), "SECRETKIND") {
		t.Error("the table names a kind none of its cards has")
	}
}

// A Director drawing cards to look at changes nothing a player can count, at
// the moment of looking and not only once the cards are back.
func TestLookingAtCardsChangesNothingAPlayerIsSent(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Omens", KindEvent, &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), OthersSee: ptr("face")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(4)})
	fx.card(deck, CreateCardInput{Name: "B", Copies: ptr(4)})
	sent := func() string {
		raw, _ := json.Marshal(mustGet(t, fx, deck, fx.tam))
		return string(raw)
	}
	before := sent()
	if _, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 3}); err != nil {
		t.Fatal(err)
	}
	if got := sent(); got != before {
		t.Errorf("a Director looking at three cards changed what a player is sent:\n before: %s\n after:  %s", before, got)
	}
}

// A card's place in a hand is when it arrived there. Turning it, showing it
// and asking to play it leave it where it is, so nothing done to a card in a
// hand reorders what its holder or anyone else is sent.
func TestMovesInPlaceKeepACardsPlaceInTheHand(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Omens", KindEvent, &SettingsInput{Arrives: ptr("down"), OthersSee: ptr("face")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"}, PlayerInput{UserID: "u-ilse"})
	var ids []string
	for _, name := range []string{"First", "Second", "Third"} {
		c := fx.card(deck, CreateCardInput{Name: name, AskFirst: true})
		res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: c, To: "hand", HeroID: "h-tam"})
		fx.must(err)
		ids = append(ids, res.CopyID)
	}
	order := func() string {
		var out []string
		for _, c := range mustGet(t, fx, deck, fx.tam).(PlayerDeck).Hands[0].Hand {
			out = append(out, c.ID)
		}
		return strings.Join(out, ",")
	}
	shown := func() string {
		var out []string
		for _, o := range mustGet(t, fx, deck, fx.ilse).(PlayerDeck).Others {
			for _, f := range o.Faces {
				out = append(out, f.Name)
			}
		}
		return strings.Join(out, ",")
	}
	want := strings.Join(ids, ",")
	for _, id := range ids {
		for _, verb := range []string{"turn", "show"} {
			if _, err := fx.svc.Act(bg, camp, deck, id, verb, fx.tam, ActInput{}); err != nil {
				t.Fatalf("%s: %v", verb, err)
			}
		}
	}
	if got := shown(); got != "First,Second,Third" {
		t.Fatalf("the other player reads %q", got)
	}
	// The first card is asked about and turned down: a private exchange.
	if _, err := fx.svc.Act(bg, camp, deck, ids[0], "play", fx.tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Act(bg, camp, deck, ids[0], "turn-down", fx.dir, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if got := order(); got != want {
		t.Errorf("the holder's hand reordered: %q, want %q", got, want)
	}
	if got := shown(); got != "First,Second,Third" {
		t.Errorf("a private ask reordered what the other player reads: %q", got)
	}
}
